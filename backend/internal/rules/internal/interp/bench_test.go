package interp_test

import (
	"sort"
	"testing"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

// constAggs answers every window with the same count, sum (in the pack's currency) or "exists".
type constAggs struct{ sum interp.Money }

func (a constAggs) Value(ref interp.AggRef) (any, bool) {
	switch ref.Fn {
	case "exists":
		return true, true
	case "sum", "daily_avg":
		return a.sum, true
	default:
		return 3.0, true
	}
}

// benchOutbound evaluates every rule an outbound transfer triggers in a pack, with typical
// facts: the interpreter's share of the decision path (windows are read from Redis in one round
// trip before it). Settings the institution must enter are given a value, so the whole
// condition runs.
func benchOutbound(b *testing.B, p *pack.Pack, amount interp.Money, extra interp.Facts) {
	b.Helper()
	kinds := []string{"transfer_outbound", "transaction_any", "activity_any"}
	var rules []*interp.Rule
	for i := range p.Rules {
		if p.Rules[i].Trigger != nil {
			if r := interp.Compile(&p.Rules[i], filled(&p.Rules[i])); r.Triggered(kinds) {
				rules = append(rules, r)
			}
		}
	}
	facts := interp.Facts{
		"txn.amount": amount, "txn.local_hour": 14.0, "device.is_new": false, "beneficiary.is_new": true,
		"txn.first_to_beneficiary": true, "customer.type": "individual", "device.bound_age": 90 * day,
		"account.age": 400 * day, "customer.is_pep": false, "party.on_poi_list": false, "device.rooted": false,
	}
	for k, v := range extra {
		facts[k] = v
	}
	sum := interp.Money{Minor: amount.Minor * 2, Currency: amount.Currency}
	env := interp.Env{Kinds: kinds, Aggs: constAggs{sum: sum}, Facts: facts}
	durations := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		start := time.Now()
		for _, r := range rules {
			interp.Evaluate(r, env)
		}
		durations = append(durations, time.Since(start))
	}
	b.StopTimer()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	b.ReportMetric(float64(durations[len(durations)*95/100].Nanoseconds()), "p95-ns/event")
	b.ReportMetric(float64(len(rules)), "rules/event")
}

// BenchmarkOutboundTransfer covers the Nigeria pack.
func BenchmarkOutboundTransfer(b *testing.B) {
	p, err := pack.Load(ng.Files)
	if err != nil {
		b.Fatal(err)
	}
	benchOutbound(b, p, ngn(25_000), nil)
}

// BenchmarkOutboundTransferGhana covers the Ghana pack: its own rules and the Nigerian ones it
// inherits (ADR-0007), a mobile money wallet debit.
func BenchmarkOutboundTransferGhana(b *testing.B) {
	benchOutbound(b, countryPack(b, "GH"), ghs(2_500), interp.Facts{
		"txn.domestic": true, "txn.direction": "debit", "account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium",
	})
}

// BenchmarkOutboundTransferRwanda covers the Rwanda pack, an eKash transfer.
func BenchmarkOutboundTransferRwanda(b *testing.B) {
	benchOutbound(b, countryPack(b, "RW"), rwf(250_000), interp.Facts{"txn.rail": "ekash", "txn.direction": "debit"})
}
