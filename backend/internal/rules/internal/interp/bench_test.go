package interp_test

import (
	"sort"
	"testing"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

// constAggs answers every window with the same count, sum or "exists".
type constAggs struct{}

func (constAggs) Value(ref interp.AggRef) (any, bool) {
	switch ref.Fn {
	case "exists":
		return true, true
	case "sum", "daily_avg":
		return ngn(50_000), true
	default:
		return 3.0, true
	}
}

// BenchmarkOutboundTransfer evaluates every rule an outbound transfer triggers in the Nigeria
// pack, with typical facts: the interpreter's share of the decision path (windows are read
// from Redis in one round trip before it).
func BenchmarkOutboundTransfer(b *testing.B) {
	p, err := pack.Load(ng.Files)
	if err != nil {
		b.Fatal(err)
	}
	kinds := []string{"transfer_outbound", "transaction_any", "activity_any"}
	var rules []*interp.Rule
	for i := range p.Rules {
		if p.Rules[i].Trigger != nil {
			if r := interp.Compile(&p.Rules[i], nil); r.Triggered(kinds) {
				rules = append(rules, r)
			}
		}
	}
	env := interp.Env{Kinds: kinds, Aggs: constAggs{}, Facts: interp.Facts{
		"txn.amount": ngn(25_000), "txn.local_hour": 14.0, "device.is_new": false, "beneficiary.is_new": true,
		"txn.first_to_beneficiary": true, "customer.type": "individual", "device.bound_age": 90 * day,
		"account.age": 400 * day, "customer.is_pep": false, "party.on_poi_list": false, "device.rooted": false,
	}}
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
