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

// TestAdminEmergencySuspension covers the break-glass path (ADR-0010): one security administrator
// suspends at once, it is flagged and never silent, a different staff member reviews it, and
// reactivation is still an ordinary two-person request.
func TestAdminEmergencySuspension(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	tenant := s.createTenant(t, "Incident Co")
	tenantRef := ids.Format(ids.PrefixTenant, tenant.id)
	tokenOps, err := s.admin.CreateStaff(t.Context(), "em-ops@riskos.example", "Em Ops", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	tokenB, err := s.admin.CreateStaff(t.Context(), "em-sec-b@riskos.example", "Em Sec B", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	tokenC, err := s.admin.CreateStaff(t.Context(), "em-sec-c@riskos.example", "Em Sec C", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	ops, _ := enroll(t, s, tokenOps)
	b, _ := enroll(t, s, tokenB)
	c, _ := enroll(t, s, tokenC)

	const reason = "Credential stuffing in progress: incident INC-204"
	emergency := func(br *staffBrowser, why string) response {
		return br.do(t, http.MethodPost, "/api/admin/v1/tenants/"+tenantRef+"/emergency-suspension", map[string]any{"reason": why})
	}
	overview := func(br *staffBrowser) adminapi.AdminOverview {
		return decode[adminapi.AdminOverview](t, br.do(t, http.MethodGet, "/api/admin/v1/overview", nil))
	}
	apiStatus := func() int { return s.call(t, http.MethodGet, "/api/v1/events", tenant.testKey, nil).status }
	waitAPI := func(want int) {
		require.Eventually(t, func() bool { return apiStatus() == want }, 5*time.Second, 100*time.Millisecond)
	}

	t.Run("only security administrators, with a real reason", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, emergency(ops, reason).status)
		assert.Equal(t, http.StatusBadRequest, emergency(b, "now").status)
		assert.Equal(t, http.StatusUnauthorized, emergency(&staffBrowser{s: s}, reason).status)
		require.Equal(t, http.StatusOK, apiStatus(), "nothing happened yet")
	})

	t.Run("a waiting request must be decided first", func(t *testing.T) {
		r := ops.do(t, http.MethodPost, "/api/admin/v1/tenants/"+tenantRef+"/status-requests",
			map[string]any{"action": "suspend", "reason": "Routine review: ticket OPS-31"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		id := decode[adminapi.TenantStatusRequest](t, r).Id
		assert.Equal(t, http.StatusConflict, emergency(b, reason).status)
		require.Equal(t, http.StatusOK, ops.do(t, http.MethodPost, "/api/admin/v1/tenant-status-requests/"+id+"/withdraw", nil).status)
	})

	beforeB, beforeC := overview(b), overview(c)
	var emergencyID string
	t.Run("it applies at once and is flagged", func(t *testing.T) {
		r := emergency(b, reason)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		got := decode[adminapi.TenantStatusRequest](t, r)
		emergencyID = got.Id
		assert.True(t, got.Emergency)
		assert.Equal(t, adminapi.TenantStatusRequestStatus("approved"), got.Status)
		assert.Nil(t, got.ReviewedBy)
		waitAPI(http.StatusForbidden)

		var n, leaked int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.suspended' AND (metadata::jsonb)->>'emergency' = 'true'`,
			tenant.id).Scan(&n))
		assert.Equal(t, 1, n, "the tenant's log records it, flagged")
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND metadata::text LIKE '%INC-204%'`, tenant.id).Scan(&leaked))
		assert.Zero(t, leaked, "the internal reason stays in the staff audit log")

		assert.Equal(t, http.StatusConflict, emergency(b, reason).status, "it is not active any more")
	})

	t.Run("it waits for someone else's review", func(t *testing.T) {
		assert.Equal(t, beforeC.EmergencyReviewsPending+1, overview(c).EmergencyReviewsPending, "on the other staff member's queue")
		assert.Equal(t, beforeB.EmergencyReviewsPending, overview(b).EmergencyReviewsPending, "not on the requester's own")

		review := func(br *staffBrowser, note string) response {
			var body any
			if note != "" {
				body = map[string]any{"note": note}
			}
			return br.do(t, http.MethodPost, "/api/admin/v1/tenant-status-requests/"+emergencyID+"/review", body)
		}
		assert.Equal(t, http.StatusForbidden, review(b, "").status, "not your own")
		r := review(c, "Confirmed against the fraud dashboard.")
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[adminapi.TenantStatusRequest](t, r)
		require.NotNil(t, got.ReviewedBy)
		assert.Equal(t, "Em Sec C", got.ReviewedBy.Name)
		assert.Equal(t, http.StatusConflict, review(ops, "").status, "reviewed once")
		assert.Equal(t, beforeC.EmergencyReviewsPending, overview(c).EmergencyReviewsPending)
	})

	t.Run("reactivation is still an ordinary two-person request", func(t *testing.T) {
		r := b.do(t, http.MethodPost, "/api/admin/v1/tenants/"+tenantRef+"/status-requests",
			map[string]any{"action": "reactivate", "reason": "Contained, credentials reset: INC-204"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		id := decode[adminapi.TenantStatusRequest](t, r).Id
		assert.Equal(t, http.StatusForbidden, b.do(t, http.MethodPost, "/api/admin/v1/tenant-status-requests/"+id+"/approve", nil).status)
		require.Equal(t, http.StatusOK, c.do(t, http.MethodPost, "/api/admin/v1/tenant-status-requests/"+id+"/approve", nil).status)
		waitAPI(http.StatusOK)
	})
}
