// Package pack defines jurisdiction rule packs (ADR-0006): declarative, versioned rule
// definitions with their legal source, plain-English text and bounded parameters. Definitions
// are data. They are validated when loaded and interpreted by the engine; nothing in a pack is
// ever executed as code.
package pack

// Kind is what a rule is.
type Kind string

// Kinds.
const (
	// KindControl is a regulatory hard control: it sets a decision or creates a task directly.
	KindControl Kind = "control"
	// KindSignal is a risk-scoring rule: it adds weight to the score.
	KindSignal Kind = "signal"
	// KindGovernance is a process requirement on how monitoring is run.
	KindGovernance Kind = "governance"
)

// Status is where a rule's authority comes from.
type Status string

// Statuses.
const (
	StatusRegulatory  Status = "regulatory"
	StatusRecommended Status = "recommended"
	StatusGovernance  Status = "governance"
)

// Verification is how far a rule's legal facts have been confirmed.
type Verification string

// Verification statuses.
const (
	VerificationConfirmed       Verification = "confirmed"
	VerificationSecondaryOnly   Verification = "secondary_source_only"
	VerificationConflicting     Verification = "conflicting"
	VerificationAwaitingRegular Verification = "awaiting_regulator"
)

// Mode is how a rule runs for a tenant.
type Mode string

// Modes.
const (
	ModeLive   Mode = "live"
	ModeShadow Mode = "shadow"
)

// Provenance is where a rule comes from, as the library tags it (ADR-0007).
type Provenance string

// Provenance tags.
const (
	// ProvenanceRegulatory follows a cited law, regulation or directive.
	ProvenanceRegulatory Provenance = "regulatory"
	// ProvenanceIndustry is a published typology or practice with no mandated number.
	ProvenanceIndustry Provenance = "industry_practice"
	// ProvenanceDesign is the library author's proposal: numbers are starting values, not law.
	ProvenanceDesign Provenance = "my_design"
)

// OffReason is why a rule ships switched off.
type OffReason string

// Reasons a rule is off by default.
const (
	// OffThresholdToConfirm: a number the rule needs was not found; it has a placeholder.
	OffThresholdToConfirm OffReason = "threshold_to_be_confirmed"
	// OffVerifyFirst: the legal basis may no longer apply and must be checked first.
	OffVerifyFirst OffReason = "verify_first"
)

// Pack is a jurisdiction pack: its metadata and its rules.
type Pack struct {
	ID           string `json:"id"`
	Jurisdiction string `json:"jurisdiction"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	// Currency and Timezone are the jurisdiction's (GHS, Africa/Accra): money settings are in
	// Currency or US dollars, and windows and hours are kept in Timezone. Packs that predate
	// them (ng) take them from DefaultLocale.
	Currency string `json:"currency,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	// Library names the library document the pack's wording is copied from.
	Library      string    `json:"library,omitempty"`
	Groups       []Group   `json:"groups"`
	Sources      []Source  `json:"sources"`
	Clocks       []Clock   `json:"clocks"`
	Gaps         []Gap     `json:"gaps"`
	Glossary     []Term    `json:"glossary"`
	LicenceTypes []Licence `json:"licence_types"`
	Capabilities []Licence `json:"capabilities"`
	// Penalties is the penalty exposure register the library lists, for display only.
	Penalties []Penalty `json:"penalties,omitempty"`
	// Inherits lists the rules of other packs that apply in this jurisdiction under their own
	// IDs, with settings changed for the country (ADR-0007). The loader adds them to Rules.
	Inherits []Inherit `json:"inherits,omitempty"`
	Rules    []Rule    `json:"-"`
	// Checksum is the SHA-256 of the pack's files: a version's content never changes.
	Checksum string `json:"-"`
}

// Locale is a jurisdiction's currency and time zone.
type Locale struct {
	Currency, Timezone string
}

// DefaultLocale is the locale of packs that predate the currency and timezone fields. Only
// Nigeria's first pack versions omit them.
func DefaultLocale(jurisdiction string) (Locale, bool) {
	if jurisdiction == "NG" {
		return Locale{Currency: "NGN", Timezone: "Africa/Lagos"}, true
	}
	return Locale{}, false
}

// Locale returns the pack's currency and time zone.
func (p *Pack) Locale() Locale {
	if p.Currency != "" {
		return Locale{Currency: p.Currency, Timezone: p.Timezone}
	}
	l, _ := DefaultLocale(p.Jurisdiction)
	return l
}

// Inherit is a rule of another pack that applies here unchanged in logic.
type Inherit struct {
	Pack string `json:"pack"`
	Rule string `json:"rule"`
	// AppliesTo replaces the parent's licence types with this pack's (empty: every institution).
	AppliesTo []string `json:"applies_to,omitempty"`
	// Overrides changes settings for the country: money settings must be restated in this
	// pack's currency.
	Overrides map[string]Override `json:"overrides,omitempty"`
	// Note says what changes for the country, in plain words.
	Note string `json:"note,omitempty"`
}

// Override changes one setting of a templated or inherited rule.
type Override struct {
	Label    string   `json:"label,omitempty"`
	Currency string   `json:"currency,omitempty"`
	Default  *float64 `json:"default,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
}

// Penalty is one line of the penalty exposure register.
type Penalty struct {
	Provision string `json:"provision"`
	Penalty   string `json:"penalty"`
	// Estimate is the amount in the local currency where the library computes one; it is an
	// inference when Inference is set (Ghana penalty units at GHS 12).
	Estimate  string   `json:"estimate,omitempty"`
	Inference bool     `json:"inference,omitempty"`
	Sources   []string `json:"sources"`
}

// Group is a family of rules as the library lists them.
type Group struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Source is a reference in the library's source list (section 9).
type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Links []Link `json:"links"`
}

// Link is a titled URL.
type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Licence is a type of institution a rule can apply to.
type Licence struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ClockKind says what a clock's duration rests on.
type ClockKind string

// Clock kinds.
const (
	// ClockLegal is a deadline a law or regulator sets.
	ClockLegal ClockKind = "legal"
	// ClockPolicy is the institution's own target, inside or in place of a legal clock.
	ClockPolicy ClockKind = "internal_policy"
	// ClockPlaceholder is a design default standing in for a legal deadline not yet found.
	ClockPlaceholder ClockKind = "design_placeholder"
	// ClockNotFound is a deadline the law sets but no readable source states: it has no hours.
	ClockNotFound ClockKind = "not_found"
)

// Clock is a deadline that becomes a product timer (section 8; ADR-0007 section 6).
type Clock struct {
	ID          string   `json:"id"`
	Duration    string   `json:"duration"`
	Duty        string   `json:"duty"`
	AppliesTo   string   `json:"applies_to"`
	Requirement string   `json:"requirement"`
	Sources     []string `json:"sources"`
	Rules       []string `json:"rules"`
	// Hours is nil for a clock that was not found: shown as "not found", never as zero.
	Hours *int `json:"hours"`
	// Kind is legal unless stated (packs before ADR-0007 list legal clocks only).
	Kind ClockKind `json:"kind,omitempty"`
	// Authority is the body the clock runs against (FIC, BoG, NCSA).
	Authority string `json:"authority,omitempty"`
}

// ClockKindOf returns the clock's kind, legal when unstated.
func (c *Clock) ClockKindOf() ClockKind {
	if c.Kind == "" {
		return ClockLegal
	}
	return c.Kind
}

// Gap is an open question from section 9 that a person must settle.
type Gap struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Detail  string   `json:"detail"`
	Rules   []string `json:"rules"`
	Sources []string `json:"sources"`
	// Kind is "conflict" (two sources disagree) or "unverified" (not found or not confirmed);
	// empty for packs before ADR-0007.
	Kind string `json:"kind,omitempty"`
	// Treatment is how the library handles it until a person settles it.
	Treatment string `json:"treatment,omitempty"`
}

// Term is a glossary entry.
type Term struct {
	Term       string `json:"term"`
	Expansion  string `json:"expansion"`
	Definition string `json:"definition"`
}

// Text is a rule's wording, copied verbatim from the library (sections 7 and 7B).
type Text struct {
	// Name is the plain-English name (section 7).
	Name string `json:"name"`
	// Description is "When it applies" / "What we look for" / "What it requires".
	Description string `json:"description"`
	// Action is "What the system does" for controls; empty for the others.
	Action string `json:"action,omitempty"`
	// Points is the "Points" column for risk-scoring rules ("30", "10 or 20").
	Points string `json:"points,omitempty"`
	// EngineeringLogic is the rule's section 7B block, for display to engineers only.
	EngineeringLogic string `json:"engineering_logic"`
	// Basis is the library's "Tag, lineage, default, basis" column, verbatim (ADR-0007 packs).
	Basis string `json:"basis,omitempty"`
}

// Rule is one machine rule definition.
type Rule struct {
	ID         string   `json:"id"`
	Group      string   `json:"group"`
	Kind       Kind     `json:"kind"`
	Status     Status   `json:"status"`
	Regulators []string `json:"regulators"`
	// AppliesTo lists licence types; empty means every institution.
	AppliesTo []string `json:"applies_to,omitempty"`
	// Requires lists tenant capabilities that must be on (for example instant_payments).
	Requires     []string     `json:"requires,omitempty"`
	Sources      []string     `json:"sources"`
	Effective    Effective    `json:"effective"`
	Verification Verification `json:"verification"`
	// Caveat is the plain-English note shown wherever the rule is (section 9).
	Caveat   string   `json:"caveat,omitempty"`
	Defaults Defaults `json:"defaults"`
	// Inputs are the data the rule needs (providers); a missing one means "not evaluated".
	Inputs     []string    `json:"inputs"`
	Trigger    *Trigger    `json:"trigger,omitempty"`
	Condition  *Cond       `json:"condition,omitempty"`
	Branches   []Branch    `json:"branches,omitempty"`
	Outcome    Outcome     `json:"outcome"`
	Governance *Governance `json:"governance,omitempty"`
	Parameters []Parameter `json:"parameters"`
	// Provenance, Lineage and OffReason are the library's tags (ADR-0007): required for rules
	// with a country prefix (GH-TM-01), absent for earlier packs.
	Provenance Provenance `json:"provenance,omitempty"`
	Lineage    *Lineage   `json:"lineage,omitempty"`
	OffReason  OffReason  `json:"off_reason,omitempty"`
	// Template is the parent rule whose logic a "Param of X" rule reuses. The loader copies the
	// parent's inputs, trigger, condition, branches, outcome, governance and parameters, then
	// applies Overrides; the rule file states only what differs.
	Template  *Ref                `json:"template,omitempty"`
	Overrides map[string]Override `json:"overrides,omitempty"`
	// InheritedFrom is the pack an inherited rule comes from; it keeps the parent's ID.
	InheritedFrom string `json:"inherited_from,omitempty"`
	// Text is filled from the pack's text file, never written in the rule files.
	Text Text `json:"-"`
}

// Ref names a rule of a pack.
type Ref struct {
	Pack string `json:"pack"`
	Rule string `json:"rule"`
}

// Lineage is how a rule relates to another pack's rules.
type Lineage struct {
	// Kind is param (country-parameterised version), analogue, cross_reference, stricter or
	// new_local.
	Kind string `json:"kind"`
	// Of lists the related rules (ATO-01, REG-12 and REG-18).
	Of []string `json:"of,omitempty"`
}

// Effective is when the rule's legal basis took effect. Date is set only when the library
// states a full date; Text always says what is known ("2022 (month not stated)").
type Effective struct {
	Date string `json:"date,omitempty"`
	Text string `json:"text"`
}

// Defaults are how the rule starts for a tenant.
type Defaults struct {
	Enabled bool `json:"enabled"`
	Mode    Mode `json:"mode"`
}

// Trigger is when the rule is evaluated.
type Trigger struct {
	Events []string `json:"events"`
	// Scope limits who is checked (for example the channel); exclusions are NOT conditions.
	Scope *Cond `json:"scope,omitempty"`
}

// Branch is one of several variants of a rule. For a signal the matching branch with the most
// points counts; for a control the first matching branch does.
type Branch struct {
	Condition Cond `json:"condition"`
	// Weight (signals) or Fee (controls) is the parameter this branch uses.
	Weight string `json:"weight,omitempty"`
	Fee    string `json:"fee,omitempty"`
	// Decision and Tasks, for a control, replace the outcome's when this branch matches: RURA
	// wants critical incidents now and moderate ones monthly (ADR-0007).
	Decision string `json:"decision,omitempty"`
	Tasks    []Task `json:"tasks,omitempty"`
	Label    string `json:"label"`
}

// Outcome is what happens when the rule applies.
type Outcome struct {
	// Decision is set by controls: decline, block, block_session, step_up, hold, review,
	// restrict, flag or fee.
	Decision string `json:"decision,omitempty"`
	// Unless, for controls, names a condition that exempts the event (REG-04's raised limit).
	Unless *Cond `json:"unless,omitempty"`
	// Hold is the parameter holding the maximum hold, for decision hold.
	Hold string `json:"hold,omitempty"`
	// Fee is the parameter holding the fee rate, for decision fee.
	Fee string `json:"fee,omitempty"`
	// Weight is the parameter holding a signal's points.
	Weight string `json:"weight,omitempty"`
	// MinDecision is the least decision a signal also requires (review, step_up).
	MinDecision string   `json:"min_decision,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	Reason      string   `json:"reason"`
	Tasks       []Task   `json:"tasks,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
	// Hints are suggested responses for the customer's systems (rate_limit_ip, revoke_session).
	Hints []string `json:"hints,omitempty"`
	// Visibility "compliance_only" hides the signal from non-compliance roles (GOV-09).
	Visibility string `json:"visibility,omitempty"`
}

// Task is a workflow item a rule creates.
type Task struct {
	Type string `json:"type"`
	// On is the event that creates the task; empty means the rule's trigger.
	On string `json:"on,omitempty"`
	// Due is the parameter holding the deadline.
	Due string   `json:"due,omitempty"`
	To  []string `json:"to,omitempty"`
}

// Governance describes a governance preset.
type Governance struct {
	// Type is policy, job, control, workflow or sla.
	Type     string `json:"type"`
	Schedule string `json:"schedule,omitempty"`
	// Uses names the parameters the preset's workflow reads (a retention period, a deadline).
	Uses []string `json:"uses,omitempty"`
}

// ParamType is a parameter's value type.
type ParamType string

// Parameter types.
const (
	// ParamMoney is an amount in minor units of Currency (kobo for NGN, pesewas for GHS,
	// whole francs for RWF, cents for USD).
	ParamMoney ParamType = "money"
	// ParamDuration is a duration in seconds.
	ParamDuration ParamType = "duration"
	// ParamCount is a whole number of things.
	ParamCount ParamType = "count"
	// ParamRatio is a dimensionless factor (0.8, 3, 0.999).
	ParamRatio ParamType = "ratio"
	// ParamPoints is a signal weight, 0 to 100.
	ParamPoints ParamType = "points"
	// ParamPercent is a fee or rate as a percentage.
	ParamPercent ParamType = "percent"
	// ParamSpeed is a speed in km/h.
	ParamSpeed ParamType = "speed"
	// ParamHour is an hour of the day, 0 to 24, in the jurisdiction's time zone.
	ParamHour ParamType = "hour"
	// ParamScore is a similarity score, 0 to 100.
	ParamScore ParamType = "score"
	// ParamFlag is a yes/no setting: 0 or 1.
	ParamFlag ParamType = "flag"
)

// Parameter is a named, bounded setting.
type Parameter struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Type     ParamType `json:"type"`
	Currency string    `json:"currency,omitempty"`
	// Default, Min and Max are in the type's unit; nil Default means "set by the institution".
	Default *float64 `json:"default"`
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
}

// Cond is a condition tree node: exactly one of All, Any, Not or a comparison.
type Cond struct {
	All   []Cond   `json:"all,omitempty"`
	Any   []Cond   `json:"any,omitempty"`
	Not   *Cond    `json:"not,omitempty"`
	Left  *Operand `json:"left,omitempty"`
	Op    string   `json:"op,omitempty"`
	Right *Operand `json:"right,omitempty"`
}

// Operand is one side of a comparison: exactly one of Fact, Param, Literal, Agg or Times.
type Operand struct {
	Fact    string     `json:"fact,omitempty"`
	Param   string     `json:"param,omitempty"`
	Literal any        `json:"literal,omitempty"`
	Agg     *Aggregate `json:"agg,omitempty"`
	Times   *Times     `json:"times,omitempty"`
}

// Times multiplies an operand by a ratio parameter: "0.80 * SUM(inflow)".
type Times struct {
	Factor string  `json:"factor"`
	Of     Operand `json:"of"`
}

// Aggregate is a windowed aggregation over one entity's events.
type Aggregate struct {
	// Fn is count, sum, daily_avg, distinct_count, distinct_count_once, max_distinct_per_value
	// or exists.
	Fn string `json:"fn"`
	// Event is the event kind aggregated over.
	Event string `json:"event"`
	// Field is the fact summed or counted distinctly.
	Field string `json:"field,omitempty"`
	// Per, for max_distinct_per_value, is the fact whose equal values are grouped.
	Per   string `json:"per,omitempty"`
	Where *Cond  `json:"where,omitempty"`
	// Window is the parameter holding the window.
	Window string `json:"window"`
	// By is the entity the window is kept for.
	By string `json:"by"`
}
