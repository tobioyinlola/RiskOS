// Package domain holds the alerts module's types and rules: kinds, statuses, severities, the
// triage state machine and grouping.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrNotFound = errors.New("alert not found")
	ErrInvalid  = errors.New("invalid alert change")
	// ErrConflict means the change is not allowed in the alert's current state.
	ErrConflict = errors.New("alert state conflict")
)

// GroupWindow is how long an open alert keeps absorbing new evaluations for the same subject:
// an evaluation within GroupWindow of the alert's last one joins it instead of raising another.
const GroupWindow = time.Hour

// MaxEvaluationsShown bounds the evaluations returned with an alert (newest first).
const MaxEvaluationsShown = 100

// Kind of alert.
type Kind string

// Kinds.
const (
	// KindRisk: an evaluation reached the environment's alert threshold.
	KindRisk Kind = "risk"
	// KindSecurity: evaluations ran with missing signals, which an attacker may induce
	// (ARCHITECTURE.md §16.8).
	KindSecurity Kind = "security"
)

// Subject types.
const (
	SubjectUser        = "user"
	SubjectEnvironment = "environment"
	// SubjectDegraded is the subject reference of security alerts for degraded evaluations.
	SubjectDegraded = "degraded_evaluation"
)

// Status of an alert.
type Status string

// Statuses.
const (
	StatusOpen          Status = "open"
	StatusInvestigating Status = "investigating"
	StatusEscalated     Status = "escalated" // attached to a case; the case decides the outcome
	StatusFalsePositive Status = "false_positive"
	StatusResolved      Status = "resolved"
)

// Terminal reports whether no further triage is possible.
func (s Status) Terminal() bool { return s == StatusFalsePositive || s == StatusResolved }

// Groupable reports whether new evaluations may still join an alert in this status.
func (s Status) Groupable() bool { return s == StatusOpen || s == StatusInvestigating }

// CheckTransition validates a status change made directly on an alert. Escalation happens only by
// creating a case with the alert; an escalated alert is closed by resolving its case.
func CheckTransition(from, to Status) error {
	switch to {
	case StatusOpen, StatusInvestigating, StatusFalsePositive, StatusResolved:
	case StatusEscalated:
		return fmt.Errorf("%w: alerts are escalated by creating a case with them", ErrInvalid)
	default:
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, to)
	}
	switch {
	case from == to:
		return nil
	case from == StatusEscalated:
		return fmt.Errorf("%w: the alert is part of a case; resolve the case instead", ErrConflict)
	case from.Terminal():
		return fmt.Errorf("%w: the alert is already %s", ErrConflict, from)
	default:
		return nil
	}
}

// Severity ranks.
var severities = []string{"low", "medium", "high", "critical"} //nolint:gochecknoglobals // constant table

// SeverityRank returns the rank of a severity (low = 0); unknown values rank lowest.
func SeverityRank(s string) int {
	for i, v := range severities {
		if v == s {
			return i
		}
	}
	return 0
}

// MaxSeverity returns the more severe of a and b.
func MaxSeverity(a, b string) string {
	if SeverityRank(b) > SeverityRank(a) {
		return b
	}
	return a
}

// Title returns the title of an alert of kind with severity.
func Title(kind Kind, severity string) string {
	if kind == KindSecurity {
		return "Risk evaluations ran with missing signals"
	}
	return strings.ToUpper(severity[:1]) + severity[1:] + "-risk activity"
}

// Signal is a signal of a grouped evaluation, as the customer saw it.
type Signal struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
}

// Trigger is an evaluation that may raise alerts: a risk alert when it reached the threshold, a
// security alert when signals were degraded.
type Trigger struct {
	EvaluationID uuid.UUID
	EventID      uuid.UUID
	EventType    string
	UserRef      string
	Score        int
	Level        string
	Decision     string
	Signals      []Signal
	Degraded     []string
	EvaluatedAt  time.Time
	Risk         bool
	Security     bool
}

// Alert is an alert's current state.
type Alert struct {
	ID              uuid.UUID
	Kind            Kind
	SubjectType     string
	SubjectRef      string
	Title           string
	Severity        string
	Status          Status
	Assignee        *string
	CaseID          *uuid.UUID
	MaxScore        int
	EvaluationCount int
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Evaluation is a grouped evaluation's snapshot.
type Evaluation struct {
	EvaluationID uuid.UUID
	EventID      uuid.UUID
	EventType    string
	Score        int
	Level        string
	Decision     string
	Signals      []Signal
	Degraded     []string
	EvaluatedAt  time.Time
}

// Activity is one entry of an alert's insert-only timeline.
type Activity struct {
	ID        uuid.UUID
	Action    string
	Actor     string
	Detail    map[string]any
	CreatedAt time.Time
}

// Detail is an alert with its evaluations and timeline.
type Detail struct {
	Alert       Alert
	Evaluations []Evaluation
	Activity    []Activity
}

// Filter selects alerts for a list.
type Filter struct {
	Status *Status
	Kind   *Kind
	Limit  int
	Before *uuid.UUID
}

// Patch is a triage change. A nil field is unchanged; an empty Assignee unassigns.
type Patch struct {
	Status   *Status
	Assignee *string
	Note     *string
}
