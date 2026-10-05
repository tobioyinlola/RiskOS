//go:build integration

package integration

import (
	"encoding/json"
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
)

// lastPackFacts are the pack facts recorded with the tenant's latest evaluation.
func lastPackFacts(t *testing.T, tenantID uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT features->'pack'->'facts' FROM evaluations WHERE tenant_id = $1 ORDER BY id DESC LIMIT 1`, tenantID).Scan(&raw))
	var facts map[string]any
	require.NoError(t, json.Unmarshal(raw, &facts))
	return facts
}

// wallet is an event on one account booked by an entity, at a time.
func wallet(id, kind, account, entity string, at time.Time, amount int64, direction string, facts map[string]any, refs map[string]any) map[string]any {
	e := money(id, kind, account, amount, "GHS", facts)
	e["occurred_at"] = at.Format(time.RFC3339Nano)
	data, _ := e["data"].(map[string]any) // built by txn
	data["direction"] = direction
	c, _ := e["compliance"].(map[string]any) // built by money
	c["booking_entity"] = entity
	if refs != nil {
		c["refs"] = refs
	}
	return e
}

// A US dollar threshold in a cedi pack is checked with the institution's recorded rate, and the
// rate is kept with the evaluation; without a rate the rule says it was not evaluated
// (ADR-0007, acceptance criterion 6).
func TestUSDThresholdNeedsARecordedRate(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Cedi FX")
	admin := s.onboard(t, a, "admin@cedi-fx.example", authz.RoleCustomerAdmin)
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", []string{"ectr_reporter"}) // the FIC names commercial banks as ECTR reporters
	decide := func(tenantKey, id string, pesewas int64) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, tenantKey, booked(id, "transfer_international", "gh-bank", pesewas, "GHS", nil))
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}

	t.Run("without a rate the dollar threshold is not evaluated", func(t *testing.T) {
		d := decide(a.testKey, "ectr-norate", 100_000_000)
		assert.Nil(t, controlOf(d, "GH-REG-05"), "GHS 1,000,000 is not guessed to be over USD 1,000")
		assert.Contains(t, notEvaluated(d, "GH-REG-05"), "fact:txn.amount_usd")
		assert.NotContains(t, lastPackFacts(t, a.id), "txn.usd_rate")
	})

	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/fx-rates/GHS", map[string]any{"usd_per_unit": 0.08})
	require.Equal(t, http.StatusOK, r.status, r.String())
	b := s.createTenant(t, "Cedi FX with a rate")
	adminB := s.onboard(t, b, "admin@cedi-fx-b.example", authz.RoleCustomerAdmin)
	activate(t, b.id, "GH", "gh", "gh-bank", "bank", []string{"ectr_reporter"})
	r = adminB.b.do(t, http.MethodPut, "/api/v1/compliance/fx-rates/GHS", map[string]any{"usd_per_unit": 0.08})
	require.Equal(t, http.StatusOK, r.status, r.String())

	t.Run("at USD 1,000 an ECTR is due, with the rate recorded", func(t *testing.T) {
		d := decide(b.testKey, "ectr-at", 1_250_000) // GHS 12,500 at 0.08 is USD 1,000.00
		c := controlOf(d, "GH-REG-05")
		require.NotNil(t, c, "%v", ruleIDs(d))
		assert.Equal(t, []string{"report.ECTR"}, c.Tasks)
		facts := lastPackFacts(t, b.id)
		rate, ok := facts["txn.usd_rate"].(map[string]any)
		require.True(t, ok, "the rate is kept with the evaluation: %v", facts)
		assert.Equal(t, "GHS", rate["currency"])
		assert.InDelta(t, 0.08, rate["usd_per_unit"], 1e-12)
		assert.NotEmpty(t, rate["as_of"])
	})
	t.Run("two cents under USD 1,000 nothing is due", func(t *testing.T) {
		d := decide(b.testKey, "ectr-under", 1_249_975) // USD 999.98
		assert.Nil(t, controlOf(d, "GH-REG-05"))
		assert.NotContains(t, notEvaluated(d, "GH-REG-05"), "fact:txn.amount_usd")
	})
}

// A wallet's daily limit counts the local business day, not the last 24 hours: what was spent
// before the day started does not count (ADR-0007).
func TestWalletLimitCountsTheLocalDay(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Accra wallets")
	activate(t, a.id, "GH", "gh", "gh-emi", "emi", nil)
	// Events may be at most a day old, so the test moves the business day's start to fall
	// between them: the day starts at the top of the hour three to four hours ago (Accra is UTC).
	now := time.Now().UTC()
	start := now.Add(-3 * time.Hour).Truncate(time.Hour)
	_, err := shared.owner.Exec(t.Context(),
		"UPDATE jurisdiction_activations SET day_starts_at = $2 WHERE tenant_id = $1 AND booking_entity = 'gh-emi'", a.id, start.Hour())
	require.NoError(t, err)
	tier := map[string]any{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"}
	spend := func(id string, at time.Time, pesewas int64) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, a.testKey, wallet(id, "transfer_outbound", "acc_wallet", "gh-emi", at, pesewas, "debit", tier, nil))
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}

	d := spend("w-yesterday", start.Add(-30*time.Minute), 200_000) // GHS 2,000 before the day began
	assert.Nil(t, controlOf(d, "GH-REG-01"))
	d = spend("w-today-1", start.Add(30*time.Minute), 150_000) // GHS 1,500: GHS 3,500 over 24 hours
	assert.Nil(t, controlOf(d, "GH-REG-01"), "only GHS 1,500 was spent today: %v", ruleIDs(d))
	d = spend("w-today-2", start.Add(time.Hour), 150_000) // GHS 3,000 today, the limit itself
	assert.Nil(t, controlOf(d, "GH-REG-01"), "GHS 3,000 is the limit, not over it")
	d = spend("w-today-3", start.Add(90*time.Minute), 1) // one pesewa over
	c := controlOf(d, "GH-REG-01")
	require.NotNil(t, c, "%v", ruleIDs(d))
	assert.Equal(t, apigen.Decision("decline"), d.Decision)
}

// GH-TM-23 (a payment received in error, sent on): an incoming payment is from a new sender the
// first time the account receives from that sender (ADR-0007).
func TestNewSenderIsRemembered(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Mistaken credits")
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", nil)
	configure(t, a.id, "gh", "GH-TM-23", "live", `{"wrong_transfer_min": 100000}`) // GHS 1,000
	at := time.Now().Add(-time.Minute)
	send := func(id, kind, direction string, pesewas int64, refs map[string]any) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, a.testKey, wallet(id, kind, "acc_mistake", "gh-bank", at, pesewas, direction, nil, refs))
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	signal := func(d apigen.RiskDecision) bool {
		return slices.ContainsFunc(d.Signals, func(s apigen.Signal) bool { return s.Code == "GH-TM-23" })
	}

	send("in-1", "transfer_inbound", "credit", 200_000, map[string]any{"sender": "snd-stranger"})
	assert.Equal(t, true, lastPackFacts(t, a.id)["txn.from_new_sender"], "the first payment from a sender")
	send("in-2", "transfer_inbound", "credit", 50_000, map[string]any{"sender": "snd-stranger"})
	assert.Equal(t, false, lastPackFacts(t, a.id)["txn.from_new_sender"], "the same sender again")

	// Only the GHS 2,000 from the new sender counts: 80% of it is GHS 1,600. Had the repeat
	// payment counted, the line would be GHS 2,000.
	d := send("out-small", "transfer_outbound", "debit", 159_999, nil)
	assert.False(t, signal(d), "a pesewa under GHS 1,600: %v", ruleIDs(d))
	d = send("out-on", "transfer_outbound", "debit", 160_000, nil)
	require.True(t, signal(d), "GHS 1,600 sent on: %v", ruleIDs(d))
	i := slices.IndexFunc(d.Signals, func(s apigen.Signal) bool { return s.Code == "GH-TM-23" })
	assert.Equal(t, 25, d.Signals[i].Weight)
}
