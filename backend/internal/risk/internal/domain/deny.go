package domain

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Errors of deny-list management.
var (
	ErrDenyInvalid   = errors.New("invalid deny-list entry")
	ErrDenyDuplicate = errors.New("this identifier is already on the deny list")
	ErrDenyLimit     = errors.New("deny-list limit reached")
	ErrDenyNotFound  = errors.New("deny-list entry not found")
)

// DenyKind is what a deny-list entry identifies.
type DenyKind string

// Deny-list kinds.
const (
	DenyUser        DenyKind = "user"
	DenyAccount     DenyKind = "account"
	DenyDevice      DenyKind = "device"
	DenyIP          DenyKind = "ip"
	DenyBeneficiary DenyKind = "beneficiary"
	DenyCountry     DenyKind = "country"
)

// DenyKinds lists every kind, in the order the console shows them.
func DenyKinds() []DenyKind {
	return []DenyKind{DenyUser, DenyAccount, DenyDevice, DenyIP, DenyBeneficiary, DenyCountry}
}

// MaxDenyEntries bounds an environment's deny list (it is held in memory for evaluation).
const MaxDenyEntries = 10_000

// SignalDenyList is the code of the reason a deny-list match adds to an evaluation. Rules cannot
// use it.
const SignalDenyList = "deny_list"

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`) //nolint:gochecknoglobals // compiled once, never mutated

// DenyEntry is one identifier on a deny list.
type DenyEntry struct {
	ID        uuid.UUID
	Kind      DenyKind
	Value     string
	Reason    string
	ExpiresAt *time.Time
	CreatedAt time.Time
	CreatedBy string
}

// NormalizeDenyValue validates value for kind and returns it in the form events carry:
// IP addresses canonical, countries upper-case, other identifiers trimmed and exact.
func NormalizeDenyValue(kind DenyKind, value string) (string, error) {
	invalid := func(msg string) error { return fmt.Errorf("%w: %s", ErrDenyInvalid, msg) }
	value = strings.TrimSpace(value)
	switch kind {
	case DenyIP:
		a, err := netip.ParseAddr(value)
		if err != nil {
			return "", invalid("value must be an IP address")
		}
		return a.Unmap().String(), nil
	case DenyCountry:
		v := strings.ToUpper(value)
		if !countryPattern.MatchString(v) {
			return "", invalid("value must be a two-letter country code")
		}
		return v, nil
	case DenyUser, DenyAccount, DenyDevice, DenyBeneficiary:
		if n := utf8.RuneCountInString(value); n < 1 || n > 128 {
			return "", invalid("value must be 1-128 characters")
		}
		return value, nil
	default:
		return "", invalid("kind must be user, account, device, ip, beneficiary or country")
	}
}

// DenyList is an environment's active entries, by kind and value.
type DenyList map[DenyKind]map[string]struct{}

// Add puts an entry on the list.
func (d DenyList) Add(kind DenyKind, value string) {
	if d[kind] == nil {
		d[kind] = map[string]struct{}{}
	}
	d[kind][value] = struct{}{}
}

// DenyMatch is one identifier of an event found on the deny list.
type DenyMatch struct {
	Kind  DenyKind
	Value string
}

// Match returns the identifiers among refs that are on the list.
func (d DenyList) Match(refs []DenyMatch) []DenyMatch {
	var out []DenyMatch
	for _, r := range refs {
		if r.Value == "" {
			continue
		}
		if _, ok := d[r.Kind][r.Value]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Signal explains a match as a named reason of weight 0: the block comes from the deny list,
// not from the score.
func (m DenyMatch) Signal() Signal {
	return Signal{Code: SignalDenyList, Label: fmt.Sprintf("On your deny list: %s %s", m.Kind, m.Value), Weight: 0}
}
