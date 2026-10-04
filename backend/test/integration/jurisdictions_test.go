//go:build integration

package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
)

// activate adds a live activation for a booking entity. The activation wizard (with its
// readiness check and second approver) arrives in phase 4; the tests set the row directly.
func activate(t *testing.T, tenantID uuid.UUID, jurisdiction, packID, entity, licence string, capabilities []string) {
	t.Helper()
	if capabilities == nil {
		capabilities = []string{}
	}
	_, err := shared.owner.Exec(t.Context(), `
		INSERT INTO jurisdiction_activations (id, tenant_id, jurisdiction, pack_id, pack_version, licence_type, capabilities,
		  booking_entity, status, fallback, signal_default, updated_at, updated_by, activated_at, activated_by)
		SELECT gen_random_uuid(), $1, $2, p.id, p.version, $5, $6, $4, 'live', false, 'pack', now(), 'test', now(), 'test'
		FROM rule_packs p WHERE p.id = $3 ORDER BY string_to_array(p.version, '.')::int[] DESC LIMIT 1`,
		tenantID, jurisdiction, packID, entity, licence, capabilities)
	require.NoError(t, err)
}

// configure sets a pack rule's mode and settings in every environment of the tenant.
func configure(t *testing.T, tenantID uuid.UUID, packID, rule, mode, settings string) {
	t.Helper()
	_, err := shared.owner.Exec(t.Context(), `
		INSERT INTO pack_rule_versions (tenant_id, environment_id, pack_id, rule_id, version, mode, settings, reason, created_at, created_by)
		SELECT tenant_id, id, $2, $3, 1, $4, $5::jsonb, 'test', now(), 'test' FROM environments WHERE tenant_id = $1`,
		tenantID, packID, rule, mode, settings)
	require.NoError(t, err)
}

// booked is a money event booked by an entity, in a currency, with compliance facts.
func booked(id, kind, entity string, amount int64, currency string, facts map[string]any) map[string]any {
	e := money(id, kind, "acc_"+id, amount, currency, facts)
	c, _ := e["compliance"].(map[string]any) // built by money
	if entity != "" {
		c["booking_entity"] = entity
	}
	return e
}

func ruleIDs(d apigen.RiskDecision) []string {
	var out []string
	for _, c := range d.Controls {
		out = append(out, c.Rule)
	}
	for _, s := range d.Signals {
		out = append(out, s.Code)
	}
	for _, n := range d.NotEvaluated {
		out = append(out, n.Rule)
	}
	return out
}

// Each event is judged by the pack of the jurisdiction that governs it (ADR-0007): the booking
// entity's, else the stated fallback, else none, never a guess.
func TestJurisdictionResolution(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Multi-country")
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", nil)
	activate(t, a.id, "RW", "rw", "rw-emi", "emi", nil)
	// An inherited Nigerian rule, live in Ghana with a cedi amount (GHS 1,000).
	configure(t, a.id, "gh", "SEC-05", "live", `{"high_value": 100000}`)
	decide := func(e map[string]any) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, a.testKey, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	domestic := map[string]any{"txn.domestic": true}

	t.Run("a cedi cash deposit booked in Ghana is judged by the Ghana pack", func(t *testing.T) {
		d := decide(booked("gh-ctr-1", "cash_deposit", "gh-bank", 5_000_000, "GHS", domestic))
		require.NotNil(t, d.Jurisdiction)
		assert.Equal(t, apigen.JurisdictionResolutionResolvedBy("booking_entity"), d.Jurisdiction.ResolvedBy)
		assert.Equal(t, "GH", *d.Jurisdiction.Jurisdiction)
		assert.True(t, strings.HasPrefix(*d.Jurisdiction.Pack, "gh@"))
		c := controlOf(d, "GH-REG-04")
		require.NotNil(t, c, "GHS 50,000 at a bank: %v", ruleIDs(d))
		assert.Equal(t, []string{"report.CTR"}, c.Tasks)
		for _, id := range ruleIDs(d) {
			assert.False(t, strings.HasPrefix(id, "RW-"), "%s is Rwanda's", id)
			assert.NotContains(t, []string{"REG-17", "REG-07"}, id, "Nigeria's controls do not judge a Ghana-booked event")
		}
	})
	t.Run("just under the cedi CTR line no report is due", func(t *testing.T) {
		d := decide(booked("gh-ctr-2", "cash_deposit", "gh-bank", 4_999_999, "GHS", domestic))
		assert.Nil(t, controlOf(d, "GH-REG-04"))
	})
	t.Run("a franc eKash transfer booked in Rwanda is judged by the Rwanda pack", func(t *testing.T) {
		d := decide(booked("rw-ek-1", "transfer_outbound", "rw-emi", 10_000_001, "RWF", map[string]any{"txn.rail": "ekash"}))
		require.NotNil(t, d.Jurisdiction)
		assert.Equal(t, "RW", *d.Jurisdiction.Jurisdiction)
		c := controlOf(d, "RW-REG-07")
		require.NotNil(t, c, "%v", ruleIDs(d))
		assert.Equal(t, apigen.Decision("decline"), d.Decision)
		for _, id := range ruleIDs(d) {
			assert.False(t, strings.HasPrefix(id, "GH-"), "%s is Ghana's", id)
		}
		d = decide(booked("rw-ek-2", "transfer_outbound", "rw-emi", 10_000_000, "RWF", map[string]any{"txn.rail": "ekash"}))
		assert.Nil(t, controlOf(d, "RW-REG-07"), "exactly RWF 10 million is allowed")
	})
	t.Run("an inherited Nigerian rule fires under Ghana with cedi amounts", func(t *testing.T) {
		d := decide(booked("gh-sec05", "transfer_outbound", "gh-bank", 200_000, "GHS", map[string]any{"ip.is_tor": true}))
		i := slices.IndexFunc(d.Signals, func(s apigen.Signal) bool { return s.Code == "SEC-05" })
		require.GreaterOrEqual(t, i, 0, "%v", ruleIDs(d))
		assert.Equal(t, 10, d.Signals[i].Weight)
		d = decide(booked("gh-sec05-small", "transfer_outbound", "gh-bank", 50_000, "GHS",
			map[string]any{"ip.is_tor": true, "user.is_admin": false}))
		assert.False(t, slices.ContainsFunc(d.Signals, func(s apigen.Signal) bool { return s.Code == "SEC-05" }),
			"GHS 500 is under the GHS 1,000 setting")
		assert.NotContains(t, ruleIDs(d), "SEC-05", "with every fact known the rule is clear, not unevaluated")
	})
	t.Run("an event naming no booking entity is judged by the stated fallback", func(t *testing.T) {
		d := decide(booked("ng-fallback", "transfer_outbound", "", 250_000, "NGN", nil))
		require.NotNil(t, d.Jurisdiction)
		assert.Equal(t, apigen.JurisdictionResolutionResolvedBy("fallback"), d.Jurisdiction.ResolvedBy)
		assert.Equal(t, "NG", *d.Jurisdiction.Jurisdiction)
	})
	t.Run("with no fallback an unresolved event is judged by no pack", func(t *testing.T) {
		b := s.createTenant(t, "No fallback")
		activate(t, b.id, "GH", "gh", "gh-bank", "bank", nil)
		_, err := shared.owner.Exec(t.Context(), "DELETE FROM jurisdiction_activations WHERE tenant_id = $1 AND fallback", b.id)
		require.NoError(t, err)
		r := s.ingest(t, b.testKey, booked("unres-1", "cash_deposit", "", 5_000_000, "GHS", domestic))
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := decode[apigen.RiskDecision](t, r)
		require.NotNil(t, d.Jurisdiction)
		assert.Equal(t, apigen.JurisdictionResolutionResolvedBy("unresolved"), d.Jurisdiction.ResolvedBy)
		assert.Nil(t, d.Jurisdiction.Jurisdiction)
		assert.Empty(t, d.Controls, "no pack was guessed")
		assert.Empty(t, d.NotEvaluated)
	})

	t.Run("developers list every active rule with what to send", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/compliance/active-rules?jurisdiction=GH", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		list := decode[struct {
			Data []apigen.ActiveComplianceRule `json:"data"`
		}](t, r).Data
		i := slices.IndexFunc(list, func(x apigen.ActiveComplianceRule) bool { return x.Rule == "GH-REG-04" })
		require.GreaterOrEqual(t, i, 0)
		got := list[i]
		assert.Equal(t, "gh-bank", got.BookingEntity)
		assert.Equal(t, apigen.ActiveComplianceRuleMode("live"), got.Mode)
		assert.Contains(t, got.Facts, "txn.domestic")
		assert.NotContains(t, got.Facts, "institution.licence_type", "RiskOS supplies it from the activation")
		for _, x := range list {
			assert.Equal(t, "GH", x.Jurisdiction)
			assert.NotEqual(t, "GH-GOV-01", x.Rule, "governance presets are processes, not event rules")
		}
		assert.False(t, slices.ContainsFunc(list, func(x apigen.ActiveComplianceRule) bool { return x.Rule == "GH-TM-16" }),
			"rules off by default are not active")
	})
	t.Run("jurisdictions and activations are listed", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/compliance/jurisdictions", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		js := decode[struct {
			Data []apigen.Jurisdiction `json:"data"`
		}](t, r).Data
		require.Len(t, js, 3)
		for _, j := range js {
			assert.True(t, j.Activated, j.Code)
		}
		r = s.call(t, http.MethodGet, "/api/v1/compliance/activations", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		acts := decode[struct {
			Data []apigen.JurisdictionActivation `json:"data"`
		}](t, r).Data
		require.Len(t, acts, 3)
		assert.True(t, acts[0].Fallback, "the fallback is listed first")
		assert.Equal(t, "NG", acts[0].Jurisdiction)
	})
	t.Run("another tenant sees only its own activations", func(t *testing.T) {
		c := s.createTenant(t, "Other tenant")
		r := s.call(t, http.MethodGet, "/api/v1/compliance/activations", c.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		acts := decode[struct {
			Data []apigen.JurisdictionActivation `json:"data"`
		}](t, r).Data
		require.Len(t, acts, 1, "only its automatic Nigeria activation")
		assert.Equal(t, "default", acts[0].BookingEntity)
		r = s.call(t, http.MethodGet, "/api/v1/compliance/active-rules?jurisdiction=GH", c.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Empty(t, decode[struct {
			Data []apigen.ActiveComplianceRule `json:"data"`
		}](t, r).Data)
		// The same booking entity name in another tenant resolves to nothing of the first's.
		r = s.ingest(t, c.testKey, booked("x-gh", "cash_deposit", "gh-bank", 5_000_000, "GHS", domestic))
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := decode[apigen.RiskDecision](t, r)
		assert.Nil(t, controlOf(d, "GH-REG-04"))
		assert.Equal(t, apigen.JurisdictionResolutionResolvedBy("fallback"), d.Jurisdiction.ResolvedBy)
	})
}
