// Package interp evaluates pack rules (ADR-0006) against an event's facts and windowed
// aggregates. It walks the validated definition; nothing is compiled to code or executed.
//
// Evaluation is three-valued: a comparison whose fact, setting or aggregate is unavailable is
// unknown, not false. all() is false if any part is false, else unknown if any part is unknown;
// any() is the mirror image; not() keeps unknown. A rule whose result is unknown is reported as
// not evaluated, with what was missing, so missing data is never read as "no risk".
package interp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

// Tri is a three-valued truth value.
type Tri int8

// Truth values.
const (
	False Tri = iota
	True
	Unknown
)

func and(a, b Tri) Tri {
	switch {
	case a == False || b == False:
		return False
	case a == Unknown || b == Unknown:
		return Unknown
	default:
		return True
	}
}

func or(a, b Tri) Tri {
	switch {
	case a == True || b == True:
		return True
	case a == Unknown || b == Unknown:
		return Unknown
	default:
		return False
	}
}

func not(a Tri) Tri {
	switch a {
	case True:
		return False
	case False:
		return True
	default:
		return Unknown
	}
}

// Money is an amount in minor units of a currency.
type Money struct {
	Minor    float64
	Currency string
}

// Facts are an event's known values by catalog name: float64 for numbers (durations in
// seconds), Money, bool, string (enum), []string (a multi-valued enum, such as the fields a
// profile change touched). An absent fact is unknown.
type Facts map[string]any

// AggRef identifies one windowed aggregate of a rule, with its settings resolved.
type AggRef struct {
	// Key identifies the stored window: the event, entity, field, grouping and filter (with
	// the filter's settings). Aggregates that differ only by function or window share it.
	Key    string
	Fn     string
	Event  string
	By     string
	Field  string
	Per    string
	Where  *pack.Cond
	Window int64
	// Calendar is day or month for a window that runs from the start of the local period; the
	// engine resolves its length at the event's time (TenantPack.Query).
	Calendar string
	// Over, for days_over, is the daily total a day must reach, in minor units of OverCurrency.
	Over         float64
	OverCurrency string
	// Retention is the longest window the key may be asked for: the window setting's maximum.
	Retention int64
	// rule is the compiled rule the filter's settings come from.
	rule *Rule
}

// Aggs answers aggregates: a float64 (count, distinct count), Money (sum, daily average) or
// bool (exists). ok is false when the value is unavailable (the store failed, or the event
// lacks the entity the window is kept for).
type Aggs interface {
	Value(ref AggRef) (v any, ok bool)
}

// Env is what one evaluation sees.
type Env struct {
	// Kinds are the pack event kinds the event counts as (transfer_outbound, transaction_any…).
	Kinds []string
	Facts Facts
	Aggs  Aggs
}

// Rule is a pack rule with its settings resolved for a tenant.
type Rule struct {
	Def    *pack.Rule
	params map[string]float64
	// currency of each money setting.
	currency map[string]string
	// unset settings: the institution must set them before the rule can run.
	unset map[string]bool
	aggs  map[*pack.Aggregate]AggRef
}

// Compile resolves a rule's settings: overrides first, then defaults. A setting with neither is
// unset, and the parts of the rule using it are unknown. Overrides must already be validated
// against the setting's bounds.
func Compile(def *pack.Rule, overrides map[string]float64) *Rule {
	r := &Rule{
		Def: def, params: map[string]float64{}, currency: map[string]string{}, unset: map[string]bool{},
		aggs: map[*pack.Aggregate]AggRef{},
	}
	for _, p := range def.Parameters {
		switch v, ok := overrides[p.Name]; {
		case ok:
			r.params[p.Name] = v
		case p.Default != nil:
			r.params[p.Name] = *p.Default
		default:
			r.unset[p.Name] = true
		}
		if p.Currency != "" {
			r.currency[p.Name] = p.Currency
		}
	}
	walkAggs(def, func(a *pack.Aggregate) { r.aggs[a] = r.ref(a) })
	return r
}

// walkAggs calls fn for every aggregate in the rule's scope, condition, branches and exemption.
func walkAggs(def *pack.Rule, fn func(*pack.Aggregate)) {
	var cond func(c *pack.Cond)
	var operand func(o *pack.Operand)
	operand = func(o *pack.Operand) {
		switch {
		case o == nil:
		case o.Agg != nil:
			fn(o.Agg)
			if o.Agg.Where != nil {
				cond(o.Agg.Where)
			}
		case o.Times != nil:
			operand(&o.Times.Of)
		}
	}
	cond = func(c *pack.Cond) {
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
	if def.Trigger != nil {
		cond(def.Trigger.Scope)
	}
	cond(def.Condition)
	for i := range def.Branches {
		cond(&def.Branches[i].Condition)
	}
	cond(def.Outcome.Unless)
}

func (r *Rule) ref(a *pack.Aggregate) AggRef {
	ref := AggRef{
		Fn: a.Fn, Event: a.Event, By: a.By, Field: a.Field, Per: a.Per, Where: a.Where,
		Window: int64(r.params[a.Window]), Calendar: a.Calendar, rule: r,
	}
	if a.Over != "" {
		factor := 1.0
		if a.OverFactor != "" {
			factor = r.params[a.OverFactor]
		}
		ref.Over, ref.OverCurrency = math.Round(r.params[a.Over]*factor), r.currency[a.Over]
	}
	for _, p := range r.Def.Parameters {
		if p.Name == a.Window {
			ref.Retention = int64(p.Max)
		}
	}
	// The key covers what is stored and the filter that decides what is stored, including the
	// filter's resolved settings, so a changed setting starts a fresh window.
	where, _ := json.Marshal(a.Where)
	var used []string
	if a.Where != nil {
		paramsIn(a.Where, func(name string) { used = append(used, fmt.Sprintf("%s=%v", name, r.params[name])) })
	}
	sort.Strings(used)
	// Every stored entry carries its amount, so sums and counts share a window; only distinct
	// counts store an identifier field, which is then part of the key.
	field := ""
	if a.Fn == "distinct_count" || a.Fn == "distinct_count_once" || a.Fn == "max_distinct_per_value" {
		field = a.Field
	}
	h := sha256.Sum256([]byte(strings.Join([]string{a.Event, a.By, field, a.Per, string(where), strings.Join(used, ",")}, "\x00")))
	ref.Key = hex.EncodeToString(h[:8])
	return ref
}

func paramsIn(c *pack.Cond, fn func(string)) {
	var operand func(o *pack.Operand)
	operand = func(o *pack.Operand) {
		switch {
		case o == nil:
		case o.Param != "":
			fn(o.Param)
		case o.Times != nil:
			fn(o.Times.Factor)
			operand(&o.Times.Of)
		}
	}
	for i := range c.All {
		paramsIn(&c.All[i], fn)
	}
	for i := range c.Any {
		paramsIn(&c.Any[i], fn)
	}
	if c.Not != nil {
		paramsIn(c.Not, fn)
	}
	operand(c.Left)
	operand(c.Right)
}

// Aggregates returns the rule's aggregates.
func (r *Rule) Aggregates() []AggRef {
	out := make([]AggRef, 0, len(r.aggs))
	for _, a := range r.aggs {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b AggRef) int { return strings.Compare(a.Key+a.Fn, b.Key+b.Fn) })
	return out
}

// Param returns a resolved setting.
func (r *Rule) Param(name string) (float64, bool) {
	v, ok := r.params[name]
	return v, ok
}

// Triggered reports whether the rule runs on an event of these kinds.
func (r *Rule) Triggered(kinds []string) bool {
	if r.Def.Trigger == nil {
		return false
	}
	for _, k := range r.Def.Trigger.Events {
		if slices.Contains(kinds, k) {
			return true
		}
	}
	return false
}

// Status is the result of evaluating a rule.
type Status string

// Statuses.
const (
	// StatusFired means the rule's condition holds: a control acts, a signal adds weight.
	StatusFired Status = "fired"
	// StatusClear means the condition does not hold (or an exemption applies).
	StatusClear Status = "clear"
	// StatusNotEvaluated means data or a setting the rule needs is unavailable.
	StatusNotEvaluated Status = "not_evaluated"
	// StatusNotApplicable means the event is not one the rule looks at.
	StatusNotApplicable Status = "not_applicable"
)

// Result is a rule's evaluation, with what it saw.
type Result struct {
	Rule   string
	Status Status
	// Missing lists what was unavailable: "fact:<name>", "setting:<name>",
	// "history:<fn> <event> by <entity>" or "currency:<fact>".
	Missing []string
	// Branch is the label of the branch that fired, for rules with branches.
	Branch string
	// Weight is a fired signal's points.
	Weight int
	// Fee is a fired fee control's rate in percent; Hold a hold control's maximum in seconds,
	// or HoldWorkingDays its maximum in the jurisdiction's working days (the caller turns them
	// into an end time with the working-day calendar).
	Fee             float64
	Hold            int64
	HoldWorkingDays int
	// ExemptionUnknown is set when a control fired because its exemption could not be checked.
	ExemptionUnknown bool
	// Seen are the values the evaluation compared, for the evaluation record.
	Seen map[string]any
}

type evaluator struct {
	r       *Rule
	env     Env
	missing map[string]bool
	seen    map[string]any
}

// Evaluate evaluates a compiled rule against an event.
func Evaluate(r *Rule, env Env) (res Result) {
	res = Result{Rule: r.Def.ID, Status: StatusNotApplicable}
	if !r.Triggered(env.Kinds) {
		return res
	}
	ev := &evaluator{r: r, env: env, missing: map[string]bool{}, seen: map[string]any{}}
	defer func() {
		res.Missing = sortedKeys(ev.missing)
		if len(ev.seen) > 0 {
			res.Seen = ev.seen
		}
	}()
	if s := r.Def.Trigger.Scope; s != nil {
		switch ev.cond(s) {
		case False:
			return res
		case Unknown:
			res.Status = StatusNotEvaluated
			return res
		}
	}
	verdict := True
	switch {
	case len(r.Def.Branches) > 0:
		verdict = ev.branches(&res)
	case r.Def.Condition != nil:
		verdict = ev.cond(r.Def.Condition)
	}
	switch verdict {
	case False:
		res.Status = StatusClear
		return res
	case Unknown:
		res.Status = StatusNotEvaluated
		return res
	}
	if u := r.Def.Outcome.Unless; u != nil {
		switch ev.cond(u) {
		case True:
			res.Status = StatusClear
			return res
		case Unknown:
			// The legal limit applies unless the exemption is shown: an unknown exemption does
			// not lift a regulatory control.
			res.ExemptionUnknown = true
		}
	}
	res.Status = StatusFired
	o := r.Def.Outcome
	if o.Weight != "" {
		res.Weight = int(r.params[o.Weight])
	}
	if o.Fee != "" {
		res.Fee = r.params[o.Fee]
	}
	if o.Hold != "" {
		if r.ParamType(o.Hold) == pack.ParamWorkingDays {
			res.HoldWorkingDays = int(r.params[o.Hold])
		} else {
			res.Hold = int64(r.params[o.Hold])
		}
	}
	return res
}

// ParamType returns the type of one of the rule's parameters ("" if it has none of that name).
func (r *Rule) ParamType(name string) pack.ParamType {
	for _, p := range r.Def.Parameters {
		if p.Name == name {
			return p.Type
		}
	}
	return ""
}

// branches picks the matching branch with the most points (signals) or the first matching one
// (controls). With no match, the result is unknown if any branch was.
func (ev *evaluator) branches(res *Result) Tri {
	verdict := False
	best := -1
	for i := range ev.r.Def.Branches {
		b := &ev.r.Def.Branches[i]
		t := ev.cond(&b.Condition)
		verdict = or(verdict, t)
		if t != True {
			continue
		}
		w := int(ev.r.params[b.Weight])
		if best < 0 || w > res.Weight {
			best, res.Weight, res.Branch = i, w, b.Label
			if b.Fee != "" {
				res.Fee = ev.r.params[b.Fee]
			}
		}
	}
	return verdict
}

func (ev *evaluator) cond(c *pack.Cond) Tri {
	switch {
	case len(c.All) > 0:
		t := True
		for i := range c.All {
			t = and(t, ev.cond(&c.All[i]))
		}
		return t
	case len(c.Any) > 0:
		t := False
		for i := range c.Any {
			t = or(t, ev.cond(&c.Any[i]))
		}
		return t
	case c.Not != nil:
		return not(ev.cond(c.Not))
	default:
		return ev.compare(c)
	}
}

// value is an operand's value.
type value struct {
	v  any
	ok bool
	// label names the operand in Seen.
	label string
}

func (ev *evaluator) operand(o *pack.Operand) value {
	switch {
	case o.Fact != "":
		v, ok := ev.env.Facts[o.Fact]
		if !ok {
			ev.missing["fact:"+o.Fact] = true
			return value{}
		}
		ev.seen[o.Fact] = v
		return value{v: v, ok: true, label: o.Fact}
	case o.Param != "":
		v, ok := ev.r.params[o.Param]
		if !ok {
			ev.missing["setting:"+o.Param] = true
			return value{}
		}
		if cur := ev.r.currency[o.Param]; cur != "" {
			return value{v: Money{Minor: v, Currency: cur}, ok: true}
		}
		return value{v: v, ok: true}
	case o.Literal != nil:
		return value{v: o.Literal, ok: true}
	case o.Agg != nil:
		ref := ev.r.aggs[o.Agg]
		for _, name := range []string{o.Agg.Window, o.Agg.Over, o.Agg.OverFactor} {
			if name != "" && ev.r.unset[name] {
				ev.missing["setting:"+name] = true
				return value{}
			}
		}
		label := fmt.Sprintf("%s %s by %s", ref.Fn, ref.Event, ref.By)
		if ev.env.Aggs == nil {
			ev.missing["history:"+label] = true
			return value{}
		}
		v, ok := ev.env.Aggs.Value(ref)
		if !ok {
			ev.missing["history:"+label] = true
			return value{}
		}
		ev.seen[label] = v
		return value{v: v, ok: true, label: label}
	default:
		factor, ok := ev.r.params[o.Times.Factor]
		if !ok {
			ev.missing["setting:"+o.Times.Factor] = true
			return value{}
		}
		of := ev.operand(&o.Times.Of)
		if !of.ok {
			return value{}
		}
		switch x := of.v.(type) {
		case Money:
			// Round away binary floating-point noise (0.8 × 1,000 is 800, not 800.0000000001).
			return value{v: Money{Minor: math.Round(x.Minor*factor*1e6) / 1e6, Currency: x.Currency}, ok: true}
		case float64:
			return value{v: math.Round(x*factor*1e6) / 1e6, ok: true}
		}
		return value{}
	}
}

func (ev *evaluator) compare(c *pack.Cond) Tri {
	left, right := ev.operand(c.Left), ev.operand(c.Right)
	if !left.ok || !right.ok {
		return Unknown
	}
	switch c.Op {
	case "eq", "ne":
		t := equal(left.v, right.v)
		if c.Op == "ne" {
			t = not(t)
		}
		return t
	case "in", "not_in":
		t := member(left.v, right.v)
		if c.Op == "not_in" {
			t = not(t)
		}
		return t
	default:
		return ev.numeric(c.Op, left, right)
	}
}

func (ev *evaluator) numeric(op string, left, right value) Tri {
	l, lcur, lok := number(left.v)
	r, rcur, rok := number(right.v)
	if !lok || !rok {
		return Unknown
	}
	if lcur != rcur {
		// Never compare amounts in different currencies (no conversion on the hot path).
		ev.missing["currency:"+left.label] = true
		return Unknown
	}
	var t bool
	switch op {
	case "gt":
		t = l > r
	case "gte":
		t = l >= r
	case "lt":
		t = l < r
	case "lte":
		t = l <= r
	case "multiple_of":
		t = r > 0 && math.Mod(l, r) == 0
	}
	if t {
		return True
	}
	return False
}

func number(v any) (n float64, currency string, ok bool) {
	switch x := v.(type) {
	case float64:
		return x, "", true
	case Money:
		return x.Minor, x.Currency, true
	case int:
		return float64(x), "", true
	case int64:
		return float64(x), "", true
	}
	return 0, "", false
}

// equal compares yes/no facts, enums (any of a multi-valued enum) and flags (0 or 1).
func equal(l, r any) Tri {
	switch x := l.(type) {
	case bool:
		y, ok := r.(bool)
		return tri(ok && x == y, ok)
	case string:
		y, ok := r.(string)
		return tri(ok && x == y, ok)
	case []string:
		y, ok := r.(string)
		return tri(ok && slices.Contains(x, y), ok)
	case float64:
		y, ok := r.(float64)
		return tri(ok && x == y, ok)
	}
	return Unknown
}

// member reports whether an enum (or any value of a multi-valued enum) is in a list.
func member(l, r any) Tri {
	list, ok := r.([]any)
	if !ok {
		return Unknown
	}
	has := func(s string) bool {
		return slices.ContainsFunc(list, func(item any) bool { return item == s })
	}
	switch x := l.(type) {
	case string:
		return tri(has(x), true)
	case []string:
		return tri(slices.ContainsFunc(x, has), true)
	}
	return Unknown
}

func tri(b, known bool) Tri {
	switch {
	case !known:
		return Unknown
	case b:
		return True
	default:
		return False
	}
}

// Where reports whether an event passes an aggregate's filter, so the store records it.
func Where(ref AggRef, facts Facts) Tri {
	if ref.Where == nil {
		return True
	}
	ev := &evaluator{r: ref.rule, env: Env{Facts: facts}, missing: map[string]bool{}, seen: map[string]any{}}
	return ev.cond(ref.Where)
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
