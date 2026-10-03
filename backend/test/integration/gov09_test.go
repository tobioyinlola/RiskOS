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
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// GOV-09, no tipping off: a compliance-only rule (INS-01, staff-linked accounts) scores, but
// nothing outside compliance roles names it: not the decision, the stored event, alerts or
// analytics. Only compliance officers read it.
func TestNoTippingOff(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	s.startWorker(t)
	a := s.createTenant(t, "No tipping off")
	admin := s.onboard(t, a, "admin@gov09.example", authz.RoleCustomerAdmin)
	officer := s.onboard(t, a, "officer@gov09.example", authz.RoleComplianceOfficer)
	analyst := s.onboard(t, a, "analyst@gov09.example", authz.RoleAnalyst)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	r = officer.b.do(t, http.MethodPut, "/api/v1/compliance/rules/INS-01/config",
		map[string]any{"mode": "live", "settings": map[string]any{}, "reason": "Insider monitoring"})
	require.Equal(t, http.StatusOK, r.status, r.String())

	leaks := func(where, body string) {
		t.Helper()
		// The rule's ID and name; the event's own payload is the institution's data and stays.
		for _, word := range []string{"INS-01", "Staff-linked account"} {
			assert.NotContains(t, strings.ToLower(body), strings.ToLower(word), "%s names the compliance-only rule", where)
		}
	}
	e := money("ins-1", "transfer_outbound", "acc_staff", 100_000, "NGN",
		map[string]any{"account.linked_to_employee": true, "account.volume_above_peer_p95": true})
	r = s.ingest(t, a.testKey, e)
	require.Equal(t, http.StatusOK, r.status, r.String())
	leaks("the decision", r.String())
	d := decode[apigen.RiskDecision](t, r)
	require.Contains(t, d.Signals, apigen.Signal{Code: "compliance_review", Label: "Compliance review", Weight: 20},
		"the points are kept and explained without naming the rule")
	assert.Equal(t, apigen.Decision("review"), d.Decision, "INS-01 requires at least a review")

	leaks("the stored event", s.call(t, http.MethodGet, "/api/v1/events/"+*d.EventId, a.testKey, nil).String())
	alerts := s.waitForAlerts(t, a.testKey, "kind=risk", 1)
	leaks("the alert", s.call(t, http.MethodGet, "/api/v1/alerts/"+alerts[0].Id, a.testKey, nil).String())
	leaks("analytics", s.call(t, http.MethodGet, "/api/v1/analytics?window=24h", a.testKey, nil).String())

	r = officer.b.do(t, http.MethodGet, "/api/v1/compliance/restricted-signals", nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	list := decode[apigen.RestrictedEvaluationList](t, r).Data
	require.Len(t, list, 1)
	assert.Equal(t, "INS-01", list[0].Signals[0].Rule)
	assert.Equal(t, d.EventId, list[0].EventId)
	for name, b := range map[string]*browser{"an administrator": admin.b, "an analyst": analyst.b} {
		r := b.do(t, http.MethodGet, "/api/v1/compliance/restricted-signals", nil)
		assert.Equal(t, http.StatusForbidden, r.status, "%s: %s", name, r.String())
	}
	r = s.call(t, http.MethodGet, "/api/v1/compliance/restricted-signals", a.testKey, nil)
	assert.Equal(t, http.StatusForbidden, r.status, "API keys never: %s", r.String())

	r = admin.b.do(t, http.MethodPost, "/api/v1/rules", map[string]any{
		"code": "compliance_review", "name": "x", "severity": "low", "weight": 1, "mode": "shadow",
		"condition": map[string]any{"field": "event.type", "op": "eq", "value": "login"},
	})
	assert.Equal(t, http.StatusBadRequest, r.status, "tenant rules cannot use the masked code: %s", r.String())
}
