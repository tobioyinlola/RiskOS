//go:build integration

package integration

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// Governance presets the workflow enforces (phase 6): untouched urgent alerts escalate after
// GOV-11's response target, confirmed fraud is tracked until it is reported to the industry
// channel (GOV-12), and REG-16's daily check closes accounts without a BVN.
func TestComplianceGovernance(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	s.startWorker(t)
	a := s.createTenant(t, "Governance")
	admin := s.onboard(t, a, "admin@governance.example", authz.RoleCustomerAdmin)
	officer := s.onboard(t, a, "officer@governance.example", authz.RoleComplianceOfficer)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	var envID uuid.UUID
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		"SELECT id FROM environments WHERE tenant_id = $1 AND kind = 'test'", a.id).Scan(&envID))

	queue := func(b *browser) apigen.ComplianceQueue {
		t.Helper()
		r := b.do(t, http.MethodGet, "/api/v1/compliance/tasks", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.ComplianceQueue](t, r)
	}

	t.Run("GOV-11: urgent alerts nobody acted on escalate after the response target", func(t *testing.T) {
		// Alerts created 40 minutes ago (their IDs carry their creation time), still open.
		alert := func(severity, subject string) string {
			t.Helper()
			created := time.Now().Add(-40 * time.Minute)
			id := ids.V7At(created)
			_, err := shared.owner.Exec(t.Context(), `INSERT INTO alerts (tenant_id, environment_id, id, kind, subject_type,
				subject_ref, title, severity, status, max_score, evaluation_count, first_seen_at, last_seen_at, created_at, updated_at)
				VALUES ($1, $2, $3, 'risk', 'user', $4, 'Late alert', $5, 'open', 80, 1, $6, $6, $6, $6)`,
				a.id, envID, id, subject, severity, created)
			require.NoError(t, err)
			return ids.Format(ids.PrefixAlert, id)
		}
		critical, low := alert("critical", "u_gov11_critical"), alert("low", "u_gov11_low")

		esc := queue(officer.b).Escalations
		has := func(rule, id string) bool {
			return slices.ContainsFunc(esc, func(e apigen.ScreeningEscalation) bool {
				return string(e.Rule) == rule && e.AlertId == id
			})
		}
		assert.True(t, has("GOV-11", critical), "a critical alert untouched for 40 minutes: %+v", esc)
		assert.False(t, has("GOV-11", low), "low-severity alerts wait for REG-14's screening deadline")
		assert.False(t, has("REG-14", critical), "REG-14's 72 hours have not passed")
		i := slices.IndexFunc(esc, func(e apigen.ScreeningEscalation) bool { return e.AlertId == critical })
		require.GreaterOrEqual(t, i, 0)
		assert.Equal(t, apigen.ScreeningEscalationSeverity("critical"), esc[i].Severity)
		assert.WithinDuration(t, time.Now().Add(-10*time.Minute), esc[i].DueAt, time.Minute, "due 30 minutes after creation")

		r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/rules/GOV-11/config",
			map[string]any{"mode": "live", "settings": map[string]any{"respond_within": 3600}, "reason": "One hour for the test"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		esc = queue(officer.b).Escalations
		assert.False(t, has("GOV-11", critical), "the response target is configurable")
	})

	t.Run("GOV-12: confirmed fraud is listed until an industry report is prepared", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Account takeover"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		caseID := decode[apigen.Case](t, r).Id
		prepare := func(b *browser) response {
			return b.do(t, http.MethodPost, "/api/v1/compliance/fraud-reports", map[string]any{"case_id": caseID})
		}
		assert.Equal(t, http.StatusBadRequest, prepare(admin.b).status, "an open case is not confirmed fraud")

		r = s.call(t, http.MethodPatch, "/api/v1/cases/"+caseID, a.testKey, map[string]any{
			"status": "resolved", "resolution": "confirmed_fraud", "resolution_note": "Credentials phished; funds moved out.",
		})
		require.Equal(t, http.StatusOK, r.status, r.String())
		unreported := func() bool {
			return slices.ContainsFunc(queue(officer.b).UnreportedFraud, func(c apigen.UnreportedFraudCase) bool { return c.CaseId == caseID })
		}
		assert.True(t, unreported(), "a confirmed-fraud case without a report is tracked")

		r = prepare(admin.b)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		report := decode[apigen.ComplianceTask](t, r)
		assert.Equal(t, "report.industry_fraud", report.Kind)
		assert.Equal(t, "GOV-12", report.Rule)
		assert.Nil(t, report.DueAt, "the source states no deadline, so none is invented")
		assert.False(t, report.Sensitive)
		assert.Equal(t, "Credentials phished; funds moved out.", report.Report["narrative"])
		assert.False(t, unreported())
		assert.Equal(t, http.StatusConflict, prepare(admin.b).status, "one report per case")

		// Filing stays a person's act: approved by a compliance officer, filed with a reference.
		act := func(b *browser, path string, body any) response {
			return b.do(t, http.MethodPost, "/api/v1/compliance/tasks/"+report.Id+path, body)
		}
		assert.Equal(t, http.StatusForbidden, act(admin.b, "/approve", nil).status)
		require.Equal(t, http.StatusOK, act(officer.b, "/approve", nil).status)
		r = act(officer.b, "/file", map[string]any{"filing_ref": "NIBSS-FR-0042"})
		require.Equal(t, http.StatusOK, r.status, r.String())

		r = act(officer.b, "/close", map[string]any{"status": "dismissed", "note": "x"})
		assert.Equal(t, http.StatusConflict, r.status, "a filed report is final")
	})

	t.Run("REG-16: the daily check closes accounts without a BVN", func(t *testing.T) {
		daily := s.rules.DailyJob(shared.pool, s.tenants, clock.System{}, shared.logger)
		send := func(id, account string, hasBVN bool) {
			t.Helper()
			r := s.ingest(t, a.testKey, money(id, "transfer_outbound", account, 50_000, "NGN", map[string]any{"account.has_bvn": hasBVN}))
			require.Equal(t, http.StatusOK, r.status, r.String())
		}
		closeTask := func() *apigen.ComplianceTask {
			for _, tk := range queue(officer.b).Data {
				if tk.Kind == "close_account" && tk.AccountRef != nil && *tk.AccountRef == "acc_nobvn" {
					return &tk
				}
			}
			return nil
		}
		registered := func(account string, want bool) func() bool {
			return func() bool {
				var has bool
				err := shared.owner.QueryRow(t.Context(), `SELECT has_bvn FROM compliance_accounts
					WHERE tenant_id = $1 AND environment_id = $2 AND account_ref = $3`, a.id, envID, account).Scan(&has)
				return err == nil && has == want
			}
		}

		send("bvn-1", "acc_nobvn", false)
		require.Eventually(t, registered("acc_nobvn", false), 20*time.Second, 100*time.Millisecond, "the task job fills the register")
		require.Nil(t, closeTask(), "a transfer is not REG-16's trigger: only the daily check opens the task")

		require.NoError(t, daily(t.Context(), rules.DailyArgs{}))
		task := closeTask()
		require.NotNil(t, task, "the daily check opens a task for the account")
		assert.Equal(t, "REG-16", task.Rule)
		require.NotNil(t, task.DueAt)
		assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), *task.DueAt, 5*time.Minute, "30 days after the account was first seen")
		require.NoError(t, daily(t.Context(), rules.DailyArgs{}))
		n := 0
		for _, tk := range queue(officer.b).Data {
			if tk.AccountRef != nil && *tk.AccountRef == "acc_nobvn" {
				n++
			}
		}
		assert.Equal(t, 1, n, "the check is idempotent")

		send("bvn-2", "acc_nobvn", true)
		require.Eventually(t, registered("acc_nobvn", true), 20*time.Second, 100*time.Millisecond)
		require.NoError(t, daily(t.Context(), rules.DailyArgs{}))
		task = closeTask()
		require.NotNil(t, task)
		assert.Equal(t, apigen.ComplianceTaskStatus("done"), task.Status, "a BVN on file ends the task")
		require.NotNil(t, task.CloseNote)
		assert.Contains(t, *task.CloseNote, "BVN")
	})
}
