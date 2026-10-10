//go:build integration

package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

const staffSSORedirect = testOrigin + "/admin/sso/callback"

func staffSSOConfig(idpURL string, required bool) admin.SSOConfig {
	return admin.SSOConfig{
		Issuer: idpURL, ClientID: "riskos-staff", ClientSecret: "staff-s3cret",
		RedirectURL: staffSSORedirect, Required: required, AllowPrivate: true,
	}
}

// ssoStart plays the browser's first step and returns the provider's authorization URL.
func ssoStart(t *testing.T, b *staffBrowser) string {
	t.Helper()
	r := b.do(t, http.MethodPost, "/api/admin/v1/auth/sso/start", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	require.NotEmpty(t, b.sso, "the request is bound to this browser")
	return decode[adminapi.StaffSSOStart](t, r).RedirectUrl
}

// ssoCallback runs a sign-in through the provider's step with claims, as the person at b.
func ssoCallback(t *testing.T, b *staffBrowser, idp *mockIDP, claims map[string]any) response {
	t.Helper()
	code, state := idp.authorizeAt(t, ssoStart(t, b), staffSSORedirect, claims)
	return b.do(t, http.MethodPost, "/api/admin/v1/auth/sso/callback", map[string]any{"code": code, "state": state})
}

func ssoLogin(t *testing.T, b *staffBrowser, r response, key *softKey) response {
	t.Helper()
	require.Equal(t, http.StatusOK, r.status, r.String())
	c := decode[adminapi.Ceremony](t, r)
	return b.do(t, http.MethodPost, "/api/admin/v1/auth/sso/login", map[string]any{"ceremony_id": c.CeremonyId, "credential": key.assert(t, c.Options)})
}

func staffAuditCount(t *testing.T, action, reasonLike string) int {
	t.Helper()
	var n int
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT count(*) FROM staff_audit_log WHERE action = $1 AND metadata::text LIKE $2`, action, "%"+reasonLike+"%").Scan(&n))
	return n
}

// TestStaffSSO covers staff sign-in through the platform's identity provider: the provider's step
// binds a subject and identifies the staff member, and the staff member's passkey still opens the
// session.
func TestStaffSSO(t *testing.T) {
	idp := newMockIDP(t)
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin(), withStaffSSO(staffSSOConfig(idp.srv.URL, false)))

	token, err := s.admin.CreateStaff(t.Context(), "sso-ops@riskos.example", "SSO Ops", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	_, key := enroll(t, s, token)
	claims := map[string]any{"sub": "idp-ops", "email": "sso-ops@riskos.example", "email_verified": true}

	t.Run("methods", func(t *testing.T) {
		r := (&staffBrowser{s: s}).do(t, http.MethodGet, "/api/admin/v1/auth/methods", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		m := decode[adminapi.StaffAuthMethods](t, r)
		assert.True(t, m.SsoEnabled)
		assert.False(t, m.SsoRequired)
	})

	t.Run("provider step, then passkey", func(t *testing.T) {
		b := &staffBrowser{s: s}
		r := ssoCallback(t, b, idp, claims)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Empty(t, b.cookie, "the provider's step alone opens no session")
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/admin/v1/tenants", nil).status)

		r = ssoLogin(t, b, r, key)
		b.signedIn(t, r)
		assert.Equal(t, "sso-ops@riskos.example", decode[adminapi.StaffSession](t, r).Staff.Email)
		assert.Equal(t, http.StatusOK, b.do(t, http.MethodGet, "/api/admin/v1/tenants", nil).status)
		assert.Equal(t, 1, staffAuditCount(t, "staff.sso_bound", "issuer"))
		assert.GreaterOrEqual(t, staffAuditCount(t, "staff.signed_in", "sso+passkey"), 1)
	})

	t.Run("later sign-ins are found by subject", func(t *testing.T) {
		b := &staffBrowser{s: s}
		other := map[string]any{"sub": "idp-ops", "email": "renamed@riskos.example", "email_verified": false}
		b.signedIn(t, ssoLogin(t, b, ssoCallback(t, b, idp, other), key))
		assert.Equal(t, 1, staffAuditCount(t, "staff.sso_bound", "issuer"), "bound once")
	})

	t.Run("another subject cannot take a bound account", func(t *testing.T) {
		b := &staffBrowser{s: s}
		r := ssoCallback(t, b, idp, map[string]any{"sub": "idp-someone-else", "email": "sso-ops@riskos.example", "email_verified": true})
		assert.Equal(t, http.StatusUnauthorized, r.status)
		assert.Equal(t, "SSO_FAILED", r.errorCode(t))
		assert.GreaterOrEqual(t, staffAuditCount(t, "staff.sso_failed", "subject_mismatch"), 1)
	})

	t.Run("unknown people and unverified emails are refused alike", func(t *testing.T) {
		unknown := ssoCallback(t, &staffBrowser{s: s}, idp, map[string]any{"sub": "idp-x", "email": "nobody@riskos.example", "email_verified": true})
		assert.Equal(t, http.StatusUnauthorized, unknown.status)
		assert.Equal(t, "SSO_FAILED", unknown.errorCode(t))

		// A staff member with a passkey but no bound subject: the provider must vouch for the email.
		tok, err := s.admin.CreateStaff(t.Context(), "unverified@riskos.example", "Unverified", authz.RoleSecurityAdmin, "integration")
		require.NoError(t, err)
		_, _ = enroll(t, s, tok)
		r := ssoCallback(t, &staffBrowser{s: s}, idp, map[string]any{"sub": "idp-unv", "email": "unverified@riskos.example", "email_verified": false})
		assert.Equal(t, http.StatusUnauthorized, r.status)
		assert.Equal(t, unknown.errorCode(t), r.errorCode(t))
		assert.GreaterOrEqual(t, staffAuditCount(t, "staff.sso_failed", "email_unverified"), 1)
	})

	t.Run("a passkey is needed first", func(t *testing.T) {
		_, err := s.admin.CreateStaff(t.Context(), "nokey@riskos.example", "No Key", authz.RoleOperationsAdmin, "integration")
		require.NoError(t, err)
		r := ssoCallback(t, &staffBrowser{s: s}, idp, map[string]any{"sub": "idp-nokey", "email": "nokey@riskos.example", "email_verified": true})
		assert.Equal(t, http.StatusUnauthorized, r.status)
		assert.GreaterOrEqual(t, staffAuditCount(t, "staff.sso_failed", "no_passkey"), 1)
	})

	t.Run("disabled staff are refused", func(t *testing.T) {
		tok, err := s.admin.CreateStaff(t.Context(), "leaver@riskos.example", "Leaver", authz.RoleOperationsAdmin, "integration")
		require.NoError(t, err)
		_, leaverKey := enroll(t, s, tok)
		b := &staffBrowser{s: s}
		lc := map[string]any{"sub": "idp-leaver", "email": "leaver@riskos.example", "email_verified": true}
		cb := ssoCallback(t, b, idp, lc) // the provider step succeeded...
		require.Equal(t, http.StatusOK, cb.status)
		require.NoError(t, s.admin.SetStaffStatus(t.Context(), "leaver@riskos.example", false, "integration"))
		// ...but the account was disabled before the passkey step.
		assert.Equal(t, http.StatusUnauthorized, ssoLogin(t, b, cb, leaverKey).status)
		assert.Equal(t, http.StatusUnauthorized, ssoCallback(t, &staffBrowser{s: s}, idp, lc).status)
	})

	t.Run("requests and ceremonies work once, in their own browser", func(t *testing.T) {
		b := &staffBrowser{s: s}
		code, state := idp.authorizeAt(t, ssoStart(t, b), staffSSORedirect, claims)
		stranger := &staffBrowser{s: s}
		ssoStart(t, stranger) // has a cookie of its own, but not this request's
		r := stranger.do(t, http.MethodPost, "/api/admin/v1/auth/sso/callback", map[string]any{"code": code, "state": state})
		assert.Equal(t, http.StatusUnauthorized, r.status, "another browser cannot finish it")
		r = b.do(t, http.MethodPost, "/api/admin/v1/auth/sso/callback", map[string]any{"code": code, "state": state})
		assert.Equal(t, http.StatusUnauthorized, r.status, "the failed attempt used the request up")

		cb := ssoCallback(t, b, idp, claims)
		require.Equal(t, http.StatusOK, cb.status)
		wrong := newSoftKey(t)
		wrong.id, wrong.user, wrong.rpHash = key.id, key.user, key.rpHash
		assert.Equal(t, http.StatusUnauthorized, ssoLogin(t, b, cb, wrong).status, "another key with the same ID fails")
		assert.Equal(t, http.StatusUnauthorized, ssoLogin(t, b, cb, key).status, "and the ceremony is used up")
	})

	t.Run("forged tokens are refused", func(t *testing.T) {
		forged, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		idp.forge = forged
		defer func() { idp.forge = nil }()
		r := ssoCallback(t, &staffBrowser{s: s}, idp, claims)
		assert.Equal(t, http.StatusUnauthorized, r.status)
	})

	t.Run("unbinding lets the next sign-in bind afresh", func(t *testing.T) {
		require.NoError(t, s.admin.UnbindSSO(t.Context(), "sso-ops@riskos.example", "integration"))
		assert.Error(t, s.admin.UnbindSSO(t.Context(), "sso-ops@riskos.example", "integration"), "nothing left to unbind")
		b := &staffBrowser{s: s}
		moved := map[string]any{"sub": "idp-ops-new", "email": "sso-ops@riskos.example", "email_verified": true}
		b.signedIn(t, ssoLogin(t, b, ssoCallback(t, b, idp, moved), key))
		assert.Equal(t, http.StatusUnauthorized, ssoCallback(t, &staffBrowser{s: s}, idp, claims).status, "the old subject no longer works")
	})
}

// TestStaffSSORequired covers a deployment that requires the provider's step.
func TestStaffSSORequired(t *testing.T) {
	idp := newMockIDP(t)
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin(), withStaffSSO(staffSSOConfig(idp.srv.URL, true)))

	token, err := s.admin.CreateStaff(t.Context(), "req-ops@riskos.example", "Req Ops", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)

	// Enrolling a passkey from the link registers it but opens no session.
	b := &staffBrowser{s: s}
	key := newSoftKey(t)
	r := b.do(t, http.MethodPost, "/api/admin/v1/auth/enroll/options", map[string]any{"token": token})
	require.Equal(t, http.StatusOK, r.status, r.String())
	c := decode[adminapi.Ceremony](t, r)
	r = b.do(t, http.MethodPost, "/api/admin/v1/auth/enroll", map[string]any{
		"token": token, "ceremony_id": c.CeremonyId, "name": "Laptop", "credential": key.register(t, c.Options),
	})
	assert.Equal(t, http.StatusForbidden, r.status, r.String())
	assert.Equal(t, "SSO_REQUIRED", r.errorCode(t))
	assert.Empty(t, b.cookie)
	var passkeys int
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT count(*) FROM staff_passkeys p JOIN staff s ON s.id = p.staff_id WHERE s.email = 'req-ops@riskos.example'`).Scan(&passkeys))
	assert.Equal(t, 1, passkeys, "the passkey is registered")

	// Direct passkey sign-in is refused.
	d := &staffBrowser{s: s}
	r = d.do(t, http.MethodPost, "/api/admin/v1/auth/login/options", nil)
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Equal(t, "SSO_REQUIRED", r.errorCode(t))
	r = d.do(t, http.MethodPost, "/api/admin/v1/auth/login", map[string]any{"ceremony_id": "x", "credential": map[string]any{}})
	assert.Equal(t, http.StatusForbidden, r.status)

	m := decode[adminapi.StaffAuthMethods](t, d.do(t, http.MethodGet, "/api/admin/v1/auth/methods", nil))
	assert.True(t, m.SsoRequired)

	// The provider's step and the passkey still sign in.
	e := &staffBrowser{s: s}
	claims := map[string]any{"sub": "idp-req", "email": "req-ops@riskos.example", "email_verified": true}
	e.signedIn(t, ssoLogin(t, e, ssoCallback(t, e, idp, claims), key))
	assert.Equal(t, http.StatusOK, e.do(t, http.MethodGet, "/api/admin/v1/tenants", nil).status)
}

// TestStaffSSONotConfigured: without a provider the endpoints say so and passkeys work as before.
func TestStaffSSONotConfigured(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	b := &staffBrowser{s: s}
	assert.Equal(t, http.StatusNotFound, b.do(t, http.MethodPost, "/api/admin/v1/auth/sso/start", nil).status)
	m := decode[adminapi.StaffAuthMethods](t, b.do(t, http.MethodGet, "/api/admin/v1/auth/methods", nil))
	assert.False(t, m.SsoEnabled)
	assert.False(t, m.SsoRequired)
}
