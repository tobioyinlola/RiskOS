//go:build integration

package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// Activating a country (ADR-0007, phase 4): a draft, a readiness check, shadow, and live only
// with a second person's approval; leaving live and changing the fallback also need one. Every
// change is kept with its rationale.
func TestActivationWorkflow(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Activations")
	admin := s.onboard(t, a, "admin@activations.example", authz.RoleCustomerAdmin)
	officer := s.onboard(t, a, "officer@activations.example", authz.RoleComplianceOfficer)
	analyst := s.onboard(t, a, "analyst@activations.example", authz.RoleAnalyst)

	r := admin.b.do(t, http.MethodPost, "/api/v1/compliance/activations", map[string]any{
		"jurisdiction": "GH", "booking_entity": "gh-emi", "licence_type": "emi", "day_starts_at": 6,
		"rationale": "Ghana e-money subsidiary licensed by the Bank of Ghana",
	})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	act := decode[apigen.JurisdictionActivation](t, r)
	assert.Equal(t, apigen.JurisdictionActivationStatus("draft"), act.Status)
	assert.False(t, act.Fallback)
	assert.Equal(t, 6, *act.DayStartsAt)
	assert.True(t, strings.HasPrefix(act.Pack, "gh@"))
	base := "/api/v1/compliance/activations/" + act.Id

	t.Run("a booking entity is activated once; bad input is refused", func(t *testing.T) {
		body := map[string]any{"jurisdiction": "GH", "booking_entity": "gh-emi", "licence_type": "emi", "rationale": "again"}
		assert.Equal(t, http.StatusConflict, admin.b.do(t, http.MethodPost, "/api/v1/compliance/activations", body).status)
		for name, body := range map[string]map[string]any{
			"no pack":         {"jurisdiction": "KE", "booking_entity": "ke", "licence_type": "bank", "rationale": "x"},
			"unknown licence": {"jurisdiction": "RW", "booking_entity": "rw", "licence_type": "casino", "rationale": "x"},
			"no rationale":    {"jurisdiction": "RW", "booking_entity": "rw", "licence_type": "bank", "rationale": " "},
		} {
			assert.Equal(t, http.StatusBadRequest, admin.b.do(t, http.MethodPost, "/api/v1/compliance/activations", body).status, name)
		}
		assert.Equal(t, http.StatusForbidden, analyst.b.do(t, http.MethodPost, "/api/v1/compliance/activations",
			map[string]any{"jurisdiction": "RW", "booking_entity": "rw", "licence_type": "bank", "rationale": "x"}).status)
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, "/api/v1/compliance/activations", a.testKey,
			map[string]any{"jurisdiction": "RW", "booking_entity": "rw", "licence_type": "bank", "rationale": "x"}).status, "people only")
	})

	readiness := func() apigen.ActivationReadiness {
		t.Helper()
		r := admin.b.do(t, http.MethodGet, base+"/readiness", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.ActivationReadiness](t, r)
	}
	status := func(m member, to, why string) response {
		return m.b.do(t, http.MethodPost, base+"/status", map[string]any{"status": to, "rationale": why})
	}

	t.Run("the readiness check names the settings the institution must enter", func(t *testing.T) {
		rd := readiness()
		assert.False(t, rd.Ready)
		i := slices.IndexFunc(rd.Items, func(it struct {
			Blocking bool                                `json:"blocking"`
			Kind     apigen.ActivationReadinessItemsKind `json:"kind"`
			Message  string                              `json:"message"`
			Rule     *string                             `json:"rule,omitempty"`
		}) bool {
			return it.Rule != nil && *it.Rule == "GH-REG-03" && it.Kind == "setting"
		})
		require.GreaterOrEqual(t, i, 0, "%+v", rd.Items)
		assert.True(t, rd.Items[i].Blocking, "a regulatory control that would not be evaluated blocks going live")
	})

	t.Run("draft to shadow at once; live is refused until ready", func(t *testing.T) {
		r := status(admin, "shadow", "Watch Ghana traffic before enforcing")
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, apigen.JurisdictionActivationStatus("shadow"), decode[apigen.JurisdictionActivation](t, r).Status)
		r = status(admin, "live", "Go live")
		assert.Equal(t, http.StatusConflict, r.status, r.String())
		assert.Contains(t, r.String(), "GH-REG-03")
	})

	// The institution enters the monthly limits the library leaves blank.
	configure(t, a.id, "gh", "GH-REG-03", "live", `{"monthly_medium": 5000000, "monthly_enhanced": 10000000}`)

	var change apigen.ActivationChange
	t.Run("going live needs another person", func(t *testing.T) {
		require.True(t, readiness().Ready, "%+v", readiness().Items)
		r := status(admin, "live", "Shadow results reviewed by the MLRO")
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		change = decode[apigen.ActivationChange](t, r)
		assert.Equal(t, apigen.ActivationChangeState("pending"), change.State)
		assert.Equal(t, http.StatusConflict, status(admin, "live", "again").status, "one pending change at a time")
		r = admin.b.do(t, http.MethodPost, "/api/v1/compliance/activation-changes/"+change.Id+"/approve", nil)
		assert.Equal(t, http.StatusForbidden, r.status, "nobody approves their own change: %s", r.String())
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/activation-changes/"+change.Id+"/approve", map[string]any{"note": "Agreed"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, apigen.ActivationChangeState("approved"), decode[apigen.ActivationChange](t, r).State)

		acts := decode[struct {
			Data []apigen.JurisdictionActivation `json:"data"`
		}](t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/activations", nil)).Data
		i := slices.IndexFunc(acts, func(x apigen.JurisdictionActivation) bool { return x.Id == act.Id })
		require.GreaterOrEqual(t, i, 0)
		got := acts[i]
		assert.Equal(t, apigen.JurisdictionActivationStatus("live"), got.Status)
		require.NotNil(t, got.ActivatedBy)
		require.NotNil(t, got.ApprovedBy)
		assert.Equal(t, change.ProposedBy, *got.ActivatedBy, "credited to the proposer")
		assert.NotEqual(t, *got.ActivatedBy, *got.ApprovedBy, "approved by the other person")
		assert.NotNil(t, got.EffectiveFrom)
	})

	t.Run("leaving live needs another person too", func(t *testing.T) {
		r := status(admin, "suspended", "Licence under review")
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		c := decode[apigen.ActivationChange](t, r)
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/activation-changes/"+c.Id+"/reject", map[string]any{"note": "Keep enforcing"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, http.StatusConflict, officer.b.do(t, http.MethodPost, "/api/v1/compliance/activation-changes/"+c.Id+"/approve", nil).status,
			"a decision is final")
	})

	t.Run("the fallback changes only with a second person", func(t *testing.T) {
		r := admin.b.do(t, http.MethodPost, base+"/fallback", map[string]any{"rationale": "Most customers are now Ghanaian"})
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		c := decode[apigen.ActivationChange](t, r)
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/activation-changes/"+c.Id+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		acts := decode[struct {
			Data []apigen.JurisdictionActivation `json:"data"`
		}](t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/activations", nil)).Data
		assert.Equal(t, act.Id, acts[0].Id, "the fallback is listed first")
		assert.Equal(t, 1, len(slices.DeleteFunc(slices.Clone(acts), func(x apigen.JurisdictionActivation) bool { return !x.Fallback })))
		// An event naming no booking entity is now judged in Ghana.
		ev := s.ingest(t, a.testKey, booked("fallback-gh", "transfer_outbound", "", 100_000, "GHS", nil))
		require.Equal(t, http.StatusOK, ev.status, ev.String())
		d := decode[apigen.RiskDecision](t, ev)
		require.NotNil(t, d.Jurisdiction)
		assert.Equal(t, "GH", *d.Jurisdiction.Jurisdiction)
	})

	t.Run("every change is kept with its rationale", func(t *testing.T) {
		r := admin.b.do(t, http.MethodGet, "/api/v1/compliance/activation-changes?activation_id="+act.Id, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		list := decode[struct {
			Data []apigen.ActivationChange `json:"data"`
		}](t, r).Data
		var got []string
		for _, c := range list {
			got = append(got, string(c.Action)+":"+string(c.State))
			assert.NotEmpty(t, c.Rationale)
		}
		assert.Equal(t, []string{"fallback:approved", "status:rejected", "status:approved", "status:applied", "create:applied"}, got)
		assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)), "jurisdiction_activation.change_approved")
	})

	t.Run("another workspace sees none of it", func(t *testing.T) {
		b := s.createTenant(t, "Other")
		r := s.call(t, http.MethodGet, "/api/v1/compliance/activation-changes", b.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Empty(t, decode[struct {
			Data []apigen.ActivationChange `json:"data"`
		}](t, r).Data)
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, base+"/readiness", b.testKey, nil).status)
	})
}

// A country's rules are configured on their own pages (an inherited Nigerian rule is configured
// apart in each country), and a rule off until confirmed is switched on only with its
// placeholder settings entered.
func TestCountryRuleConfiguration(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Country config")
	admin := s.onboard(t, a, "admin@country-config.example", authz.RoleCustomerAdmin)
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", nil)
	put := func(path string, body any) response {
		return admin.b.do(t, http.MethodPut, "/api/v1/compliance/rules/"+path, body)
	}

	r := put("ATO-04/config?jurisdiction=GH", map[string]any{"mode": "shadow", "settings": map[string]any{}, "reason": "Calibrating in Ghana"})
	require.Equal(t, http.StatusOK, r.status, r.String())
	gh := decode[apigen.PackRuleConfig](t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/ATO-04/config?jurisdiction=GH", nil))
	ng := decode[apigen.PackRuleConfig](t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/ATO-04/config", nil))
	assert.Equal(t, apigen.PackRuleMode("shadow"), gh.Mode)
	assert.NotEqual(t, gh.Mode, ng.Mode, "Nigeria's ATO-04 is untouched")
	hist := admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/ATO-04/history?jurisdiction=GH", nil)
	require.Equal(t, http.StatusOK, hist.status, hist.String())

	r = put("GH-TM-23/config", map[string]any{"mode": "live", "settings": map[string]any{}, "reason": "Turn it on"})
	assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	assert.Contains(t, r.String(), "wrong_transfer_min")
	r = put("GH-TM-23/config", map[string]any{"mode": "shadow", "settings": map[string]any{"wrong_transfer_min": 100_000}, "reason": "GHS 1,000 per our MLRO"})
	assert.Equal(t, http.StatusOK, r.status, r.String())
}

// An open question checked against the original text is recorded once, with who, when and a
// note, and shows on every rule it names.
func TestOpenQuestionVerification(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Verification")
	admin := s.onboard(t, a, "admin@verification.example", authz.RoleCustomerAdmin)
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", nil)
	body := map[string]any{"note": "Checked against the BoG directive of 1 March 2024", "link": "https://www.bog.gov.gh/"}

	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, "/api/v1/compliance/gaps/C12/verify?jurisdiction=GH", a.testKey, body).status)
	r := admin.b.do(t, http.MethodPost, "/api/v1/compliance/gaps/C12/verify?jurisdiction=GH", body)
	require.Equal(t, http.StatusCreated, r.status, r.String())
	assert.Equal(t, http.StatusConflict, admin.b.do(t, http.MethodPost, "/api/v1/compliance/gaps/C12/verify?jurisdiction=GH", body).status)
	assert.Equal(t, http.StatusNotFound, admin.b.do(t, http.MethodPost, "/api/v1/compliance/gaps/C99/verify?jurisdiction=GH", body).status)
	assert.Equal(t, http.StatusBadRequest, admin.b.do(t, http.MethodPost, "/api/v1/compliance/gaps/C1/verify?jurisdiction=GH",
		map[string]any{"note": "x", "link": "javascript:alert(1)"}).status)

	gaps := decode[apigen.ComplianceGapList](t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/gaps?jurisdiction=GH", nil)).Data
	i := slices.IndexFunc(gaps, func(g apigen.ComplianceGap) bool { return g.Id == "C12" })
	require.GreaterOrEqual(t, i, 0)
	require.NotNil(t, gaps[i].Verification)
	assert.NotEmpty(t, gaps[i].Verification.VerifiedBy)
	for _, id := range []string{"GH-REG-01", "GH-REG-03"} {
		rule := ruleOf(t, admin.b.do(t, http.MethodGet, "/api/v1/compliance/rules/"+id+"?jurisdiction=GH", nil))
		require.NotNil(t, rule.Verifications, id)
		assert.Equal(t, "C12", (*rule.Verifications)[0].Gap)
	}
}
