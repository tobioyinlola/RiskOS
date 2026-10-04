package pack_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

func TestKinds(t *testing.T) {
	for name, tc := range map[string]struct {
		in   pack.KindInput
		want []string
	}{
		"a debit":              {pack.KindInput{Type: "transaction", Direction: "debit"}, []string{"transfer_outbound", "transaction_any", "activity_any"}},
		"a credit":             {pack.KindInput{Type: "transaction", Direction: "credit"}, []string{"transfer_inbound", "transaction_any", "activity_any"}},
		"a card payment":       {pack.KindInput{Type: "transaction", Direction: "debit", Channel: "card"}, []string{"card_authorization", "transaction_any", "activity_any"}},
		"a named cash payment": {pack.KindInput{Type: "transaction", Kind: "cash_withdrawal"}, []string{"cash_withdrawal", "cash_any", "transaction_any", "activity_any"}},
		"a successful login":   {pack.KindInput{Type: "login", Outcome: "success"}, []string{"login", "auth_success", "activity_any"}},
		"a failed login":       {pack.KindInput{Type: "login", Outcome: "failure"}, []string{"auth_failure", "activity_any"}},
		"a device change":      {pack.KindInput{Type: "device_change"}, []string{"device_bound", "activity_any"}},
		"an export":            {pack.KindInput{Type: "activity", Kind: "data_export"}, []string{"data_export", "activity_any"}},
		"an unnamed activity":  {pack.KindInput{Type: "activity"}, nil},
		"a forex deal":         {pack.KindInput{Type: "transaction", Kind: "forex_sell"}, []string{"forex_sell", "forex_any", "transaction_any", "activity_any"}},
		"a SIM replacement":    {pack.KindInput{Type: "activity", Kind: "sim_replaced"}, []string{"sim_replaced", "sim_change", "activity_any"}},
	} {
		assert.Equal(t, tc.want, pack.Kinds(tc.in), name)
	}
	// Customers can name every kind a rule triggers on, except RiskOS's own workflow events and
	// the group kinds an event also counts as.
	named := map[string]bool{}
	for _, t := range []string{"transaction", "login", "device_change", "beneficiary_added", "session_change", "profile_change", "activity"} {
		for _, k := range pack.KindsFor(t) {
			named[k] = true
		}
	}
	for k := range pack.Events {
		switch k {
		case "transaction_any", "activity_any", "alert_screening_due", "suspicion_confirmed", "report_submit", "daily_job",
			"suspicion_recorded", "forex_any", "sim_change", "cash_any":
			assert.False(t, named[k], k)
		default:
			assert.True(t, named[k], "%s cannot be sent", k)
		}
	}
}

func TestSuppliedFacts(t *testing.T) {
	ok := map[string]any{
		"device.rooted": true, "customer.type": "corporate", "account.age": 3600.0, "screening.sanctions_score": 100.0,
		"txn.amount_usd": map[string]any{"amount": 1_000_000.0, "currency": "USD"},
	}
	for name, v := range ok {
		assert.NoError(t, pack.CheckSuppliedFact(name, v), name)
	}
	for name, v := range map[string]any{
		"device.is_new":             true,                                               // history
		"report.category":           "fraud",                                            // workflow
		"card.id":                   "crd_1",                                            // identifiers go in refs
		"txn.amount":                1.0,                                                // read from the event
		"screening.sanctions_score": 101.0,                                              // out of range
		"account.age":               -1.0,                                               // negative
		"account.kyc_tier_norm":     map[string]any{"amount": 1.5, "currency": "NGN"},   // fractional minor units
		"txn.amount_usd":            map[string]any{"amount": 100.0, "currency": "usd"}, // not an ISO code
		"nope":                      true,
	} {
		assert.Error(t, pack.CheckSuppliedFact(name, v), name)
	}
}

func TestRefs(t *testing.T) {
	assert.ErrorIs(t, pack.CheckRef("bvn", "22212345678"), pack.ErrSensitive)
	assert.ErrorIs(t, pack.CheckRef("phone", "+2348031234567"), pack.ErrSensitive)
	assert.NoError(t, pack.CheckRef("bvn", "bvn_ref_9"))
	assert.NoError(t, pack.CheckRef("card", "12345678901"), "only BVN and phone shapes are checked here; card numbers by the Luhn scan")
}
