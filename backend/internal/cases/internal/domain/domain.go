// Package domain holds the cases module's types and rules.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrNotFound = errors.New("case not found")
	ErrInvalid  = errors.New("invalid case")
	// ErrConflict means the change is not allowed in the case's current state.
	ErrConflict = errors.New("case state conflict")
)

// Bounds.
const (
	MaxTitle          = 200
	MaxDescription    = 5000
	MaxAssignee       = 254
	MaxResolutionNote = 5000
	MaxNote           = 10000
	MaxAlertsOnCreate = 50
)

// Status of a case.
type Status string

// Statuses.
const (
	StatusOpen          Status = "open"
	StatusInvestigating Status = "investigating"
	StatusResolved      Status = "resolved" // final
)

// Resolution of a case.
type Resolution string

// Resolutions.
const (
	ResolutionConfirmedFraud Resolution = "confirmed_fraud"
	ResolutionFalsePositive  Resolution = "false_positive"
	ResolutionLegitimate     Resolution = "legitimate"
	ResolutionInconclusive   Resolution = "inconclusive"
)

// Valid reports whether r is a known resolution.
func (r Resolution) Valid() bool {
	switch r {
	case ResolutionConfirmedFraud, ResolutionFalsePositive, ResolutionLegitimate, ResolutionInconclusive:
		return true
	default:
		return false
	}
}

// ValidPriority reports whether p is a known priority (the risk band scale).
func ValidPriority(p string) bool {
	switch p {
	case "minimal", "low", "elevated", "high", "critical":
		return true
	default:
		return false
	}
}

// DefaultPriority is a case's priority without alerts or an explicit choice.
const DefaultPriority = "elevated"

// Case is a case's current state.
type Case struct {
	ID             uuid.UUID
	Title          string
	Description    string
	Status         Status
	Priority       string
	Assignee       *string
	Resolution     *Resolution
	ResolutionNote *string
	ResolvedAt     *time.Time
	ResolvedBy     *string
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      time.Time
}

// Note is an append-only case note.
type Note struct {
	ID        uuid.UUID
	Body      string
	Author    string
	CreatedAt time.Time
}

// Activity is an entry of a case's insert-only timeline.
type Activity struct {
	ID        uuid.UUID
	Action    string
	Actor     string
	Detail    map[string]any
	CreatedAt time.Time
}

// CreateInput describes a new case. An empty Priority is derived from the alerts.
type CreateInput struct {
	Title       string
	Description string
	Priority    string
	Assignee    string
	AlertIDs    []uuid.UUID
}

// Patch changes a case. A nil field is unchanged; an empty Assignee unassigns.
type Patch struct {
	Title          *string
	Description    *string
	Priority       *string
	Assignee       *string
	Status         *Status
	Resolution     *Resolution
	ResolutionNote *string
}

// Filter selects cases for a list.
type Filter struct {
	Status *Status
	Limit  int
	Before *uuid.UUID
}
