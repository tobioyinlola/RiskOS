package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errors of rule change approval.
var (
	ErrChangeNotFound = errors.New("rule change not found")
	ErrChangePending  = errors.New("a change to this rule is already awaiting approval")
	ErrChangeDecided  = errors.New("the change has already been decided")
	ErrChangeStale    = errors.New("the rule has changed since this change was proposed; propose it again")
	// ErrSelfApproval: the person who proposed a change cannot approve it (two-person rule).
	ErrSelfApproval = errors.New("a change must be approved by someone other than its proposer")
	// ErrApproverNotPerson: approvals are made by people signed in to the console, not keys.
	ErrApproverNotPerson = errors.New("rule changes are approved by a person in the console")
)

// ChangeKind is what a proposed rule change does.
type ChangeKind string

// Change kinds.
const (
	ChangeCreate   ChangeKind = "create"
	ChangeUpdate   ChangeKind = "update"
	ChangeRollback ChangeKind = "rollback"
)

// ChangeStatus is where a proposed change stands.
type ChangeStatus string

// Change statuses.
const (
	ChangePending  ChangeStatus = "pending"
	ChangeApproved ChangeStatus = "approved"
	ChangeRejected ChangeStatus = "rejected"
)

// RuleChange is a proposed change to a production rule awaiting a second person (four-eyes,
// ARCHITECTURE.md §10). Proposed holds the full content the rule will have; its Version,
// CreatedAt and CreatedBy are set when the change is applied.
type RuleChange struct {
	ID     uuid.UUID
	RuleID uuid.UUID
	Code   string
	Kind   ChangeKind
	// BaseVersion is the rule's current version when proposed (nil for a new rule).
	BaseVersion    *int
	Proposed       RuleVersion
	Status         ChangeStatus
	ProposedBy     string
	ProposedAt     time.Time
	DecidedBy      *string
	DecidedAt      *time.Time
	Reason         *string
	AppliedVersion *int
}
