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

func ruleBody(code string, weight int) map[string]any {
	return map[string]any{
		"code": code, "name": "Large transfer", "severity": "high", "weight": weight, "mode": "shadow",
		"condition": map[string]any{"field": "transaction.amount", "op": "gte", "value": 1000000},
	}
}

// TestRuleChangeApproval covers the two-person rule for production rule changes.
func TestRuleChangeApproval(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Four eyes")
	admin := s.onboard(t, a, "admin@foureyes.example", authz.RoleCustomerAdmin)
	rm := s.onboard(t, a, "rm@foureyes.example", authz.RoleRiskManager)
	admin.b.env, rm.b.env = "production", "production"

	r := admin.b.do(t, http.MethodPost, "/api/v1/api-keys", map[string]any{"name": "rules-ci", "scopes": []string{"rules:read", "rules:write"}})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	prodKey := decode[apigen.ApiKeyWithSecret](t, r).Secret

	t.Run("test environments apply changes at once", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, ruleBody("large_transfer", 20))
		assert.Equal(t, http.StatusCreated, r.status, r.String())
	})

	t.Run("API keys can never hold the approval permission", func(t *testing.T) {
		r := admin.b.do(t, http.MethodPost, "/api/v1/api-keys", map[string]any{"name": "approver", "scopes": []string{"rules:approve"}})
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	})

	var ruleID string
	t.Run("production changes wait for a second person", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/rules", prodKey, ruleBody("large_transfer", 20))
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		c := decode[apigen.RuleChange](t, r)
		assert.Equal(t, apigen.RuleChangeStatus("pending"), c.Status)
		assert.Equal(t, apigen.RuleChangeKind("create"), c.Kind)
		assert.Nil(t, c.BaseVersion)
		assert.Empty(t, decode[apigen.RuleList](t, s.call(t, http.MethodGet, "/api/v1/rules", prodKey, nil)).Data, "nothing applied yet")
		assert.Equal(t, http.StatusConflict, s.call(t, http.MethodPost, "/api/v1/rules", prodKey, ruleBody("large_transfer", 30)).status,
			"one pending proposal per code")

		r = admin.b.do(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		approved := decode[apigen.RuleChange](t, r)
		assert.Equal(t, apigen.RuleChangeStatus("approved"), approved.Status)
		require.NotNil(t, approved.AppliedVersion)
		assert.Equal(t, 1, *approved.AppliedVersion)
		rules := decode[apigen.RuleList](t, s.call(t, http.MethodGet, "/api/v1/rules", prodKey, nil)).Data
		require.Len(t, rules, 1)
		assert.Equal(t, c.RuleId, rules[0].Id, "the rule gets the ID the proposal announced")
		ruleID = rules[0].Id

		assert.Equal(t, http.StatusConflict, admin.b.do(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/approve", nil).status,
			"a decision is final")
	})

	t.Run("nobody approves their own change", func(t *testing.T) {
		r := rm.b.do(t, http.MethodPatch, "/api/v1/rules/"+ruleID, map[string]any{"weight": 35})
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		c := decode[apigen.RuleChange](t, r)
		require.NotNil(t, c.BaseVersion)
		assert.Equal(t, 1, *c.BaseVersion)

		rule := decode[apigen.Rule](t, rm.b.do(t, http.MethodGet, "/api/v1/rules/"+ruleID, nil))
		assert.Equal(t, 20, rule.Weight, "still the approved content")
		require.NotNil(t, rule.PendingChangeId)
		assert.Equal(t, c.Id, *rule.PendingChangeId)
		assert.Equal(t, http.StatusConflict, admin.b.do(t, http.MethodPatch, "/api/v1/rules/"+ruleID, map[string]any{"weight": 40}).status,
			"one pending change per rule")

		r = rm.b.do(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/approve", nil)
		assert.Equal(t, http.StatusForbidden, r.status, r.String())
		r = admin.b.do(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		rule = decode[apigen.Rule](t, admin.b.do(t, http.MethodGet, "/api/v1/rules/"+ruleID, nil))
		assert.Equal(t, 35, rule.Weight)
		assert.Equal(t, 2, rule.Version)
		assert.Nil(t, rule.PendingChangeId)
	})

	t.Run("API keys cannot approve", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/rules/"+ruleID+"/rollback", prodKey, map[string]any{"version": 1})
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		c := decode[apigen.RuleChange](t, r)
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/approve", prodKey, nil).status)

		r = rm.b.do(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/reject", map[string]any{"reason": "keep the higher weight"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		rejected := decode[apigen.RuleChange](t, r)
		assert.Equal(t, apigen.RuleChangeStatus("rejected"), rejected.Status)
		require.NotNil(t, rejected.Reason)
		assert.Equal(t, "keep the higher weight", *rejected.Reason)
		assert.Equal(t, 2, decode[apigen.Rule](t, s.call(t, http.MethodGet, "/api/v1/rules/"+ruleID, prodKey, nil)).Version,
			"a rejected change changes nothing")
	})

	t.Run("the proposer can withdraw", func(t *testing.T) {
		r := rm.b.do(t, http.MethodPatch, "/api/v1/rules/"+ruleID, map[string]any{"enabled": false})
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		c := decode[apigen.RuleChange](t, r)
		assert.Equal(t, http.StatusOK, rm.b.do(t, http.MethodPost, "/api/v1/rule-changes/"+c.Id+"/reject", nil).status)

		all := decode[apigen.RuleChangeList](t, admin.b.do(t, http.MethodGet, "/api/v1/rule-changes?status=all", nil)).Data
		assert.Len(t, all, 4)
		assert.Empty(t, decode[apigen.RuleChangeList](t, admin.b.do(t, http.MethodGet, "/api/v1/rule-changes", nil)).Data,
			"nothing pending")
	})

	t.Run("every step is audited", func(t *testing.T) {
		actions := auditActions(t, admin.b.do(t, http.MethodGet, "/api/v1/audit-logs?limit=100", nil))
		for _, a := range []string{"rule.change_proposed", "rule.change_approved", "rule.change_rejected", "rule.created", "rule.updated"} {
			assert.Contains(t, actions, a)
		}
	})
}
