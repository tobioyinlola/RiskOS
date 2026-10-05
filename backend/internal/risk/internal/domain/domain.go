// Package domain holds the risk engine's contract: signals, the score formula, levels and the
// decision policy (ARCHITECTURE.md §10). It has no I/O.
package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Level is the risk level derived from the score.
type Level string

// Levels (risk bands), in ascending order (docs/design-system.md, RISK_ENGINE.md).
const (
	LevelMinimal  Level = "minimal"
	LevelLow      Level = "low"
	LevelElevated Level = "elevated"
	LevelHigh     Level = "high"
	LevelCritical Level = "critical"
)

// Thresholds are the lowest scores of the bands above minimal. They must increase strictly.
type Thresholds struct {
	Low      int `json:"low"`
	Elevated int `json:"elevated"`
	High     int `json:"high"`
	Critical int `json:"critical"`
}

// DefaultThresholds are the product defaults: 0-19 minimal, 20-49 low, 50-69 elevated,
// 70-89 high, 90-100 critical. Tenants can move them.
func DefaultThresholds() Thresholds { return Thresholds{Low: 20, Elevated: 50, High: 70, Critical: 90} }

// Valid reports whether the thresholds increase strictly within 1..100.
func (t Thresholds) Valid() bool {
	return t.Low >= 1 && t.Low < t.Elevated && t.Elevated < t.High && t.High < t.Critical && t.Critical <= MaxScore
}

// Decision is the recommended action. Decisions are advisory: the customer's flow enforces them.
type Decision string

// Decisions. The score policy produces allow, step_up, review and block; regulatory controls of a
// rule pack (ADR-0006) also set the action the law requires.
const (
	DecisionAllow  Decision = "allow"
	DecisionReview Decision = "review"
	DecisionStepUp Decision = "step_up"
	DecisionBlock  Decision = "block"
	// DecisionFee allows the event and charges a regulatory fee (a cash withdrawal charge).
	DecisionFee Decision = "fee"
	// DecisionFlag allows the event and flags it for a required follow-up.
	DecisionFlag Decision = "flag"
	// DecisionRestrict limits the account until a requirement is met (a missing BVN).
	DecisionRestrict Decision = "restrict"
	// DecisionHold holds the funds for review, for at most the control's hold period.
	DecisionHold Decision = "hold"
	// DecisionBlockSession ends the session and requires fresh authentication.
	DecisionBlockSession Decision = "block_session"
	// DecisionDecline declines this transaction (a legal limit).
	DecisionDecline Decision = "decline"
)

// ParseDecision returns the decision named s.
func ParseDecision(s string) (Decision, bool) {
	d := Decision(s)
	return d, decisionRank(d) >= 0
}

// Signal is one explained reason contributing to the score.
type Signal struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
	// Mode is set on restricted signals only: live or shadow.
	Mode string `json:"mode,omitempty"`
}

// The masked form of a compliance-only signal (GOV-09): it explains its points without saying
// what compliance is looking at. Tenant rules cannot use the code.
const (
	SignalComplianceReview = "compliance_review"
	ComplianceReviewLabel  = "Compliance review"
)

// MaxScore caps the score.
const MaxScore = 100

// Score is the sum of signal weights, capped at MaxScore. Every point of the score is therefore
// explained by a listed signal (ARCHITECTURE.md §10).
func Score(signals []Signal) int {
	total := 0
	for _, s := range signals {
		total += s.Weight
		if total >= MaxScore {
			return MaxScore
		}
	}
	return total
}

// LevelForScore maps a score to its band with the given thresholds.
func LevelForScore(score int, t Thresholds) Level {
	switch {
	case score >= t.Critical:
		return LevelCritical
	case score >= t.High:
		return LevelHigh
	case score >= t.Elevated:
		return LevelElevated
	case score >= t.Low:
		return LevelLow
	default:
		return LevelMinimal
	}
}

// DefaultDecision is the default policy without overrides: MINIMAL and LOW allow, ELEVATED goes
// to review, HIGH steps up, CRITICAL goes to review. BLOCK is never a default: auto-block is an
// opt-in with shadow evidence and a kill switch (build step 8).
func DefaultDecision(l Level) Decision {
	switch l {
	case LevelElevated, LevelCritical:
		return DecisionReview
	case LevelHigh:
		return DecisionStepUp
	default:
		return DecisionAllow
	}
}

// Features is the engine's input: the normalized event, as stored with the evaluation so the
// evaluation can be reproduced.
type Features struct {
	EventType  string
	Normalized json.RawMessage
}

// Input describes an evaluation request.
type Input struct {
	Features Features
	EventKey string
	// EventID is the stored event, or nil for a dry run.
	EventID   *uuid.UUID
	RequestID string
}

// Result is a stored evaluation.
type Result struct {
	ID              uuid.UUID
	CreatedAt       time.Time
	EventID         *uuid.UUID
	EngineVersion   string
	Score           int
	Level           Level
	Decision        Decision
	Signals         []Signal
	DegradedSignals []string
	// WarmingSignals were held back while history rebuilds after a feature-store reset.
	WarmingSignals []string
	RequestID      string
	// Explainability and reproducibility (stored, not returned to the customer).
	PolicyReasons []string
	ShadowSignals []Signal
	RuleVersions  []string
	// Controls are the rule pack's regulatory controls that applied to the event (ADR-0006),
	// live or in shadow; NotEvaluated the pack rules it triggered whose data was unavailable.
	Controls     []Control
	NotEvaluated []NotEvaluated
	// Jurisdiction is which country's pack judged the event (ADR-0007); nil without activations.
	Jurisdiction *JurisdictionResolution
	// Restricted are compliance-only signals and controls as they fired (GOV-09). Stored, never
	// returned with the evaluation: only compliance roles read them, through the compliance API.
	Restricted []Signal
	// RestrictedControls are compliance-only controls as they fired (GH-REG-11's FIC order):
	// Controls holds them masked. Not stored here (Restricted names them); the caller records
	// the compliance work they require in the same transaction.
	RestrictedControls []Control
	// RaiseAlert is set when the decision reaches the environment's alert threshold. Not stored:
	// the caller acts on it in the same transaction (dry runs never raise alerts).
	RaiseAlert bool
}

// JurisdictionResolution is which country's pack judged an event and why (ADR-0007).
type JurisdictionResolution struct {
	Jurisdiction string `json:"jurisdiction,omitempty"`
	Pack         string `json:"pack,omitempty"`
	// ResolvedBy is booking_entity, fallback, unresolved or inactive.
	ResolvedBy    string `json:"resolved_by"`
	BookingEntity string `json:"booking_entity,omitempty"`
	// DataSubjectCountry is the event's compliance.data_subject_country when it differs from
	// the governing country; DataSubjectPack the pack whose data-subject duties (RW-SEC-04) ran
	// for it, empty when the tenant has no activation there.
	DataSubjectCountry string `json:"data_subject_country,omitempty"`
	DataSubjectPack    string `json:"data_subject_pack,omitempty"`
}

// Control is a regulatory control of the workspace's rule pack that applied to an event.
type Control struct {
	Rule string `json:"rule"`
	Name string `json:"name"`
	// Mode is live (it set the decision) or shadow (recorded only).
	Mode string `json:"mode"`
	// Decision is the action the control requires; empty for controls that only create tasks.
	Decision Decision `json:"decision,omitempty"`
	Reason   string   `json:"reason"`
	// Tasks are what the institution must do: report.CTR, case.open, edd...
	Tasks []string `json:"tasks"`
	// FeePercent and HoldSeconds detail fee and hold decisions; Branch names the variant that
	// applied.
	FeePercent  float64 `json:"fee_percent,omitempty"`
	HoldSeconds int64   `json:"hold_seconds,omitempty"`
	// HoldUntil is when a hold ends: the event time plus HoldSeconds, or the close of the last
	// working day of a hold counted in working days (GH-REG-11).
	HoldUntil *time.Time `json:"hold_until,omitempty"`
	Branch    string     `json:"branch,omitempty"`
	// Jurisdiction is the country whose pack the control belongs to: the governing one, or the
	// data subject's for a duty that follows the person (RW-SEC-04).
	Jurisdiction string `json:"jurisdiction,omitempty"`
	// ExemptionUnknown is set when the control applied because its exemption could not be
	// checked (the legal limit applies until it is shown).
	ExemptionUnknown bool `json:"exemption_unknown,omitempty"`
}

// NotEvaluated is a pack rule an event triggered that could not be evaluated: what it needed
// was unavailable ("fact:telco.sim_swap", "setting:floor", "history:count ... by card").
type NotEvaluated struct {
	Rule    string   `json:"rule"`
	Missing []string `json:"missing"`
}

// SignalCount is how often a signal fired over a window (analytics).
type SignalCount struct {
	Code  string
	Label string
	Hits  int
}

// ErrBacktestInvalid means a backtest request is malformed; the message says why.
var ErrBacktestInvalid = errors.New("invalid backtest")
