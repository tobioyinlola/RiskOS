//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// TestAdminTenantQuota covers a monthly event-volume quota set by staff: validation, measurement
// by the worker, an overage announced once a month on both sides, and nothing ever refused.
func TestAdminTenantQuota(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	tenant := s.createTenant(t, "Quota Co")
	other := s.createTenant(t, "No Quota Co")
	ref := ids.Format(ids.PrefixTenant, tenant.id)
	token, err := s.admin.CreateStaff(t.Context(), "quota-a@riskos.example", "Quota A", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	a, _ := enroll(t, s, token)
	path := "/api/admin/v1/tenants/" + ref + "/quota"
	job := s.admin.UsageJob()

	var envID string
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT id::text FROM environments WHERE tenant_id = $1 AND kind = 'test'`, tenant.id).Scan(&envID))
	seed := func(n int) {
		t.Helper()
		_, err := shared.owner.Exec(t.Context(), `
INSERT INTO events (tenant_id, environment_id, id, received_at, event_key, type, occurred_at, user_ref,
                    raw_payload, normalized, payload_hash, evaluation_id, risk_score, risk_level, decision, request_id)
SELECT $1::uuid, $2::uuid, gen_random_uuid(), now(), 'q-' || gen_random_uuid(), 'transaction', now(), 'usr_q',
       '{}', '{}'::jsonb, decode(repeat('00', 32), 'hex'), gen_random_uuid(), 10, 'low', 'allow', 'req-q'
FROM generate_series(1, $3::int)`, tenant.id, envID, n)
		require.NoError(t, err)
	}
	count := func(query string) (n int) {
		require.NoError(t, shared.owner.QueryRow(t.Context(), query, tenant.id).Scan(&n))
		return n
	}

	t.Run("no quota until staff set one", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, (&staffBrowser{s: s}).do(t, http.MethodGet, path, nil).status)
		r := a.do(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Nil(t, decode[adminapi.TenantQuota](t, r).Quota)
	})

	t.Run("a quota is sensible and has a reason", func(t *testing.T) {
		put := func(events int64, reason string) int {
			return a.do(t, http.MethodPut, path, map[string]any{"monthly_events": events, "reason": reason}).status
		}
		assert.Equal(t, http.StatusBadRequest, put(0, "Contract signed: ticket OPS-90"))
		assert.Equal(t, http.StatusBadRequest, put(-5, "Contract signed: ticket OPS-90"))
		assert.Equal(t, http.StatusBadRequest, put(100, "short"))
		assert.Equal(t, http.StatusBadRequest, put(2_000_000_000_000, "Contract signed: ticket OPS-90"))
	})

	t.Run("it is measured, and an overage is announced once", func(t *testing.T) {
		r := a.do(t, http.MethodPut, path, map[string]any{"monthly_events": 100, "reason": "Contract signed: ticket OPS-90"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[adminapi.TenantQuota](t, r)
		require.NotNil(t, got.Quota)
		assert.EqualValues(t, 100, got.Quota.MonthlyEvents)
		assert.Equal(t, "Quota A", got.Quota.UpdatedBy.Name)
		assert.Nil(t, got.Quota.Usage.Month, "not measured yet")

		seed(60)
		require.NoError(t, job(t.Context(), admin.UsageCheckArgs{}))
		got = decode[adminapi.TenantQuota](t, a.do(t, http.MethodGet, path, nil))
		assert.EqualValues(t, 60, got.Quota.Usage.Events)
		assert.False(t, got.Quota.Usage.Exceeded)
		require.NotNil(t, got.Quota.Usage.Month)
		assert.Equal(t, 1, got.Quota.Usage.Month.Day(), "the first day of the month")
		assert.Zero(t, count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.quota_exceeded'`))

		seed(60)
		require.NoError(t, job(t.Context(), admin.UsageCheckArgs{}))
		got = decode[adminapi.TenantQuota](t, a.do(t, http.MethodGet, path, nil))
		assert.EqualValues(t, 120, got.Quota.Usage.Events)
		assert.True(t, got.Quota.Usage.Exceeded)
		assert.Equal(t, 1, count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.quota_exceeded'`))
		assert.Equal(t, 1, count(`SELECT count(*) FROM staff_audit_log WHERE tenant_id = $1 AND action = 'tenant_quota.exceeded'`))

		// Measured again, still over: not announced again this month.
		seed(10)
		require.NoError(t, job(t.Context(), admin.UsageCheckArgs{}))
		assert.Equal(t, 1, count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.quota_exceeded'`))
		assert.Equal(t, 1, count(`SELECT count(*) FROM staff_audit_log WHERE tenant_id = $1 AND action = 'tenant_quota.exceeded'`))
	})

	t.Run("a new month is announced again, and nothing is ever refused", func(t *testing.T) {
		_, err := shared.owner.Exec(t.Context(),
			`UPDATE tenant_quotas SET exceeded_month = exceeded_month - interval '1 month' WHERE tenant_id = $1`, tenant.id)
		require.NoError(t, err)
		require.NoError(t, job(t.Context(), admin.UsageCheckArgs{}))
		assert.Equal(t, 2, count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.quota_exceeded'`))

		assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/events", tenant.testKey, nil).status,
			"the quota refuses nothing")
	})

	t.Run("both audit logs record the change, and only the staff one has the reason", func(t *testing.T) {
		assert.Equal(t, 1, count(`SELECT count(*) FROM staff_audit_log WHERE tenant_id = $1 AND action = 'tenant_quota.set'`))
		assert.Equal(t, 1, count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.quota_changed'`))
		assert.Zero(t, count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND metadata LIKE '%OPS-90%'`))
	})

	t.Run("tenants without a quota are not measured, and clearing removes it", func(t *testing.T) {
		var otherRows int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM tenant_quotas WHERE tenant_id = $1`, other.id).Scan(&otherRows))
		assert.Zero(t, otherRows)

		r := a.do(t, http.MethodDelete, path, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Nil(t, decode[adminapi.TenantQuota](t, r).Quota)
		assert.Equal(t, http.StatusOK, a.do(t, http.MethodDelete, path, nil).status, "clearing again is not an error")
		assert.Equal(t, 404, a.do(t, http.MethodGet, "/api/admin/v1/tenants/ten_00000000000000000000000000/quota", nil).status)
	})
}
