package engine

import (
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	rulepack "github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// ReplayEvent is a stored evaluation a backtest puts through a pack again.
type ReplayEvent struct {
	Kinds  []string
	Facts  rulepack.Facts
	Replay features.Replay
}

// Tally counts what a pack's rules did over replayed events.
type Tally struct {
	Triggered, Fired, Clear, NotEvaluated int
	// Missing counts what made rules not evaluated ("fact:telco.sim_swap": 12).
	Missing map[string]int
	// FiredAt are the indexes of the events a rule fired on, in replay order.
	FiredAt []int
}

// memoryAggs answers the interpreter from in-memory windows.
type memoryAggs struct {
	m *features.MemoryWindows
	r features.Replay
	p *rulepack.TenantPack
}

func (a memoryAggs) Value(ref rulepack.Window) (any, bool) {
	entity := a.r.Entities[ref.By]
	if entity == "" {
		return nil, false
	}
	if ref.Fn == "days_over" && a.r.Currency != ref.OverCurrency {
		return nil, false
	}
	fn, window := a.p.Query(ref, a.r.At)
	v := a.m.Value(ref.Key, entity, fn, window, a.r.At, a.r.Currency)
	if !v.OK {
		return nil, false
	}
	switch ref.Fn {
	case "exists":
		return v.Value > 0, true
	case "sum", "daily_avg":
		if a.r.Currency == "" {
			return nil, false
		}
		return rulepack.Money{Minor: v.Value, Currency: a.r.Currency}, true
	default:
		return v.Value, true
	}
}

// Backtest replays events, in order, through a pack with windows kept in memory, and counts what
// its rules do. Events are recorded in windows before their rules are evaluated, as live.
func Backtest(p *rulepack.TenantPack, events []ReplayEvent) Tally {
	t := Tally{Missing: map[string]int{}}
	m := features.NewMemoryWindows()
	for i, ev := range events {
		record, _ := p.Windows(ev.Kinds)
		done := map[string]bool{}
		for _, ref := range record {
			entity := ev.Replay.Entities[ref.By]
			k := ref.Key + "|" + entity
			rec, known := rulepack.Record(ref, ev.Facts)
			if entity == "" || !rec || done[k] {
				continue
			}
			done[k] = true
			op := features.WindowOp{Unknown: !known}
			if distinctFn(ref.Fn) {
				op.Field, op.Per = ev.Replay.Fields[ref.Field], ev.Replay.Fields[ref.Per]
			}
			m.Record(ref.Key, entity, op, ev.Replay)
		}
		outcomes := p.Evaluate(rulepack.Env{Kinds: ev.Kinds, Facts: ev.Facts, Aggs: memoryAggs{m: m, r: ev.Replay, p: p}})
		for _, o := range outcomes {
			t.Triggered++
			switch o.Status {
			case rulepack.StatusFired:
				t.Fired++
				t.FiredAt = append(t.FiredAt, i)
			case rulepack.StatusClear:
				t.Clear++
			case rulepack.StatusNotEvaluated:
				t.NotEvaluated++
				for _, miss := range o.Missing {
					t.Missing[miss]++
				}
			}
		}
	}
	return t
}
