package domain

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Errors of rule management.
var (
	ErrRuleNotFound    = errors.New("rule not found")
	ErrRuleLimit       = errors.New("rule limit reached")
	ErrRuleInvalid     = errors.New("invalid rule")
	ErrRuleCodeTaken   = errors.New("rule code already used in this environment")
	ErrRuleConflict    = errors.New("rule changed concurrently")
	ErrSettingsInvalid = errors.New("invalid risk settings")
)

// RuleMode decides whether a matching rule affects the score.
type RuleMode string

// Rule modes.
const (
	// ModeActive rules add their weight to the score.
	ModeActive RuleMode = "active"
	// ModeShadow rules are evaluated and their matches recorded with the evaluation, but never
	// change the response: the safe way to try a rule on live traffic (ARCHITECTURE.md §10).
	ModeShadow RuleMode = "shadow"
)

// RuleVersion is one immutable version of a rule.
type RuleVersion struct {
	Version      int
	Name         string
	Description  string
	Severity     string
	Weight       int
	Mode         RuleMode
	Enabled      bool
	Condition    json.RawMessage
	CreatedAt    time.Time
	CreatedBy    string
	RestoredFrom *int
}

// Rule is a rule with its current version.
type Rule struct {
	ID        uuid.UUID
	Code      string
	CreatedAt time.Time
	Current   RuleVersion
	// PendingChange is a proposed change awaiting approval (production), if any.
	PendingChange *uuid.UUID
}

// VersionRef identifies a rule version in evaluations: "<rule code>@<version>".
func (r Rule) VersionRef() string {
	return r.Code + "@" + strconv.Itoa(r.Current.Version)
}

// MaxRulesPerEnvironment bounds the rules evaluated per event (ARCHITECTURE.md §16, threat 9).
// Rules are disabled rather than deleted, so disabled rules count. Concurrent creations can
// overshoot it by at most their number; it is a resource bound, not a security boundary.
const MaxRulesPerEnvironment = 200
