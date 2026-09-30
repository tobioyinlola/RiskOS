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

// Levels.
const (
	LevelLow      Level = "low"
	LevelMedium   Level = "medium"
	LevelHigh     Level = "high"
	LevelCritical Level = "critical"
)

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

// LevelForScore maps a score to its level with the product-default bands: 0-29 low,
// 30-59 medium, 60-79 high, 80-100 critical. Tenant-specific bands arrive with build step 4.
func LevelForScore(score int) Level {
	switch {
	case score >= 80:
		return LevelCritical
	case score >= 60:
		return LevelHigh
	case score >= 30:
		return LevelMedium
	default:
		return LevelLow
	}
}

// DefaultDecision is the design's default policy without overrides: LOW and MEDIUM allow, HIGH
// steps up, CRITICAL goes to review. BLOCK is never a default (opt-in only, build step 4).
func DefaultDecision(l Level) Decision {
	switch l {
	case LevelHigh:
		return DecisionStepUp
	case LevelCritical:
		return DecisionReview
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
}
