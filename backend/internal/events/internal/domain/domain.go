// Package domain holds the event model and the rules applied before an event is stored:
// normalization, payload hashing, the accepted time window and sensitive-data detection.
// It has no I/O.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// Errors of the events module.
var (
	ErrInvalidEvent        = errors.New("invalid event")
	ErrStaleEvent          = errors.New("stale event")
	ErrSensitiveData       = errors.New("sensitive data in event")
	ErrIdempotencyConflict = errors.New("event_id already used with a different payload")
	ErrNotFound            = errors.New("event not found")
)

// Accepted time window for occurred_at relative to server time (EVENTS.md).
const (
	MaxFutureSkew = 5 * time.Minute
	MaxAge        = 24 * time.Hour
)

// Actor references the customer-side entities involved. IDs are opaque customer references.
type Actor struct {
	UserID    string `json:"user_id"`
	AccountID string `json:"account_id,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// Context describes where the event came from.
type Context struct {
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Country   string `json:"country,omitempty"`
	Geo       *Geo   `json:"geo,omitempty"`
}

// Geo is where the end user was, rounded to 2 decimals (about 1 km) when normalized.
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Event is a validated-by-contract event, before normalization.
type Event struct {
	Key        string
	Type       string
	OccurredAt time.Time
	Actor      Actor
	Context    Context
	// Data is the type-specific object, already schema-validated.
	Data json.RawMessage
	// Compliance is what the rule pack needs about the event (EVENTS.md#compliance), or nil.
	Compliance *rules.EventContext
}

// Normalized is the canonical form the engine evaluates and the payload hash covers. Field order
// is fixed by the struct, so equal events always encode to identical bytes.
type Normalized struct {
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Actor      Actor           `json:"actor"`
	Context    Context         `json:"context"`
	Data       json.RawMessage `json:"data"`
	// Compliance is omitted when empty, so events without it hash as before.
	Compliance *rules.EventContext `json:"compliance,omitempty"`
}

// Normalize validates what the contract cannot express and returns the canonical form: UTC
// timestamps at microsecond precision (what PostgreSQL stores), canonical IP text (IPv4-mapped
// IPv6 unmapped) and compact data. It also returns the parsed IP.
func Normalize(e Event) (Normalized, *netip.Addr, error) {
	n := Normalized{
		Type:       e.Type,
		OccurredAt: e.OccurredAt.UTC().Truncate(time.Microsecond),
		Actor:      e.Actor,
		Context:    e.Context,
	}
	var ip *netip.Addr
	if e.Context.IP != "" {
		addr, err := netip.ParseAddr(e.Context.IP)
		if err != nil || addr.Zone() != "" {
			return Normalized{}, nil, fmt.Errorf("%w: context.ip is not an IP address", ErrInvalidEvent)
		}
		addr = addr.Unmap()
		n.Context.IP = addr.String()
		ip = &addr
	}
	if g := e.Context.Geo; g != nil {
		// Coarse enough not to pinpoint a person, precise enough for travel speeds.
		n.Context.Geo = &Geo{Lat: math.Round(g.Lat*100) / 100, Lon: math.Round(g.Lon*100) / 100}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, e.Data); err != nil {
		return Normalized{}, nil, fmt.Errorf("%w: data is not valid JSON", ErrInvalidEvent)
	}
	n.Data = compact.Bytes()
	if err := rules.CheckEvent(e.Type, e.Compliance); err != nil {
		if errors.Is(err, rules.ErrSensitiveData) {
			return Normalized{}, nil, err
		}
		return Normalized{}, nil, fmt.Errorf("%w: %s", ErrInvalidEvent,
			strings.TrimPrefix(err.Error(), rules.ErrInvalidEvent.Error()+": "))
	}
	if c := e.Compliance; c != nil && (c.Kind != "" || len(c.Facts) > 0 || len(c.Refs) > 0) {
		n.Compliance = c
	}
	return n, ip, nil
}

// Encode returns the canonical bytes of n.
func (n Normalized) Encode() ([]byte, error) {
	return json.Marshal(n)
}

// PayloadHash identifies a payload for idempotency: two requests with the same event_id are the
// same event exactly when their normalized payloads are byte-identical.
func PayloadHash(normalized []byte) []byte {
	sum := sha256.Sum256(normalized)
	return sum[:]
}

// CheckWindow rejects events outside the accepted time window: ErrStaleEvent when older than
// MaxAge, ErrInvalidEvent when more than MaxFutureSkew ahead of now.
func CheckWindow(occurredAt, now time.Time) error {
	switch {
	case occurredAt.After(now.Add(MaxFutureSkew)):
		return fmt.Errorf("%w: occurred_at must not be more than 5 minutes in the future", ErrInvalidEvent)
	case occurredAt.Before(now.Add(-MaxAge)):
		return fmt.Errorf("%w: occurred_at must not be more than 24 hours in the past", ErrStaleEvent)
	}
	return nil
}

// ContainsCardNumber reports whether any string value in the JSON document contains a
// card-number-like sequence: 13-19 digits (spaces and dashes allowed between them) that passes
// the Luhn check and starts with a card-network prefix (2-6). RiskOS must never receive card
// numbers (ARCHITECTURE.md §16.3); numeric customer identifiers that look like one are rejected
// too, so customers should use prefixed or opaque identifiers.
func ContainsCardNumber(doc []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(doc))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if s, ok := tok.(string); ok && stringHasCardNumber(s) {
			return true
		}
	}
}

func stringHasCardNumber(s string) bool {
	digits := make([]byte, 0, 19)
	flush := func() bool {
		found := false
		// Check every window of 13..19 consecutive digits in the run.
		for start := 0; start < len(digits) && !found; start++ {
			for n := 13; n <= 19 && start+n <= len(digits); n++ {
				if w := digits[start : start+n]; w[0] >= '2' && w[0] <= '6' && luhn(w) {
					found = true
					break
				}
			}
		}
		digits = digits[:0]
		return found
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			if len(digits) < 64 {
				digits = append(digits, c)
			}
		case (c == ' ' || c == '-') && len(digits) > 0:
			// separators inside a card number
		default:
			if flush() {
				return true
			}
		}
	}
	return flush()
}

func luhn(digits []byte) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// Outcome is the stored result of an ingestion, replayed verbatim for idempotent retries.
type Outcome struct {
	RequestID       string     `json:"request_id"`
	EventID         *uuid.UUID `json:"event_id"`
	EvaluationID    uuid.UUID  `json:"evaluation_id"`
	Score           int        `json:"risk_score"`
	Level           string     `json:"risk_level"`
	Decision        string     `json:"decision"`
	EngineVersion   string     `json:"engine_version"`
	Signals         []Signal   `json:"signals"`
	DegradedSignals []string   `json:"degraded_signals"`
	// WarmingSignals is absent from outcomes stored before it existed.
	WarmingSignals []string `json:"warming_signals,omitempty"`
	// Controls and NotEvaluated are the rule pack's results (absent from older outcomes).
	Controls     []Control      `json:"controls,omitempty"`
	NotEvaluated []NotEvaluated `json:"not_evaluated,omitempty"`
	// Jurisdiction is which country's pack judged the event (absent from older outcomes).
	Jurisdiction *Jurisdiction `json:"jurisdiction,omitempty"`
}

// Jurisdiction mirrors the risk module's jurisdiction resolution in the stored outcome.
type Jurisdiction struct {
	Jurisdiction       string `json:"jurisdiction,omitempty"`
	Pack               string `json:"pack,omitempty"`
	ResolvedBy         string `json:"resolved_by"`
	BookingEntity      string `json:"booking_entity,omitempty"`
	DataSubjectCountry string `json:"data_subject_country,omitempty"`
	DataSubjectPack    string `json:"data_subject_pack,omitempty"`
}

// Control mirrors the risk module's rule pack control in the stored outcome.
type Control struct {
	Rule             string     `json:"rule"`
	Name             string     `json:"name"`
	Mode             string     `json:"mode"`
	Decision         string     `json:"decision,omitempty"`
	Reason           string     `json:"reason"`
	Tasks            []string   `json:"tasks"`
	FeePercent       float64    `json:"fee_percent,omitempty"`
	HoldSeconds      int64      `json:"hold_seconds,omitempty"`
	HoldUntil        *time.Time `json:"hold_until,omitempty"`
	Branch           string     `json:"branch,omitempty"`
	Jurisdiction     string     `json:"jurisdiction,omitempty"`
	ExemptionUnknown bool       `json:"exemption_unknown,omitempty"`
}

// NotEvaluated mirrors a rule pack rule that could not be evaluated.
type NotEvaluated struct {
	Rule    string   `json:"rule"`
	Missing []string `json:"missing"`
}

// Signal mirrors the risk module's signal in the stored outcome.
type Signal struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
}

// Summary is an event in a list.
type Summary struct {
	ID         uuid.UUID
	Key        string
	Type       string
	OccurredAt time.Time
	ReceivedAt time.Time
	Score      int
	Level      string
	Decision   string
	User       string
	Account    *string
	Device     *string
	Country    *string
	// Txn is set for transactions.
	Txn *TxnSummary
}

// Detail is a stored event with its payloads and evaluation.
type Detail struct {
	ID           uuid.UUID
	Key          string
	Type         string
	OccurredAt   time.Time
	ReceivedAt   time.Time
	RawPayload   []byte
	Normalized   []byte
	EvaluationID uuid.UUID
	Evaluation   Outcome
}

// Page selects a page of a newest-first listing.
type Page struct {
	Limit  int
	Before *uuid.UUID
	Filter Filter
}
