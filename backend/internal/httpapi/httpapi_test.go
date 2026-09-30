package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases"
	"github.com/tobioyinlola/RiskOS/backend/internal/events"
	"github.com/tobioyinlola/RiskOS/backend/internal/httpapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

type fixture struct {
	handler http.Handler
	logs    *bytes.Buffer
}

func newFixture(t testing.TB, probes map[string]error) fixture {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := logging.New(logs, slog.LevelDebug, "test")
	tel, err := telemetry.Setup(context.Background(), config.Telemetry{}, "test", config.EnvTest)
	require.NoError(t, err)

	checks := make([]health.Check, 0, len(probes))
	for name, perr := range probes {
		checks = append(checks, health.Check{Name: name, Timeout: time.Second, Probe: func(context.Context) error { return perr }})
	}
	authModule, auditModule := testModules()
	eventsMod, riskMod, alertsMod, casesMod := appModules()
	return fixture{
		logs: logs,
		handler: must(t)(httpapi.New(httpapi.Deps{
			Logger:    logger,
			Clock:     clock.System{},
			HTTP:      config.HTTP{RequestTimeout: 5 * time.Second, MaxBodyBytes: 1024},
			Telemetry: tel,
			Health:    health.NewService(logger, checks...),
			Auth:      authModule,
			Audit:     auditModule,
			Events:    eventsMod, Risk: riskMod, Alerts: alertsMod, Cases: casesMod,
			Limiter: allowAll{},
		})),
	}
}

func (f fixture) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func healthy() map[string]error { return map[string]error{"postgres": nil, "redis": nil} }

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) apigen.ErrorDetail {
	t.Helper()
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	dec := json.NewDecoder(rec.Body)
	dec.DisallowUnknownFields() // the contract is exactly {"error":{code,message,request_id}}
	var body apigen.ErrorResponse
	require.NoError(t, dec.Decode(&body))
	return body.Error
}

func TestLiveness(t *testing.T) {
	f := newFixture(t, map[string]error{"postgres": errors.New("down")})
	rec := f.do(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/live", nil))

	require.Equal(t, http.StatusOK, rec.Code, "liveness must not depend on dependencies")
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
	assert.Regexp(t, `^req_[0-9a-z]{26}$`, rec.Header().Get("X-Request-ID"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		probes     map[string]error
		wantStatus int
		wantBody   string
	}{
		{"all dependencies up", healthy(), http.StatusOK, `{"status":"ok","checks":{"postgres":"ok","redis":"ok"}}`},
		{"postgres down", map[string]error{"postgres": errors.New("dial tcp: refused"), "redis": nil}, http.StatusServiceUnavailable, `{"status":"unavailable","checks":{"postgres":"unavailable","redis":"ok"}}`},
		{"redis down", map[string]error{"postgres": nil, "redis": errors.New("i/o timeout")}, http.StatusServiceUnavailable, `{"status":"unavailable","checks":{"postgres":"ok","redis":"unavailable"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.probes)
			rec := f.do(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/ready", nil))
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "refused", "internal error details must not leak")
		})
	}
}

// TestErrorContract is acceptance criterion 3: malformed requests get the spec's error contract,
// and the request_id in the body matches the response header and the log line.
func TestErrorContract(t *testing.T) {
	tests := []struct {
		name       string
		req        func() *http.Request
		wantStatus int
		wantCode   string
	}{
		{"unknown route", func() *http.Request { return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil) }, http.StatusNotFound, "NOT_FOUND"},
		{"versioned route without a key", func() *http.Request {
			return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/api-keys", nil)
		}, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"unknown versioned route without a key", func() *http.Request {
			return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/nope", nil)
		}, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"wrong method", func() *http.Request {
			return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/health/live", strings.NewReader(`{"malformed":`))
		}, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"declared body too large", func() *http.Request {
			return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/events", strings.NewReader(strings.Repeat("x", 2048)))
		}, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, healthy())
			rec := f.do(tt.req())

			assert.Equal(t, tt.wantStatus, rec.Code)
			e := decodeError(t, rec)
			assert.Equal(t, tt.wantCode, e.Code)
			assert.NotEmpty(t, e.Message)
			assert.Equal(t, rec.Header().Get("X-Request-ID"), e.RequestId)
			assert.Contains(t, f.logs.String(), `"request_id":"`+e.RequestId+`"`, "request_id must appear in logs")
		})
	}
}

func TestRequestIDPropagation(t *testing.T) {
	tests := []struct {
		name    string
		inbound string
		echoed  bool
	}{
		{"safe inbound id is echoed", "client-abc_123.x", true},
		{"unsafe inbound id is replaced", "bad id\r\nInjected: yes", false},
		{"overlong inbound id is replaced", strings.Repeat("a", 65), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, healthy())
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/live", nil)
			req.Header.Set("X-Request-ID", tt.inbound)
			got := f.do(req).Header().Get("X-Request-ID")
			if tt.echoed {
				assert.Equal(t, tt.inbound, got)
			} else {
				assert.NotEqual(t, tt.inbound, got)
				assert.Regexp(t, `^req_`, got)
			}
		})
	}
}

func TestAccessLogOmitsSecrets(t *testing.T) {
	f := newFixture(t, healthy())
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/live?api_key=sk_live_leak", nil)
	req.Header.Set("Authorization", "Bearer xyz-secret")
	f.do(req)

	logs := f.logs.String()
	assert.Contains(t, logs, `"route":"/health/live"`)
	assert.NotContains(t, logs, "sk_live_leak", "query strings are never logged")
	assert.NotContains(t, logs, "xyz-secret", "headers are never logged")
}

// BenchmarkLiveness measures the full middleware stack and routing for the cheapest endpoint,
// excluding the network. It is the in-process latency baseline (docs/PERFORMANCE_BASELINE.md).
func BenchmarkLiveness(b *testing.B) {
	logger := logging.New(io.Discard, slog.LevelInfo, "bench") // production logger, incl. redaction
	tel, err := telemetry.Setup(context.Background(), config.Telemetry{}, "bench", config.EnvTest)
	require.NoError(b, err)
	authModule, auditModule := testModules()
	eventsMod, riskMod, alertsMod, casesMod := appModules()
	h := must(b)(httpapi.New(httpapi.Deps{
		Logger:    logger,
		Clock:     clock.System{},
		HTTP:      config.HTTP{RequestTimeout: 5 * time.Second, MaxBodyBytes: 1 << 20},
		Telemetry: tel,
		Health:    health.NewService(logger),
		Auth:      authModule,
		Audit:     auditModule,
		Events:    eventsMod, Risk: riskMod, Alerts: alertsMod, Cases: casesMod,
		Limiter: allowAll{},
	}))
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/health/live", nil)
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

// noKeys is a key store that knows no keys: every /api/v1 request is unauthenticated. Tests of
// authenticated behavior run against PostgreSQL in test/integration.
type noKeys struct{}

func (noKeys) QueryRow(context.Context, string, ...any) pgx.Row { return noRow{} }

type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }

// noDB fails any transaction; these tests never reach one.
type noDB struct{}

func (noDB) Run(context.Context, func(context.Context) error) error {
	return errors.New("no database in this test")
}

func testModules() (*auth.Module, *audit.Module) {
	auditModule := audit.New(noDB{}, clock.System{})
	return auth.New(noKeys{}, noDB{}, auditModule.Recorder(), clock.System{}, auth.Config{
		Pepper: []byte(strings.Repeat("p", 32)),
	}), auditModule
}

// allowAll is a rate limiter that never limits.
type allowAll struct{}

func (allowAll) Allow(context.Context, ...ratelimit.Rule) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true}, nil
}

// appModules returns the events and risk modules over a database that is never reached and a
// Redis that does not exist; these tests never evaluate an event.
// noJobs discards enqueued jobs.
type noJobs struct{}

func (noJobs) Enqueue(context.Context, jobs.Args) error { return nil }

func appModules() (*events.Module, *risk.Module, *alerts.Module, *cases.Module) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	riskModule := risk.New(rdb, noDB{}, audit.New(noDB{}, clock.System{}).Recorder(), clock.System{},
		slog.New(slog.DiscardHandler), risk.Config{FeatureHashKey: []byte(strings.Repeat("f", 32)), FeatureTimeout: 10 * time.Millisecond})
	alertsModule := alerts.New(noDB{}, audit.New(noDB{}, clock.System{}).Recorder(), clock.System{}, slog.New(slog.DiscardHandler))
	casesModule := cases.New(noDB{}, alertsModule.Linker(), audit.New(noDB{}, clock.System{}).Recorder(), clock.System{})
	return events.New(noDB{}, riskModule.Evaluator(), riskModule.RuleTester(), noJobs{}, clock.System{}), riskModule, alertsModule, casesModule
}

func must(t testing.TB) func(http.Handler, error) http.Handler {
	return func(h http.Handler, err error) http.Handler {
		t.Helper()
		require.NoError(t, err)
		return h
	}
}
