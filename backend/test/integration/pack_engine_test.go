//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// money returns a transaction event of kind for account, amount in minor units, with facts.
func money(id, kind, account string, amount int64, currency string, facts map[string]any) map[string]any {
	e := txn(id)
	e["actor"] = map[string]any{"user_id": "u_" + account, "account_id": account, "device_id": "dev_" + account}
	data, _ := e["data"].(map[string]any) // built by txn
	data["amount"], data["currency"] = amount, currency
	c := map[string]any{"kind": kind}
	if len(facts) > 0 {
		c["facts"] = facts
	}
	e["compliance"] = c
	return e
}

func controlOf(d apigen.RiskDecision, rule string) *apigen.ControlResult {
	for i := range d.Controls {
		if d.Controls[i].Rule == rule {
			return &d.Controls[i]
		}
	}
	return nil
}

func notEvaluated(d apigen.RiskDecision, rule string) []string {
	for _, n := range d.NotEvaluated {
		if n.Rule == rule {
			return n.Missing
		}
	}
	return nil
}

// The Nigeria pack runs on live traffic (ADR-0006): controls set the action the law requires,
// windows are kept in Redis per entity, and missing data is reported, never read as no risk.
func TestRulePackEngine(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Pack engine")
	// Risk signals at the pack's defaults (monitoring), as for profiles set up before automatic
	// profiles started them off: these tests read what the signals record.
	_, err := shared.owner.Exec(t.Context(), "UPDATE jurisdiction_activations SET signal_default = 'pack' WHERE tenant_id = $1", a.id)
	require.NoError(t, err)
	key := a.testKey
	decide := func(e map[string]any) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, key, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	bound := map[string]any{"device.bound_age": 3600, "account.age": 400 * 86400}

	t.Run("without a compliance profile no pack runs", func(t *testing.T) {
		none := s.createTenant(t, "No profile")
		withoutCompliance(t, none.id)
		r := s.ingest(t, none.testKey, money("np-1", "transfer_outbound", "acc_np", 5_000_000, "NGN", bound))
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := decode[apigen.RiskDecision](t, r)
		assert.Empty(t, d.Controls)
		assert.Empty(t, d.NotEvaluated)
		assert.Equal(t, apigen.Decision("allow"), d.Decision)
	})

	admin := s.onboard(t, a, "admin@pack-engine.example", authz.RoleCustomerAdmin)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{"instant_payments", "cash_fee"}})
	require.Equal(t, http.StatusOK, r.status, r.String())

	t.Run("REG-01 declines above ₦20,000 within 24 hours of a new device", func(t *testing.T) {
		d := decide(money("r1-1", "transfer_outbound", "acc_r1", 2_000_001, "NGN", bound))
		assert.Equal(t, apigen.Decision("decline"), d.Decision)
		c := controlOf(d, "REG-01")
		require.NotNil(t, c, "%+v", d.Controls)
		assert.Equal(t, apigen.ControlResultMode("live"), c.Mode)
		assert.Equal(t, apigen.Decision("decline"), *c.Decision)

		d = decide(money("r1-2", "transfer_outbound", "acc_r1", 2_000_000, "NGN", bound))
		assert.Nil(t, controlOf(d, "REG-01"), "exactly ₦20,000 is allowed")
		assert.NotEqual(t, apigen.Decision("decline"), d.Decision)

		d = decide(money("r1-3", "transfer_outbound", "acc_r1", 2_000_001, "NGN", nil))
		assert.Nil(t, controlOf(d, "REG-01"))
		assert.Contains(t, notEvaluated(d, "REG-01"), "fact:device.bound_age", "binding unknown: not evaluated, not allowed by default")
	})

	atm := func(id, account string, naira int64, currency string, write bool) apigen.RiskDecision {
		t.Helper()
		e := money(id, "atm_withdrawal", account, naira*100, currency, nil)
		path := "/api/v1/events"
		if !write {
			path = "/api/v1/risk/evaluate"
		}
		r := s.call(t, http.MethodPost, path, key, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	t.Run("REG-09 sums ATM withdrawals per customer and day", func(t *testing.T) {
		assert.Nil(t, controlOf(atm("atm-1", "acc_atm", 60_000, "NGN", true), "REG-09"))
		dry := atm("atm-dry", "acc_atm", 50_000, "NGN", false)
		assert.Equal(t, apigen.Decision("decline"), dry.Decision, "a dry run sees ₦110,000")
		assert.Nil(t, controlOf(atm("atm-2", "acc_atm", 30_000, "NGN", true), "REG-09"),
			"the dry run was not recorded: ₦90,000")
		d := atm("atm-3", "acc_atm", 20_000, "NGN", true)
		require.NotNil(t, controlOf(d, "REG-09"), "₦110,000 in a day: %+v", d)
		assert.Equal(t, apigen.Decision("decline"), d.Decision)
		// Another customer's withdrawals are another window.
		assert.Nil(t, controlOf(atm("atm-4", "acc_atm_other", 20_000, "NGN", true), "REG-09"))
	})

	t.Run("an amount in another currency is not compared with a naira limit", func(t *testing.T) {
		d := atm("atm-usd", "acc_usd", 200_000, "USD", true)
		assert.Nil(t, controlOf(d, "REG-09"))
		missing := notEvaluated(d, "REG-09")
		assert.True(t, slices.ContainsFunc(missing, func(m string) bool { return len(m) > 9 && m[:9] == "currency:" }), "%v", missing)
	})

	t.Run("REG-08 charges the cash withdrawal fee", func(t *testing.T) {
		d := decide(money("fee-1", "cash_withdrawal", "acc_fee", 160_000_000, "NGN", map[string]any{"customer.type": "individual"}))
		c := controlOf(d, "REG-08")
		require.NotNil(t, c, "%+v", d.Controls)
		assert.Equal(t, apigen.Decision("fee"), *c.Decision)
		require.NotNil(t, c.FeePercent)
		assert.InDelta(t, 3.0, *c.FeePercent, 1e-9)
	})

	t.Run("an event whose window filter cannot be checked makes the window unavailable", func(t *testing.T) {
		// ATO-06 counts failed OTP or PIN checks; a failed SSO login says nothing about either.
		login := func(id, outcome, method string) apigen.RiskDecision {
			e := map[string]any{
				"event_id": id, "type": "login", "occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
				"actor": map[string]any{"user_id": "u_ato", "account_id": "acc_ato", "device_id": "dev_ato"},
				"data":  map[string]any{"outcome": outcome, "method": method},
			}
			return decide(e)
		}
		login("ato-1", "failure", "otp")
		assert.Empty(t, notEvaluated(login("ato-2", "success", "password"), "ATO-06"), "the window is complete")
		login("ato-3", "failure", "sso")
		assert.Contains(t, notEvaluated(login("ato-4", "success", "password"), "ATO-06"), "history:count auth_failure by account")
	})

	t.Run("controls are stored with the event", func(t *testing.T) {
		d := decide(money("store-1", "transfer_outbound", "acc_store", 2_500_000, "NGN", bound))
		got := decode[apigen.EventDetail](t, s.call(t, http.MethodGet, "/api/v1/events/"+*d.EventId, key, nil))
		assert.NotNil(t, controlOf(got.Evaluation, "REG-01"))
		assert.Equal(t, apigen.Decision("decline"), got.Evaluation.Decision)
	})

	t.Run("another tenant's windows are its own", func(t *testing.T) {
		b := s.createTenant(t, "Pack engine B")
		withoutCompliance(t, b.id)
		for i := range 3 {
			r := s.ingest(t, b.testKey, money(fmt.Sprintf("b-%d", i), "atm_withdrawal", "acc_atm", 90_000*100, "NGN", nil))
			require.Equal(t, http.StatusOK, r.status, r.String())
			assert.Empty(t, decode[apigen.RiskDecision](t, r).Controls, "tenant B has no profile")
		}
	})
}

// Behaviour history added for rule pack facts: export size baselines (SEC-12) and account
// activity (MUL-05's dormancy).
func TestPackHistoryFacts(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Pack history")
	admin := s.onboard(t, a, "admin@pack-history.example", authz.RoleCustomerAdmin)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	for rule, body := range map[string]map[string]any{
		"SEC-12": {"mode": "live", "settings": map[string]any{}, "reason": "Watch exports"},
		"MUL-05": {"mode": "live", "settings": map[string]any{"large": 100_000_000}, "reason": "Our large amount"},
	} {
		r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/rules/"+rule+"/config", body)
		require.Equal(t, http.StatusOK, r.status, r.String())
	}

	export := func(i, rows int) apigen.RiskDecision {
		t.Helper()
		e := map[string]any{
			"event_id": fmt.Sprintf("exp-%d", i), "type": "activity", "occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
			"actor":      map[string]any{"user_id": "u_exporter"},
			"compliance": map[string]any{"kind": "data_export", "facts": map[string]any{"export.rows": rows}},
		}
		r := s.ingest(t, a.testKey, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	for i := range 5 {
		assert.Contains(t, notEvaluated(export(i, 100), "SEC-12"), "fact:export.rows_to_user_baseline",
			"export %d: no baseline yet", i)
	}
	d := export(5, 600)
	assert.Empty(t, notEvaluated(d, "SEC-12"))
	assert.True(t, slices.ContainsFunc(d.Signals, func(s apigen.Signal) bool { return s.Code == "SEC-12" }),
		"six times the user's usual export: %+v", d.Signals)

	first := decide(t, s, a.testKey, money("dorm-1", "transfer_outbound", "acc_dorm", 200_000_000, "NGN", nil))
	assert.Contains(t, notEvaluated(first, "MUL-05"), "fact:account.inactive_for", "a first transfer: inactivity unknown")
	second := decide(t, s, a.testKey, money("dorm-2", "transfer_outbound", "acc_dorm", 200_000_000, "NGN", nil))
	assert.NotContains(t, notEvaluated(second, "MUL-05"), "fact:account.inactive_for", "the account was seen a minute ago")
}

func decide(t *testing.T, s stack, key string, e map[string]any) apigen.RiskDecision {
	t.Helper()
	r := s.ingest(t, key, e)
	require.Equal(t, http.StatusOK, r.status, r.String())
	return decode[apigen.RiskDecision](t, r)
}
