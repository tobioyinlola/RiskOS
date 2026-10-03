package pack

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// ErrInvalid wraps every validation failure, each with the path to the problem.
var ErrInvalid = errors.New("invalid rule pack")

var ( //nolint:gochecknoglobals // compiled once, never mutated
	idPattern   = regexp.MustCompile(`^(REG|ATO|VEL|MUL|BEH|CNP|LEN|SEC|INS|SIM|CRY|PON|GOV)-[0-9]{2}$`)
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)
)

// kindFor is the kind a rule ID's prefix implies.
func kindFor(prefix string) Kind {
	switch prefix {
	case "REG":
		return KindControl
	case "GOV":
		return KindGovernance
	default:
		return KindSignal
	}
}

func oneOf(v string, values ...string) bool { return slices.Contains(values, v) }

// Bounds on a condition tree, as for tenant rules (RISK_ENGINE.md).
const (
	maxDepth = 8
	maxNodes = 120
)

// Validate checks a whole pack: metadata, every rule, and cross-references.
func Validate(p *Pack) error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...))
	}
	if p.ID == "" || p.Version == "" || p.Jurisdiction == "" || p.Name == "" {
		fail("pack: id, version, jurisdiction and name are required")
	}
	ids := func(n int, id func(int) string, what string) map[string]bool {
		m := map[string]bool{}
		for i := range n {
			if id(i) == "" || m[id(i)] {
				fail("%s[%d]: missing or duplicate id %q", what, i, id(i))
			}
			m[id(i)] = true
		}
		return m
	}
	groups := ids(len(p.Groups), func(i int) string { return p.Groups[i].ID }, "groups")
	sources := ids(len(p.Sources), func(i int) string { return p.Sources[i].ID }, "sources")
	licences := ids(len(p.LicenceTypes), func(i int) string { return p.LicenceTypes[i].ID }, "licence_types")
	capabilities := ids(len(p.Capabilities), func(i int) string { return p.Capabilities[i].ID }, "capabilities")
	ruleIDs := ids(len(p.Rules), func(i int) string { return p.Rules[i].ID }, "rules")
	for _, c := range p.Clocks {
		for _, r := range c.Rules {
			if !ruleIDs[r] {
				fail("clocks.%s: unknown rule %s", c.ID, r)
			}
		}
		for _, s := range c.Sources {
			if !sources[s] {
				fail("clocks.%s: unknown source %s", c.ID, s)
			}
		}
		if c.Hours <= 0 || c.Duty == "" || c.Requirement == "" {
			fail("clocks.%s: hours, duty and requirement are required", c.ID)
		}
	}
	for _, g := range p.Gaps {
		for _, r := range g.Rules {
			if !ruleIDs[r] {
				fail("gaps.%s: unknown rule %s", g.ID, r)
			}
		}
		for _, s := range g.Sources {
			if !sources[s] {
				fail("gaps.%s: unknown source %s", g.ID, s)
			}
		}
	}
	for i, t := range p.Glossary {
		if t.Term == "" || t.Definition == "" {
			fail("glossary[%d]: term and definition are required", i)
		}
	}
	for i := range p.Rules {
		v := ruleValidator{r: &p.Rules[i], fail: fail, used: map[string]bool{}}
		v.validate(groups, sources, licences, capabilities)
	}
	return errors.Join(errs...)
}

type ruleValidator struct {
	r     *Rule
	fail  func(format string, args ...any)
	used  map[string]bool
	nodes int
}

func (v *ruleValidator) errf(format string, args ...any) {
	v.fail("%s: "+format, append([]any{v.r.ID}, args...)...)
}

func (v *ruleValidator) validate(groups, sources, licences, capabilities map[string]bool) {
	r := v.r
	if !idPattern.MatchString(r.ID) {
		v.errf("id must look like REG-01")
		return
	}
	prefix := r.ID[:3]
	wantKind := kindFor(prefix)
	wantStatus := map[Kind]Status{KindControl: StatusRegulatory, KindSignal: StatusRecommended, KindGovernance: StatusGovernance}[wantKind]
	if r.Kind != wantKind || r.Status != wantStatus {
		v.errf("kind and status must be %s and %s", wantKind, wantStatus)
	}
	if !groups[r.Group] {
		v.errf("unknown group %q", r.Group)
	}
	if len(r.Regulators) == 0 {
		v.errf("at least one regulator is required")
	}
	for _, g := range r.Regulators {
		if !Regulators[g] {
			v.errf("unknown regulator %q", g)
		}
	}
	if len(r.Sources) == 0 {
		v.errf("at least one legal or evidence source is required")
	}
	for _, s := range r.Sources {
		if !sources[s] {
			v.errf("unknown source %q", s)
		}
	}
	for _, l := range r.AppliesTo {
		if !licences[l] {
			v.errf("unknown licence type %q", l)
		}
	}
	for _, c := range r.Requires {
		if !capabilities[c] {
			v.errf("unknown capability %q", c)
		}
	}
	if r.Effective.Text == "" {
		v.errf("effective.text is required")
	}
	if r.Effective.Date != "" {
		if _, err := time.Parse(time.DateOnly, r.Effective.Date); err != nil {
			v.errf("effective.date must be YYYY-MM-DD")
		}
	}
	switch r.Verification {
	case VerificationConfirmed, VerificationSecondaryOnly:
	case VerificationConflicting, VerificationAwaitingRegular:
		if r.Caveat == "" {
			v.errf("a %s rule needs a caveat", r.Verification)
		}
	default:
		v.errf("unknown verification %q", r.Verification)
	}
	if r.Verification == VerificationAwaitingRegular && r.Defaults.Enabled {
		v.errf("a rule awaiting the regulator must be off by default")
	}
	if r.Status == StatusRegulatory && r.Verification != VerificationConfirmed && r.Caveat == "" {
		v.errf("an unconfirmed regulatory control needs a caveat")
	}
	if r.Defaults.Mode != ModeLive && r.Defaults.Mode != ModeShadow {
		v.errf("defaults.mode must be live or shadow")
	}
	if len(r.Inputs) == 0 {
		v.errf("inputs are required")
	}
	for _, in := range r.Inputs {
		if _, ok := Inputs[in]; !ok {
			v.errf("unknown input %q", in)
		}
	}
	v.parameters()
	switch r.Kind {
	case KindGovernance:
		v.governance()
	case KindControl, KindSignal:
		v.evaluated()
	}
	for _, p := range r.Parameters {
		if !v.used[p.Name] {
			v.errf("parameter %q is never used", p.Name)
		}
	}
}

func (v *ruleValidator) parameters() {
	seen := map[string]bool{}
	for _, p := range v.r.Parameters {
		path := "parameters." + p.Name
		if !namePattern.MatchString(p.Name) || seen[p.Name] {
			v.errf("%s: missing, malformed or duplicate name", path)
		}
		seen[p.Name] = true
		if p.Label == "" {
			v.errf("%s: a plain-English label is required", path)
		}
		lo, hi := p.Min, p.Max
		switch p.Type {
		case ParamMoney:
			if p.Currency != "NGN" && p.Currency != "USD" {
				v.errf("%s: currency must be NGN or USD", path)
			}
		case ParamPoints:
			if lo < 0 || hi > 100 {
				v.errf("%s: points are 0 to 100", path)
			}
		case ParamFlag:
			if lo < 0 || hi > 1 {
				v.errf("%s: a flag is 0 or 1", path)
			}
		case ParamHour:
			if lo < 0 || hi > 24 {
				v.errf("%s: an hour is 0 to 24", path)
			}
		case ParamPercent, ParamScore:
			if lo < 0 || hi > 100 {
				v.errf("%s: 0 to 100", path)
			}
		case ParamDuration, ParamCount, ParamRatio, ParamSpeed:
			if lo < 0 {
				v.errf("%s: cannot be negative", path)
			}
		default:
			v.errf("%s: unknown type %q", path, p.Type)
		}
		if p.Type != ParamMoney && p.Currency != "" {
			v.errf("%s: only money has a currency", path)
		}
		if lo > hi {
			v.errf("%s: min is above max", path)
		}
		if p.Default != nil && (*p.Default < lo || *p.Default > hi) {
			v.errf("%s: the default is outside its bounds", path)
		}
	}
}

func (v *ruleValidator) param(name, path string) (Parameter, bool) {
	for _, p := range v.r.Parameters {
		if p.Name == name {
			v.used[name] = true
			return p, true
		}
	}
	v.errf("%s: unknown parameter %q", path, name)
	return Parameter{}, false
}

func (v *ruleValidator) paramOfType(name, path string, t ParamType) {
	if p, ok := v.param(name, path); ok && p.Type != t {
		v.errf("%s: parameter %q must be of type %s", path, name, t)
	}
}

func (v *ruleValidator) governance() {
	r := v.r
	if r.Governance == nil || !oneOf(r.Governance.Type, "policy", "job", "control", "workflow", "sla") {
		v.errf("governance.type must be policy, job, control, workflow or sla")
	}
	if r.Trigger != nil || r.Condition != nil || len(r.Branches) > 0 || r.Outcome.Decision != "" || r.Outcome.Weight != "" {
		v.errf("governance presets have no trigger, condition, decision or weight")
	}
	if r.Outcome.Reason == "" {
		v.errf("outcome.reason is required")
	}
	if r.Governance != nil {
		for _, name := range r.Governance.Uses {
			v.param(name, "governance.uses")
		}
	}
}

func (v *ruleValidator) evaluated() {
	r := v.r
	if r.Governance != nil {
		v.errf("only governance presets have governance")
	}
	if r.Trigger == nil || len(r.Trigger.Events) == 0 {
		v.errf("trigger.events is required")
	} else {
		for _, e := range r.Trigger.Events {
			if _, ok := Events[e]; !ok {
				v.errf("trigger: unknown event %q", e)
			}
		}
		if r.Trigger.Scope != nil {
			v.cond(*r.Trigger.Scope, "trigger.scope", 1)
		}
	}
	o := r.Outcome
	if o.Reason == "" {
		v.errf("outcome.reason is required")
	}
	if r.Condition != nil && len(r.Branches) > 0 {
		v.errf("use either condition or branches, not both")
	}
	if r.Condition != nil {
		v.cond(*r.Condition, "condition", 1)
	}
	for i, b := range r.Branches {
		path := fmt.Sprintf("branches[%d]", i)
		if b.Label == "" {
			v.errf("%s: a label is required", path)
		}
		v.cond(b.Condition, path+".condition", 1)
		switch r.Kind {
		case KindSignal:
			v.paramOfType(b.Weight, path+".weight", ParamPoints)
		case KindControl:
			v.paramOfType(b.Fee, path+".fee", ParamPercent)
		}
	}
	tasksOnly := len(o.Tasks) > 0
	for i, t := range o.Tasks {
		path := fmt.Sprintf("outcome.tasks[%d]", i)
		if _, ok := Tasks[t.Type]; !ok {
			v.errf("%s: unknown task %q", path, t.Type)
		}
		if t.Due != "" {
			v.paramOfType(t.Due, path+".due", ParamDuration)
		}
		if t.On != "" {
			if _, ok := Events[t.On]; !ok {
				v.errf("%s: unknown event %q", path, t.On)
			}
		} else {
			tasksOnly = false
		}
	}
	if r.Condition == nil && len(r.Branches) == 0 && !tasksOnly {
		v.errf("a condition or branches are required (unless every task has its own trigger)")
	}
	for _, h := range o.Hints {
		if !Hints[h] {
			v.errf("outcome: unknown hint %q", h)
		}
	}
	if o.Visibility != "" && o.Visibility != "compliance_only" {
		v.errf("outcome.visibility must be compliance_only or empty")
	}
	if o.Unless != nil {
		v.cond(*o.Unless, "outcome.unless", 1)
	}
	switch r.Kind {
	case KindControl:
		v.control()
	case KindSignal:
		v.signal()
	}
}

func (v *ruleValidator) control() {
	o := v.r.Outcome
	if o.Decision == "" && len(o.Tasks) == 0 {
		v.errf("a control sets a decision or creates a task")
	}
	if o.Decision != "" && !Decisions[o.Decision] {
		v.errf("outcome: unknown decision %q", o.Decision)
	}
	if o.Weight != "" || o.MinDecision != "" || o.Severity != "" {
		v.errf("controls have no weight, minimum decision or severity")
	}
	switch o.Decision {
	case "hold":
		v.paramOfType(o.Hold, "outcome.hold", ParamDuration)
	case "fee":
		if o.Fee != "" {
			v.paramOfType(o.Fee, "outcome.fee", ParamPercent)
		} else if len(v.r.Branches) == 0 {
			v.errf("a fee control needs outcome.fee or branches with fees")
		}
	}
}

func (v *ruleValidator) signal() {
	o := v.r.Outcome
	if o.Decision != "" || o.Unless != nil || o.Hold != "" || o.Fee != "" {
		v.errf("signals add weight; they do not set a decision")
	}
	if len(v.r.Branches) == 0 {
		v.paramOfType(o.Weight, "outcome.weight", ParamPoints)
	} else if o.Weight != "" {
		v.errf("a branched signal takes its weights from the branches")
	}
	if o.MinDecision != "" && !MinDecisions[o.MinDecision] {
		v.errf("outcome.min_decision must be review or step_up")
	}
	if !oneOf(o.Severity, "low", "medium", "high") {
		v.errf("outcome.severity must be low, medium or high")
	}
}

func (v *ruleValidator) cond(c Cond, path string, depth int) {
	v.nodes++
	if depth > maxDepth || v.nodes > maxNodes {
		v.errf("%s: the condition is too deep or too large", path)
		return
	}
	set := 0
	for _, b := range []bool{len(c.All) > 0, len(c.Any) > 0, c.Not != nil, c.Op != ""} {
		if b {
			set++
		}
	}
	if set != 1 {
		v.errf("%s: exactly one of all, any, not or a comparison", path)
		return
	}
	for i, sub := range c.All {
		v.cond(sub, fmt.Sprintf("%s.all[%d]", path, i), depth+1)
	}
	for i, sub := range c.Any {
		v.cond(sub, fmt.Sprintf("%s.any[%d]", path, i), depth+1)
	}
	if c.Not != nil {
		v.cond(*c.Not, path+".not", depth+1)
	}
	if c.Op != "" {
		v.compare(c, path)
	}
}

// operandType is an operand's value type: a fact type, or a parameter type mapped onto one.
type operandType struct {
	name    string
	fact    FactType
	param   ParamType
	literal any
	isParam bool
	isLit   bool
	ok      bool
}

func (v *ruleValidator) operand(o *Operand, path string, depth int) operandType {
	if o == nil {
		v.errf("%s: missing operand", path)
		return operandType{}
	}
	set := 0
	for _, b := range []bool{o.Fact != "", o.Param != "", o.Literal != nil, o.Agg != nil, o.Times != nil} {
		if b {
			set++
		}
	}
	if set != 1 {
		v.errf("%s: exactly one of fact, param, literal, agg or times", path)
		return operandType{}
	}
	switch {
	case o.Fact != "":
		return v.factType(o.Fact, path)
	case o.Param != "":
		p, ok := v.param(o.Param, path)
		return operandType{param: p.Type, isParam: true, ok: ok}
	case o.Literal != nil:
		return operandType{literal: o.Literal, isLit: true, ok: true}
	case o.Agg != nil:
		return v.agg(*o.Agg, path+".agg", depth)
	default:
		v.paramOfType(o.Times.Factor, path+".times.factor", ParamRatio)
		of := v.operand(&o.Times.Of, path+".times.of", depth)
		if of.isLit {
			v.errf("%s: cannot multiply a literal", path)
		}
		return of
	}
}

func (v *ruleValidator) factType(name, path string) operandType {
	f, ok := Facts[name]
	if !ok {
		v.errf("%s: unknown fact %q", path, name)
		return operandType{}
	}
	if !slices.Contains(v.r.Inputs, f.Input) {
		v.errf("%s: fact %q comes from input %q, which the rule does not list", path, name, f.Input)
	}
	return operandType{name: name, fact: f.Type, ok: true}
}

func (v *ruleValidator) agg(a Aggregate, path string, depth int) operandType {
	fn, ok := AggFns[a.Fn]
	if !ok {
		v.errf("%s: unknown aggregation %q", path, a.Fn)
		return operandType{}
	}
	if !slices.Contains(v.r.Inputs, "history") {
		v.errf("%s: aggregations need the history input", path)
	}
	if _, ok := Events[a.Event]; !ok {
		v.errf("%s: unknown event %q", path, a.Event)
	}
	if !Entities[a.By] {
		v.errf("%s: unknown entity %q", path, a.By)
	}
	v.paramOfType(a.Window, path+".window", ParamDuration)
	switch {
	case fn.Field == "" && a.Field != "":
		v.errf("%s: %s takes no field", path, a.Fn)
	case fn.Field != "":
		if t := v.factType(a.Field, path+".field"); t.ok && t.fact != fn.Field {
			v.errf("%s: %s needs a %s field", path, a.Fn, fn.Field)
		}
	}
	if (a.Fn == "max_distinct_per_value") != (a.Per != "") {
		v.errf("%s: per is used by max_distinct_per_value only, which needs it", path)
	}
	if a.Per != "" {
		v.factType(a.Per, path+".per")
	}
	if a.Where != nil {
		v.cond(*a.Where, path+".where", depth+1)
	}
	return operandType{fact: fn.Result, ok: true}
}

func (v *ruleValidator) compare(c Cond, path string) {
	left := v.operand(c.Left, path+".left", 1)
	right := v.operand(c.Right, path+".right", 1)
	if !left.ok || !right.ok {
		return
	}
	if left.isLit {
		v.errf("%s: a literal belongs on the right", path)
		return
	}
	switch {
	case oneOf(c.Op, "gt", "gte", "lt", "lte", "multiple_of"):
		v.numeric(left, right, path, c.Op)
	case c.Op == "eq" || c.Op == "ne":
		v.equality(left, right, path)
	case c.Op == "in" || c.Op == "not_in":
		v.membership(left, right, path)
	default:
		v.errf("%s: unknown operator %q", path, c.Op)
	}
}

func numericFact(t FactType) bool {
	return t == FactMoney || t == FactDuration || t == FactCount || t == FactRatio || t == FactHour ||
		t == FactSpeed || t == FactScore
}

// numeric checks a threshold comparison: never against a literal number (thresholds are
// parameters), and only between the same kind of quantity.
func (v *ruleValidator) numeric(left, right operandType, path, op string) {
	if right.isLit {
		v.errf("%s: compare against a parameter, not a literal number", path)
		return
	}
	lt := left.fact
	if left.isParam {
		v.errf("%s: put the fact or aggregate on the left", path)
		return
	}
	if !numericFact(lt) {
		v.errf("%s: %s is not a number", path, lt)
		return
	}
	if op == "multiple_of" && lt != FactMoney {
		v.errf("%s: multiple_of applies to money", path)
	}
	if right.isParam {
		if !slices.Contains(paramFits(lt), right.param) {
			v.errf("%s: a %s cannot be compared with a %s parameter", path, lt, right.param)
		}
		return
	}
	if right.fact != lt {
		v.errf("%s: a %s cannot be compared with a %s", path, lt, right.fact)
	}
}

func (v *ruleValidator) equality(left, right operandType, path string) {
	switch {
	case left.isParam && left.param == ParamFlag:
		if n, ok := right.literal.(float64); !right.isLit || !ok || (n != 0 && n != 1) {
			v.errf("%s: a flag is compared with 0 or 1", path)
		}
	case left.isParam:
		v.errf("%s: only flags are compared for equality", path)
	case left.fact == FactBool:
		if _, ok := right.literal.(bool); !right.isLit || !ok {
			v.errf("%s: a yes/no fact is compared with true or false", path)
		}
	case left.fact == FactEnum:
		if s, ok := right.literal.(string); !right.isLit || !ok || !slices.Contains(Facts[left.name].Values, s) {
			v.errf("%s: %s is compared with one of its values %v", path, left.name, Facts[left.name].Values)
		}
	default:
		v.errf("%s: use gt, gte, lt or lte for numbers", path)
	}
}

func (v *ruleValidator) membership(left, right operandType, path string) {
	if left.fact != FactEnum {
		v.errf("%s: in and not_in apply to choices", path)
		return
	}
	list, ok := right.literal.([]any)
	if !right.isLit || !ok || len(list) == 0 || len(list) > 100 {
		v.errf("%s: compare with a list of 1 to 100 values", path)
		return
	}
	for _, item := range list {
		if s, ok := item.(string); !ok || !slices.Contains(Facts[left.name].Values, s) {
			v.errf("%s: %v is not one of %s's values %v", path, item, left.name, Facts[left.name].Values)
		}
	}
}
