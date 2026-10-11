//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// TestAdminTenantLimits covers per-tenant request limits set by staff (ADR-0010): validation, the
// effect on that tenant alone, the audit entries on both sides, and clearing the override.
func TestAdminTenantLimits(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	limited := s.createTenant(t, "Limited Co")
	other := s.createTenant(t, "Unlimited Co")
	ref := ids.Format(ids.PrefixTenant, limited.id)
	token, err := s.admin.CreateStaff(t.Context(), "lim-a@riskos.example", "Lim A", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	a, _ := enroll(t, s, token)

	path := "/api/admin/v1/tenants/" + ref + "/limits"
	call := func(tenantKey string) int { return s.call(t, http.MethodGet, "/api/v1/events", tenantKey, nil).status }
	body := func(perSecond float64, burst int, reason string) map[string]any {
		return map[string]any{
			"per_key":    map[string]any{"per_second": perSecond, "burst": burst},
			"per_tenant": map[string]any{"per_second": 10000, "burst": 10000},
			"reason":     reason,
		}
	}
	const reason = "Abusive polling reported: ticket OPS-77"

	t.Run("a staff session is required, and the platform's limits are shown", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, (&staffBrowser{s: s}).do(t, http.MethodGet, path, nil).status)
		r := a.do(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[adminapi.TenantLimits](t, r)
		assert.Nil(t, got.Override)
		assert.Equal(t, 10000, got.Defaults.PerKey.Burst)
		assert.Equal(t, http.StatusNotFound, a.do(t, http.MethodGet, "/api/admin/v1/tenants/ten_"+"00000000000000000000000000"+"/limits", nil).status)
	})

	t.Run("limits must be sensible and have a reason", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, a.do(t, http.MethodPut, path, body(0, 5, reason)).status, "a rate above zero")
		assert.Equal(t, http.StatusBadRequest, a.do(t, http.MethodPut, path, body(1, 0, reason)).status, "a burst of at least one")
		assert.Equal(t, http.StatusBadRequest, a.do(t, http.MethodPut, path, body(1, 5, "short")).status)
		assert.Equal(t, http.StatusBadRequest, a.do(t, http.MethodPut, path, body(200000, 5, reason)).status)
	})

	t.Run("an override limits that tenant and no other", func(t *testing.T) {
		require.Equal(t, http.StatusOK, call(limited.testKey), "before")
		// One request a second with a burst of two: refused on the third in quick succession, and
		// the debt drains in a couple of seconds once the override is cleared (the limiter's state
		// is per key and is not reset by a change of limit).
		r := a.do(t, http.MethodPut, path, body(1, 2, reason))
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[adminapi.TenantLimits](t, r)
		require.NotNil(t, got.Override)
		assert.Equal(t, 2, got.Override.PerKey.Burst)
		assert.Equal(t, "Lim A", got.Override.UpdatedBy.Name)

		require.Eventually(t, func() bool {
			// The burst is two, so within a moment the third request in a row is refused.
			for range 3 {
				if call(limited.testKey) == http.StatusTooManyRequests {
					return true
				}
			}
			return false
		}, 5*time.Second, 50*time.Millisecond)
		for range 5 {
			assert.Equal(t, http.StatusOK, call(other.testKey), "another tenant keeps the platform's limits")
		}
	})

	t.Run("both audit logs record it, and only the staff one has the reason", func(t *testing.T) {
		var staffN, tenantN, leaked int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM staff_audit_log WHERE tenant_id = $1 AND action = 'tenant_limits.set'`, limited.id).Scan(&staffN))
		assert.Equal(t, 1, staffN)
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.limits_changed'`, limited.id).Scan(&tenantN))
		assert.Equal(t, 1, tenantN)
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND metadata LIKE '%OPS-77%'`, limited.id).Scan(&leaked))
		assert.Zero(t, leaked)
	})

	t.Run("clearing restores the platform's limits", func(t *testing.T) {
		r := a.do(t, http.MethodDelete, path, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Nil(t, decode[adminapi.TenantLimits](t, r).Override)
		require.Eventually(t, func() bool { return call(limited.testKey) == http.StatusOK }, 10*time.Second, 100*time.Millisecond)
		for range 10 {
			require.Equal(t, http.StatusOK, call(limited.testKey))
		}
		// Clearing again is not an error: the state asked for already holds.
		assert.Equal(t, http.StatusOK, a.do(t, http.MethodDelete, path, nil).status)
	})
}
