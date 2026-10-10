//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

const testOrigin = "http://localhost:3000"

var b64 = base64.RawURLEncoding //nolint:gochecknoglobals // test shorthand

// field reads m[key] as a T, failing the test otherwise.
func field[T any](t *testing.T, m map[string]any, key string) T {
	t.Helper()
	v, ok := m[key].(T)
	require.True(t, ok, "field %s", key)
	return v
}

// softKey is a software passkey: an ES256 key with "none" attestation, user verification and a
// sign counter, enough for the server's real WebAuthn verification.
type softKey struct {
	key    *ecdsa.PrivateKey
	id     []byte
	user   []byte
	count  uint32
	rpHash [32]byte
}

func newSoftKey(t *testing.T) *softKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softKey{key: k, id: id}
}

func clientData(t *testing.T, typ, challenge string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": testOrigin, "crossOrigin": false})
	require.NoError(t, err)
	return raw
}

func (k *softKey) authData(flags byte, attested []byte) []byte {
	k.count++
	out := append([]byte{}, k.rpHash[:]...)
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, k.count)
	return append(out, attested...)
}

// register answers navigator.credentials.create options.
func (k *softKey) register(t *testing.T, options map[string]any) map[string]any {
	t.Helper()
	pk := field[map[string]any](t, options, "publicKey")
	rp := field[map[string]any](t, pk, "rp")
	user := field[map[string]any](t, pk, "user")
	var err error
	k.user, err = b64.DecodeString(field[string](t, user, "id"))
	require.NoError(t, err)
	k.rpHash = sha256.Sum256([]byte(field[string](t, rp, "id")))
	x, y := k.key.X.FillBytes(make([]byte, 32)), k.key.Y.FillBytes(make([]byte, 32))
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	require.NoError(t, err)
	attested := make([]byte, 16) // AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(k.id)))
	attested = append(append(attested, k.id...), cose...)
	att, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": k.authData(0x45, attested)})
	require.NoError(t, err)
	return map[string]any{
		"id": b64.EncodeToString(k.id), "rawId": b64.EncodeToString(k.id), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(clientData(t, "webauthn.create", field[string](t, pk, "challenge"))),
			"attestationObject": b64.EncodeToString(att), "transports": []string{"internal"},
		},
		"clientExtensionResults": map[string]any{}, "authenticatorAttachment": "platform",
	}
}

// assert answers navigator.credentials.get options (discoverable: it names its user).
func (k *softKey) assert(t *testing.T, options map[string]any) map[string]any {
	t.Helper()
	pk := field[map[string]any](t, options, "publicKey")
	cd := clientData(t, "webauthn.get", field[string](t, pk, "challenge"))
	ad := k.authData(0x05, nil)
	sum := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	require.NoError(t, err)
	return map[string]any{
		"id": b64.EncodeToString(k.id), "rawId": b64.EncodeToString(k.id), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(cd), "authenticatorData": b64.EncodeToString(ad),
			"signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString(k.user),
		},
		"clientExtensionResults": map[string]any{}, "authenticatorAttachment": "platform",
	}
}

// staffBrowser keeps a staff session cookie and its CSRF token.
type staffBrowser struct {
	s      stack
	cookie string
	csrf   string
	grant  string
	// sso is the cookie binding a single sign-on request to this browser.
	sso string
}

func (b *staffBrowser) do(t *testing.T, method, path string, body any) response {
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
	if method != http.MethodGet {
		req.Header.Set("Origin", testOrigin)
	}
	if b.cookie != "" {
		req.AddCookie(&http.Cookie{Name: "riskos_staff", Value: b.cookie})
	}
	if b.sso != "" {
		req.AddCookie(&http.Cookie{Name: "riskos_staff_sso", Value: b.sso})
	}
	if b.csrf != "" {
		req.Header.Set("X-CSRF-Token", b.csrf)
	}
	if b.grant != "" {
		req.Header.Set("RiskOS-Support-Grant", b.grant)
	}
	rec := httptest.NewRecorder()
	b.s.handler.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() { //nolint:bodyclose // recorder
		if c.Name == "riskos_staff_sso" {
			b.sso = c.Value
		}
		if c.Name == "riskos_staff" {
			b.cookie = c.Value
		}
	}
	return response{status: rec.Code, raw: rec.Body.Bytes(), header: rec.Header()}
}

func (b *staffBrowser) signedIn(t *testing.T, r response) {
	t.Helper()
	require.Equal(t, http.StatusOK, r.status, r.String())
	b.csrf = decode[adminapi.StaffSession](t, r).CsrfToken
}

// enroll registers a staff member's passkey from their enrollment token.
func enroll(t *testing.T, s stack, token string) (*staffBrowser, *softKey) {
	t.Helper()
	b := &staffBrowser{s: s}
	key := newSoftKey(t)
	r := b.do(t, http.MethodPost, "/api/admin/v1/auth/enroll/options", map[string]any{"token": token})
	require.Equal(t, http.StatusOK, r.status, r.String())
	c := decode[adminapi.Ceremony](t, r)
	b.signedIn(t, b.do(t, http.MethodPost, "/api/admin/v1/auth/enroll", map[string]any{
		"token": token, "ceremony_id": c.CeremonyId, "name": "Laptop", "credential": key.register(t, c.Options),
	}))
	return b, key
}

func staffLogin(t *testing.T, s stack, key *softKey) response {
	t.Helper()
	b := &staffBrowser{s: s}
	r := b.do(t, http.MethodPost, "/api/admin/v1/auth/login/options", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	c := decode[adminapi.Ceremony](t, r)
	return b.do(t, http.MethodPost, "/api/admin/v1/auth/login", map[string]any{"ceremony_id": c.CeremonyId, "credential": key.assert(t, c.Options)})
}

// TestAdminRealm covers staff accounts, passkey-only sign-in, four-eyes support access that is
// read-only, time-boxed and recorded in the tenant's audit log, and the cross-tenant view.
func TestAdminRealm(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	tenant := s.createTenant(t, "Supported Co")
	require.Equal(t, http.StatusOK, s.ingest(t, tenant.testKey, actorTxn("support-evt", "u_sup", "acc_sup", "dev_sup", 1000)).status)

	tokenA, err := s.admin.CreateStaff(t.Context(), "ops-a@riskos.example", "Ops A", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	tokenB, err := s.admin.CreateStaff(t.Context(), "sec-b@riskos.example", "Sec B", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	_, err = s.admin.CreateStaff(t.Context(), "ops-a@riskos.example", "Again", authz.RoleOperationsAdmin, "integration")
	require.Error(t, err, "emails are unique")

	a, keyA := enroll(t, s, tokenA)
	b, _ := enroll(t, s, tokenB)

	t.Run("passkeys only, short sessions", func(t *testing.T) {
		r := (&staffBrowser{s: s}).do(t, http.MethodPost, "/api/admin/v1/auth/enroll/options", map[string]any{"token": tokenA})
		assert.Equal(t, http.StatusUnauthorized, r.status, "an enrollment link works once")
		assert.Equal(t, http.StatusUnauthorized, (&staffBrowser{s: s}).do(t, http.MethodGet, "/api/admin/v1/tenants", nil).status)

		sess := decode[adminapi.StaffSession](t, a.do(t, http.MethodGet, "/api/admin/v1/auth/session", nil))
		assert.Equal(t, "Ops A", sess.Staff.Name)
		assert.WithinDuration(t, time.Now().Add(15*time.Minute), sess.IdleExpiresAt, time.Minute)
		assert.WithinDuration(t, time.Now().Add(4*time.Hour), sess.ExpiresAt, time.Minute)

		r = staffLogin(t, s, keyA)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, "ops-a@riskos.example", decode[adminapi.StaffSession](t, r).Staff.Email)

		forged := newSoftKey(t)
		forged.id, forged.user = keyA.id, keyA.user
		forged.rpHash = keyA.rpHash
		assert.Equal(t, http.StatusUnauthorized, staffLogin(t, s, forged).status, "a different key with the same ID fails")

		noCSRF := &staffBrowser{s: s, cookie: a.cookie}
		r = noCSRF.do(t, http.MethodPost, "/api/admin/v1/support-access", map[string]any{"tenant_id": "ten_x", "reason": "testing CSRF", "duration_minutes": 30})
		assert.Equal(t, http.StatusForbidden, r.status, "unsafe requests need the CSRF token")
	})

	var accessID string
	t.Run("four eyes", func(t *testing.T) {
		r := a.do(t, http.MethodPost, "/api/admin/v1/support-access", map[string]any{
			"tenant_id": "ten_" + strings.Repeat("0", 26), "reason": "Customer ticket 4812", "duration_minutes": 30,
		})
		assert.Equal(t, http.StatusNotFound, r.status, r.String())
		r = a.do(t, http.MethodPost, "/api/admin/v1/support-access", map[string]any{
			"tenant_id": ids.Format(ids.PrefixTenant, uuid.New()), "reason": "Customer ticket 4812", "duration_minutes": 30,
		})
		assert.Equal(t, http.StatusNotFound, r.status, r.String())
		tenants := decode[adminapi.AdminTenantList](t, a.do(t, http.MethodGet, "/api/admin/v1/tenants", nil))
		var tenantRef string
		for _, tt := range tenants.Data {
			if tt.Name == "Supported Co" {
				tenantRef = tt.Id
			}
		}
		require.NotEmpty(t, tenantRef)
		r = a.do(t, http.MethodPost, "/api/admin/v1/support-access", map[string]any{
			"tenant_id": tenantRef, "reason": "Customer ticket 4812: webhook failures", "duration_minutes": 30,
		})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		req := decode[adminapi.SupportAccess](t, r)
		accessID = req.Id
		assert.Equal(t, adminapi.SupportAccessStatus("pending"), req.Status)
		assert.False(t, req.Active)

		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodPost, "/api/admin/v1/support-access/"+accessID+"/approve", nil).status,
			"nobody approves their own request")
		a.grant = accessID
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodGet, "/api/v1/events", nil).status, "pending access does not work")
		a.grant = ""

		r = b.do(t, http.MethodPost, "/api/admin/v1/support-access/"+accessID+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		approved := decode[adminapi.SupportAccess](t, r)
		assert.True(t, approved.Active)
		require.NotNil(t, approved.ExpiresAt)
		assert.WithinDuration(t, time.Now().Add(30*time.Minute), *approved.ExpiresAt, time.Minute)
		assert.Equal(t, http.StatusConflict, b.do(t, http.MethodPost, "/api/admin/v1/support-access/"+accessID+"/approve", nil).status)
	})

	t.Run("read-only, the requester's own, recorded for the tenant", func(t *testing.T) {
		a.grant = accessID
		defer func() { a.grant = "" }()
		r := a.do(t, http.MethodGet, "/api/v1/events", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Contains(t, r.String(), "support-evt")
		assert.Equal(t, http.StatusOK, a.do(t, http.MethodGet, "/api/v1/alerts", nil).status)
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodGet, "/api/v1/risk/settings", nil).status, "settings need risk:configure")
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodPost, "/api/v1/cases", map[string]any{"title": "x"}).status, "never writes")
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodGet, "/api/v1/auth/session", nil).status)

		b.grant = accessID
		assert.Equal(t, http.StatusForbidden, b.do(t, http.MethodGet, "/api/v1/events", nil).status, "access is the requester's only")
		b.grant = ""
		noSession := &staffBrowser{s: s, grant: accessID}
		assert.Equal(t, http.StatusUnauthorized, noSession.do(t, http.MethodGet, "/api/v1/events", nil).status)

		logs := decode[apigen.AuditLogList](t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", tenant.testKey, nil))
		var actions []string
		var viewedBy string
		for _, e := range logs.Data {
			actions = append(actions, e.Action)
			if e.Action == "support_access.viewed" {
				viewedBy = string(e.Actor.Type)
			}
		}
		for _, want := range []string{"support_access.requested", "support_access.approved", "support_access.viewed"} {
			assert.Contains(t, actions, want, "the tenant sees it in its own audit log")
		}
		assert.Equal(t, "staff", viewedBy)
	})

	t.Run("only while it lasts, never with other credentials", func(t *testing.T) {
		a.grant = accessID
		defer func() { a.grant = "" }()
		// An API key alongside the grant, or a bad environment, is refused outright.
		r := s.call(t, http.MethodGet, "/api/v1/events", tenant.testKey, nil)
		require.Equal(t, http.StatusOK, r.status)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/events", nil)
		req.Header.Set("Authorization", "Bearer "+tenant.testKey)
		req.Header.Set("RiskOS-Support-Grant", accessID)
		req.AddCookie(&http.Cookie{Name: "riskos_staff", Value: a.cookie})
		rec := httptest.NewRecorder()
		s.handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "a grant never combines with an API key")

		req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/events", nil)
		req.Header.Set("RiskOS-Support-Grant", accessID)
		req.Header.Set("RiskOS-Environment", "staging")
		req.AddCookie(&http.Cookie{Name: "riskos_staff", Value: a.cookie})
		rec = httptest.NewRecorder()
		s.handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		// A suspended tenant cannot be read.
		_, err := shared.owner.Exec(t.Context(), `UPDATE tenants SET status = 'suspended' WHERE id = $1`, tenant.id)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodGet, "/api/v1/events", nil).status, "suspended tenant")
		_, err = shared.owner.Exec(t.Context(), `UPDATE tenants SET status = 'active' WHERE id = $1`, tenant.id)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, a.do(t, http.MethodGet, "/api/v1/events", nil).status)

		// Past its end, access stops even without a revocation.
		_, err = shared.owner.Exec(t.Context(), `UPDATE support_access SET expires_at = now() - interval '1 second' WHERE status = 'approved'`)
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodGet, "/api/v1/events", nil).status, "expired access")
		list := decode[adminapi.SupportAccessList](t, a.do(t, http.MethodGet, "/api/admin/v1/support-access", nil))
		require.NotEmpty(t, list.Data)
		assert.Equal(t, adminapi.SupportAccessStatus("expired"), list.Data[0].Status)
		_, err = shared.owner.Exec(t.Context(), `UPDATE support_access SET expires_at = now() + interval '30 minutes' WHERE status = 'approved'`)
		require.NoError(t, err)
	})

	t.Run("revocation and the security view", func(t *testing.T) {
		r := b.do(t, http.MethodPost, "/api/admin/v1/support-access/"+accessID+"/revoke", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, adminapi.SupportAccessStatus("revoked"), decode[adminapi.SupportAccess](t, r).Status)
		a.grant = accessID
		assert.Equal(t, http.StatusForbidden, a.do(t, http.MethodGet, "/api/v1/events", nil).status)
		a.grant = ""

		overview := decode[adminapi.SecurityOverview](t, a.do(t, http.MethodGet, "/api/admin/v1/security", nil))
		var found bool
		for _, row := range overview.Data {
			if row.TenantName == "Supported Co" {
				found = true
			}
		}
		assert.True(t, found)
		assert.NotContains(t, a.do(t, http.MethodGet, "/api/admin/v1/security", nil).String(), "support-evt", "counts, never tenant data")

		audit := decode[adminapi.StaffAuditList](t, a.do(t, http.MethodGet, "/api/admin/v1/audit-log", nil))
		var staffActions []string
		for _, e := range audit.Data {
			staffActions = append(staffActions, e.Action)
		}
		for _, want := range []string{"staff.created", "staff.passkey_enrolled", "staff.signed_in", "staff.sign_in_failed",
			"support_access.requested", "support_access.approved", "support_access.viewed", "support_access.revoked"} {
			assert.Contains(t, staffActions, want)
		}
	})

	t.Run("signing out ends support access too", func(t *testing.T) {
		r := b.do(t, http.MethodPost, "/api/admin/v1/auth/logout", nil)
		require.Equal(t, http.StatusNoContent, r.status, r.String())
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/admin/v1/auth/session", nil).status)
		b.grant = accessID
		assert.Equal(t, http.StatusUnauthorized, b.do(t, http.MethodGet, "/api/v1/events", nil).status)
		b.grant = ""
	})

	t.Run("disabling a staff member ends their sessions", func(t *testing.T) {
		require.NoError(t, s.admin.SetStaffStatus(t.Context(), "ops-a@riskos.example", false, "integration"))
		assert.Equal(t, http.StatusUnauthorized, a.do(t, http.MethodGet, "/api/admin/v1/auth/session", nil).status)
		assert.Equal(t, http.StatusUnauthorized, staffLogin(t, s, keyA).status)
	})
}
