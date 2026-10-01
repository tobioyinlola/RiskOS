// Package domain holds the risk engine's contract: signals, the score formula, levels and the
// decision policy (ARCHITECTURE.md §10). It has no I/O.
package domain

import (
	"encoding/json"
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

// Decisions.
const (
	DecisionAllow  Decision = "allow"
	DecisionReview Decision = "review"
	DecisionStepUp Decision = "step_up"
	DecisionBlock  Decision = "block"
)

// Signal is one explained reason contributing to the score.
type Signal struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
}

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
	RequestID       string
	// Explainability and reproducibility (stored, not returned to the customer).
	PolicyReasons []string
	ShadowSignals []Signal
	RuleVersions  []string
	// RaiseAlert is set when the decision reaches the environment's alert threshold. Not stored:
	// the caller acts on it in the same transaction (dry runs never raise alerts).
	RaiseAlert bool
}

// SignalCount is how often a signal fired over a window (analytics).
type SignalCount struct {
	Code  string
	Label string
	Hits  int
}
