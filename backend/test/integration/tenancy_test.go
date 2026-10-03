//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases"
	"github.com/tobioyinlola/RiskOS/backend/internal/events"
	"github.com/tobioyinlola/RiskOS/backend/internal/httpapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

const testPepper = "integration-pepper-0123456789abcdef"

// stack is one API instance wired exactly like cmd/api, on the application role.
type stack struct {
	handler  http.Handler
	tenants  *tenants.Module
	auth     *auth.Module
	alerts   *alerts.Module
	webhooks *webhooks.Module
	risk     *risk.Module
	admin    *admin.Module
	rules    *rules.Module
}

// stackConfig holds the optional settings of a test stack.
type stackConfig struct {
	webhooks webhooks.Config
	console  auth.ConsoleConfig
	// throttle enables console sign-in throttling (in Redis).
	throttle bool
	// historyWarmup, when set, enables feature-store reset detection.
	historyWarmup time.Duration
	// inviteEmails, when set, queues invite emails.
	inviteEmails jobs.Enqueuer
	// admin enables the internal admin realm.
	admin bool
	// signup enables self-serve sign-up; google, when set, Google sign-in.
	signup bool
	google *auth.GoogleConfig
}

// withSignup enables self-serve sign-up (and Google sign-in with google, if not nil).
func withSignup(google *auth.GoogleConfig) stackOption {
	return func(c *stackConfig) { c.signup, c.google = true, google }
}

// selfServe adapts the tenants module to auth.TenantCreator, like cmd/api.
type selfServe struct{ tenants *tenants.Module }

func (s selfServe) CreateSelfServe(ctx context.Context, name, actor string, now time.Time, then func(ctx context.Context, tenantID uuid.UUID) error) (uuid.UUID, error) {
	t, err := s.tenants.Create(ctx, tenants.CreateParams{Name: name, Region: "eu-west-1", Actor: actor, SelfServe: true, Now: now},
		func(ctx context.Context, t tenants.Tenant) error { return then(ctx, t.ID) })
	return t.ID, err
}

// withAdmin enables the internal admin realm and support access.
func withAdmin() stackOption {
	return func(c *stackConfig) { c.admin = true }
}

// withInviteEmails makes new invites queue an email job on q.
func withInviteEmails(q jobs.Enqueuer) stackOption {
	return func(c *stackConfig) { c.inviteEmails = q }
}

// withHistoryWarmup enables feature-store reset detection with the given warm-up.
func withHistoryWarmup(d time.Duration) stackOption {
	return func(c *stackConfig) { c.historyWarmup = d }
}

type stackOption func(*stackConfig)

func withWebhooks(c webhooks.Config) stackOption { return func(s *stackConfig) { s.webhooks = c } }

func withConsole(c auth.ConsoleConfig, throttle bool) stackOption {
	return func(s *stackConfig) { s.console, s.throttle = c, throttle }
}

// testWebhookKey derives webhook secrets in integration tests.
const testWebhookKey = "integration-webhook-key-0123456789abcdef"

// testWebhookConfig delivers to local test servers (private addresses in the test environment)
// on a fast retry schedule.
func testWebhookConfig() webhooks.Config {
	return webhooks.Config{
		SigningKey: []byte(testWebhookKey), Timeout: 2 * time.Second, AllowPrivateTest: true,
		RetryDelays:  []time.Duration{20 * time.Millisecond, 20 * time.Millisecond},
		DisableAfter: time.Hour,
	}
}

func newStack(t *testing.T, cacheTTL time.Duration) stack {
	t.Helper()
	return newStackWithLimits(t, cacheTTL, generousLimits())
}

// generousLimits keep rate limiting out of the way of tests that are not about it.
func generousLimits() httpapi.RateLimits {
	return httpapi.RateLimits{
		PerKey:    ratelimit.Limit{PerSecond: 10000, Burst: 10000},
		PerTenant: ratelimit.Limit{PerSecond: 10000, Burst: 10000},
	}
}

func newStackWithLimits(t *testing.T, cacheTTL time.Duration, limits httpapi.RateLimits) stack {
	t.Helper()
	return newStackWith(t, cacheTTL, limits, shared.rdb)
}

// newStackWith builds a stack whose risk engine reads features from featureRedis, so tests can
// give it an unreachable one.
func newStackWith(t *testing.T, cacheTTL time.Duration, limits httpapi.RateLimits, featureRedis risk.Redis, opts ...stackOption) stack {
	t.Helper()
	sc := stackConfig{webhooks: testWebhookConfig()}
	for _, o := range opts {
		o(&sc)
	}
	var limiter auth.Limiter
	if sc.throttle {
		limiter = ratelimit.New(shared.rdb, 50*time.Millisecond)
	}
	tel, err := telemetry.Setup(t.Context(), config.Telemetry{}, "integration", config.EnvTest)
	require.NoError(t, err)
	clk := clock.System{}
	runner := db.NewTxRunner(shared.pool, db.WithBeginTimeout(time.Second))
	auditModule := audit.New(runner, clk)
	tenantsModule := tenants.New(runner, auditModule.Recorder())
	authCfg := auth.Config{
		Pepper: []byte(testPepper), CacheTTL: cacheTTL, Console: sc.console, Limiter: limiter, Logger: shared.logger,
		InviteEmails: sc.inviteEmails,
		Cookie:       auth.CookieConfig{ConsoleURL: "http://console.test", Origins: []string{"http://console.test"}},
	}
	if sc.signup {
		authCfg.Signup.Tenants, authCfg.Signup.Google = selfServe{tenantsModule}, sc.google
	}
	authModule := auth.New(shared.pool, runner, auditModule.Recorder(), clk, authCfg)
	riskModule := newRisk(featureRedis, runner, auditModule, clk, func(c *risk.Config) {
		if sc.historyWarmup > 0 {
			c.StateDB, c.HistoryWarmup = shared.pool, sc.historyWarmup
		}
	})
	queue, err := jobs.NewInserter(shared.pool, shared.logger)
	require.NoError(t, err)
	var alertsModule *alerts.Module
	webhooksModule := webhooks.New(runner, auditModule.Recorder(), clk, queue, sc.webhooks, webhooks.NotifierFunc(
		func(ctx context.Context, id uuid.UUID) error { return alertsModule.WebhookEndpointDisabled(ctx, id) }), shared.logger)
	alertsModule = alerts.New(runner, auditModule.Recorder(), clk, webhooksModule.Publisher(), shared.logger)
	var adminModule *admin.Module
	if sc.admin {
		rpid, origins := authModule.PasskeyRP()
		adminModule, err = admin.New(shared.pool, tenants.New(runner, auditModule.Recorder()), runner, auditModule.Recorder(), clk,
			admin.Config{Secret: []byte("integration-admin-secret-0123456789abcdef"), RPID: rpid, Origins: origins, Logger: shared.logger})
		require.NoError(t, err)
		authModule.SetSupport(adminModule)
	}
	casesModule := cases.New(runner, alertsModule.Linker(), auditModule.Recorder(), clk, webhooksModule.Publisher())
	rulesModule := rules.New(runner, auditModule.Recorder())
	rulesModule.ConnectWorkflow(alertsModule, casesModule)
	h, err := httpapi.New(httpapi.Deps{
		Tenants: tenants.New(runner, auditModule.Recorder()),
		Rules:   rulesModule,
		Logger:  shared.logger, Clock: clk, Telemetry: tel,
		HTTP:   config.HTTP{RequestTimeout: 5 * time.Second, MaxBodyBytes: 64 << 10},
		Health: health.NewService(shared.logger), Auth: authModule, Audit: auditModule,
		Events:   events.New(runner, riskModule.Evaluator(), riskModule.RuleTester(), queue, clk),
		Risk:     riskModule,
		Alerts:   alertsModule,
		Cases:    casesModule,
		Webhooks: webhooksModule,
		Admin:    adminModule,
		Limiter:  ratelimit.New(shared.rdb, 50*time.Millisecond), RateLimits: limits,
	})
	require.NoError(t, err)
	return stack{
		handler:  h,
		tenants:  tenants.New(runner, auditModule.Recorder()),
		auth:     authModule,
		alerts:   alertsModule,
		webhooks: webhooksModule,
		risk:     riskModule,
		admin:    adminModule,
		rules:    rulesModule,
	}
}

// testFeatureKey pseudonymizes identifiers in the integration Redis.
const testFeatureKey = "integration-feature-key-0123456789abcdef"

// newRisk returns the risk module wired like cmd/api.
func newRisk(rdb risk.Redis, runner *db.TxRunner, auditModule *audit.Module, clk clock.Clock, opts ...func(*risk.Config)) *risk.Module {
	cfg := risk.Config{
		FeatureHashKey: []byte(testFeatureKey), FeatureTimeout: 250 * time.Millisecond,
		Packs: rules.New(runner, auditModule.Recorder()),
	}
	for _, o := range opts {
		o(&cfg)
	}
	return risk.New(rdb, runner, auditModule.Recorder(), clk, shared.logger, cfg)
}

type tenantFixture struct {
	id      uuid.UUID
	testKey string
}

// createTenant bootstraps a tenant the way riskosctl does.
func (s stack) createTenant(t *testing.T, name string) tenantFixture {
	t.Helper()
	var key auth.CreatedKey
	tn, err := s.tenants.Create(t.Context(), tenants.CreateParams{Name: name, Region: "eu-west-1", Actor: "riskosctl:integration"},
		func(ctx context.Context, _ tenants.Tenant) error {
			var err error
			key, err = s.auth.CreateKey(ctx, auth.CreateKeyInput{Name: "bootstrap", Scopes: authz.CustomerPermissions()})
			return err
		})
	require.NoError(t, err)
	return tenantFixture{id: tn.ID, testKey: key.Secret}
}

// productionKey issues a production key for a tenant through the operator path.
func (s stack) productionKey(t *testing.T, tenantID uuid.UUID) string {
	t.Helper()
	ctx, err := s.tenants.OperatorScope(t.Context(), tenantID, tenancy.EnvironmentProduction, "riskosctl:integration")
	require.NoError(t, err)
	key, err := s.auth.CreateKey(ctx, auth.CreateKeyInput{Name: "prod", Scopes: authz.CustomerPermissions()})
	require.NoError(t, err)
	return key.Secret
}

type response struct {
	status int
	raw    []byte
	header http.Header
}

func (r response) String() string { return string(r.raw) }

// decode parses a response into a generated API type, rejecting unknown fields, so every
// assertion also checks that the response matches the contract.
func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	dec := json.NewDecoder(bytes.NewReader(r.raw))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&v), r.String())
	return v
}

func (r response) errorCode(t *testing.T) string {
	t.Helper()
	return decode[apigen.ErrorResponse](t, r).Error.Code
}

func (s stack) call(t *testing.T, method, path, key string, body any) response {
	t.Helper()
	rec := s.rawCall(t, method, path, key, body)
	return response{status: rec.Code, raw: rec.Body.Bytes(), header: rec.Header()}
}

// rawCall sends body as JSON and returns the recorder, for tests that inspect headers.
func (s stack) rawCall(t *testing.T, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload string
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		payload = string(b)
	}
	return s.rawBody(t, method, path, key, payload)
}

// rawBody sends payload verbatim (possibly malformed).
func (s stack) rawBody(t *testing.T, method, path, key, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(payload))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if payload != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s stack) createKey(t *testing.T, key, name string, scopes ...string) (id, secret string) {
	t.Helper()
	r := s.call(t, http.MethodPost, "/api/v1/api-keys", key, map[string]any{"name": name, "scopes": scopes})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	k := decode[apigen.ApiKeyWithSecret](t, r)
	return k.ApiKey.Id, k.Secret
}

func listIDs(t *testing.T, r response) []string {
	t.Helper()
	var out []string
	for _, k := range decode[apigen.ApiKeyList](t, r).Data {
		out = append(out, k.Id)
	}
	return out
}

func auditActions(t *testing.T, r response) []string {
	t.Helper()
	var out []string
	for _, e := range decode[apigen.AuditLogList](t, r).Data {
		out = append(out, e.Action)
	}
	return out
}

func TestTenantBootstrapAndWhoami(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Acme")

	assert.True(t, strings.HasPrefix(a.testKey, "rsk_test_"))
	r := s.call(t, http.MethodGet, "/api/v1/whoami", a.testKey, nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	who := decode[apigen.Whoami](t, r)
	assert.Equal(t, ids.Format(ids.PrefixTenant, a.id), who.TenantId)
	assert.Equal(t, apigen.Environment("test"), who.Environment)
	assert.ElementsMatch(t, []apigen.Scope{
		"api_keys:read", "api_keys:write", "audit_logs:read", "events:write", "events:read", "risk:evaluate",
		"rules:read", "rules:write", "risk:configure", "alerts:read", "alerts:write", "cases:read", "cases:write",
		"webhooks:read", "webhooks:write", "team:read", "team:write", "compliance:read", "compliance:engineering",
	}, who.Scopes)

	// The bootstrap is audited: tenant, both environments and the first key.
	logs := s.call(t, http.MethodGet, "/api/v1/audit-logs", a.testKey, nil)
	require.Equal(t, http.StatusOK, logs.status)
	for _, e := range decode[apigen.AuditLogList](t, logs).Data {
		assert.Equal(t, "riskosctl:integration", e.Actor.Id)
	}
	assert.ElementsMatch(t, []string{"tenant.created", "environment.created", "environment.created", "api_key.created"}, auditActions(t, logs))
}

func TestUnauthenticatedRequestsAreRejectedUniformly(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Uniform")
	cases := map[string]string{
		"no key":         "",
		"malformed":      "not-a-key",
		"unknown lookup": "rsk_test_aaaaaaaaaaaa_" + strings.Repeat("b", 32),
		"wrong secret":   a.testKey[:len(a.testKey)-1] + map[bool]string{true: "a", false: "b"}[!strings.HasSuffix(a.testKey, "a")],
		"wrong env tag":  strings.Replace(a.testKey, "rsk_test_", "rsk_live_", 1),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			r := s.call(t, http.MethodGet, "/api/v1/whoami", key, nil)
			assert.Equal(t, http.StatusUnauthorized, r.status)
			assert.Equal(t, "UNAUTHENTICATED", r.errorCode(t))
		})
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Lifecycle")

	id, secret := s.createKey(t, a.testKey, "ci", "api_keys:read")
	r := s.call(t, http.MethodGet, "/api/v1/whoami", secret, nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, id, decode[apigen.Whoami](t, r).ApiKeyId)

	// The new key can read but not write.
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/api-keys", secret, nil).status)
	denied := s.call(t, http.MethodPost, "/api/v1/api-keys", secret, map[string]any{"name": "x", "scopes": []string{"api_keys:read"}})
	assert.Equal(t, http.StatusForbidden, denied.status)
	assert.Equal(t, "FORBIDDEN", denied.errorCode(t))

	// Listing never exposes secrets.
	list := s.call(t, http.MethodGet, "/api/v1/api-keys", a.testKey, nil)
	require.Equal(t, http.StatusOK, list.status)
	assert.NotContains(t, list.String(), secret[len(secret)-32:])
	assert.Contains(t, listIDs(t, list), id)

	// Rotation with no overlap: the old key stops working at once, the new one works.
	rot := s.call(t, http.MethodPost, "/api/v1/api-keys/"+id+"/rotate", a.testKey, map[string]any{"overlap_seconds": 0})
	require.Equal(t, http.StatusCreated, rot.status, rot.String())
	rotated := decode[apigen.ApiKeyWithSecret](t, rot)
	newSecret := rotated.Secret
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/whoami", newSecret, nil).status)
	assert.Eventually(t, func() bool {
		return s.call(t, http.MethodGet, "/api/v1/whoami", secret, nil).status == http.StatusUnauthorized
	}, 3*time.Second, 50*time.Millisecond, "rotated key must expire after the overlap")
	assert.Equal(t, http.StatusConflict, s.call(t, http.MethodPost, "/api/v1/api-keys/"+id+"/rotate", a.testKey, nil).status,
		"a rotated key cannot be rotated again")

	// Revocation is immediate on the instance that revoked, and idempotent.
	newID := rotated.ApiKey.Id
	rev := s.call(t, http.MethodPost, "/api/v1/api-keys/"+newID+"/revoke", a.testKey, nil)
	require.Equal(t, http.StatusOK, rev.status)
	assert.Equal(t, apigen.ApiKeyStatus("revoked"), decode[apigen.ApiKey](t, rev).Status)
	assert.Equal(t, http.StatusUnauthorized, s.call(t, http.MethodGet, "/api/v1/whoami", newSecret, nil).status)
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodPost, "/api/v1/api-keys/"+newID+"/revoke", a.testKey, nil).status)

	// Every change is in the audit log, once.
	logs := s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)
	count := map[string]int{}
	for _, action := range auditActions(t, logs) {
		count[action]++
	}
	assert.Equal(t, 1, count["api_key.rotated"])
	assert.Equal(t, 1, count["api_key.revoked"], "revoking twice audits once")
	assert.Equal(t, 3, count["api_key.created"], "bootstrap, ci, rotation replacement")
}

func TestScopeEscalationIsForbidden(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Escalation")
	_, writer := s.createKey(t, a.testKey, "writer", "api_keys:read", "api_keys:write")

	r := s.call(t, http.MethodPost, "/api/v1/api-keys", writer, map[string]any{"name": "sneaky", "scopes": []string{"audit_logs:read"}})
	assert.Equal(t, http.StatusForbidden, r.status, "a key cannot mint a key with a scope it lacks")

	adminID, _ := s.createKey(t, a.testKey, "admin-like", "api_keys:read", "api_keys:write", "audit_logs:read")
	r = s.call(t, http.MethodPost, "/api/v1/api-keys/"+adminID+"/rotate", writer, nil)
	assert.Equal(t, http.StatusForbidden, r.status, "a key cannot rotate a more powerful key and receive its secret")

	r = s.call(t, http.MethodPost, "/api/v1/api-keys", a.testKey, map[string]any{"name": "x", "scopes": []string{"tenants:manage"}})
	assert.Equal(t, http.StatusBadRequest, r.status, "unknown scopes are rejected")
}

// TestCrossTenantMatrix is isolation layer 3 (TENANCY.md): every operation that takes an ID is
// called with IDs of another tenant and of the other environment of the same tenant, and must
// answer exactly like a nonexistent ID.
func TestCrossTenantMatrix(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Tenant A")
	b := s.createTenant(t, "Tenant B")
	bKeyID, _ := s.createKey(t, b.testKey, "b-key", "api_keys:read")

	aProd := s.productionKey(t, a.id)
	aProdKeyID, _ := s.createKey(t, aProd, "a-prod-key", "api_keys:read")
	nonexistent := ids.Format(ids.PrefixAPIKey, ids.NewV7())

	foreign := map[string]string{
		"other tenant":          bKeyID,
		"other environment":     aProdKeyID,
		"nonexistent (control)": nonexistent,
	}
	ops := map[string]func(id string) response{
		"rotate": func(id string) response {
			return s.call(t, http.MethodPost, "/api/v1/api-keys/"+id+"/rotate", a.testKey, nil)
		},
		"revoke": func(id string) response {
			return s.call(t, http.MethodPost, "/api/v1/api-keys/"+id+"/revoke", a.testKey, nil)
		},
	}
	for opName, op := range ops {
		for idName, id := range foreign {
			t.Run(opName+"/"+idName, func(t *testing.T) {
				r := op(id)
				assert.Equal(t, http.StatusNotFound, r.status, r.String())
				e := decode[apigen.ErrorResponse](t, r).Error
				assert.Equal(t, "NOT_FOUND", e.Code)
				assert.Equal(t, "The requested resource does not exist.", e.Message)
			})
		}
	}

	t.Run("lists never include foreign keys", func(t *testing.T) {
		list := listIDs(t, s.call(t, http.MethodGet, "/api/v1/api-keys?limit=100", a.testKey, nil))
		assert.NotContains(t, list, bKeyID)
		assert.NotContains(t, list, aProdKeyID)
	})
	t.Run("audit log never includes foreign entries", func(t *testing.T) {
		logs := s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)
		assert.NotContains(t, logs.String(), bKeyID)
		assert.NotContains(t, logs.String(), aProdKeyID)
	})
	t.Run("foreign keys were not changed", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/api-keys", b.testKey, nil)
		for _, k := range decode[apigen.ApiKeyList](t, r).Data {
			assert.Equal(t, apigen.ApiKeyStatus("active"), k.Status)
		}
	})
	t.Run("a cursor from another tenant reveals nothing", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/api-keys?cursor="+bKeyID, a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status)
		for _, id := range listIDs(t, r) {
			assert.NotEqual(t, bKeyID, id)
		}
	})
	t.Run("a wrong-type ID is not found", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/api-keys/"+ids.Format(ids.PrefixTenant, b.id)+"/revoke", a.testKey, nil)
		assert.Equal(t, http.StatusNotFound, r.status)
	})
}

// TestRevocationPropagatesWithinCacheTTL: a key revoked on one instance stops working on another
// instance within the lookup cache TTL (the revocation staleness bound).
func TestRevocationPropagatesWithinCacheTTL(t *testing.T) {
	const ttl = time.Second
	instance1 := newStack(t, ttl)
	instance2 := newStack(t, ttl)
	a := instance1.createTenant(t, "Revocation")
	id, secret := instance1.createKey(t, a.testKey, "victim", "api_keys:read")

	require.Equal(t, http.StatusOK, instance2.call(t, http.MethodGet, "/api/v1/whoami", secret, nil).status, "warms instance 2's cache")
	require.Equal(t, http.StatusOK, instance1.call(t, http.MethodPost, "/api/v1/api-keys/"+id+"/revoke", a.testKey, nil).status)
	revokedAt := time.Now()

	assert.Equal(t, http.StatusUnauthorized, instance1.call(t, http.MethodGet, "/api/v1/whoami", secret, nil).status,
		"the revoking instance rejects the key immediately")
	require.Eventually(t, func() bool {
		return instance2.call(t, http.MethodGet, "/api/v1/whoami", secret, nil).status == http.StatusUnauthorized
	}, 3*ttl, 20*time.Millisecond)
	assert.Less(t, time.Since(revokedAt), ttl+500*time.Millisecond, "rejected within the TTL on every instance")
}

// TestRowLevelSecurity checks isolation layer 2 directly in SQL, as the application role.
func TestRowLevelSecurity(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "RLS A")
	b := s.createTenant(t, "RLS B")
	ctx := t.Context()

	count := func(q string, args ...any) int {
		var n int
		require.NoError(t, shared.pool.QueryRow(ctx, q, args...).Scan(&n))
		return n
	}
	t.Run("no scope sees nothing", func(t *testing.T) {
		for _, table := range []string{"tenants", "environments", "api_keys", "audit_log", "audit_chain_heads"} {
			assert.Zero(t, count("SELECT count(*) FROM "+table), table)
		}
	})
	t.Run("a scope sees only its own tenant", func(t *testing.T) {
		tx, err := shared.pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, a.id.String())
		require.NoError(t, err)
		var n int
		require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE id = $1`, b.id).Scan(&n))
		assert.Zero(t, n, "tenant B is invisible even when asked for by ID")
		require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM environments WHERE tenant_id = $1`, b.id).Scan(&n))
		assert.Zero(t, n)
	})
	t.Run("writing another tenant's rows is rejected", func(t *testing.T) {
		tx, err := shared.pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, a.id.String())
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO environments (id, tenant_id, kind) VALUES ($1, $2, 'test')`, uuid.New(), b.id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "row-level security")
	})
	t.Run("the application role cannot bypass or disable RLS", func(t *testing.T) {
		var bypass, super bool
		require.NoError(t, shared.pool.QueryRow(ctx, `SELECT rolbypassrls, rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&bypass, &super))
		assert.False(t, bypass)
		assert.False(t, super)
		_, err := shared.pool.Exec(ctx, `ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY`)
		assert.Error(t, err, "only the owner can change RLS")
	})
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Append Only")
	ctx := t.Context()

	tx, err := shared.pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, a.id.String())
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE audit_log SET action = 'forged' WHERE tenant_id = $1`, a.id)
	assert.ErrorContains(t, err, "permission denied", "the application role has no UPDATE")

	_, err = shared.owner.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, a.id)
	assert.ErrorContains(t, err, "append-only", "even the owner cannot delete through the trigger")
	_, err = shared.owner.Exec(ctx, `UPDATE audit_log SET action = 'forged' WHERE tenant_id = $1`, a.id)
	assert.ErrorContains(t, err, "append-only")
}

func TestAuditChainVerifiesAndDetectsTampering(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Tamper")
	for i := range 3 {
		s.createKey(t, a.testKey, fmt.Sprintf("k%d", i), "api_keys:read")
	}
	aProd := s.productionKey(t, a.id)
	s.createKey(t, aProd, "prod-k", "api_keys:read")

	res, err := audit.Verify(t.Context(), shared.owner, a.id)
	require.NoError(t, err)
	require.True(t, res.Valid, res.Reason)
	assert.Equal(t, int64(9), res.Entries, "4 bootstrap + 3 keys + prod key + its child, across both environments")

	// Tamper as a privileged attacker would: bypass the trigger and rewrite one entry.
	ctx := t.Context()
	_, err = shared.owner.Exec(ctx, `ALTER TABLE audit_log DISABLE TRIGGER audit_log_no_update_delete`)
	require.NoError(t, err)
	_, err = shared.owner.Exec(ctx, `UPDATE audit_log SET resource_id = 'key_forged' WHERE tenant_id = $1 AND seq = 5`, a.id)
	require.NoError(t, err)
	_, err = shared.owner.Exec(ctx, `ALTER TABLE audit_log ENABLE TRIGGER audit_log_no_update_delete`)
	require.NoError(t, err)

	res, err = audit.Verify(ctx, shared.owner, a.id)
	require.NoError(t, err)
	assert.False(t, res.Valid)
	assert.Equal(t, int64(5), res.FirstInvalidSeq)
	assert.Contains(t, res.Reason, "does not match its hash")
}

func TestConcurrentAuditAppendsKeepChainIntact(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Concurrent")

	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			r := s.call(t, http.MethodPost, "/api/v1/api-keys", a.testKey, map[string]any{"name": fmt.Sprintf("c%d", i), "scopes": []string{"api_keys:read"}})
			assert.Equal(t, http.StatusCreated, r.status)
		})
	}
	wg.Wait()

	res, err := audit.Verify(t.Context(), shared.owner, a.id)
	require.NoError(t, err)
	assert.True(t, res.Valid, res.Reason)
	assert.Equal(t, int64(4+n), res.Entries, "every append got its own, gap-free sequence number")
}

func TestSuspendedTenantIsRejected(t *testing.T) {
	s := newStack(t, 0) // no cache: the status change is visible immediately
	a := s.createTenant(t, "Suspended")
	_, err := shared.owner.Exec(t.Context(), `UPDATE tenants SET status = 'suspended' WHERE id = $1`, a.id)
	require.NoError(t, err)

	r := s.call(t, http.MethodGet, "/api/v1/whoami", a.testKey, nil)
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Equal(t, "TENANT_SUSPENDED", r.errorCode(t))

	_, err = s.tenants.OperatorScope(t.Context(), a.id, tenancy.EnvironmentProduction, "riskosctl:integration")
	assert.ErrorIs(t, err, tenants.ErrNotActive, "operators cannot issue keys for a suspended tenant either")
}
