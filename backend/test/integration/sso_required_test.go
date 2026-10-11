//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// TestSingleSignOnRequired covers a workspace that allows only single sign-on: password sign-in
// and invite acceptance open no session, the person asking must have used the provider
// themselves, other workspaces are unaffected, and the operator can lift the requirement.
func TestSingleSignOnRequired(t *testing.T) {
	idp := newMockIDP(t)
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{SSOAllowPrivate: true}, false))
	tenant := s.createTenant(t, "Required Co")
	admin := s.onboard(t, tenant, "admin@required.example", authz.RoleCustomerAdmin)
	s.onboard(t, tenant, "analyst@required.example", authz.RoleAnalyst)
	other := s.createTenant(t, "Bystander Co")
	s.onboard(t, other, "admin@bystander.example", authz.RoleCustomerAdmin)

	connection := func(extra map[string]any) map[string]any {
		body := map[string]any{"issuer": idp.srv.URL, "client_id": "riskos", "enabled": true}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	// ssoSignIn signs the person in at the provider and returns their browser, past the second
	// factor when they have one enrolled (secret is their authenticator's, empty for none).
	ssoSignIn := func(t *testing.T, email, sub, secret string) (*browser, response) {
		t.Helper()
		b := s.browser()
		r := b.do(t, http.MethodPost, "/api/v1/auth/sso/start", map[string]any{"email": email})
		require.Equal(t, http.StatusOK, r.status, r.String())
		code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl,
			map[string]any{"sub": sub, "email": email, "email_verified": true})
		r = b.do(t, http.MethodPost, "/api/v1/auth/sso/callback", map[string]any{"code": code, "state": state})
		if r.status == http.StatusOK && secret != "" {
			v := b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/verify", map[string]any{"code": totp(t, secret, time.Now().Add(30*time.Second))})
			require.Equal(t, http.StatusOK, v.status, v.String())
			b.refresh(t)
		}
		return b, r
	}
	passwordLogin := func(email, password string) response {
		return s.browser().do(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": password})
	}

	t.Run("it needs a connection that has worked, and a person who has used it", func(t *testing.T) {
		r := admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"client_secret": "s3cret", "required": true}))
		assert.Equal(t, http.StatusBadRequest, r.status, "not in the same step as creating the connection")
		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"client_secret": "s3cret"}))
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.False(t, decode[apigen.SSOConnection](t, r).Required)

		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"required": true}))
		assert.Equal(t, http.StatusBadRequest, r.status, "the administrator has not signed in through the provider")
		assert.Equal(t, http.StatusOK, passwordLogin("admin@required.example", testPassword).status, "passwords still work")
		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"enabled": false, "required": true}))
		assert.Equal(t, http.StatusBadRequest, r.status, "not while the connection is off")
	})

	t.Run("an administrator who signed in through the provider can require it", func(t *testing.T) {
		b, r := ssoSignIn(t, "admin@required.example", "idp-admin", admin.secret)
		require.Equal(t, http.StatusOK, r.status, r.String())
		r = b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"client_secret": "rotated", "required": true}))
		assert.Equal(t, http.StatusBadRequest, r.status, "the connection is not changed in the same step")
		r = b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"required": true}))
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.True(t, decode[apigen.SSOConnection](t, r).Required)
		settings := decode[apigen.SSOSettings](t, b.do(t, http.MethodGet, "/api/v1/sso", nil))
		require.NotNil(t, settings.Connection)
		assert.True(t, settings.Connection.Required)
		admin.b = b
	})

	t.Run("passwords no longer open a session", func(t *testing.T) {
		r := passwordLogin("analyst@required.example", testPassword)
		assert.Equal(t, http.StatusForbidden, r.status)
		assert.Contains(t, r.String(), "SSO_REQUIRED")
		assert.Empty(t, r.header.Values("Set-Cookie"), "no session is opened")
		assert.Equal(t, http.StatusUnauthorized, passwordLogin("analyst@required.example", "not the password").status,
			"a wrong password is not told apart from before")
		assert.Equal(t, http.StatusUnauthorized, passwordLogin("nobody@required.example", testPassword).status)
		assert.Equal(t, http.StatusOK, passwordLogin("admin@bystander.example", testPassword).status, "another workspace is unaffected")
	})

	t.Run("single sign-on still works, and so does a new person's invite", func(t *testing.T) {
		_, r := ssoSignIn(t, "analyst@required.example", "idp-analyst", "")
		require.Equal(t, http.StatusOK, r.status, r.String())

		b := s.browser()
		r = b.do(t, http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{
			"token": s.inviteToken(t, tenant, "newcomer@required.example", authz.RoleAnalyst), "name": "New Person", "password": testPassword,
		})
		assert.Equal(t, http.StatusForbidden, r.status, "the invite makes the account but a password is not a way in")
		assert.Contains(t, r.String(), "SSO_REQUIRED")
		assert.Empty(t, r.header.Values("Set-Cookie"))
		assert.Equal(t, http.StatusForbidden, passwordLogin("newcomer@required.example", testPassword).status)
		_, r = ssoSignIn(t, "newcomer@required.example", "idp-newcomer", "")
		assert.Equal(t, http.StatusOK, r.status, r.String())
	})

	t.Run("the operator can lift it when the provider is down", func(t *testing.T) {
		ctx, err := s.tenants.OperatorScope(t.Context(), tenant.id, tenancy.EnvironmentTest, "riskosctl:integration")
		require.NoError(t, err)
		lifted, err := s.auth.LiftSSORequirement(ctx)
		require.NoError(t, err)
		assert.True(t, lifted)
		lifted, err = s.auth.LiftSSORequirement(ctx)
		require.NoError(t, err)
		assert.False(t, lifted, "nothing left to lift")

		assert.Equal(t, http.StatusOK, passwordLogin("analyst@required.example", testPassword).status)
		settings := decode[apigen.SSOSettings](t, admin.b.do(t, http.MethodGet, "/api/v1/sso", nil))
		require.NotNil(t, settings.Connection)
		assert.False(t, settings.Connection.Required)
		assert.True(t, settings.Connection.Enabled, "the connection itself is kept")

		actions := auditActions(t, admin.b.do(t, http.MethodGet, "/api/v1/audit-logs?limit=100", nil))
		for _, a := range []string{"sso.requirement_lifted", "user.login_failed"} {
			assert.Contains(t, actions, a)
		}
	})
}
