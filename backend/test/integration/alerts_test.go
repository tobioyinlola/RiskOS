//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// startWorker runs the alert job handler, as cmd/worker does, until the test ends.
func (s stack) startWorker(t *testing.T) {
	t.Helper()
	registry := jobs.NewRegistry()
	jobs.Register(registry, s.alerts.RaiseJob(s.tenants))
	queue, err := jobs.NewQueue(shared.pool, registry, jobs.QueueConfig{Concurrency: 4, JobTimeout: 10 * time.Second}, shared.logger)
	require.NoError(t, err)
	require.NoError(t, queue.Start(context.WithoutCancel(t.Context())))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, queue.Stop(ctx))
	})
}

// waitForAlerts polls the alert list until it has at least n alerts matching query and the
// newest evaluation counts have settled, then returns them.
func (s stack) waitForAlerts(t *testing.T, key, query string, n int) []apigen.Alert {
	t.Helper()
	var list apigen.AlertList
	require.Eventually(t, func() bool {
		r := s.call(t, http.MethodGet, "/api/v1/alerts?"+query, key, nil)
		if r.status != http.StatusOK {
			return false
		}
		list = decode[apigen.AlertList](t, r)
		return len(list.Data) >= n
	}, 20*time.Second, 50*time.Millisecond, "waiting for %d alerts (%s)", n, query)
	return list.Data
}

// alertJobs counts the alert jobs enqueued for an evaluation.
func alertJobs(t *testing.T, evaluationID string) int {
	t.Helper()
	id, err := ids.Parse(ids.PrefixEvaluation, evaluationID)
	require.NoError(t, err)
	var n int
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT count(*) FROM river_job WHERE kind = 'alerts_raise' AND args->>'evaluation_id' = $1`, id.String()).Scan(&n))
	return n
}

// alwaysReview is a rule that makes every transaction CRITICAL (so REVIEW).
func alwaysReview() map[string]any {
	return map[string]any{
		"code": "always_review", "name": "Review everything", "severity": "critical", "weight": 90, "mode": "active",
		"condition": map[string]any{"field": "transaction.amount", "op": "gte", "value": 1},
	}
}

func (s stack) mustRule(t *testing.T, key string, rule map[string]any) {
	t.Helper()
	r := s.call(t, http.MethodPost, "/api/v1/rules", key, rule)
	require.Equal(t, http.StatusCreated, r.status, r.String())
}

// TestEndToEndAlertToResolution is the spec's end-to-end flow (§ testing): tenant → key → event
// → risk → alert → case → resolution, with every step in the audit log.
func TestEndToEndAlertToResolution(t *testing.T) {
	s := newStack(t, time.Second)
	s.startWorker(t)
	a := s.createTenant(t, "End to end")
	u := actor{"u_e2e", "acc_e2e", "dev_e2e"}

	// History, then the spec's scenario: CRITICAL, REVIEW.
	for range 6 {
		s.decide(t, a.testKey, debit(u, "ben_usual", "NG", "NGN", 200000))
	}
	u.device = "dev_new"
	d := s.decide(t, a.testKey, debit(u, "ben_new", "GH", "NGN", 5000000))
	require.Equal(t, apigen.Decision("review"), d.Decision)

	alertList := s.waitForAlerts(t, a.testKey, "kind=risk", 1)
	require.Len(t, alertList, 1, "only the REVIEW decision raised an alert")
	al := alertList[0]
	assert.Equal(t, apigen.AlertStatus("open"), al.Status)
	assert.Equal(t, apigen.Severity("critical"), al.Severity)
	assert.Equal(t, "u_e2e", al.Subject.Ref)
	assert.Equal(t, 84, al.MaxScore)

	detail := decode[apigen.AlertDetail](t, s.call(t, http.MethodGet, "/api/v1/alerts/"+al.Id, a.testKey, nil))
	require.Len(t, detail.Evaluations, 1)
	assert.Equal(t, d.EvaluationId, detail.Evaluations[0].EvaluationId)
	assert.Equal(t, *d.EventId, detail.Evaluations[0].EventId)
	assert.Len(t, detail.Evaluations[0].Signals, 5)
	assert.Equal(t, "created", detail.Activity[0].Action)
	assert.Equal(t, "system:alerts", detail.Activity[0].Actor)

	// Triage, then escalate into a case.
	r := s.call(t, http.MethodPatch, "/api/v1/alerts/"+al.Id, a.testKey,
		map[string]any{"status": "investigating", "assignee": "analyst@example.com", "note": "Looking at the device."})
	require.Equal(t, http.StatusOK, r.status, r.String())
	assert.Equal(t, "analyst@example.com", *decode[apigen.Alert](t, r).Assignee)

	r = s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{
		"title": "Possible account takeover", "alert_ids": []string{al.Id}, "assignee": "analyst@example.com",
	})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	c := decode[apigen.Case](t, r)
	assert.Equal(t, apigen.Severity("critical"), c.Priority, "priority follows the alerts")
	assert.Equal(t, apigen.CaseStatus("open"), c.Status)
	escalated := decode[apigen.AlertDetail](t, s.call(t, http.MethodGet, "/api/v1/alerts/"+al.Id, a.testKey, nil)).Alert
	assert.Equal(t, apigen.AlertStatus("escalated"), escalated.Status)
	assert.Equal(t, c.Id, *escalated.CaseId)

	r = s.call(t, http.MethodPost, "/api/v1/cases/"+c.Id+"/notes", a.testKey, map[string]any{"body": "Customer confirmed they did not send this."})
	require.Equal(t, http.StatusCreated, r.status, r.String())

	r = s.call(t, http.MethodPatch, "/api/v1/cases/"+c.Id, a.testKey, map[string]any{
		"status": "resolved", "resolution": "confirmed_fraud", "resolution_note": "Account takeover; credentials reset.",
	})
	require.Equal(t, http.StatusOK, r.status, r.String())
	resolved := decode[apigen.Case](t, r)
	assert.Equal(t, apigen.CaseStatus("resolved"), resolved.Status)
	assert.Equal(t, apigen.CaseResolution("confirmed_fraud"), *resolved.Resolution)
	assert.NotNil(t, resolved.ResolvedAt)

	cd := decode[apigen.CaseDetail](t, s.call(t, http.MethodGet, "/api/v1/cases/"+c.Id, a.testKey, nil))
	require.Len(t, cd.Alerts, 1)
	assert.Equal(t, apigen.AlertStatus("resolved"), cd.Alerts[0].Status, "resolving the case closed its alert")
	require.Len(t, cd.Notes, 1)
	assert.Equal(t, []string{"created", "resolved"}, activityActions(cd.Activity))

	actions := auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil))
	for _, want := range []string{"alert.created", "alert.updated", "case.created", "case.note_added", "case.resolved"} {
		assert.Contains(t, actions, want)
	}
	res, err := audit.Verify(t.Context(), shared.owner, a.id)
	require.NoError(t, err)
	assert.True(t, res.Valid, "the audit chain verifies with system (alerts job) entries in it: %s", res.Reason)
}

func activityActions(entries []apigen.ActivityEntry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Action)
	}
	return out
}

func TestAlertGroupingAndThreshold(t *testing.T) {
	s := newStack(t, time.Second)
	s.startWorker(t)
	a := s.createTenant(t, "Grouping")
	s.mustRule(t, a.testKey, alwaysReview())

	u1 := actor{"u_group_1", "acc_g1", "dev_g1"}
	u2 := actor{"u_group_2", "acc_g2", "dev_g2"}
	for range 3 {
		require.Equal(t, apigen.Decision("review"), s.decide(t, a.testKey, debit(u1, "ben_g", "NG", "NGN", 5000)).Decision)
	}
	s.decide(t, a.testKey, debit(u2, "ben_g", "NG", "NGN", 5000))

	require.Eventually(t, func() bool {
		list := decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts?kind=risk", a.testKey, nil))
		total := 0
		for _, al := range list.Data {
			total += al.EvaluationCount
		}
		return total == 4
	}, 20*time.Second, 50*time.Millisecond)
	list := decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts?kind=risk", a.testKey, nil)).Data
	require.Len(t, list, 2, "one alert per user")
	counts := map[string]int{}
	for _, al := range list {
		counts[al.Subject.Ref] = al.EvaluationCount
	}
	assert.Equal(t, map[string]int{"u_group_1": 3, "u_group_2": 1}, counts, "same user within an hour: grouped")

	t.Run("step_up does not alert by default", func(t *testing.T) {
		r := s.call(t, http.MethodPatch, "/api/v1/rules/"+ruleID(t, s, a.testKey, "always_review"), a.testKey, map[string]any{"weight": 65})
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := s.decide(t, a.testKey, debit(actor{"u_step", "acc_step", "dev_step"}, "ben_g", "NG", "NGN", 5000))
		require.Equal(t, apigen.Decision("step_up"), d.Decision)
		assert.Zero(t, alertJobs(t, d.EvaluationId), "no alert job for a decision below the threshold")
	})
	t.Run("the threshold is a tenant setting", func(t *testing.T) {
		settings := decode[apigen.RiskSettings](t, s.call(t, http.MethodGet, "/api/v1/risk/settings", a.testKey, nil))
		require.NotNil(t, settings.AlertMinDecision)
		assert.Equal(t, apigen.RiskSettingsAlertMinDecision("review"), *settings.AlertMinDecision)
		r := s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, map[string]any{
			"weights": map[string]any{}, "high_value_default": 1000000, "high_value_by_currency": map[string]any{},
			"fail_policies": map[string]any{"transaction": "closed", "login": "open"}, "alert_min_decision": "step_up",
		})
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := s.decide(t, a.testKey, debit(actor{"u_step2", "acc_step2", "dev_step2"}, "ben_g", "NG", "NGN", 5000))
		require.Equal(t, apigen.Decision("step_up"), d.Decision)
		assert.Equal(t, 1, alertJobs(t, d.EvaluationId))
		s.waitForAlerts(t, a.testKey, "kind=risk", 3)
	})
	t.Run("dry runs and replays never enqueue", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/risk/evaluate", a.testKey, debit(u1, "ben_g", "NG", "NGN", 5000))
		require.Equal(t, http.StatusOK, r.status)
		assert.Zero(t, alertJobs(t, decode[apigen.RiskDecision](t, r).EvaluationId))

		e := debit(u1, "ben_g", "NG", "NGN", 5000)
		first := s.decide(t, a.testKey, e)
		s.decide(t, a.testKey, e)
		assert.Equal(t, 1, alertJobs(t, first.EvaluationId), "a replay returns the stored decision and enqueues nothing")
	})
}

func ruleID(t *testing.T, s stack, key, code string) string {
	t.Helper()
	for _, r := range decode[apigen.RuleList](t, s.call(t, http.MethodGet, "/api/v1/rules", key, nil)).Data {
		if r.Code == code {
			return r.Id
		}
	}
	t.Fatalf("rule %s not found", code)
	return ""
}

func TestSecurityAlertsForDegradedEvaluations(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { _ = dead.Close() })
	s := newStackWith(t, time.Second, generousLimits(), dead)
	s.startWorker(t)
	a := s.createTenant(t, "Degraded alerts")
	u := actor{"u_blind", "acc_blind", "dev_blind"}

	lg := s.decide(t, a.testKey, login(u, "success"))
	require.Equal(t, apigen.Decision("allow"), lg.Decision, "logins fail open")
	txn := s.decide(t, a.testKey, debit(u, "ben_b", "NG", "NGN", 5000))
	require.Equal(t, apigen.Decision("review"), txn.Decision, "transactions fail closed")

	sec := s.waitForAlerts(t, a.testKey, "kind=security", 1)
	require.Len(t, sec, 1, "one security alert per environment and hour")
	require.Eventually(t, func() bool {
		d := decode[apigen.AlertDetail](t, s.call(t, http.MethodGet, "/api/v1/alerts/"+sec[0].Id, a.testKey, nil))
		return d.Alert.EvaluationCount == 2
	}, 20*time.Second, 50*time.Millisecond, "both degraded evaluations grouped")
	assert.Equal(t, apigen.Severity("high"), sec[0].Severity)
	assert.Equal(t, apigen.AlertSubjectType("environment"), sec[0].Subject.Type)

	risk := s.waitForAlerts(t, a.testKey, "kind=risk", 1)
	assert.Len(t, risk, 1, "the fail-closed REVIEW also raised a risk alert")
}

func TestAlertAndCaseRules(t *testing.T) {
	s := newStack(t, time.Second)
	s.startWorker(t)
	a := s.createTenant(t, "Triage rules")
	s.mustRule(t, a.testKey, alwaysReview())
	for i := range 3 {
		s.decide(t, a.testKey, debit(actor{fmt.Sprintf("u_rules_%d", i), "acc", "dev"}, "ben", "NG", "NGN", 5000))
	}
	list := s.waitForAlerts(t, a.testKey, "kind=risk", 3)
	first, second, third := list[0].Id, list[1].Id, list[2].Id
	patch := func(id string, body map[string]any) response {
		return s.call(t, http.MethodPatch, "/api/v1/alerts/"+id, a.testKey, body)
	}

	t.Run("alert status machine", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, patch(first, map[string]any{"status": "escalated"}).status, "not in the contract")
		require.Equal(t, http.StatusOK, patch(first, map[string]any{"status": "false_positive"}).status)
		r := patch(first, map[string]any{"status": "open"})
		assert.Equal(t, http.StatusConflict, r.status, "final")
		assert.Equal(t, http.StatusConflict, patch(first, map[string]any{"assignee": "x"}).status)
		unchanged := patch(second, map[string]any{"status": "open"})
		require.Equal(t, http.StatusOK, unchanged.status)
		assert.Empty(t, activityActions(decode[apigen.AlertDetail](t, s.call(t, http.MethodGet, "/api/v1/alerts/"+second, a.testKey, nil)).Activity)[1:],
			"a no-op change adds nothing to the timeline")
		assert.Equal(t, http.StatusNotFound, patch("alr_01j9x2k7m3q4r5s6t7v8w9x0yz", map[string]any{"status": "open"}).status)
	})
	t.Run("filters", func(t *testing.T) {
		fp := decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts?status=false_positive", a.testKey, nil))
		require.Len(t, fp.Data, 1)
		assert.Equal(t, first, fp.Data[0].Id)
		page := decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts?limit=2", a.testKey, nil))
		require.Len(t, page.Data, 2)
		require.NotNil(t, page.NextCursor)
		rest := decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts?limit=2&cursor="+url.QueryEscape(*page.NextCursor), a.testKey, nil))
		assert.Len(t, rest.Data, 1)
	})
	t.Run("cases", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Closed alert", "alert_ids": []string{first}})
		assert.Equal(t, http.StatusConflict, r.status, "a closed alert cannot be escalated")
		assert.Empty(t, decode[apigen.CaseList](t, s.call(t, http.MethodGet, "/api/v1/cases", a.testKey, nil)).Data,
			"the failed creation rolled back")

		c := decode[apigen.Case](t, s.call(t, http.MethodPost, "/api/v1/cases", a.testKey,
			map[string]any{"title": "Two alerts", "alert_ids": []string{second, third}, "priority": "low"}))
		assert.Equal(t, apigen.Severity("low"), c.Priority, "an explicit priority wins")
		assert.Equal(t, http.StatusConflict, patch(second, map[string]any{"status": "resolved"}).status, "escalated: resolve the case")
		assert.Equal(t, http.StatusConflict, s.call(t, http.MethodPost, "/api/v1/cases", a.testKey,
			map[string]any{"title": "Again", "alert_ids": []string{second}}).status, "already in a case")

		path := "/api/v1/cases/" + c.Id
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"status": "resolved"}).status,
			"resolution required")
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"resolution": "legitimate"}).status,
			"resolution only when resolving")
		r = s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"status": "investigating", "assignee": "lead@example.com", "priority": "high"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		r = s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"status": "resolved", "resolution": "false_positive"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		cd := decode[apigen.CaseDetail](t, s.call(t, http.MethodGet, path, a.testKey, nil))
		for _, al := range cd.Alerts {
			assert.Equal(t, apigen.AlertStatus("false_positive"), al.Status)
		}
		assert.Equal(t, http.StatusConflict, s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"title": "x"}).status, "final")
		assert.Equal(t, http.StatusCreated, s.call(t, http.MethodPost, path+"/notes", a.testKey, map[string]any{"body": "Follow-up."}).status,
			"notes are still allowed")
		assert.Equal(t, []string{"created", "updated", "resolved"}, activityActions(cd.Activity))
	})
}

func TestAlertScopesAndIsolation(t *testing.T) {
	s := newStack(t, time.Second)
	s.startWorker(t)
	a := s.createTenant(t, "Alerts A")
	b := s.createTenant(t, "Alerts B")
	s.mustRule(t, a.testKey, alwaysReview())
	s.decide(t, a.testKey, debit(actor{"u_iso", "acc", "dev"}, "ben", "NG", "NGN", 5000))
	al := s.waitForAlerts(t, a.testKey, "", 1)[0]
	c := decode[apigen.Case](t, s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Mine"}))
	assert.Equal(t, apigen.Severity("medium"), c.Priority, "default priority without alerts")

	t.Run("other tenant sees nothing", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, "/api/v1/alerts/"+al.Id, b.testKey, nil).status)
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPatch, "/api/v1/alerts/"+al.Id, b.testKey, map[string]any{"status": "resolved"}).status)
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, "/api/v1/cases/"+c.Id, b.testKey, nil).status)
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPost, "/api/v1/cases/"+c.Id+"/notes", b.testKey, map[string]any{"body": "x"}).status)
		assert.Empty(t, decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts", b.testKey, nil)).Data)
		assert.Empty(t, decode[apigen.CaseList](t, s.call(t, http.MethodGet, "/api/v1/cases", b.testKey, nil)).Data)
		r := s.call(t, http.MethodPost, "/api/v1/cases", b.testKey, map[string]any{"title": "Steal", "alert_ids": []string{al.Id}})
		assert.Equal(t, http.StatusNotFound, r.status, "another tenant's alert cannot be escalated")
	})
	t.Run("scopes", func(t *testing.T) {
		_, reader := s.createKey(t, a.testKey, "alert reader", "alerts:read")
		assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/alerts/"+al.Id, reader, nil).status)
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPatch, "/api/v1/alerts/"+al.Id, reader, map[string]any{"status": "resolved"}).status)
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodGet, "/api/v1/cases", reader, nil).status)
		_, caser := s.createKey(t, a.testKey, "case writer", "cases:write", "cases:read")
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, "/api/v1/cases", caser,
			map[string]any{"title": "No alert rights", "alert_ids": []string{al.Id}}).status,
			"escalating alerts also needs alerts:write")
		assert.Equal(t, http.StatusCreated, s.call(t, http.MethodPost, "/api/v1/cases", caser, map[string]any{"title": "Fine"}).status)
	})
	t.Run("history tables are insert-only", func(t *testing.T) {
		for _, table := range []string{"alert_activity", "alert_evaluations", "case_notes", "case_activity"} {
			_, err := shared.owner.Exec(t.Context(), "UPDATE "+table+" SET tenant_id = tenant_id")
			assert.Error(t, err, table)
		}
	})
	t.Run("rls hides alerts and cases without a scope", func(t *testing.T) {
		for _, table := range []string{"alerts", "cases", "alert_evaluations", "case_notes"} {
			var n int
			require.NoError(t, shared.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
			assert.Zero(t, n, table)
		}
	})
}

func TestRaiseJobIsIdempotent(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Idempotent raise")
	who := decode[apigen.Whoami](t, s.call(t, http.MethodGet, "/api/v1/whoami", a.testKey, nil))
	envID, err := ids.Parse(ids.PrefixEnvironment, who.EnvironmentId)
	require.NoError(t, err)

	args := alerts.RaiseArgs{
		TenantID: a.id, EnvironmentID: envID, EvaluationID: uuid.Must(uuid.NewV7()), EventID: uuid.Must(uuid.NewV7()),
		EventType: "transaction", UserRef: "u_retry", Score: 90, Level: "critical", Decision: "review",
		Signals: []alerts.RaiseSignal{{Code: "always_review", Label: "Review everything", Weight: 90}}, Degraded: []string{},
		EvaluatedAt: time.Now(), Risk: true,
	}
	run := s.alerts.RaiseJob(s.tenants)
	require.NoError(t, run(t.Context(), args))
	require.NoError(t, run(t.Context(), args), "a retried job")
	list := decode[apigen.AlertList](t, s.call(t, http.MethodGet, "/api/v1/alerts", a.testKey, nil)).Data
	require.Len(t, list, 1)
	assert.Equal(t, 1, list[0].EvaluationCount, "the retry added nothing")

	t.Run("jobs of unknown environments are dropped", func(t *testing.T) {
		other := args
		other.EnvironmentID = uuid.Must(uuid.NewV7())
		assert.NoError(t, run(t.Context(), other))
	})
}
