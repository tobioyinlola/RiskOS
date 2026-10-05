package rules

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

// Event errors.
var (
	// ErrInvalidEvent means an event's compliance block does not fit the rule pack's vocabulary.
	ErrInvalidEvent = errors.New("invalid compliance data")
	// ErrSensitiveData means a reference looks like the personal data it should stand for.
	ErrSensitiveData = pack.ErrSensitive
)

// EventContext is an event's compliance block (EVENTS.md#compliance): the pack event kind, facts
// from the customer's systems and providers, and references for the entities rules keep windows
// for. Maps encode with sorted keys, so the normalized event stays canonical.
type EventContext struct {
	Kind  string            `json:"kind,omitempty"`
	Facts map[string]any    `json:"facts,omitempty"`
	Refs  map[string]string `json:"refs,omitempty"`
	// BookingEntity names the licensed entity that holds the customer relationship: its
	// activation's jurisdiction governs the event (ADR-0007).
	BookingEntity string `json:"booking_entity,omitempty"`
	// DataSubjectCountry is the country of the person the data is about (data-protection rules).
	DataSubjectCountry string `json:"data_subject_country,omitempty"`
}

// KindInput is what kind derivation reads from an event.
type KindInput = pack.KindInput

// Kinds returns the pack event kinds an event counts as.
func Kinds(in KindInput) []string { return pack.Kinds(in) }

// CheckEvent checks an event's compliance block against the catalog: the kind goes with the
// event type (an activity must name one), every fact is a supplied fact of the right type, and
// no reference looks like the personal data it stands for.
func CheckEvent(eventType string, c *EventContext) error {
	if c == nil || c.Kind == "" {
		if eventType == "activity" {
			return fmt.Errorf("%w: an activity event needs compliance.kind", ErrInvalidEvent)
		}
		if c == nil {
			return nil
		}
	}
	if c.Kind != "" && !slices.Contains(pack.KindsFor(eventType), c.Kind) {
		return fmt.Errorf("%w: compliance.kind %s does not go with a %s event", ErrInvalidEvent, c.Kind, eventType)
	}
	for _, name := range slices.Sorted(maps.Keys(c.Facts)) {
		if err := pack.CheckSuppliedFact(name, c.Facts[name]); err != nil {
			return fmt.Errorf("%w: compliance.facts: %w", ErrInvalidEvent, err)
		}
	}
	for _, kind := range slices.Sorted(maps.Keys(c.Refs)) {
		if err := pack.CheckRef(kind, c.Refs[kind]); err != nil {
			return err
		}
	}
	return nil
}
