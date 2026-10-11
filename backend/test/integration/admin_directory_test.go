//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// TestAdminDirectoryAndOverview covers the staff console's tenant directory, tenant detail and
// work queue (ADR-0010): identity, status and counts only, and a queue that excludes what the
// signed-in staff member cannot decide themselves.
func TestAdminDirectoryAndOverview(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	tenant := s.createTenant(t, "Directory Co")
	tokenA, err := s.admin.CreateStaff(t.Context(), "dir-a@riskos.example", "Dir A", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	tokenB, err := s.admin.CreateStaff(t.Context(), "dir-b@riskos.example", "Dir B", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	a, _ := enroll(t, s, tokenA)
	b, _ := enroll(t, s, tokenB)
	tenantRef := ids.Format(ids.PrefixTenant, tenant.id)

	t.Run("signed-out requests are refused", func(t *testing.T) {
		for _, path := range []string{"/api/admin/v1/overview", "/api/admin/v1/tenants", "/api/admin/v1/tenants/" + tenantRef} {
			assert.Equal(t, http.StatusUnauthorized, (&staffBrowser{s: s}).do(t, http.MethodGet, path, nil).status, path)
		}
	})

	t.Run("the directory shows identity, status and counts", func(t *testing.T) {
		list := decode[adminapi.AdminTenantList](t, a.do(t, http.MethodGet, "/api/admin/v1/tenants", nil))
		var found *adminapi.AdminTenant
		for i := range list.Data {
			if list.Data[i].Id == tenantRef {
				found = &list.Data[i]
			}
		}
		require.NotNil(t, found, "the tenant is listed")
		assert.Equal(t, "Directory Co", found.Name)
		assert.Equal(t, adminapi.AdminTenantStatus("active"), found.Status)
		assert.Equal(t, adminapi.AdminTenantOrigin("operator"), found.Origin)
		assert.True(t, found.ProductionEnabled)
		assert.GreaterOrEqual(t, found.ActiveApiKeys, int64(1), "the bootstrap key")
		assert.Nil(t, found.OnboardingStatus, "operator-created workspaces have no onboarding")
	})

	t.Run("one tenant", func(t *testing.T) {
		r := a.do(t, http.MethodGet, "/api/admin/v1/tenants/"+tenantRef, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := decode[adminapi.AdminTenantDetail](t, r)
		assert.Equal(t, tenantRef, d.Tenant.Id)
		assert.Equal(t, tenantRef, d.Security.TenantId)
		assert.Empty(t, d.RecentStaffActions)

		unknown := "ten_" + strings.Repeat("0", 26)
		assert.Equal(t, http.StatusNotFound, a.do(t, http.MethodGet, "/api/admin/v1/tenants/"+unknown, nil).status)
		assert.Equal(t, http.StatusNotFound, a.do(t, http.MethodGet, "/api/admin/v1/tenants/not-an-id", nil).status)
	})

	t.Run("the work queue leaves out your own requests", func(t *testing.T) {
		overview := func(br *staffBrowser) adminapi.AdminOverview {
			r := br.do(t, http.MethodGet, "/api/admin/v1/overview", nil)
			require.Equal(t, http.StatusOK, r.status, r.String())
			return decode[adminapi.AdminOverview](t, r)
		}
		beforeA, beforeB := overview(a), overview(b)

		r := a.do(t, http.MethodPost, "/api/admin/v1/support-access", map[string]any{
			"tenant_id": tenantRef, "reason": "Customer ticket 5001: directory test", "duration_minutes": 30,
		})
		require.Equal(t, http.StatusCreated, r.status, r.String())

		assert.Equal(t, beforeA.SupportPending, overview(a).SupportPending, "nobody decides their own request")
		assert.Equal(t, beforeB.SupportPending+1, overview(b).SupportPending, "the other staff member has it waiting")
		assert.GreaterOrEqual(t, overview(a).StaffActive, int64(2))
		assert.Equal(t, overview(a).TenantsTotal, overview(a).TenantsActive+overview(a).TenantsSuspended+disabledTenants(t))
	})

	t.Run("staff activity appears on the tenant", func(t *testing.T) {
		d := decode[adminapi.AdminTenantDetail](t, a.do(t, http.MethodGet, "/api/admin/v1/tenants/"+tenantRef, nil))
		var actions []string
		for _, e := range d.RecentStaffActions {
			actions = append(actions, e.Action)
		}
		assert.Contains(t, actions, "support_access.requested")
	})
}

// disabledTenants counts tenants with the status the overview does not break out.
func disabledTenants(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, shared.owner.QueryRow(t.Context(), `SELECT count(*) FROM tenants WHERE status = 'disabled'`).Scan(&n))
	return n
}
