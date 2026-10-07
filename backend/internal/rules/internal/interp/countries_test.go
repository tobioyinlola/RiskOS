package interp_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/gh"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/rw"
)

// The Ghana and Rwanda rules are tested at every numeric threshold of their conditions and
// filters (ADR-0007, phase 3). For each comparison the test builds an event on which that
// comparison alone decides the rule: the rest of the condition is solved so that every other
// part of an "all" holds and every other part of an "any" does not, using only values the
// comparison does not read. The comparison's value is then set just below, at and just above
// its threshold, and the interpreter must fire exactly when the comparison holds; without the
// value the rule must not be evaluated. The thresholds are the pack's defaults, which the
// library check ties to the library's wording.

// countryPack loads a country pack with its Nigerian parent.
func countryPack(t testing.TB, country string) *pack.Pack {
	t.Helper()
	parent, err := pack.Load(ng.Files)
	require.NoError(t, err)
	files := map[string]fs.FS{"GH": gh.Files, "RW": rw.Files}[country]
	p, err := pack.Load(files, parent)
	require.NoError(t, err)
	return p
}

// filled are a rule's settings with each setting the institution must enter given a value in
// its bounds, so the whole condition can run.
func filled(r *pack.Rule) map[string]float64 {
	out := map[string]float64{}
	for _, p := range r.Parameters {
		if p.Default == nil {
			v := math.Round((p.Min + p.Max) / 2)
			if p.Type == pack.ParamRatio {
				v = (p.Min + p.Max) / 2
			}
			out[p.Name] = v
		}
	}
	return out
}

// aggID names a window the way the test's aggregate store answers it: everything that decides
// the stored value.
func aggID(fn, event, by, field, per string, where *pack.Cond, window int64, calendar string, over float64) string {
	w, _ := json.Marshal(where)
	return fmt.Sprintf("agg:%s(%s.%s/%s) by %s @%d%s over %.0f where %s", fn, event, field, per, by, window, calendar, over, w)
}

func refID(ref interp.AggRef) string {
	return aggID(ref.Fn, ref.Event, ref.By, ref.Field, ref.Per, ref.Where, ref.Window, ref.Calendar, ref.Over)
}

// store answers aggregates from the solved values.
type store map[string]any

func (s store) Value(ref interp.AggRef) (any, bool) {
	v, ok := s[refID(ref)]
	return v, ok
}

// solver builds facts and aggregate values that give conditions a wanted truth.
type solver struct {
	r        *pack.Rule
	params   map[string]float64
	currency string // the pack's currency, for amounts compared with no setting
}

func (s *solver) paramValue(name string) (any, bool) {
	v, ok := s.params[name]
	if !ok {
		return nil, false
	}
	for _, p := range s.r.Parameters {
		if p.Name == name && p.Currency != "" {
			return interp.Money{Minor: v, Currency: p.Currency}, true
		}
	}
	return v, true
}

// variable names an operand that is an input (a fact or an aggregate), or "".
func (s *solver) variable(o *pack.Operand) string {
	switch {
	case o.Fact != "":
		return "fact:" + o.Fact
	case o.Agg != nil:
		a := o.Agg
		over := 0.0
		if a.Over != "" {
			factor := 1.0
			if a.OverFactor != "" {
				factor = s.params[a.OverFactor]
			}
			over = math.Round(s.params[a.Over] * factor)
		}
		return aggID(a.Fn, a.Event, a.By, a.Field, a.Per, a.Where, int64(s.params[a.Window]), a.Calendar, over)
	}
	return ""
}

// value is an operand's value under an assignment; ok is false when it is unknown.
func (s *solver) value(o *pack.Operand, asg map[string]any) (any, bool) {
	switch {
	case o.Param != "":
		return s.paramValue(o.Param)
	case o.Literal != nil:
		return o.Literal, true
	case o.Times != nil:
		f, ok := s.params[o.Times.Factor]
		if !ok {
			return nil, false
		}
		of, ok := s.value(&o.Times.Of, asg)
		if !ok {
			return nil, false
		}
		switch x := of.(type) {
		case interp.Money:
			return interp.Money{Minor: math.Round(x.Minor*f*1e6) / 1e6, Currency: x.Currency}, true
		case float64:
			return math.Round(x*f*1e6) / 1e6, true
		}
		return nil, false
	default:
		v, ok := asg[s.variable(o)]
		return v, ok
	}
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case interp.Money:
		return x.Minor, true
	}
	return 0, false
}

// holds is a comparison's truth under an assignment: 1, 0, or -1 when unknown.
func (s *solver) holds(c *pack.Cond, asg map[string]any) int {
	l, lok := s.value(c.Left, asg)
	r, rok := s.value(c.Right, asg)
	if !lok || !rok {
		return -1
	}
	b := func(x bool) int {
		if x {
			return 1
		}
		return 0
	}
	switch c.Op {
	case "eq", "ne":
		eq := fmt.Sprint(l) == fmt.Sprint(r)
		if ls, ok := l.([]string); ok {
			eq = slices.Contains(ls, fmt.Sprint(r))
		}
		return b(eq == (c.Op == "eq"))
	case "in", "not_in":
		list, _ := r.([]any)
		in := slices.ContainsFunc(list, func(x any) bool { return fmt.Sprint(x) == fmt.Sprint(l) })
		return b(in == (c.Op == "in"))
	}
	ln, _ := num(l)
	rn, _ := num(r)
	switch c.Op {
	case "gt":
		return b(ln > rn)
	case "gte":
		return b(ln >= rn)
	case "lt":
		return b(ln < rn)
	case "lte":
		return b(ln <= rn)
	case "multiple_of":
		return b(rn > 0 && math.Mod(ln, rn) == 0)
	}
	return -1
}

// step is the smallest meaningful change of a compared value: one minor unit, one event, one
// second, or a thousandth of a ratio or score.
func (s *solver) step(o *pack.Operand) float64 {
	switch {
	case o.Agg != nil:
		return 1
	case o.Fact != "":
		switch pack.Facts[o.Fact].Type {
		case pack.FactRatio, pack.FactScore, pack.FactSpeed:
			return 0.001
		}
	}
	return 1
}

// base is a value for an unknown operand compared with another unknown one.
func (s *solver) base(o *pack.Operand) any {
	if o.Agg != nil {
		switch o.Agg.Fn {
		case "sum", "daily_avg":
			return interp.Money{Minor: 1_000_000, Currency: s.currency}
		case "exists":
			return true
		}
		return 10.0
	}
	info := pack.Facts[o.Fact]
	switch info.Type {
	case pack.FactMoney:
		return interp.Money{Minor: 1_000_000, Currency: s.currency}
	case pack.FactBool:
		return true
	case pack.FactEnum:
		if len(info.Values) > 0 {
			return info.Values[0]
		}
		return "x"
	}
	return 10.0
}

// choose gives the left operand a value making the comparison want, against the known right
// value r.
func (s *solver) choose(c *pack.Cond, r any, want bool) (any, bool) {
	other := "__not_this__"
	switch c.Op {
	case "eq", "ne":
		if (c.Op == "eq") == want {
			return r, true
		}
		switch x := r.(type) {
		case bool:
			return !x, true
		case float64:
			return x + 1, true
		}
		return other, true
	case "in", "not_in":
		list, _ := r.([]any)
		if (c.Op == "in") == want && len(list) > 0 {
			return list[0], true
		}
		return other, true
	}
	rn, ok := num(r)
	if !ok {
		return nil, false
	}
	d := s.step(c.Left)
	var v float64
	switch c.Op {
	case "gt":
		v = map[bool]float64{true: rn + d, false: rn}[want]
	case "gte":
		v = map[bool]float64{true: rn, false: rn - d}[want]
	case "lt":
		v = map[bool]float64{true: rn - d, false: rn}[want]
	case "lte":
		v = map[bool]float64{true: rn, false: rn + d}[want]
	case "multiple_of":
		v = map[bool]float64{true: rn * 3, false: rn*3 + d}[want]
	default:
		return nil, false
	}
	if m, ok := r.(interp.Money); ok {
		return interp.Money{Minor: v, Currency: m.Currency}, true
	}
	return v, true
}

// solve finds an assignment extending asg under which c has the truth want, leaving reserved
// inputs unset, and passes it to k; it backtracks when k fails.
func (s *solver) solve(c *pack.Cond, want bool, asg map[string]any, reserved string, k func(map[string]any) bool) bool {
	switch {
	case len(c.All) > 0 || len(c.Any) > 0:
		list, every := c.All, want
		if len(c.Any) > 0 {
			list, every = c.Any, !want
		}
		if every {
			// Every child must have the truth want.
			var chain func(i int, a map[string]any) bool
			chain = func(i int, a map[string]any) bool {
				if i == len(list) {
					return k(a)
				}
				return s.solve(&list[i], want, a, reserved, func(b map[string]any) bool { return chain(i+1, b) })
			}
			return chain(0, asg)
		}
		for i := range list {
			if s.solve(&list[i], want, asg, reserved, k) {
				return true
			}
		}
		return false
	case c.Not != nil:
		return s.solve(c.Not, !want, asg, reserved, k)
	}
	switch s.holds(c, asg) {
	case 1:
		return want && k(asg)
	case 0:
		return !want && k(asg)
	}
	lv := s.variable(c.Left)
	if _, known := asg[lv]; known || lv == "" {
		return s.solveRight(c, want, maps.Clone(asg), reserved, k)
	}
	// An unknown operand on the right gets a base value first, at a few sizes.
	o := c.Right
	if o.Times != nil {
		o = &o.Times.Of
	}
	rv := s.variable(o)
	if _, known := asg[rv]; rv == "" || known {
		return s.chooseLeft(c, want, maps.Clone(asg), reserved, k)
	}
	if rv == reserved {
		return false
	}
	for _, scale := range []float64{1, 1000, 1_000_000} {
		a := maps.Clone(asg)
		switch b := s.base(o).(type) {
		case interp.Money:
			a[rv] = interp.Money{Minor: b.Minor * scale, Currency: b.Currency}
		case float64:
			a[rv] = b * scale
		default:
			a[rv] = b
		}
		if s.chooseLeft(c, want, a, reserved, k) {
			return true
		}
	}
	return false
}

// chooseLeft, with the right operand known, gives the unknown left input a value.
func (s *solver) chooseLeft(c *pack.Cond, want bool, a map[string]any, reserved string, k func(map[string]any) bool) bool {
	lv := s.variable(c.Left)
	r, ok := s.value(c.Right, a)
	if lv == reserved || !ok {
		return false
	}
	v, ok := s.choose(c, r, want)
	if !ok {
		return false
	}
	a[lv] = v
	return k(a)
}

// solveRight, with the left operand known, picks the unknown input on the right: the right
// operand itself or the operand a ratio multiplies.
func (s *solver) solveRight(c *pack.Cond, want bool, asg map[string]any, reserved string, k func(map[string]any) bool) bool {
	o, factor := c.Right, 1.0
	if c.Right.Times != nil {
		o, factor = &c.Right.Times.Of, s.params[c.Right.Times.Factor]
	}
	rv := s.variable(o)
	l, ok := s.value(c.Left, asg)
	if rv == "" || rv == reserved || !ok || factor == 0 {
		return false
	}
	if _, known := asg[rv]; known {
		return false
	}
	ln, ok := num(l)
	if !ok {
		return false
	}
	x := ln / factor
	for _, v := range []float64{x, math.Floor(x) - 1, math.Ceil(x) + 1, x / 2, x * 2, 0} {
		a := maps.Clone(asg)
		switch b := s.base(o).(type) {
		case interp.Money:
			a[rv] = interp.Money{Minor: v, Currency: b.Currency}
		case float64:
			a[rv] = v
		default:
			return false
		}
		if h := s.holds(c, a); h >= 0 && (h == 1) == want && k(a) {
			return true
		}
	}
	return false
}

// goal is a condition and the truth it must have.
type goal struct {
	c    *pack.Cond
	want bool
}

func (s *solver) solveAll(goals []goal, asg map[string]any, reserved string) (map[string]any, bool) {
	// Parts that must hold have one way to; parts that must not hold usually have several, so
	// they are solved last, around what the others fixed.
	goals = slices.Clone(goals)
	slices.SortStableFunc(goals, func(a, b goal) int {
		switch {
		case a.want == b.want:
			return 0
		case a.want:
			return -1
		}
		return 1
	})
	var out map[string]any
	var chain func(i int, a map[string]any) bool
	chain = func(i int, a map[string]any) bool {
		if i == len(goals) {
			out = a
			return true
		}
		return s.solve(goals[i].c, goals[i].want, a, reserved, func(b map[string]any) bool { return chain(i+1, b) })
	}
	return out, chain(0, asg)
}

// rightInputs gives the inputs a comparison's threshold reads a base value, if unset.
func (s *solver) rightInputs(c *pack.Cond, asg map[string]any) {
	for _, o := range []*pack.Operand{c.Right, timesOf(c.Right)} {
		if o == nil {
			continue
		}
		if v := s.variable(o); v != "" {
			if _, ok := asg[v]; !ok {
				asg[v] = s.base(o)
			}
		}
	}
}

func timesOf(o *pack.Operand) *pack.Operand {
	if o.Times == nil {
		return nil
	}
	return &o.Times.Of
}

// decisive gives, for each value of a comparison's input just below, at and just above its
// threshold, an event on which the comparison alone decides. It first solves the other parts
// without the input, so one event serves every value; when they read the same input (the two
// edges of a band) it solves them again around each value, keeping the values they allow.
func (s *solver) decisive(l leaf, goals []goal) map[string]map[string]any {
	lv := s.variable(l.c.Left)
	out := map[string]map[string]any{}
	if asg, ok := s.solveAll(goals, map[string]any{}, lv); ok {
		s.rightInputs(l.c, asg)
		for _, v := range s.variants(l.c, asg) {
			a := maps.Clone(asg)
			a[lv] = v
			out[fmt.Sprint(v)] = a
		}
		return out
	}
	start := map[string]any{}
	s.rightInputs(l.c, start)
	for _, v := range s.variants(l.c, start) {
		a := maps.Clone(start)
		a[lv] = v
		if solved, ok := s.solveAll(goals, a, ""); ok {
			out[fmt.Sprint(v)] = solved
		}
	}
	return out
}

// leaf is a comparison with the goals that make it decide its tree, and whether the tree's
// truth equals the comparison's (an odd number of "not" above it reverses it).
type leaf struct {
	c     *pack.Cond
	path  string
	goals []goal
	same  bool
}

// leaves lists a tree's comparisons, each with the sibling goals that make it decisive.
func leaves(c *pack.Cond, path string, goals []goal, same bool, out *[]leaf) {
	switch {
	case len(c.All) > 0 || len(c.Any) > 0:
		list, sibling, name := c.All, true, "all"
		if len(c.Any) > 0 {
			list, sibling, name = c.Any, false, "any"
		}
		for i := range list {
			g := slices.Clone(goals)
			for j := range list {
				if j != i {
					// Siblings of an "all" hold, siblings of an "any" do not; under a "not" the
					// tree's truth is reversed but the siblings' role is the same.
					g = append(g, goal{&list[j], sibling})
				}
			}
			leaves(&list[i], fmt.Sprintf("%s.%s[%d]", path, name, i), g, same, out)
		}
	case c.Not != nil:
		leaves(c.Not, path+".not", goals, !same, out)
	default:
		*out = append(*out, leaf{c: c, path: path, goals: goals, same: same})
	}
}

// condition is a rule's condition as one tree: its branches are alternatives.
func condition(r *pack.Rule) *pack.Cond {
	if len(r.Branches) > 0 {
		c := &pack.Cond{}
		for _, b := range r.Branches {
			c.Any = append(c.Any, b.Condition)
		}
		return c
	}
	return r.Condition
}

// numeric reports whether a comparison has a threshold to test either side of.
func numeric(c *pack.Cond) bool {
	return slices.Contains([]string{"gt", "gte", "lt", "lte"}, c.Op)
}

// variants are the values just below, at and just above a comparison's threshold, or the
// values that make it hold and not hold.
func (s *solver) variants(c *pack.Cond, asg map[string]any) []any {
	r, ok := s.value(c.Right, asg)
	if !ok {
		return nil
	}
	if numeric(c) {
		rn, _ := num(r)
		d := s.step(c.Left)
		var out []any
		for _, v := range []float64{rn - d, rn, rn + d} {
			if v < 0 {
				continue
			}
			if m, ok := r.(interp.Money); ok {
				out = append(out, interp.Money{Minor: v, Currency: m.Currency})
			} else {
				out = append(out, v)
			}
		}
		return out
	}
	yes, _ := s.choose(c, r, true)
	no, _ := s.choose(c, r, false)
	return []any{yes, no}
}

func split(asg map[string]any) (interp.Facts, store) {
	facts, aggs := interp.Facts{}, store{}
	for k, v := range asg {
		if name, ok := strings.CutPrefix(k, "fact:"); ok {
			facts[name] = v
		} else {
			aggs[k] = v
		}
	}
	return facts, aggs
}

// countryCoverage counts, per rule, the statuses its cases reached.
type countryCoverage map[string]map[interp.Status]bool

func (cc countryCoverage) add(id string, s interp.Status) {
	if cc[id] == nil {
		cc[id] = map[interp.Status]bool{}
	}
	cc[id][s] = true
}

// gates are the settings a condition compares with a literal ("use your own limit" = 1), set
// to that literal: the parts behind a switch that is off by default are tested with it on.
func gates(c *pack.Cond, out map[string]float64) {
	if c == nil {
		return
	}
	for i := range c.All {
		gates(&c.All[i], out)
	}
	for i := range c.Any {
		gates(&c.Any[i], out)
	}
	gates(c.Not, out)
	if c.Left != nil && c.Left.Param != "" && c.Op == "eq" {
		if v, ok := c.Right.Literal.(float64); ok {
			out[c.Left.Param] = v
		}
	}
}

// testCountryRule returns the comparisons no event could isolate, with the switches off or on.
func testCountryRule(t *testing.T, p *pack.Pack, def *pack.Rule, cov countryCoverage) []string {
	tested := map[string]bool{}
	all := testCountryRuleWith(t, p, def, cov, nil, tested)
	on := map[string]float64{}
	gates(condition(def), on)
	if len(on) > 0 {
		testCountryRuleWith(t, p, def, cov, on, tested)
	}
	var untested []string
	for _, path := range all {
		if !tested[path] {
			untested = append(untested, def.ID+" "+path)
		}
	}
	return untested
}

func testCountryRuleWith(t *testing.T, p *pack.Pack, def *pack.Rule, cov countryCoverage, on map[string]float64, tested map[string]bool) (paths []string) {
	params := filled(def)
	maps.Copy(params, on)
	compiled := interp.Compile(def, params)
	s := &solver{r: def, params: map[string]float64{}, currency: p.Locale().Currency}
	for name := range params {
		s.params[name] = params[name]
	}
	for _, prm := range def.Parameters {
		if prm.Default != nil {
			if _, set := s.params[prm.Name]; !set {
				s.params[prm.Name] = *prm.Default
			}
		}
	}
	kinds := def.Trigger.Events[:1]
	run := func(asg map[string]any) interp.Result {
		facts, aggs := split(asg)
		return interp.Evaluate(compiled, interp.Env{Kinds: kinds, Facts: facts, Aggs: aggs})
	}
	cond := condition(def)
	if cond == nil {
		// A workflow clock with no condition fires on its trigger.
		assert.Equal(t, interp.StatusFired, run(map[string]any{}).Status, def.ID)
		cov.add(def.ID, interp.StatusFired)
		return nil
	}
	var frame []goal
	if def.Trigger.Scope != nil {
		frame = append(frame, goal{def.Trigger.Scope, true})
	}
	if def.Outcome.Unless != nil {
		frame = append(frame, goal{def.Outcome.Unless, false})
	}
	var ls []leaf
	leaves(cond, "condition", nil, true, &ls)
	n := 0
	for _, l := range ls {
		lv := s.variable(l.c.Left)
		if lv == "" {
			continue
		}
		paths = append(paths, l.path)
		cases := s.decisive(l, append(slices.Clone(frame), l.goals...))
		if len(cases) < 2 {
			continue
		}
		n++
		tested[l.path] = true
		name := def.ID + "/" + l.path
		if on != nil {
			name += fmt.Sprintf(" with %v", on)
		}
		t.Run(name, func(t *testing.T) {
			var some map[string]any
			for _, v := range slices.Sorted(maps.Keys(cases)) {
				a := cases[v]
				some = a
				h := s.holds(l.c, a)
				require.GreaterOrEqual(t, h, 0)
				want := interp.StatusClear
				if (h == 1) == l.same {
					want = interp.StatusFired
				}
				res := run(a)
				assert.Equal(t, want, res.Status, "%s = %v (missing %v)", lv, v, res.Missing)
				cov.add(def.ID, res.Status)
			}
			// Without the compared value the rule is not evaluated, never clear.
			a := maps.Clone(some)
			delete(a, lv)
			res := run(a)
			assert.Equal(t, interp.StatusNotEvaluated, res.Status, "without %s", lv)
			assert.NotEmpty(t, res.Missing)
			cov.add(def.ID, res.Status)
		})
	}
	if on == nil {
		assert.Positive(t, n, "%s: no comparison could be tested on its own", def.ID)
	}
	// With nothing known the rule is not evaluated.
	if res := run(map[string]any{}); res.Status == interp.StatusNotEvaluated {
		cov.add(def.ID, res.Status)
	}
	return paths
}

// testCountryFilters checks every numeric comparison in the rule's window filters (the "just
// under the limit" bands among them) just below, at and just above its edge.
func testCountryFilters(t *testing.T, p *pack.Pack, def *pack.Rule) int {
	params := filled(def)
	compiled := interp.Compile(def, params)
	s := &solver{r: def, params: map[string]float64{}, currency: p.Locale().Currency}
	for _, prm := range def.Parameters {
		if prm.Default != nil {
			s.params[prm.Name] = *prm.Default
		}
	}
	maps.Copy(s.params, params)
	n := 0
	for _, ref := range compiled.Aggregates() {
		if ref.Where == nil {
			continue
		}
		var ls []leaf
		leaves(ref.Where, "where", nil, true, &ls)
		for _, l := range ls {
			lv := s.variable(l.c.Left)
			if !numeric(l.c) || !strings.HasPrefix(lv, "fact:") {
				continue
			}
			cases := s.decisive(l, l.goals)
			n++
			t.Run(def.ID+"/"+ref.Fn+" "+ref.Event+"/"+l.path, func(t *testing.T) {
				require.Len(t, cases, 3, "below, at and above the edge of %s", lv)
				var some map[string]any
				for _, v := range slices.Sorted(maps.Keys(cases)) {
					a := cases[v]
					some = a
					facts, _ := split(a)
					want := interp.False
					if (s.holds(l.c, a) == 1) == l.same {
						want = interp.True
					}
					assert.Equal(t, want, interp.Where(ref, facts), "%s = %v", lv, v)
				}
				a := maps.Clone(some)
				delete(a, lv)
				facts, _ := split(a)
				assert.Equal(t, interp.Unknown, interp.Where(ref, facts), "without %s", lv)
			})
		}
	}
	return n
}

// branchOnly are the comparisons that only choose between branches.
var branchOnly = map[string][]string{ //nolint:gochecknoglobals // test table
	"GH": nil,
	"RW": {
		"RW-SEC-01 condition.any[2].all[0]", "RW-SEC-01 condition.any[2].all[1]",
		"RW-SEC-04 condition.any[0].all[0]", "RW-SEC-04 condition.any[0].all[1]",
	},
}

func TestCountryRulesAtEveryThreshold(t *testing.T) {
	for _, country := range []string{"GH", "RW"} {
		t.Run(country, func(t *testing.T) {
			p := countryPack(t, country)
			cov := countryCoverage{}
			filters := 0
			var untested []string
			for i := range p.Rules {
				def := &p.Rules[i]
				if def.Kind == pack.KindGovernance || def.InheritedFrom != "" {
					// Governance presets are workflows (phase 5); inherited rules are the
					// Nigerian rules, tested in rules_test.go, with local amounts (below).
					continue
				}
				untested = append(untested, testCountryRule(t, p, def, cov)...)
				filters += testCountryFilters(t, p, def)
			}
			for i := range p.Rules {
				def := &p.Rules[i]
				if def.Kind == pack.KindGovernance || def.InheritedFrom != "" || condition(def) == nil {
					continue
				}
				for _, s := range []interp.Status{interp.StatusFired, interp.StatusClear, interp.StatusNotEvaluated} {
					assert.True(t, cov[def.ID][s], "%s has no %s case", def.ID, s)
				}
			}
			assert.Positive(t, filters)
			// A comparison no event can isolate is one inside a branch that a broader branch
			// also matches: it decides which branch applies (the decision and tasks), not
			// whether the rule fires. countryBranchCases covers each.
			assert.ElementsMatch(t, branchOnly[country], untested)
		})
	}
}

// The Nigerian rules a country inherits run on its own currency: their money settings are in
// cedis or francs, and a naira amount is never compared with them.
func TestInheritedRulesRunOnLocalAmounts(t *testing.T) {
	for _, tc := range []struct{ country, currency string }{{"GH", "GHS"}, {"RW", "RWF"}} {
		p := countryPack(t, tc.country)
		n := 0
		for i := range p.Rules {
			def := &p.Rules[i]
			if def.InheritedFrom == "" {
				continue
			}
			n++
			for _, prm := range def.Parameters {
				if prm.Type == pack.ParamMoney {
					assert.Contains(t, []string{tc.currency, "USD"}, prm.Currency, "%s %s", def.ID, prm.Name)
				}
			}
		}
		assert.Positive(t, n, tc.country)
	}
}
