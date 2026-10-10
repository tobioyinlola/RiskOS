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

// TestAdminTenantStatusFourEyes walks ADR-0010: a tenant is suspended and reactivated only by one
// staff member's request and another's approval, the change lands with the decision, and the
// tenant's own audit log records the effect without the internal reason.
func TestAdminTenantStatusFourEyes(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	tenant := s.createTenant(t, "Suspension Co")
	tenantRef := ids.Format(ids.PrefixTenant, tenant.id)
	tokenA, err := s.admin.CreateStaff(t.Context(), "susp-a@riskos.example", "Susp A", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	tokenB, err := s.admin.CreateStaff(t.Context(), "susp-b@riskos.example", "Susp B", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	a, _ := enroll(t, s, tokenA)
	b, _ := enroll(t, s, tokenB)

	const secretReason = "Fraud ring suspected: ticket SEC-7781"
	request := func(br *staffBrowser, action, reason string) response {
		return br.do(t, http.MethodPost, "/api/admin/v1/tenants/"+tenantRef+"/status-requests",
			map[string]any{"action": action, "reason": reason})
	}
	decide := func(br *staffBrowser, id, verb, note string) response {
		var body any
		if note != "" {
			body = map[string]any{"note": note}
		}
		return br.do(t, http.MethodPost, "/api/admin/v1/tenant-status-requests/"+id+"/"+verb, body)
	}
	pending := func(br *staffBrowser) int64 {
		return decode[adminapi.AdminOverview](t, br.do(t, http.MethodGet, "/api/admin/v1/overview", nil)).StatusRequestsPending
	}
	tenantStatus := func() string {
		d := decode[adminapi.AdminTenantDetail](t, a.do(t, http.MethodGet, "/api/admin/v1/tenants/"+tenantRef, nil))
		return string(d.Tenant.Status)
	}
	apiStatus := func() int { return s.call(t, http.MethodGet, "/api/v1/events", tenant.testKey, nil).status }
	// The key lookup is cached for a second; wait for the status to take effect.
	waitAPI := func(want int) {
		require.Eventually(t, func() bool { return apiStatus() == want }, 5*time.Second, 100*time.Millisecond)
	}
	require.Equal(t, http.StatusOK, apiStatus())

	t.Run("a request needs a real reason and a state it can change", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, request(a, "suspend", "short").status)
		assert.Equal(t, http.StatusBadRequest, request(a, "delete", secretReason).status, "not in the contract")
		assert.Equal(t, http.StatusConflict, request(a, "reactivate", secretReason).status, "it is not suspended")
		assert.Equal(t, http.StatusUnauthorized, request(&staffBrowser{s: s}, "suspend", secretReason).status)
	})

	var suspendID string
	t.Run("nothing changes until another staff member approves", func(t *testing.T) {
		waitingBefore := pending(b)
		r := request(a, "suspend", secretReason)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		req := decode[adminapi.TenantStatusRequest](t, r)
		suspendID = req.Id
		assert.Equal(t, adminapi.TenantStatusRequestStatus("pending"), req.Status)
		assert.Equal(t, "Susp A", req.RequestedBy.Name)

		assert.Equal(t, http.StatusConflict, request(b, "suspend", secretReason).status, "one pending request per tenant")
		assert.Equal(t, http.StatusForbidden, decide(a, suspendID, "approve", "").status, "nobody decides their own request")
		assert.Equal(t, http.StatusForbidden, decide(b, suspendID, "withdraw", "").status, "only the requester withdraws")
		assert.Equal(t, waitingBefore+1, pending(b), "it waits for the other staff member")
		assert.Equal(t, "active", tenantStatus())
		assert.Equal(t, http.StatusOK, apiStatus())
	})

	t.Run("approval suspends the tenant at once, and the tenant is told without the reason", func(t *testing.T) {
		r := decide(b, suspendID, "approve", "Confirmed with the on-call lead.")
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[adminapi.TenantStatusRequest](t, r)
		assert.Equal(t, adminapi.TenantStatusRequestStatus("approved"), got.Status)
		require.NotNil(t, got.DecidedBy)
		assert.Equal(t, "Susp B", got.DecidedBy.Name)

		assert.Equal(t, "suspended", tenantStatus())
		waitAPI(http.StatusForbidden)
		assert.Equal(t, http.StatusConflict, decide(b, suspendID, "approve", "").status, "decided once")

		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.suspended'`, tenant.id).Scan(&n))
		assert.Equal(t, 1, n, "the tenant's own audit log records the effect")
		var leaked int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND metadata::text LIKE '%SEC-7781%'`, tenant.id).Scan(&leaked))
		assert.Zero(t, leaked, "the internal reason stays in the staff audit log")

		d := decode[adminapi.AdminTenantDetail](t, a.do(t, http.MethodGet, "/api/admin/v1/tenants/"+tenantRef, nil))
		var actions []string
		for _, e := range d.RecentStaffActions {
			actions = append(actions, e.Action)
		}
		assert.Contains(t, actions, "tenant_status.requested")
		assert.Contains(t, actions, "tenant_status.approved")
	})

	t.Run("withdrawing and rejecting leave the tenant as it is", func(t *testing.T) {
		r := request(a, "reactivate", "Customer fixed the issue: ticket SEC-7790")
		require.Equal(t, http.StatusCreated, r.status, r.String())
		id := decode[adminapi.TenantStatusRequest](t, r).Id
		w := decide(a, id, "withdraw", "")
		require.Equal(t, http.StatusOK, w.status, w.String())
		assert.Equal(t, adminapi.TenantStatusRequestStatus("withdrawn"), decode[adminapi.TenantStatusRequest](t, w).Status)
		assert.Equal(t, http.StatusConflict, decide(b, id, "approve", "").status, "a withdrawn request cannot be approved")

		r = request(a, "reactivate", "Customer fixed the issue: ticket SEC-7790")
		require.Equal(t, http.StatusCreated, r.status, r.String())
		id = decode[adminapi.TenantStatusRequest](t, r).Id
		rej := decide(b, id, "reject", "Not until the review is closed.")
		require.Equal(t, http.StatusOK, rej.status, rej.String())
		got := decode[adminapi.TenantStatusRequest](t, rej)
		assert.Equal(t, adminapi.TenantStatusRequestStatus("rejected"), got.Status)
		require.NotNil(t, got.DecisionNote)
		assert.Equal(t, "suspended", tenantStatus())
	})

	t.Run("reactivation restores access", func(t *testing.T) {
		r := request(b, "reactivate", "Review closed, no further action: SEC-7781")
		require.Equal(t, http.StatusCreated, r.status, r.String())
		id := decode[adminapi.TenantStatusRequest](t, r).Id
		require.Equal(t, http.StatusOK, decide(a, id, "approve", "").status)
		assert.Equal(t, "active", tenantStatus())
		waitAPI(http.StatusOK)

		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.reactivated'`, tenant.id).Scan(&n))
		assert.Equal(t, 1, n)
	})

	t.Run("the list shows the history", func(t *testing.T) {
		list := decode[adminapi.TenantStatusRequestList](t, a.do(t, http.MethodGet, "/api/admin/v1/tenant-status-requests", nil))
		statuses := map[string]int{}
		for _, r := range list.Data {
			if r.TenantId == tenantRef {
				statuses[string(r.Status)]++
			}
		}
		assert.Equal(t, 2, statuses["approved"])
		assert.Equal(t, 1, statuses["withdrawn"])
		assert.Equal(t, 1, statuses["rejected"])
	})
}
