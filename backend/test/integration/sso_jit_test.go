//go:build integration

package integration

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// fakeDNS answers TXT lookups from a table, like the zone a workspace controls.
type fakeDNS struct {
	mu      sync.Mutex
	records map[string][]string
}

func (d *fakeDNS) set(host string, values ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.records == nil {
		d.records = map[string][]string{}
	}
	d.records[host] = values
}

func (d *fakeDNS) lookup(_ context.Context, name string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.records[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// TestJustInTimeAccounts covers letting people at a verified domain in without an invite: the
// domain is proved in DNS and held by one workspace, the account gets a fixed non-administrator
// role, the provider must vouch for the very address, and an existing account always wins.
func TestJustInTimeAccounts(t *testing.T) {
	idp := newMockIDP(t)
	dns := &fakeDNS{}
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb,
		withConsole(auth.ConsoleConfig{SSOAllowPrivate: true, TXTLookup: dns.lookup}, false))
	tenant := s.createTenant(t, "Just In Time Co")
	admin := s.onboard(t, tenant, "admin@jit.example", authz.RoleCustomerAdmin)
	rival := s.createTenant(t, "Rival Co")
	rivalAdmin := s.onboard(t, rival, "admin@rival.example", authz.RoleCustomerAdmin)
	s.onboard(t, rival, "taken@jit.example", authz.RoleAnalyst)

	connection := func(extra map[string]any) map[string]any {
		body := map[string]any{"issuer": idp.srv.URL, "client_id": "riskos", "enabled": true}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	signIn := func(email string, claims map[string]any) (*browser, response) {
		b := s.browser()
		r := b.do(t, http.MethodPost, "/api/v1/auth/sso/start", map[string]any{"email": email})
		if r.status != http.StatusOK {
			return b, r
		}
		code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl, claims)
		return b, b.do(t, http.MethodPost, "/api/v1/auth/sso/callback", map[string]any{"code": code, "state": state})
	}
	vouched := func(sub, email string) map[string]any {
		return map[string]any{"sub": sub, "email": email, "email_verified": true, "name": "New Hire"}
	}
	usersWithEmail := func(email string) (n int, tenantID string) {
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*), coalesce(max(tenant_id::text), '') FROM users WHERE email = $1`, email).Scan(&n, &tenantID))
		return n, tenantID
	}

	t.Run("a domain is claimed, proved in DNS and held by one workspace", func(t *testing.T) {
		r := admin.b.do(t, http.MethodPost, "/api/v1/sso/domains", map[string]any{"domain": "jit.example"})
		assert.Equal(t, http.StatusBadRequest, r.status, "single sign-on is not set up yet")
		require.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPut, "/api/v1/sso",
			connection(map[string]any{"client_secret": "s3cret"})).status)

		for _, bad := range []string{"localhost", "http://jit.example", "a@jit.example", "1.2.3.4"} {
			assert.Equal(t, http.StatusBadRequest,
				admin.b.do(t, http.MethodPost, "/api/v1/sso/domains", map[string]any{"domain": bad}).status, bad)
		}
		r = admin.b.do(t, http.MethodPost, "/api/v1/sso/domains", map[string]any{"domain": "JIT.example"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		claim := decode[apigen.SSODomain](t, r)
		assert.Equal(t, "jit.example", claim.Domain)
		assert.False(t, claim.Verified)
		assert.Equal(t, "_riskos-challenge.jit.example", claim.TxtHost)
		assert.Regexp(t, `^riskos-verification=[a-z2-7]{32}$`, claim.TxtValue)
		assert.Equal(t, http.StatusConflict, admin.b.do(t, http.MethodPost, "/api/v1/sso/domains",
			map[string]any{"domain": "jit.example"}).status, "claimed twice")

		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"jit_enabled": true}))
		assert.Equal(t, http.StatusBadRequest, r.status, "no verified domain yet")
		r = admin.b.do(t, http.MethodPost, "/api/v1/sso/domains/jit.example/verify", nil)
		assert.Equal(t, http.StatusBadRequest, r.status, "the record is not in DNS")
		dns.set(claim.TxtHost, "v=spf1 -all", "riskos-verification=somebody-else")
		assert.Equal(t, http.StatusBadRequest,
			admin.b.do(t, http.MethodPost, "/api/v1/sso/domains/jit.example/verify", nil).status, "a record with another value")

		dns.set(claim.TxtHost, "v=spf1 -all", claim.TxtValue)
		r = admin.b.do(t, http.MethodPost, "/api/v1/sso/domains/jit.example/verify", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.True(t, decode[apigen.SSODomain](t, r).Verified)
		assert.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPost, "/api/v1/sso/domains/jit.example/verify", nil).status, "again")

		// Another workspace that publishes its own proof cannot take a domain that is held.
		require.Equal(t, http.StatusOK, rivalAdmin.b.do(t, http.MethodPut, "/api/v1/sso",
			connection(map[string]any{"client_secret": "s3cret"})).status)
		r = rivalAdmin.b.do(t, http.MethodPost, "/api/v1/sso/domains", map[string]any{"domain": "jit.example"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		theirs := decode[apigen.SSODomain](t, r)
		assert.NotEqual(t, claim.TxtValue, theirs.TxtValue, "the proof is per workspace")
		dns.set(claim.TxtHost, claim.TxtValue, theirs.TxtValue)
		assert.Equal(t, http.StatusConflict,
			rivalAdmin.b.do(t, http.MethodPost, "/api/v1/sso/domains/jit.example/verify", nil).status)
	})

	t.Run("nobody is let in until the workspace turns it on, and not as an administrator", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, s.browser().do(t, http.MethodPost, "/api/v1/auth/sso/start",
			map[string]any{"email": "newhire@jit.example"}).status, "off by default: an unknown email")

		r := admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"jit_enabled": true, "jit_role": "customer_admin"}))
		assert.Equal(t, http.StatusBadRequest, r.status, "never an administrator")
		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(map[string]any{"jit_enabled": true, "jit_role": "analyst"}))
		require.Equal(t, http.StatusOK, r.status, r.String())
		conn := decode[apigen.SSOConnection](t, r)
		assert.True(t, conn.JitEnabled)
		assert.Equal(t, apigen.JITRole("analyst"), conn.JitRole)
		r = admin.b.do(t, http.MethodPut, "/api/v1/sso", connection(nil))
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.True(t, decode[apigen.SSOConnection](t, r).JitEnabled, "a save that leaves it out keeps it")
	})

	t.Run("the first sign-in creates the account", func(t *testing.T) {
		b, r := signIn("NewHire@JIT.example", vouched("idp-newhire", "newhire@jit.example"))
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, apigen.SessionState("enrollment_required"), decode[apigen.AuthState](t, r).State,
			"RiskOS's second factor still applies")
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/v1/alerts", nil).status)

		var role, createdBy string
		var hasPassword bool
		var subject *string
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT role, created_by, password_hash IS NOT NULL, sso_subject FROM users WHERE email = 'newhire@jit.example'`,
		).Scan(&role, &createdBy, &hasPassword, &subject))
		assert.Equal(t, "analyst", role)
		assert.Equal(t, "sso:jit", createdBy)
		assert.False(t, hasPassword, "there is no password to guess")
		require.NotNil(t, subject)
		assert.Equal(t, "idp-newhire", *subject)
		_, tenantID := usersWithEmail("newhire@jit.example")
		assert.NotEqual(t, "", tenantID)

		// The second time is an ordinary single sign-on sign-in for an existing account.
		_, r = signIn("newhire@jit.example", vouched("idp-newhire", "newhire@jit.example"))
		assert.Equal(t, http.StatusOK, r.status, r.String())
		n, _ := usersWithEmail("newhire@jit.example")
		assert.Equal(t, 1, n)

		actions := auditActions(t, admin.b.do(t, http.MethodGet, "/api/v1/audit-logs?limit=100", nil))
		for _, a := range []string{"sso.domain_added", "sso.domain_verified", "user.created", "user.sso_authenticated"} {
			assert.Contains(t, actions, a)
		}
	})

	t.Run("the provider must vouch for the address, and the domain must be the verified one", func(t *testing.T) {
		_, r := signIn("careless@jit.example", map[string]any{"sub": "idp-careless", "email": "careless@jit.example", "email_verified": false})
		assert.Equal(t, http.StatusUnauthorized, r.status, "unverified email")
		_, r = signIn("careless@jit.example", vouched("idp-careless", "someone-else@jit.example"))
		assert.Equal(t, http.StatusUnauthorized, r.status, "a different address")
		_, r = signIn("careless@jit.example", vouched("idp-newhire", "careless@jit.example"))
		assert.Equal(t, http.StatusUnauthorized, r.status, "a subject already bound to another account")
		n, _ := usersWithEmail("careless@jit.example")
		assert.Zero(t, n, "none of those created an account")

		assert.Equal(t, http.StatusNotFound, s.browser().do(t, http.MethodPost, "/api/v1/auth/sso/start",
			map[string]any{"email": "someone@unclaimed.example"}).status, "a domain nobody has verified")
	})

	t.Run("an existing account always wins", func(t *testing.T) {
		_, r := signIn("taken@jit.example", vouched("idp-taken", "taken@jit.example"))
		require.Equal(t, http.StatusOK, r.status, r.String())
		n, owner := usersWithEmail("taken@jit.example")
		assert.Equal(t, 1, n)
		var tenantOfTaken string
		require.NoError(t, shared.owner.QueryRow(t.Context(), `SELECT id::text FROM tenants WHERE name = 'Rival Co'`).Scan(&tenantOfTaken))
		assert.Equal(t, tenantOfTaken, owner, "the account stayed in its own workspace")
	})

	t.Run("switching it off stops sign-ins already under way", func(t *testing.T) {
		b := s.browser()
		r := b.do(t, http.MethodPost, "/api/v1/auth/sso/start", map[string]any{"email": "late@jit.example"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		code, state := idp.authorize(t, decode[apigen.SSOStart](t, r).RedirectUrl, vouched("idp-late", "late@jit.example"))
		require.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPut, "/api/v1/sso",
			connection(map[string]any{"jit_enabled": false})).status)
		assert.Equal(t, http.StatusUnauthorized,
			b.do(t, http.MethodPost, "/api/v1/auth/sso/callback", map[string]any{"code": code, "state": state}).status)
		n, _ := usersWithEmail("late@jit.example")
		assert.Zero(t, n)
		assert.Equal(t, http.StatusNotFound, s.browser().do(t, http.MethodPost, "/api/v1/auth/sso/start",
			map[string]any{"email": "late@jit.example"}).status)
	})

	t.Run("releasing the domain, or removing single sign-on, ends it", func(t *testing.T) {
		require.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPut, "/api/v1/sso",
			connection(map[string]any{"jit_enabled": true})).status)
		list := decode[apigen.SSODomainList](t, admin.b.do(t, http.MethodGet, "/api/v1/sso/domains", nil))
		require.Len(t, list.Data, 1)
		require.Equal(t, http.StatusNoContent, admin.b.do(t, http.MethodDelete, "/api/v1/sso/domains/jit.example", nil).status)
		assert.Equal(t, http.StatusNotFound, admin.b.do(t, http.MethodDelete, "/api/v1/sso/domains/jit.example", nil).status)
		assert.Equal(t, http.StatusNotFound, s.browser().do(t, http.MethodPost, "/api/v1/auth/sso/start",
			map[string]any{"email": "another@jit.example"}).status, "no verified domain left")

		// The accounts it created stay, and the domain is free for another workspace to prove.
		n, _ := usersWithEmail("newhire@jit.example")
		assert.Equal(t, 1, n)
		require.Equal(t, http.StatusNoContent, admin.b.do(t, http.MethodDelete, "/api/v1/sso", nil).status)
		list = decode[apigen.SSODomainList](t, admin.b.do(t, http.MethodGet, "/api/v1/sso/domains", nil))
		assert.Empty(t, list.Data)
	})
}
