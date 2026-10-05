//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// What the rules page shows about the workspace's own rules: how often each matched, per day,
// and what a condition (saved or not) would have matched among recent events. And the rule packs
// of every loaded jurisdiction, to browse.
func TestRuleInsights(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Rule insights")
	withoutCompliance(t, a.id)
	u := actor{"u_ins", "acc_ins", "dev_ins"}

	r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, largeTransferRule())
	require.Equal(t, http.StatusCreated, r.status, r.String())
	active := decode[apigen.Rule](t, r)
	shadow := largeTransferRule()
	shadow["code"], shadow["name"], shadow["mode"] = "watch_ngn_transfer", "Watch large transfers", "shadow"
	r = s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, shadow)
	require.Equal(t, http.StatusCreated, r.status, r.String())
	watched := decode[apigen.Rule](t, r)

	s.decide(t, a.testKey, debit(u, "ben_a", "NG", "NGN", 2_000_000))
	s.decide(t, a.testKey, debit(u, "ben_b", "NG", "NGN", 3_000_000))
	s.decide(t, a.testKey, debit(u, "ben_c", "NG", "NGN", 1_000))

	t.Run("matches per rule and per day", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/rules/stats?days=7", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		st := decode[apigen.RuleStats](t, r)
		assert.Equal(t, 7, st.Days)
		assert.EqualValues(t, 3, st.Evaluations)
		require.Len(t, st.Rules, 2)
		for _, rs := range st.Rules {
			assert.EqualValues(t, 2, rs.Matches, "%s: active and monitoring rules alike", rs.RuleId)
			assert.NotNil(t, rs.LastMatchedAt)
			require.Len(t, rs.Daily, 7, "one entry per day")
			assert.EqualValues(t, 2, rs.Daily[6].Matches, "today, the last day")
			assert.Contains(t, []string{active.Id, watched.Id}, rs.RuleId)
		}
		r = s.call(t, http.MethodGet, "/api/v1/rules/stats?days=31", a.testKey, nil)
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	})

	t.Run("a condition tried on recent events", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/rules/test-recent", a.testKey, map[string]any{
			"condition": map[string]any{"all": []any{
				map[string]any{"field": "transaction.amount", "op": "gt", "value": 2_500_000},
			}},
		})
		require.Equal(t, http.StatusOK, r.status, r.String())
		res := decode[apigen.RecentRuleTest](t, r)
		assert.Equal(t, 3, res.Checked)
		assert.Equal(t, 1, res.Matched)
		assert.False(t, res.Truncated)
		require.Len(t, res.Sample, 1)
		assert.Equal(t, "transaction", res.Sample[0].EventType)
		require.NotNil(t, res.Sample[0].Amount)
		assert.EqualValues(t, 3_000_000, *res.Sample[0].Amount)
		assert.NotEmpty(t, res.Sample[0].EventId)

		r = s.call(t, http.MethodPost, "/api/v1/rules/test-recent", a.testKey, map[string]any{
			"condition": map[string]any{"field": "nope", "op": "eq", "value": 1},
		})
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
		assert.Contains(t, r.String(), "unknown field")
	})

	t.Run("the packs of every jurisdiction, to browse", func(t *testing.T) {
		b := s.createTenant(t, "Pack browser")
		r := s.call(t, http.MethodGet, "/api/v1/compliance/packs", b.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		list := decode[struct {
			Data []apigen.CompliancePackSummary `json:"data"`
		}](t, r)
		byCode := map[string]apigen.CompliancePackSummary{}
		for _, p := range list.Data {
			byCode[p.Jurisdiction] = p
		}
		require.Len(t, byCode, 3, "Nigeria, Ghana and Rwanda (ADR-0007)")
		ng := byCode["NG"]
		assert.True(t, ng.Current, "the workspace's own pack")
		assert.Equal(t, 19, ng.Controls)
		assert.Equal(t, 52, ng.Signals)
		assert.Equal(t, 12, ng.Governance)
		assert.False(t, byCode["GH"].Current)
		require.NotNil(t, byCode["GH"].Activated)
		assert.False(t, *byCode["GH"].Activated, "browsing is not activating")

		r = s.call(t, http.MethodGet, "/api/v1/compliance/rules?jurisdiction=NG", b.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Len(t, decode[apigen.ComplianceRuleList](t, r).Data, 83)
		r = s.call(t, http.MethodGet, "/api/v1/compliance/rules?jurisdiction=GH", b.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		gh := decode[apigen.ComplianceRuleList](t, r).Data
		own := 0
		for _, rule := range gh {
			assert.Nil(t, rule.Applicable, "%s: browsing without an activation says nothing about applicability", rule.Id)
			if strings.HasPrefix(rule.Id, "GH-") {
				own++
				assert.NotNil(t, rule.Provenance, rule.Id)
			} else {
				assert.Equal(t, "ng", *rule.InheritedFrom, rule.Id)
			}
		}
		assert.Equal(t, 68, own, "the Ghana library's rules")
		r = s.call(t, http.MethodGet, "/api/v1/compliance/rules?jurisdiction=KE", b.testKey, nil)
		assert.Equal(t, http.StatusNotFound, r.status, "no Kenya pack is loaded: %s", r.String())
		r = s.call(t, http.MethodGet, "/api/v1/compliance/pack?jurisdiction=KE", b.testKey, nil)
		assert.Equal(t, http.StatusNotFound, r.status, r.String())
	})
}
