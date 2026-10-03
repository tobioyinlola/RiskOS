//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

func ruleOf(t *testing.T, r response) apigen.ComplianceRule {
	t.Helper()
	require.Equal(t, http.StatusOK, r.status, r.String())
	return decode[apigen.ComplianceRule](t, r)
}

func paramValue(rule apigen.ComplianceRule, name string) *float64 {
	for _, p := range rule.Parameters {
		if p.Name == name {
			return p.Value
		}
	}
	return nil
}

// Institutions configure pack rules per environment within the pack's bounds; every change is a
// version, audited, and in production a change that could loosen a regulatory control needs a
// second person (ADR-0006).
func TestPackRuleConfiguration(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Pack config")
	admin := s.onboard(t, a, "admin@pack-config.example", authz.RoleCustomerAdmin)
	officer := s.onboard(t, a, "officer@pack-config.example", authz.RoleComplianceOfficer)
	analyst := s.onboard(t, a, "analyst@pack-config.example", authz.RoleAnalyst)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{"instant_payments"}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	put := func(m member, rule string, body map[string]any) response {
		t.Helper()
		return m.b.do(t, http.MethodPut, "/api/v1/compliance/rules/"+rule+"/config", body)
	}

	t.Run("test environment: applied at once, versioned and audited", func(t *testing.T) {
		before := ruleOf(t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/ATO-01", nil))
		assert.Equal(t, apigen.PackRuleMode("shadow"), before.Mode)
		assert.Zero(t, before.ConfigVersion)
		assert.Nil(t, paramValue(before, "floor"), "the floor is the institution's to set")

		r := put(admin, "ATO-01", map[string]any{"mode": "live", "settings": map[string]any{"floor": 10_000_000}, "reason": "Our ATO floor"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		cfg := decode[apigen.PackRuleConfig](t, r)
		assert.Equal(t, 1, cfg.Version)
		after := ruleOf(t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/ATO-01", nil))
		assert.Equal(t, apigen.PackRuleMode("live"), after.Mode)
		require.NotNil(t, paramValue(after, "floor"))
		assert.InDelta(t, 10_000_000, *paramValue(after, "floor"), 0)

		r = put(admin, "ATO-01", map[string]any{"mode": "live", "settings": map[string]any{"floor": 10_000_000}, "reason": "again"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, 1, decode[apigen.PackRuleConfig](t, r).Version, "no change, no version")

		require.Equal(t, http.StatusOK, put(admin, "ATO-01", map[string]any{"mode": "shadow", "settings": map[string]any{}, "reason": "back"}).status)
		r = admin.b.do(t, http.MethodPost, "/api/v1/compliance/rules/ATO-01/rollback", map[string]any{"version": 1, "reason": "restore"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		restored := decode[apigen.PackRuleConfig](t, r)
		assert.Equal(t, 3, restored.Version)
		history := decode[apigen.PackRuleVersionList](t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/ATO-01/history", nil)).Data
		require.Len(t, history, 3)
		require.NotNil(t, history[0].RestoredFrom)
		assert.Equal(t, 1, *history[0].RestoredFrom)
		assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)), "pack_rule.configured")
	})

	t.Run("settings stay within the pack's bounds", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"above the legal maximum":   {"mode": "live", "settings": map[string]any{"new_device_cap": 3_000_000}, "reason": "x"},
			"not a setting of the rule": {"mode": "live", "settings": map[string]any{"floor": 1}, "reason": "x"},
			"a fraction of a kobo":      {"mode": "live", "settings": map[string]any{"new_device_cap": 1.5}, "reason": "x"},
			"no reason":                 {"mode": "live", "settings": map[string]any{}, "reason": " "},
		} {
			r := put(admin, "REG-01", body)
			assert.Equal(t, http.StatusBadRequest, r.status, "%s: %s", name, r.String())
		}
	})

	t.Run("only people with compliance:configure", func(t *testing.T) {
		body := map[string]any{"mode": "off", "settings": map[string]any{}, "reason": "x"}
		assert.Equal(t, http.StatusForbidden, put(analyst, "VEL-01", body).status)
		r := s.call(t, http.MethodPut, "/api/v1/compliance/rules/VEL-01/config", a.testKey, body)
		assert.Equal(t, http.StatusForbidden, r.status, "API keys never configure: %s", r.String())
	})

	// Production: the officer and the admin act as two people.
	admin.b.env, officer.b.env = "production", "production"
	prodKey := s.productionKey(t, a.id)

	t.Run("production: a regulatory control's settings need a second person", func(t *testing.T) {
		r := put(admin, "REG-01", map[string]any{"mode": "live", "settings": map[string]any{"new_device_cap": 1_000_000}, "reason": "Stricter cap"})
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		change := decode[apigen.PackRuleChange](t, r)
		assert.Equal(t, apigen.PackRuleChangeStatus("pending"), change.Status)
		assert.Equal(t, 0, ruleOf(t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/REG-01", nil)).ConfigVersion, "not applied yet")

		r = put(admin, "REG-01", map[string]any{"mode": "shadow", "settings": map[string]any{}, "reason": "x"})
		assert.Equal(t, http.StatusConflict, r.status, "one pending change per rule: %s", r.String())

		r = admin.b.do(t, http.MethodPost, "/api/v1/compliance/changes/"+change.Id+"/approve", nil)
		assert.Equal(t, http.StatusForbidden, r.status, "nobody approves their own change: %s", r.String())

		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/changes/"+change.Id+"/approve", map[string]any{"note": "Agreed"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		approved := decode[apigen.PackRuleChange](t, r)
		require.NotNil(t, approved.AppliedVersion)
		assert.Equal(t, 1, *approved.AppliedVersion)
		history := decode[apigen.PackRuleVersionList](t, officer.b.do(t, http.MethodGet, "/api/v1/compliance/rules/REG-01/history", nil)).Data
		require.Len(t, history, 1)
		assert.Equal(t, approved.ProposedBy, history[0].CreatedBy, "credited to the proposer")
		assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", prodKey, nil)), "pack_rule.change_approved")

		// The engine runs with the approved ₦10,000 cap.
		d := decode[apigen.RiskDecision](t, s.ingest(t, prodKey, money("prod-r1", "transfer_outbound", "acc_prod", 1_500_000, "NGN",
			map[string]any{"device.bound_age": 3600, "account.age": 400 * 86400})))
		assert.Equal(t, apigen.Decision("decline"), d.Decision, "%+v", d.Controls)
	})

	t.Run("production: switching a control off needs approval, switching it on does not", func(t *testing.T) {
		r := put(admin, "REG-12", map[string]any{"mode": "off", "settings": map[string]any{}, "reason": "PEP screening elsewhere"})
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		pending := decode[apigen.PackRuleChange](t, r)
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/changes/"+pending.Id+"/reject", map[string]any{"note": "Not without a replacement"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, apigen.PackRuleChangeStatus("rejected"), decode[apigen.PackRuleChange](t, r).Status)

		r = put(admin, "VEL-01", map[string]any{"mode": "live", "settings": map[string]any{"transfers": 4}, "reason": "Live in production"})
		assert.Equal(t, http.StatusOK, r.status, "risk-scoring rules apply at once: %s", r.String())
	})

	t.Run("approving a stale change is refused", func(t *testing.T) {
		propose := func(m member, body map[string]any) apigen.PackRuleChange {
			t.Helper()
			r := put(m, "REG-17", body)
			require.Equal(t, http.StatusAccepted, r.status, r.String())
			return decode[apigen.PackRuleChange](t, r)
		}
		approve := func(m member, id string) response {
			return m.b.do(t, http.MethodPost, "/api/v1/compliance/changes/"+id+"/approve", nil)
		}
		off := propose(admin, map[string]any{"mode": "off", "settings": map[string]any{}, "reason": "CTRs filed elsewhere for now"})
		require.Equal(t, http.StatusOK, approve(officer, off.Id).status)
		lower := propose(admin, map[string]any{"mode": "off", "settings": map[string]any{"ctr_individual": 400_000_000}, "reason": "Lower CTR threshold"})
		assert.Equal(t, 1, lower.BaseVersion)
		// Meanwhile the rule moves: switching it back on tightens it, so it applies at once.
		r := put(officer, "REG-17", map[string]any{"mode": "live", "settings": map[string]any{}, "reason": "Back on"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, 2, decode[apigen.PackRuleConfig](t, r).Version)
		r = approve(officer, lower.Id)
		assert.Equal(t, http.StatusConflict, r.status, "proposed against version 1, the rule is at 2: %s", r.String())
		r = admin.b.do(t, http.MethodPost, "/api/v1/compliance/changes/"+lower.Id+"/reject", map[string]any{"note": "Withdrawn"})
		require.Equal(t, http.StatusOK, r.status, "the proposer withdraws it: %s", r.String())
	})

	t.Run("changes are listed and environments are separate", func(t *testing.T) {
		pending := decode[apigen.PackRuleChangeList](t, officer.b.do(t, http.MethodGet, "/api/v1/compliance/changes?status=pending", nil)).Data
		assert.Empty(t, pending)
		all := decode[apigen.PackRuleChangeList](t, officer.b.do(t, http.MethodGet, "/api/v1/compliance/changes", nil)).Data
		assert.Len(t, all, 4, "REG-01, REG-12 and REG-17 twice")
		test := decode[apigen.PackRuleChangeList](t, s.call(t, http.MethodGet, "/api/v1/compliance/changes", a.testKey, nil)).Data
		assert.Empty(t, test, "the test environment has no proposals")
		assert.Equal(t, 0, ruleOf(t, s.call(t, http.MethodGet, "/api/v1/compliance/rules/REG-01", a.testKey, nil)).ConfigVersion,
			"production's REG-01 change does not touch the test environment")
	})
}

// A backtest replays recent events through a rule with candidate settings and with the current
// ones, windows rebuilt in memory, without touching live state.
func TestPackRuleBacktest(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Backtest")
	admin := s.onboard(t, a, "admin@backtest.example", authz.RoleCustomerAdmin)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	// ATM withdrawals of ₦60,000, ₦30,000 and ₦20,000 by one customer in a day.
	for i, naira := range []int64{60_000, 30_000, 20_000} {
		r := s.ingest(t, a.testKey, money(fmt.Sprintf("bt-%d", i), "atm_withdrawal", "acc_bt", naira*100, "NGN", nil))
		require.Equal(t, http.StatusOK, r.status, r.String())
	}
	backtest := func(body map[string]any) response {
		return s.call(t, http.MethodPost, "/api/v1/compliance/rules/REG-09/backtest", a.testKey, body)
	}
	r = backtest(map[string]any{"days": 1, "settings": map[string]any{"atm_daily": 8_000_000}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	bt := decode[apigen.Backtest](t, r)
	assert.Equal(t, 3, bt.Replayed)
	assert.Equal(t, 3, bt.Current.Triggered)
	assert.Equal(t, 1, bt.Current.Fired, "the ₦100,000 limit is passed once, at ₦110,000")
	assert.Equal(t, 2, bt.Candidate.Fired, "an ₦80,000 limit is passed at ₦90,000 and ₦110,000")
	assert.Len(t, bt.Candidate.FiredSamples, 2)

	for name, tc := range map[string]struct {
		rule   string
		body   map[string]any
		status int
	}{
		"above the legal maximum": {"REG-09", map[string]any{"settings": map[string]any{"atm_daily": 20_000_000}}, http.StatusBadRequest},
		"too long a period":       {"REG-09", map[string]any{"days": 31}, http.StatusBadRequest},
		"a governance preset":     {"GOV-01", map[string]any{}, http.StatusBadRequest},
		"no such rule":            {"ZZZ-99", map[string]any{}, http.StatusNotFound},
	} {
		r := s.call(t, http.MethodPost, "/api/v1/compliance/rules/"+tc.rule+"/backtest", a.testKey, tc.body)
		assert.Equal(t, tc.status, r.status, "%s: %s", name, r.String())
		if tc.rule == "GOV-01" {
			assert.Contains(t, r.String(), "is a workflow", name)
		}
	}
	_, compliance := s.createKey(t, a.testKey, "compliance only", authz.ComplianceRead)
	r = s.call(t, http.MethodPost, "/api/v1/compliance/rules/REG-09/backtest", compliance, map[string]any{})
	assert.Equal(t, http.StatusForbidden, r.status, "backtests read events: %s", r.String())

	// The backtest left the live window alone: one more ₦1 withdrawal still sees ₦110,001.
	d := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, money("bt-live", "atm_withdrawal", "acc_bt", 100, "NGN", nil)))
	assert.Equal(t, apigen.Decision("decline"), d.Decision)
}

// Institutions set exchange rates so rules on US dollar amounts (REG-10) can convert; an amount
// the event sends wins, and without a rate the rule is not evaluated.
func TestComplianceFXRates(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "FX")
	admin := s.onboard(t, a, "admin@fx.example", authz.RoleCustomerAdmin)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())

	r = admin.b.do(t, http.MethodPut, "/api/v1/compliance/fx-rates/NGN", map[string]any{"usd_per_unit": 0.00065})
	require.Equal(t, http.StatusOK, r.status, r.String())
	for name, path := range map[string]string{"dollars": "/api/v1/compliance/fx-rates/USD", "not a currency": "/api/v1/compliance/fx-rates/XYZ"} {
		r := admin.b.do(t, http.MethodPut, path, map[string]any{"usd_per_unit": 1})
		assert.Equal(t, http.StatusBadRequest, r.status, "%s: %s", name, r.String())
	}
	r = s.call(t, http.MethodPut, "/api/v1/compliance/fx-rates/GHS", a.testKey, map[string]any{"usd_per_unit": 0.08})
	assert.Equal(t, http.StatusForbidden, r.status, "people only: %s", r.String())
	rates := decode[apigen.FXRateList](t, s.call(t, http.MethodGet, "/api/v1/compliance/fx-rates", a.testKey, nil)).Data
	require.Len(t, rates, 1)
	assert.InDelta(t, 0.00065, rates[0].UsdPerUnit, 1e-12)
	assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)), "compliance.fx_rate_set")

	abroad := func(id string, naira int64, facts map[string]any) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, a.testKey, money(id, "transfer_international", "acc_fx", naira*100, "NGN", facts))
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	c := controlOf(abroad("fx-1", 16_000_000, nil), "REG-10")
	require.NotNil(t, c, "₦16,000,000 is $10,400: a foreign transaction report is due")
	assert.Equal(t, []string{"report.FTR"}, c.Tasks)
	assert.Nil(t, controlOf(abroad("fx-2", 15_000_000, nil), "REG-10"), "₦15,000,000 is $9,750")
	sent := abroad("fx-3", 15_000_000, map[string]any{"txn.amount_usd": map[string]any{"amount": 1_000_000, "currency": "USD"}})
	assert.NotNil(t, controlOf(sent, "REG-10"), "the amount the event sends wins")

	r = admin.b.do(t, http.MethodDelete, "/api/v1/compliance/fx-rates/NGN", nil)
	require.Equal(t, http.StatusNoContent, r.status, r.String())
	assert.Equal(t, http.StatusNotFound, admin.b.do(t, http.MethodDelete, "/api/v1/compliance/fx-rates/NGN", nil).status)
}
