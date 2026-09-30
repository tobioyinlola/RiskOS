package domain

import "slices"

// decisionRank orders decisions by severity.
func decisionRank(d Decision) int {
	switch d {
	case DecisionStepUp:
		return 1
	case DecisionReview:
		return 2
	case DecisionBlock:
		return 3
	default:
		return 0
	}
}

// AtLeast returns the more severe of d and floor.
func AtLeast(d, floor Decision) Decision {
	if decisionRank(floor) > decisionRank(d) {
		return floor
	}
	return d
}

// FailPolicy is what a degraded evaluation falls back to.
type FailPolicy string

// Fail policies (ARCHITECTURE.md §12).
const (
	FailOpen   FailPolicy = "open"   // allow, with degraded_signals listed
	FailClosed FailPolicy = "closed" // at least REVIEW
)

// Settings are a tenant environment's risk configuration.
type Settings struct {
	// Weights overrides built-in signal weights (0..MaxSignalWeight).
	Weights map[SignalCode]int
	// HighValueDefault is the high-value threshold in minor units for currencies without their
	// own entry in HighValueByCurrency.
	HighValueDefault    int64
	HighValueByCurrency map[string]int64
	// FailPolicies by event type; types without an entry use FailClosed.
	FailPolicies map[string]FailPolicy
}

// DefaultHighValue is the placeholder high-value threshold (minor units) until a tenant sets its
// own per currency.
const DefaultHighValue int64 = 1_000_000

// DefaultSettings returns the product defaults: spec weights, a 1,000,000 minor-unit high-value
// threshold, fail-closed for transactions and fail-open for logins (ARCHITECTURE.md §12).
func DefaultSettings() Settings {
	return Settings{
		Weights:             map[SignalCode]int{},
		HighValueDefault:    DefaultHighValue,
		HighValueByCurrency: map[string]int64{},
		FailPolicies:        map[string]FailPolicy{"transaction": FailClosed, "login": FailOpen},
	}
}

// Weight returns the effective weight of a built-in signal.
func (s Settings) Weight(spec SignalSpec) int {
	if w, ok := s.Weights[spec.Code]; ok {
		return w
	}
	return spec.DefaultWeight
}

// HighValueThreshold returns the threshold for currency.
func (s Settings) HighValueThreshold(currency string) int64 {
	if t, ok := s.HighValueByCurrency[currency]; ok {
		return t
	}
	return s.HighValueDefault
}

// FailPolicyFor returns the fail policy of an event type.
func (s Settings) FailPolicyFor(eventType string) FailPolicy {
	if p, ok := s.FailPolicies[eventType]; ok {
		return p
	}
	return FailClosed
}

// stepUpCapable lists event types whose flow can challenge the user (e.g. with a one-time code).
// For other types STEP-UP is not actionable, so HIGH goes to REVIEW.
func stepUpCapable(eventType string) bool {
	return eventType == "login" || eventType == "transaction"
}

// PolicyInput is everything the decision policy looks at.
type PolicyInput struct {
	EventType string
	Level     Level
	Signals   []Signal
	Degraded  []string
	// HighValue reports whether the event's amount is at or above the tenant's threshold.
	HighValue bool
	Settings  Settings
}

// PolicyResult is the decision and why it deviated from the default table.
type PolicyResult struct {
	Decision Decision
	// Reasons lists the overrides applied, e.g. "degraded_high_value_floor". Stored with the
	// evaluation for explainability.
	Reasons []string
}

// Decide applies the decision policy (ARCHITECTURE.md §10, §12, §16.8):
//
//   - default table: LOW/MEDIUM allow, HIGH step-up, CRITICAL review;
//   - MEDIUM with a strengthenable signal: STEP-UP;
//   - HIGH on an event type that cannot be stepped up: REVIEW;
//   - degraded evaluation: the event type's fail policy (closed: at least REVIEW);
//   - degraded and high-value: at least REVIEW whatever the fail policy (induced-degradation guard).
//
// BLOCK is never produced: automatic blocking is a per-tenant opt-in behind shadow-mode evidence
// (decision 12) and arrives with its guardrails in build step 8.
func Decide(in PolicyInput) PolicyResult {
	d := DefaultDecision(in.Level)
	var reasons []string

	switch in.Level {
	case LevelMedium:
		for _, s := range in.Signals {
			if spec, ok := BuiltinSignal(s.Code); ok && spec.Strengthenable {
				d = DecisionStepUp
				reasons = append(reasons, "strengthenable_signal")
				break
			}
		}
	case LevelHigh:
		if !stepUpCapable(in.EventType) {
			d = DecisionReview
			reasons = append(reasons, "step_up_not_available")
		}
	}

	if len(in.Degraded) > 0 {
		if in.Settings.FailPolicyFor(in.EventType) == FailClosed && decisionRank(d) < decisionRank(DecisionReview) {
			d = DecisionReview
			reasons = append(reasons, "fail_closed")
		}
		if in.HighValue && decisionRank(d) < decisionRank(DecisionReview) {
			d = DecisionReview
			reasons = append(reasons, "degraded_high_value_floor")
		}
	}
	slices.Sort(reasons)
	return PolicyResult{Decision: d, Reasons: slices.Compact(reasons)}
}
