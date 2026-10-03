package engine

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	rulepack "github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// WindowSource records and reads rule pack windows and the activity history.
type WindowSource interface {
	Windows(ctx context.Context, ops []features.WindowOp, e features.WindowEvent, write bool) ([][]features.WindowValue, features.Activity, bool)
}

// Floor is a least decision a pack rule requires, with the reason recorded in the evaluation.
type Floor struct {
	Decision domain.Decision
	Reason   string
}

// PackOutput is what a tenant's rule pack adds to an evaluation (ADR-0006).
type PackOutput struct {
	Ref          string
	Kinds        []string
	Signals      []domain.Signal
	Shadow       []domain.Signal
	Controls     []domain.Control
	NotEvaluated []domain.NotEvaluated
	Floors       []Floor
	// Facts and Seen are recorded with the evaluation so it can be reproduced: the facts pack
	// rules saw, and the values each fired rule compared.
	Facts rulepack.Facts
	Seen  map[string]map[string]any
	// Replay is what a backtest needs to replay the event.
	Replay features.Replay
}

// windowAggs answers the interpreter from one round trip's window values.
type windowAggs struct {
	e      features.Event
	values map[string]features.WindowValue
}

// valueKey identifies one aggregate of one entity's window.
func valueKey(key, entity, fn string, window int64) string {
	return fmt.Sprintf("%s|%s|%s|%d", key, entity, fn, window)
}

func (a windowAggs) Value(ref rulepack.Window) (any, bool) {
	entity := a.e.Entity(ref.By)
	if entity == "" {
		return nil, false
	}
	v, ok := a.values[valueKey(ref.Key, entity, ref.Fn, ref.Window)]
	if !ok || !v.OK {
		return nil, false
	}
	switch ref.Fn {
	case "exists":
		return v.Value > 0, true
	case "sum", "daily_avg":
		// Sums are in the event's currency (the store sums only that currency).
		if a.e.Transaction == nil || a.e.Transaction.Currency == "" {
			return nil, false
		}
		return rulepack.Money{Minor: v.Value, Currency: a.e.Transaction.Currency}, true
	default:
		return v.Value, true
	}
}

func distinctFn(fn string) bool {
	return fn == "distinct_count" || fn == "distinct_count_once" || fn == "max_distinct_per_value"
}

// EvaluatePack evaluates a tenant's rule pack: it records the event in the pack's windows, reads
// the windows the triggered rules need and the activity history (one round trip), then evaluates
// each triggered rule. Windows are unavailable while behavioural history warms up after a reset.
func (en Engine) EvaluatePack(ctx context.Context, e features.Event, v features.Values, p *rulepack.TenantPack, write bool, eventKey string) PackOutput {
	out := PackOutput{Ref: p.Ref, Kinds: e.Kinds(), Replay: e.Replay()}
	if len(out.Kinds) == 0 {
		return out
	}
	record, read := p.Windows(out.Kinds)
	// Filters are checked on what is known before the round trip (the event, the customer's
	// facts and behaviour history); none uses the activity history.
	pre := withUSD(features.PackFacts(e, v, features.Activity{}), p)
	ops := map[string]*features.WindowOp{}
	var order []string
	op := func(ref rulepack.Window) *features.WindowOp {
		entity := e.Entity(ref.By)
		if entity == "" {
			return nil
		}
		k := ref.Key + "|" + entity
		if o, ok := ops[k]; ok {
			return o
		}
		o := &features.WindowOp{Key: ref.Key, EntityKind: ref.By, EntityID: entity}
		ops[k] = o
		order = append(order, k)
		return o
	}
	if !v.Warming {
		for _, ref := range record {
			rec, known := rulepack.Record(ref, pre)
			o := op(ref)
			if !rec || o == nil {
				continue
			}
			o.Record = true
			o.Unknown = o.Unknown || !known
			o.Retention = max(o.Retention, time.Duration(ref.Retention)*time.Second)
			if distinctFn(ref.Fn) {
				o.Field, o.Per = e.FieldValue(ref.Field), e.FieldValue(ref.Per)
			}
		}
		for _, ref := range read {
			o := op(ref)
			if o == nil {
				continue
			}
			q := features.WindowQuery{Fn: ref.Fn, Window: time.Duration(ref.Window) * time.Second}
			if !slices.Contains(o.Queries, q) {
				o.Queries = append(o.Queries, q)
			}
		}
	}
	list := make([]features.WindowOp, len(order))
	for i, k := range order {
		list[i] = *ops[k]
	}
	we := features.WindowEvent{ID: eventKey, At: e.OccurredAt, UserID: e.Actor.UserID, AccountID: e.Actor.AccountID}
	if t := e.Transaction; t != nil {
		we.Amount, we.Currency = t.Amount, t.Currency
	}
	values, activity, ok := en.windows.Windows(ctx, list, we, write)
	aggs := windowAggs{e: e, values: map[string]features.WindowValue{}}
	if ok {
		for i, o := range list {
			for j, q := range o.Queries {
				aggs.values[valueKey(o.Key, o.EntityID, q.Fn, int64(q.Window.Seconds()))] = values[i][j]
			}
		}
	}
	if v.Warming {
		activity = features.Activity{}
	}
	out.Facts = withUSD(features.PackFacts(e, v, activity), p)
	out.Seen = map[string]map[string]any{}
	for _, o := range p.Evaluate(rulepack.Env{Kinds: out.Kinds, Facts: out.Facts, Aggs: aggs}) {
		switch o.Status {
		case rulepack.StatusNotEvaluated:
			out.NotEvaluated = append(out.NotEvaluated, domain.NotEvaluated{Rule: o.Rule, Missing: o.Missing})
			continue
		case rulepack.StatusFired:
		default:
			continue
		}
		out.Seen[o.Rule] = o.Seen
		live := o.Mode == rulepack.ModeLive
		if o.Kind == rulepack.KindSignal {
			sig := domain.Signal{Code: o.Rule, Label: o.Name, Weight: o.Weight}
			switch {
			case !live:
				out.Shadow = append(out.Shadow, sig)
			case sig.Weight > 0:
				out.Signals = append(out.Signals, sig)
			}
			if d, ok := domain.ParseDecision(o.MinDecision); live && ok && o.MinDecision != "" {
				out.Floors = append(out.Floors, Floor{Decision: d, Reason: "min_decision:" + o.Rule})
			}
			continue
		}
		c := domain.Control{
			Rule: o.Rule, Name: o.Name, Mode: string(o.Mode), Reason: o.Reason, Tasks: nonNilTasks(o.Tasks),
			FeePercent: o.Fee, HoldSeconds: o.Hold, Branch: o.Branch, ExemptionUnknown: o.ExemptionUnknown,
		}
		if d, ok := domain.ParseDecision(o.Decision); ok && o.Decision != "" {
			c.Decision = d
			if live {
				out.Floors = append(out.Floors, Floor{Decision: d, Reason: "control:" + o.Rule})
			}
		}
		out.Controls = append(out.Controls, c)
	}
	return out
}

// withUSD adds the event's amount in US dollars from the institution's rates, unless the event
// sent it.
func withUSD(f rulepack.Facts, p *rulepack.TenantPack) rulepack.Facts {
	amount, ok := f["txn.amount"].(rulepack.Money)
	if _, sent := f["txn.amount_usd"]; sent || !ok {
		return f
	}
	if usd, ok := p.AmountUSD(amount); ok {
		f["txn.amount_usd"] = usd
	}
	return f
}

func nonNilTasks(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}
