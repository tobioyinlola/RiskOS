//go:build integration

package integration

import (
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
