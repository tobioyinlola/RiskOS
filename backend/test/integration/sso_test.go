//go:build integration

package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// mockIDP is a minimal OpenID Connect provider: discovery, keys, and a token endpoint that
// checks the PKCE verifier and issues the ID token a test prepared for a code.
type mockIDP struct {
	srv *httptest.Server
	key *rsa.PrivateKey
	// forge, when set, signs ID tokens with a key the provider does not publish.
	forge *rsa.PrivateKey
	mu    sync.Mutex
	code  map[string]idpGrant
}

type idpGrant struct {
	challenge string
	claims    map[string]any
}

func newMockIDP(t *testing.T) *mockIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	m := &mockIDP{key: key, code: map[string]idpGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer": m.srv.URL, "authorization_endpoint": m.srv.URL + "/authorize",
			"token_endpoint": m.srv.URL + "/token", "jwks_uri": m.srv.URL + "/jwks",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		g, ok := m.code[r.PostForm.Get("code")]
		delete(m.code, r.PostForm.Get("code"))
		m.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]string{"error": "invalid_grant"})
			return
		}
		writeJSON(w, map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60, "id_token": m.sign(t, g.claims)})
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (m *mockIDP) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	key := m.key
	if m.forge != nil {
		key = m.forge
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}}, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(claims)
	require.NoError(t, err)
	obj, err := signer.Sign(raw)
	require.NoError(t, err)
	out, err := obj.CompactSerialize()
	require.NoError(t, err)
	return out
}

// authorize plays the person signing in at the provider: it reads the authorization URL and
// prepares a code whose ID token carries claims (plus the standard ones and, unless overridden,
// the request's nonce). It returns the code and the state to send back.
func (m *mockIDP) authorize(t *testing.T, authURL string, claims map[string]any) (code, state string) {
	t.Helper()
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, m.srv.URL+"/authorize", u.Scheme+"://"+u.Host+u.Path)
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.Equal(t, "http://console.test/sso/callback", q.Get("redirect_uri"))
	require.NotEmpty(t, q.Get("nonce"))
	now := time.Now()
	full := map[string]any{"iss": m.srv.URL, "aud": q.Get("client_id"), "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "nonce": q.Get("nonce")}
	for k, v := range claims {
		full[k] = v
	}
	code = "code-" + rand.Text()
	m.mu.Lock()
	m.code[code] = idpGrant{challenge: q.Get("code_challenge"), claims: full}
	m.mu.Unlock()
	return code, q.Get("state")
}

// TestSingleSignOn covers OIDC sign-in end to end against a mock provider.
func TestSingleSignOn(t *testing.T) {
	idp := newMockIDP(t)
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{SSOAllowPrivate: true}, false))
	tenant := s.createTenant(t, "SSO Co")
	admin := s.onboard(t, tenant, "admin@sso.example", authz.RoleCustomerAdmin)
	s.onboard(t, tenant, "analyst@sso.example", authz.RoleAnalyst)

	start := func(b *browser, email string) response {
		return b.do(t, http.MethodPost, "/api/v1/auth/sso/start", map[string]any{"email": email})
	}
	callback := func(b *browser, code, state string) response {
		return b.do(t, http.MethodPost, "/api/v1/auth/sso/callback", map[string]any{"code": code, "state": state})
	}

	t.Run("not set up", func(t *testing.T) {
		r := start(s.browser(), "admin@sso.example")
		assert.Equal(t, http.StatusNotFound, r.status)
		assert.Contains(t, r.String(), "SSO_NOT_CONFIGURED")
		assert.Equal(t, http.StatusNotFound, start(s.browser(), "nobody@sso.example").status, "unknown emails look the same")
		settings := decode[apigen.SSOSettings](t, admin.b.do(t, http.MethodGet, "/api/v1/sso", nil))
		assert.Equal(t, "http://console.test/sso/callback", settings.RedirectUri)
		assert.Nil(t, settings.Connection)
	})

	t.Run("configure", func(t *testing.T) {
		r := admin.b.do(t, http.MethodPut, "/api/v1/sso", map[string]any{"issuer": "http://127.0.0.1:1", "client_id": "riskos", "client_secret": "s3cret", "enabled": true})
		assert.Equal(t, http.StatusBadRequest, r.status, "an issuer RiskOS cannot read is refused")
		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", map[string]any{"issuer": idp.srv.URL, "client_id": "riskos", "enabled": true})
		assert.Equal(t, http.StatusBadRequest, r.status, "the first setup needs the secret")
		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", map[string]any{"issuer": idp.srv.URL, "client_id": "riskos", "client_secret": "s3cret", "enabled": true})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.NotContains(t, r.String(), "s3cret")
		settings := decode[apigen.SSOSettings](t, admin.b.do(t, http.MethodGet, "/api/v1/sso", nil))
		require.NotNil(t, settings.Connection)
		assert.Equal(t, idp.srv.URL, settings.Connection.Issuer)
		var sealed []byte
		require.NoError(t, shared.owner.QueryRow(t.Context(), `SELECT client_secret_enc FROM sso_connections WHERE tenant_id = $1`, tenant.id).Scan(&sealed))
		assert.NotContains(t, string(sealed), "s3cret", "the client secret is stored encrypted")
	})

	t.Run("sign in, then RiskOS's second factor", func(t *testing.T) {
		b := s.browser()
		r := start(b, "Admin@SSO.example")
		require.Equal(t, http.StatusOK, r.status, r.String())
		require.NotEmpty(t, b.sso, "the request is bound to this browser")
		code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl,
			map[string]any{"sub": "idp-admin", "email": "admin@sso.example", "email_verified": true})

		assert.Equal(t, http.StatusUnauthorized, callback(s.browser(), code, state).status, "another browser cannot finish it")
		// The failed attempt consumed the request: the real browser starts again.
		r = start(b, "admin@sso.example")
		require.Equal(t, http.StatusOK, r.status)
		code, state = idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl,
			map[string]any{"sub": "idp-admin", "email": "admin@sso.example", "email_verified": true})
		r = callback(b, code, state)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, apigen.SessionState("mfa_required"), decode[apigen.AuthState](t, r).State)
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/v1/alerts", nil).status, "no access before the second factor")
		r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/verify", map[string]any{"code": totp(t, admin.secret, time.Now().Add(30*time.Second))})
		require.Equal(t, http.StatusOK, r.status, r.String())
		b.refresh(t)
		assert.Equal(t, http.StatusOK, b.do(t, http.MethodGet, "/api/v1/alerts", nil).status)

		assert.Equal(t, http.StatusUnauthorized, callback(b, code, state).status, "a state works once")
	})

	t.Run("the provider must vouch for the account", func(t *testing.T) {
		try := func(email string, claims map[string]any) int {
			b := s.browser()
			r := start(b, email)
			require.Equal(t, http.StatusOK, r.status, r.String())
			code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl, claims)
			return callback(b, code, state).status
		}
		assert.Equal(t, http.StatusUnauthorized, try("analyst@sso.example",
			map[string]any{"sub": "idp-analyst", "email": "analyst@sso.example", "email_verified": false}), "unverified email")
		assert.Equal(t, http.StatusUnauthorized, try("analyst@sso.example",
			map[string]any{"sub": "idp-analyst", "email": "someone-else@sso.example", "email_verified": true}), "another email")
		assert.Equal(t, http.StatusUnauthorized, try("admin@sso.example",
			map[string]any{"sub": "idp-impostor", "email": "admin@sso.example", "email_verified": true}), "the bound subject must match")
		assert.Equal(t, http.StatusUnauthorized, try("analyst@sso.example",
			map[string]any{"sub": "idp-admin", "email": "analyst@sso.example", "email_verified": true}), "a subject belongs to one account")
		assert.Equal(t, http.StatusUnauthorized, try("analyst@sso.example",
			map[string]any{"sub": "idp-analyst", "email": "analyst@sso.example", "email_verified": true, "nonce": "replayed"}), "the nonce must match")
		assert.Equal(t, http.StatusOK, try("analyst@sso.example",
			map[string]any{"sub": "idp-analyst", "email": "analyst@sso.example", "email_verified": "true"}))
	})

	t.Run("tokens and requests must verify", func(t *testing.T) {
		try := func(claims map[string]any) int {
			b := s.browser()
			r := start(b, "admin@sso.example")
			require.Equal(t, http.StatusOK, r.status, r.String())
			code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl, claims)
			return callback(b, code, state).status
		}
		good := func(extra map[string]any) map[string]any {
			c := map[string]any{"sub": "idp-admin", "email": "admin@sso.example", "email_verified": true}
			for k, v := range extra {
				c[k] = v
			}
			return c
		}
		assert.Equal(t, http.StatusOK, try(good(nil)), "the baseline signs in")
		assert.Equal(t, http.StatusUnauthorized, try(good(map[string]any{"aud": "another-client"})), "audience")
		assert.Equal(t, http.StatusUnauthorized, try(good(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})), "expired token")
		assert.Equal(t, http.StatusUnauthorized, try(good(map[string]any{"iss": "https://impostor.example"})), "issuer")

		forged, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		idp.forge = forged
		assert.Equal(t, http.StatusUnauthorized, try(good(nil)), "a key the provider does not publish")
		idp.forge = nil

		// A request that outlived its ten minutes.
		b := s.browser()
		r := start(b, "admin@sso.example")
		require.Equal(t, http.StatusOK, r.status)
		code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl, good(nil))
		_, err = shared.owner.Exec(t.Context(), `UPDATE sso_logins SET expires_at = now() - interval '1 second'`)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, callback(b, code, state).status, "expired request")

		// Switching the connection off stops sign-ins already under way.
		b = s.browser()
		r = start(b, "admin@sso.example")
		require.Equal(t, http.StatusOK, r.status)
		code, state = idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl, good(nil))
		require.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPut, "/api/v1/sso",
			map[string]any{"issuer": idp.srv.URL, "client_id": "riskos", "enabled": false}).status)
		assert.Equal(t, http.StatusUnauthorized, callback(b, code, state).status, "connection switched off")
		assert.Equal(t, http.StatusNotFound, start(s.browser(), "admin@sso.example").status)
		require.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPut, "/api/v1/sso",
			map[string]any{"issuer": idp.srv.URL, "client_id": "riskos", "enabled": true}).status)
	})

	t.Run("audit and removal", func(t *testing.T) {
		actions := auditActions(t, admin.b.do(t, http.MethodGet, "/api/v1/audit-logs?limit=100", nil))
		for _, a := range []string{"sso.configured", "user.sso_authenticated", "user.sso_failed"} {
			assert.Contains(t, actions, a)
		}
		require.Equal(t, http.StatusNoContent, admin.b.do(t, http.MethodDelete, "/api/v1/sso", nil).status)
		assert.Equal(t, http.StatusNotFound, start(s.browser(), "admin@sso.example").status)
		var bound int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM users WHERE tenant_id = $1 AND sso_subject IS NOT NULL`, tenant.id).Scan(&bound))
		assert.Zero(t, bound, "removing SSO forgets the bound subjects")
	})
}
