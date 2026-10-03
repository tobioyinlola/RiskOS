//go:build integration

package integration

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

func taskOfKind(tasks []apigen.ComplianceTask, kind string) *apigen.ComplianceTask {
	i := slices.IndexFunc(tasks, func(t apigen.ComplianceTask) bool { return t.Kind == kind })
	if i < 0 {
		return nil
	}
	return &tasks[i]
}

// The reporting workflow (phase 5): fired controls create compliance tasks with their legal
// deadlines; reports are approved only by compliance officers (GOV-06), suspicious transaction
// reports only when goAML would accept them (REG-19) and only compliance officers ever see them
// (GOV-09); filing is a person's act, recorded with the regulator's reference.
func TestComplianceWorkflow(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	s.startWorker(t)
	a := s.createTenant(t, "Workflow")
	admin := s.onboard(t, a, "admin@workflow.example", authz.RoleCustomerAdmin)
	officer := s.onboard(t, a, "officer@workflow.example", authz.RoleComplianceOfficer)
	analyst := s.onboard(t, a, "analyst@workflow.example", authz.RoleAnalyst)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	require.Equal(t, http.StatusOK, admin.b.do(t, http.MethodPut, "/api/v1/compliance/fx-rates/NGN", map[string]any{"usd_per_unit": 0.00065}).status)

	queue := func(b *browser) apigen.ComplianceQueue {
		t.Helper()
		r := b.do(t, http.MethodGet, "/api/v1/compliance/tasks", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.ComplianceQueue](t, r)
	}
	act := func(b *browser, method, path string, body any) response {
		return b.do(t, method, "/api/v1/compliance/tasks/"+path, body)
	}

	// An international transfer of $10,400 requires a foreign transaction report within a day.
	e := money("ftr-1", "transfer_international", "acc_wf", 1_600_000_000, "NGN", nil)
	r = s.ingest(t, a.testKey, e)
	require.Equal(t, http.StatusOK, r.status, r.String())
	var ftr *apigen.ComplianceTask
	require.Eventually(t, func() bool {
		ftr = taskOfKind(queue(officer.b).Data, "report.FTR")
		return ftr != nil
	}, 20*time.Second, 100*time.Millisecond, "the task job creates the FTR")
	assert.Equal(t, "REG-10", ftr.Rule)
	require.NotNil(t, ftr.DueAt)
	assert.WithinDuration(t, time.Now().Add(23*time.Hour+59*time.Minute), *ftr.DueAt, 5*time.Minute, "due a day after the transfer")
	assert.False(t, ftr.Overdue)
	assert.EqualValues(t, 1_600_000_000, ftr.Report["amount"])
	assert.NotNil(t, taskOfKind(queue(analyst.b).Data, "report.FTR"), "threshold reports are not secret")

	t.Run("only a compliance officer approves; filing needs the regulator's reference", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, act(admin.b, http.MethodPost, ftr.Id+"/approve", nil).status, "GOV-06")
		assert.Equal(t, http.StatusConflict, act(officer.b, http.MethodGet, ftr.Id+"/export", nil).status, "a draft is not exported")
		r := act(officer.b, http.MethodPost, ftr.Id+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, apigen.ComplianceTaskStatus("approved"), decode[apigen.ComplianceTask](t, r).Status)
		assert.Equal(t, http.StatusOK, act(admin.b, http.MethodGet, ftr.Id+"/export", nil).status, "an approved threshold report is exported for filing")
		assert.Equal(t, http.StatusBadRequest, act(officer.b, http.MethodPost, ftr.Id+"/file", map[string]any{"filing_ref": " "}).status)
		r = act(officer.b, http.MethodPost, ftr.Id+"/file", map[string]any{"filing_ref": "NFIU-FTR-2026-0001"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		filed := decode[apigen.ComplianceTask](t, r)
		assert.Equal(t, apigen.ComplianceTaskStatus("filed"), filed.Status)
		assert.Equal(t, "NFIU-FTR-2026-0001", *filed.FilingRef)
	})

	t.Run("a suspicious transaction report: its clock, its checks and its secrecy", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Pooling pattern"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		caseID := decode[apigen.Case](t, r).Id

		assert.Equal(t, http.StatusForbidden, analyst.b.do(t, http.MethodPost, "/api/v1/compliance/suspicion", map[string]any{"case_id": caseID}).status)
		assert.Equal(t, http.StatusForbidden, admin.b.do(t, http.MethodPost, "/api/v1/compliance/suspicion", map[string]any{"case_id": caseID}).status,
			"not even administrators (GOV-09)")
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/suspicion", map[string]any{"case_id": caseID, "note": "Funds pooled from many strangers"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		str := decode[apigen.ComplianceTask](t, r)
		assert.Equal(t, "report.STR", str.Kind)
		assert.True(t, str.Sensitive)
		require.NotNil(t, str.DueAt)
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), *str.DueAt, time.Minute, "24 hours from confirmed suspicion (REG-14)")

		for name, b := range map[string]*browser{"an analyst": analyst.b, "an administrator": admin.b} {
			assert.Nil(t, taskOfKind(queue(b).Data, "report.STR"), "%s does not see the STR", name)
			assert.Equal(t, http.StatusNotFound, act(b, http.MethodGet, str.Id, nil).status, "%s cannot find it by ID either", name)
		}
		logs := s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil).String()
		assert.NotContains(t, logs, "report.STR", "the audit log says compliance work happened, not what")

		r = act(officer.b, http.MethodPost, str.Id+"/approve", nil)
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
		assert.Contains(t, r.String(), "This report would be rejected by goAML: the predicate offence is missing")
		r = act(officer.b, http.MethodPatch, str.Id+"/report", map[string]any{
			"category": "fraud", "predicate_offence": "Advance-fee fraud", "indicators": []string{"Many unrelated senders", "Rapid onward transfers"},
		})
		require.Equal(t, http.StatusOK, r.status, r.String())
		r = act(officer.b, http.MethodPost, str.Id+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, http.StatusConflict, act(officer.b, http.MethodPatch, str.Id+"/report", map[string]any{"narrative": "x"}).status,
			"an approved report does not change")
	})

	t.Run("due diligence tasks are done or dismissed with a note", func(t *testing.T) {
		r := s.ingest(t, a.testKey, money("pep-1", "transfer_outbound", "acc_pep", 100_000, "NGN", map[string]any{"customer.is_pep": true}))
		require.Equal(t, http.StatusOK, r.status, r.String())
		var edd *apigen.ComplianceTask
		require.Eventually(t, func() bool {
			edd = taskOfKind(queue(officer.b).Data, "edd")
			return edd != nil
		}, 20*time.Second, 100*time.Millisecond, "REG-12 requires enhanced due diligence")
		assert.Nil(t, edd.DueAt, "the pack sets no deadline for it")
		assert.Equal(t, http.StatusBadRequest, act(officer.b, http.MethodPost, edd.Id+"/close", map[string]any{"status": "dismissed"}).status)
		r = act(admin.b, http.MethodPost, edd.Id+"/close", map[string]any{"status": "done"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, http.StatusConflict, act(admin.b, http.MethodPost, edd.Id+"/close", map[string]any{"status": "done"}).status)
		assert.Equal(t, http.StatusForbidden, act(analyst.b, http.MethodPost, edd.Id+"/close", map[string]any{"status": "done"}).status)
	})

	t.Run("the queue lists screening escalations for alert readers", func(t *testing.T) {
		q := queue(officer.b)
		assert.NotNil(t, q.Escalations)
		assert.Empty(t, q.Escalations, "no alert is 72 hours old in this test")
		// The read behind escalations: alerts still open since before a time (REG-14).
		s.waitForAlerts(t, a.testKey, "status=open", 1)
		ctx, err := s.tenants.OperatorScope(t.Context(), a.id, tenancy.EnvironmentTest, "riskosctl:integration")
		require.NoError(t, err)
		late, err := s.alerts.OpenBefore(ctx, time.Now().Add(time.Hour), 10)
		require.NoError(t, err)
		assert.NotEmpty(t, late, "open alerts created before the cutoff")
		late, err = s.alerts.OpenBefore(ctx, time.Now().Add(-time.Hour), 10)
		require.NoError(t, err)
		assert.Empty(t, late, "none created an hour ago")
		r := s.call(t, http.MethodGet, "/api/v1/compliance/tasks", a.testKey, nil)
		assert.Equal(t, http.StatusOK, r.status, "API keys read the queue: %s", r.String())
		assert.Nil(t, taskOfKind(decode[apigen.ComplianceQueue](t, r).Data, "report.STR"), "and never see STRs")
	})
}
