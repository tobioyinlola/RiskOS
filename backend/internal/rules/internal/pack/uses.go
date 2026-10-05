package pack

import (
	"maps"
	"slices"
)

// Uses lists what a rule reads from an event (ADR-0007, for the active-rules endpoint): the facts
// a customer sends (facts RiskOS computes itself are left out) and the entities its time windows
// are kept for.
func (r *Rule) Uses() (facts, entities []string) {
	fs, es := map[string]bool{}, map[string]bool{}
	var cond func(c *Cond)
	var operand func(o *Operand)
	fact := func(name string) {
		if name == "" {
			return
		}
		if f, ok := Facts[name]; ok && CheckSuppliedFact(name, sample(f)) == nil {
			fs[name] = true
		}
	}
	operand = func(o *Operand) {
		switch {
		case o == nil:
		case o.Fact != "":
			fact(o.Fact)
		case o.Agg != nil:
			es[o.Agg.By] = true
			cond(o.Agg.Where)
		case o.Times != nil:
			operand(&o.Times.Of)
		}
	}
	cond = func(c *Cond) {
		if c == nil {
			return
		}
		for i := range c.All {
			cond(&c.All[i])
		}
		for i := range c.Any {
			cond(&c.Any[i])
		}
		cond(c.Not)
		operand(c.Left)
		operand(c.Right)
	}
	if r.Trigger != nil {
		cond(r.Trigger.Scope)
	}
	cond(r.Condition)
	for i := range r.Branches {
		cond(&r.Branches[i].Condition)
	}
	cond(r.Outcome.Unless)
	return slices.Sorted(maps.Keys(fs)), slices.Sorted(maps.Keys(es))
}

// sample is a valid value of a fact's type, to ask whether a customer may send it.
func sample(f FactInfo) any {
	switch f.Type {
	case FactBool:
		return true
	case FactEnum:
		if len(f.Values) > 0 {
			return f.Values[0]
		}
		return ""
	case FactMoney:
		return map[string]any{"amount": 0.0, "currency": "USD"}
	default:
		return 0.0
	}
}

// RefEntities are the window entities a customer sends a reference for in compliance.refs; the
// others (account, customer, device...) come from the event itself.
func RefEntities() []string {
	return []string{"card", "bvn", "phone", "address", "loan", "inflow", "merchant", "api_key", "national_id", "agent"}
}
