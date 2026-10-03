package domain

import (
	"slices"
	"time"
)

// decisionRank orders decisions by severity; -1 for an unknown decision. Fee and flag allow the
// event with an action; restrict, hold, block_session, decline and block stop or limit it.
func decisionRank(d Decision) int {
	return slices.Index([]Decision{
		DecisionAllow, DecisionFee, DecisionFlag, DecisionStepUp, DecisionReview, DecisionRestrict,
		DecisionHold, DecisionBlockSession, DecisionDecline, DecisionBlock,
	}, d)
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

// AutoBlockMode is a tenant environment's automatic blocking (ARCHITECTURE.md decision 12).
type AutoBlockMode string

// Automatic blocking modes.
const (
	// AutoBlockOff: Critical scores get REVIEW (the default).
	AutoBlockOff AutoBlockMode = "off"
	// AutoBlockShadow: Critical scores get REVIEW and are marked would_block, as evidence.
	AutoBlockShadow AutoBlockMode = "shadow"
	// AutoBlockOn: Critical scores get BLOCK.
	AutoBlockOn AutoBlockMode = "on"
)

// AutoBlockShadowPeriod is how long an environment gathers shadow evidence before blocking can
// be switched on (an assumption to confirm with customers, decision 12).
const AutoBlockShadowPeriod = 14 * 24 * time.Hour

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
	// AlertMinDecision is the least severe decision that raises an alert: DecisionReview
	// (default) or DecisionStepUp.
	AlertMinDecision Decision
	// Thresholds are the band boundaries.
	Thresholds Thresholds
	// AutoBlock is the automatic blocking mode; AutoBlockSince is when shadow evidence started
	// (nil when off). An empty mode in an update keeps the current one.
	AutoBlock      AutoBlockMode
	AutoBlockSince *time.Time
}

// AutoBlockEligibleAt returns when blocking can be switched on, or nil when it is off.
func (s Settings) AutoBlockEligibleAt() *time.Time {
	if s.AutoBlockSince == nil {
		return nil
	}
	t := s.AutoBlockSince.Add(AutoBlockShadowPeriod)
	return &t
}

// DecisionFor returns the decision a level gets before any override: the default table, with
// BLOCK for Critical when automatic blocking is on.
func (s Settings) DecisionFor(l Level) Decision {
	if l == LevelCritical && s.AutoBlock == AutoBlockOn {
		return DecisionBlock
	}
	return DefaultDecision(l)
}

// RaisesAlert reports whether decision d reaches the environment's alert threshold.
func (s Settings) RaisesAlert(d Decision) bool {
	floor := s.AlertMinDecision
	if floor == "" {
		floor = DecisionReview
	}
	return decisionRank(d) >= decisionRank(floor)
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
		AlertMinDecision:    DecisionReview,
		Thresholds:          DefaultThresholds(),
		AutoBlock:           AutoBlockOff,
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
	// DenyListed reports whether an identifier of the event is on the tenant's deny list.
	DenyListed bool
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
//   - degraded and high-value: at least REVIEW whatever the fail policy (induced-degradation guard);
//   - CRITICAL with automatic blocking in shadow: REVIEW, marked would_block; on: BLOCK;
//   - an identifier on the tenant's deny list: BLOCK, whatever the score.
//
// BLOCK is produced only by a deny-list match or by automatic blocking, a per-environment opt-in
// behind 14 days of shadow evidence with a kill switch (decision 12). Named signals explain it.
func Decide(in PolicyInput) PolicyResult {
	d := DefaultDecision(in.Level)
	var reasons []string

	switch in.Level {
	case LevelLow:
		// A low score with a signal step-up directly addresses (a new device, a new country, a
		// recent credential change) is challenged rather than allowed.
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
	if in.Level == LevelCritical {
		switch in.Settings.AutoBlock {
		case AutoBlockShadow:
			reasons = append(reasons, "would_block")
		case AutoBlockOn:
			d = DecisionBlock
			reasons = append(reasons, "auto_block")
		case AutoBlockOff:
		}
	}
	// A deny-list match blocks whatever the score: the tenant named this identifier.
	if in.DenyListed {
		d = DecisionBlock
		reasons = append(reasons, "deny_list")
	}
	slices.Sort(reasons)
	return PolicyResult{Decision: d, Reasons: slices.Compact(reasons)}
}
