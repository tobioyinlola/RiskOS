//go:build integration

package integration

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP, as authenticator apps compute it
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// browser is a console user's browser: it keeps the session cookie, sends the CSRF token and
// the console's Origin, and picks an environment.
type browser struct {
	s      stack
	cookie string
	csrf   string
	env    string
	origin string
}

func (s stack) browser() *browser { return &browser{s: s, origin: "http://console.test"} }

func (b *browser) do(t *testing.T, method, path string, body any) response {
	t.Helper()
	var payload string
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		payload = string(raw)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(payload))
	if payload != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if b.cookie != "" {
		req.AddCookie(&http.Cookie{Name: "riskos_session", Value: b.cookie})
	}
	if b.csrf != "" {
		req.Header.Set("X-CSRF-Token", b.csrf)
	}
	if b.env != "" {
		req.Header.Set("RiskOS-Environment", b.env)
	}
	if b.origin != "" && method != http.MethodGet {
		req.Header.Set("Origin", b.origin)
	}
	rec := httptest.NewRecorder()
	b.s.handler.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() { //nolint:bodyclose // recorder
		if c.Name == "riskos_session" {
			b.cookie = c.Value
		}
	}
	return response{status: rec.Code, raw: rec.Body.Bytes(), header: rec.Header()}
}

// refresh reads the session (and its CSRF token).
func (b *browser) refresh(t *testing.T) apigen.ConsoleSession {
	t.Helper()
	r := b.do(t, http.MethodGet, "/api/v1/auth/session", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	sess := decode[apigen.ConsoleSession](t, r)
	b.csrf = sess.CsrfToken
	return sess
}

// totp computes an authenticator-app code independently of the server (RFC 6238).
func totp(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	require.NoError(t, err)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30)) //nolint:gosec // positive
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[off:off+4])&0x7fffffff)%1_000_000)
}

// inviteToken invites email to a tenant through the operator path and returns the link token.
func (s stack) inviteToken(t *testing.T, tenant tenantFixture, email string, role authz.Role) string {
	t.Helper()
	ctx, err := s.tenants.OperatorScope(t.Context(), tenant.id, tenancy.EnvironmentTest, "riskosctl:integration")
	require.NoError(t, err)
	link, err := s.auth.Invite(ctx, email, role)
	require.NoError(t, err)
	u, err := url.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "/accept-invite", u.Path)
	token, ok := strings.CutPrefix(u.Fragment, "token=")
	require.True(t, ok, "the token is in the fragment, never sent to a server")
	return token
}

// member is a signed-in console user with their factors.
type member struct {
	b        *browser
	secret   string
	recovery []string
}

const testPassword = "a long enough passphrase"

// onboard accepts an invite, enrolls an authenticator app and returns the signed-in user.
func (s stack) onboard(t *testing.T, tenant tenantFixture, email string, role authz.Role) member {
	t.Helper()
	b := s.browser()
	r := b.do(t, http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{
		"token": s.inviteToken(t, tenant, email, role), "name": "Test " + string(role), "password": testPassword,
	})
	require.Equal(t, http.StatusOK, r.status, r.String())
	require.Equal(t, apigen.SessionState("enrollment_required"), decode[apigen.AuthState](t, r).State)
	r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/enroll", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	enroll := decode[apigen.TotpEnrollment](t, r)
	r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/confirm", map[string]any{"code": totp(t, enroll.Secret, time.Now())})
	require.Equal(t, http.StatusOK, r.status, r.String())
	st := decode[apigen.AuthState](t, r)
	require.Equal(t, apigen.SessionState("active"), st.State)
	require.NotNil(t, st.RecoveryCodes)
	b.refresh(t)
	return member{b: b, secret: enroll.Secret, recovery: *st.RecoveryCodes}
}

func TestConsoleOnboardingAndSession(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Console")
	b := s.browser()
	token := s.inviteToken(t, a, "Admin@Bank.example", authz.RoleCustomerAdmin)

	t.Run("weak password is refused", func(t *testing.T) {
		r := b.do(t, http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{"token": token, "name": "Ada", "password": "admin@bank.example"})
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	})
	r := b.do(t, http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{"token": token, "name": "Ada", "password": testPassword})
	require.Equal(t, http.StatusOK, r.status, r.String())
	assert.NotEmpty(t, b.cookie, "session cookie set")
	assert.Equal(t, http.StatusConflict, s.browser().do(t, http.MethodPost, "/api/v1/auth/invites/accept",
		map[string]any{"token": token, "name": "Eve", "password": testPassword}).status, "an invite works once")

	t.Run("no access before a second factor", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/v1/alerts", nil).status)
		sess := b.refresh(t)
		assert.Equal(t, apigen.SessionState("enrollment_required"), sess.State)
		assert.Empty(t, sess.Permissions)
	})

	r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/enroll", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	enroll := decode[apigen.TotpEnrollment](t, r)
	assert.Contains(t, enroll.OtpauthUri, "otpauth://totp/RiskOS:admin@bank.example?")
	assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/confirm", map[string]any{"code": "000000"}).status)
	r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/confirm", map[string]any{"code": totp(t, enroll.Secret, time.Now())})
	require.Equal(t, http.StatusOK, r.status, r.String())
	st := decode[apigen.AuthState](t, r)
	require.Equal(t, apigen.SessionState("active"), st.State)
	require.NotNil(t, st.RecoveryCodes)
	require.Len(t, *st.RecoveryCodes, 10)
	recovery := *st.RecoveryCodes

	sess := b.refresh(t)
	assert.Equal(t, "admin@bank.example", sess.User.Email)
	assert.Equal(t, apigen.Role("customer_admin"), sess.User.Role)
	assert.Equal(t, "Console", sess.Tenant.Name)
	assert.True(t, sess.Mfa.Totp)
	assert.Equal(t, 10, sess.Mfa.RecoveryCodesRemaining)
	assert.ElementsMatch(t, []apigen.Environment{"test", "production"}, sess.Environments)

	t.Run("the session reads and writes the tenant's data", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, b.do(t, http.MethodGet, "/api/v1/alerts", nil).status)
		r := b.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "Opened from the console"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		c := decode[apigen.Case](t, r)
		assert.Regexp(t, `^usr_`, c.CreatedBy, "actions are attributed to the user")
		logs := auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil))
		for _, want := range []string{"user.invited", "user.created", "user.mfa_enrolled", "user.login"} {
			assert.Contains(t, logs, want)
		}
	})
	t.Run("csrf and origin are enforced on unsafe requests", func(t *testing.T) {
		csrf := b.csrf
		b.csrf = ""
		assert.Equal(t, http.StatusForbidden, b.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "x"}).status)
		b.csrf = "forged"
		assert.Equal(t, http.StatusForbidden, b.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "x"}).status)
		b.csrf = csrf
		b.origin = "https://evil.example"
		assert.Equal(t, http.StatusForbidden, b.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "x"}).status)
		b.origin = "http://console.test"
	})
	t.Run("environments are chosen per request", func(t *testing.T) {
		b.env = "production"
		prodCase := decode[apigen.Case](t, b.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "Production case"}))
		b.env = "test"
		assert.Equal(t, http.StatusNotFound, b.do(t, http.MethodGet, "/api/v1/cases/"+prodCase.Id, nil).status)
		b.env = "staging"
		assert.Equal(t, http.StatusBadRequest, b.do(t, http.MethodGet, "/api/v1/cases", nil).status)
		b.env = ""
	})
	t.Run("sign out, then sign in with password and a second factor", func(t *testing.T) {
		require.Equal(t, http.StatusNoContent, b.do(t, http.MethodPost, "/api/v1/auth/logout", nil).status)
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/v1/alerts", nil).status)

		n := s.browser()
		for _, bad := range []map[string]any{
			{"email": "admin@bank.example", "password": "wrong password here"},
			{"email": "nobody@bank.example", "password": testPassword},
		} {
			r := n.do(t, http.MethodPost, "/api/v1/auth/login", bad)
			assert.Equal(t, http.StatusUnauthorized, r.status)
			assert.Equal(t, "INVALID_CREDENTIALS", r.errorCode(t), "unknown email and wrong password look the same")
		}
		r := n.do(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "ADMIN@bank.example", "password": testPassword})
		require.Equal(t, http.StatusOK, r.status, r.String())
		st := decode[apigen.AuthState](t, r)
		assert.Equal(t, apigen.SessionState("mfa_required"), st.State)
		require.NotNil(t, st.MfaMethods)
		assert.ElementsMatch(t, []apigen.AuthStateMfaMethods{"totp", "recovery_code"}, *st.MfaMethods)
		assert.Equal(t, http.StatusUnauthorized, n.do(t, http.MethodGet, "/api/v1/alerts", nil).status, "password alone is not enough")

		// The current step's code was used at enrollment; a recovery code completes sign-in.
		r = n.do(t, http.MethodPost, "/api/v1/auth/mfa/recovery/verify", map[string]any{"code": strings.ToUpper(recovery[0])})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, 9, *decode[apigen.AuthState](t, r).RecoveryCodesRemaining)
		n.refresh(t)
		assert.Equal(t, http.StatusOK, n.do(t, http.MethodGet, "/api/v1/alerts", nil).status)
		assert.Equal(t, http.StatusUnauthorized, s.browser().do(t, http.MethodPost, "/api/v1/auth/mfa/recovery/verify",
			map[string]any{"code": recovery[0]}).status, "no session")
		r = n.do(t, http.MethodPost, "/api/v1/auth/mfa/recovery/verify", map[string]any{"code": recovery[0]})
		assert.Equal(t, http.StatusUnauthorized, r.status, "a recovery code works once")
	})
}

func TestConsoleReauthRolesAndTeam(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{ReauthWindow: time.Second}, false))
	a := s.createTenant(t, "Team")
	b := s.createTenant(t, "Other team")
	admin := s.onboard(t, a, "admin@team.example", authz.RoleCustomerAdmin)
	analyst := s.onboard(t, a, "analyst@team.example", authz.RoleAnalyst)
	outsider := s.onboard(t, b, "admin@other.example", authz.RoleCustomerAdmin)

	t.Run("high-impact actions need a fresh second factor", func(t *testing.T) {
		time.Sleep(1100 * time.Millisecond)
		r := admin.b.do(t, http.MethodPost, "/api/v1/api-keys", map[string]any{"name": "ci", "scopes": []string{"events:write"}})
		assert.Equal(t, http.StatusForbidden, r.status)
		assert.Equal(t, "REAUTH_REQUIRED", r.errorCode(t))
		assert.Equal(t, http.StatusOK, admin.b.do(t, http.MethodGet, "/api/v1/alerts", nil).status, "reading is not affected")
		r = admin.b.do(t, http.MethodPost, "/api/v1/auth/mfa/recovery/verify", map[string]any{"code": admin.recovery[0]})
		require.Equal(t, http.StatusOK, r.status, r.String())
		r = admin.b.do(t, http.MethodPost, "/api/v1/api-keys", map[string]any{"name": "ci", "scopes": []string{"events:write"}})
		assert.Equal(t, http.StatusCreated, r.status, r.String())
		assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)), "user.reauthenticated")
	})
	t.Run("roles decide what a user may do", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, analyst.b.do(t, http.MethodGet, "/api/v1/alerts", nil).status)
		assert.Equal(t, http.StatusForbidden, analyst.b.do(t, http.MethodGet, "/api/v1/team/members", nil).status)
		assert.Equal(t, http.StatusForbidden, analyst.b.do(t, http.MethodGet, "/api/v1/webhooks", nil).status)
		assert.Equal(t, http.StatusForbidden, analyst.b.do(t, http.MethodGet, "/api/v1/risk/settings", nil).status)
	})
	t.Run("team management", func(t *testing.T) {
		members := decode[apigen.TeamMemberList](t, admin.b.do(t, http.MethodGet, "/api/v1/team/members", nil)).Data
		require.Len(t, members, 2)
		byEmail := map[string]apigen.TeamMember{}
		for _, m := range members {
			byEmail[m.Email] = m
			assert.True(t, m.MfaEnabled)
		}
		self := byEmail["admin@team.example"].Id
		other := byEmail["analyst@team.example"].Id

		r := admin.b.do(t, http.MethodPost, "/api/v1/auth/mfa/recovery/verify", map[string]any{"code": admin.recovery[1]})
		require.Equal(t, http.StatusOK, r.status)
		assert.Equal(t, http.StatusConflict, admin.b.do(t, http.MethodPatch, "/api/v1/team/members/"+self, map[string]any{"role": "analyst"}).status,
			"you cannot change yourself (and so cannot remove the last administrator)")
		r = admin.b.do(t, http.MethodPatch, "/api/v1/team/members/"+other, map[string]any{"role": "risk_manager"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, http.StatusUnauthorized, analyst.b.do(t, http.MethodGet, "/api/v1/alerts", nil).status,
			"a change of access ends the user's sessions")

		r = admin.b.do(t, http.MethodPost, "/api/v1/team/invites", map[string]any{"email": "admin@team.example", "role": "analyst"})
		assert.Equal(t, http.StatusConflict, r.status, "the email already has an account")
		r = admin.b.do(t, http.MethodPost, "/api/v1/team/invites", map[string]any{"email": "new@team.example", "role": "developer"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		created := decode[apigen.CreatedInvite](t, r)
		assert.Contains(t, created.InviteUrl, "http://console.test/accept-invite#token=")
		invites := decode[apigen.InviteList](t, admin.b.do(t, http.MethodGet, "/api/v1/team/invites", nil)).Data
		require.NotEmpty(t, invites)
		assert.Equal(t, apigen.InviteStatus("pending"), invites[0].Status)
		require.Equal(t, http.StatusNoContent, admin.b.do(t, http.MethodPost, "/api/v1/team/invites/"+created.Invite.Id+"/revoke", nil).status)
		token := created.InviteUrl[strings.Index(created.InviteUrl, "token=")+len("token="):]
		assert.Equal(t, http.StatusConflict, s.browser().do(t, http.MethodPost, "/api/v1/auth/invites/accept",
			map[string]any{"token": token, "name": "Late", "password": testPassword}).status, "a revoked invite cannot be used")
	})
	t.Run("tenants are isolated", func(t *testing.T) {
		c := decode[apigen.Case](t, admin.b.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "Team A only"}))
		assert.Equal(t, http.StatusNotFound, outsider.b.do(t, http.MethodGet, "/api/v1/cases/"+c.Id, nil).status)
		for _, m := range decode[apigen.TeamMemberList](t, outsider.b.do(t, http.MethodGet, "/api/v1/team/members", nil)).Data {
			assert.Equal(t, "admin@other.example", m.Email)
		}
	})
	t.Run("user tables are hidden without a scope", func(t *testing.T) {
		for _, table := range []string{"users", "console_sessions", "user_invites", "webauthn_credentials"} {
			var n int
			require.NoError(t, shared.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
			assert.Zero(t, n, table)
		}
		var hash string
		require.NoError(t, shared.owner.QueryRow(t.Context(), "SELECT password_hash FROM users WHERE email = 'admin@team.example'").Scan(&hash))
		assert.True(t, strings.HasPrefix(hash, "$argon2id$"), "passwords are stored as Argon2id hashes only")
	})
}

func TestConsoleSignInThrottling(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, true))
	a := s.createTenant(t, "Throttle")
	m := s.onboard(t, a, "victim@throttle.example", authz.RoleAnalyst)
	_ = m
	n := s.browser()
	var last response
	for range 11 {
		last = n.do(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "victim@throttle.example", "password": "guess guess guess"})
	}
	assert.Equal(t, http.StatusTooManyRequests, last.status, last.String())
	assert.Equal(t, "RATE_LIMITED", last.errorCode(t))
	r := n.do(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "victim@throttle.example", "password": testPassword})
	assert.Equal(t, http.StatusTooManyRequests, r.status, "throttled per email, even with the right password")

	t.Run("passkey steps reject malformed responses", func(t *testing.T) {
		r := m.b.do(t, http.MethodPost, "/api/v1/auth/mfa/passkeys/login", map[string]any{"credential": map[string]any{"id": "x"}})
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusConflict}, r.status, r.String())
		r = m.b.do(t, http.MethodPost, "/api/v1/auth/mfa/passkeys/login/options", nil)
		assert.Equal(t, http.StatusConflict, r.status, "no passkey registered")
	})
}
