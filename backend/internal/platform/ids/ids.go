// Package ids generates and parses opaque, type-prefixed identifiers.
//
// Domain IDs (ARCHITECTURE.md §7) are UUIDv7: stored as `uuid` in PostgreSQL and exposed as
// "<prefix>_<26 base32 chars>", e.g. "key_01j9x2k7m3q4r5s6t7v8w9x0yz". They sort by creation time,
// are not enumerable, and a prefix mismatch (a tenant ID passed where a key ID is expected) is
// rejected at parse time. Request correlation IDs use New and are random, not UUIDs.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// alphabet is lowercase Crockford base32: no padding and no ambiguous characters (i, l, o, u),
// so IDs are URL-, header- and log-safe.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// encodedLen is the base32 length of 128 bits.
const encodedLen = 26

// New returns prefix + "_" + 26 characters encoding 128 bits of cryptographic randomness.
// crypto/rand.Read never returns an error (it aborts the program on catastrophic failure), so
// the signature stays error-free.
func New(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])

	out := make([]byte, 0, len(prefix)+1+encodedLen)
	out = append(out, prefix...)
	out = append(out, '_')
	return string(appendBase32(out, b[:]))
}

// NewRequestID returns a new request correlation ID ("req_…").
func NewRequestID() string { return New("req") }

// Prefixes of exposed domain IDs. A prefix is part of the public API; never change one.
const (
	PrefixTenant          = "ten"
	PrefixEnvironment     = "env"
	PrefixAPIKey          = "key"
	PrefixAudit           = "aud"
	PrefixEvent           = "evt"
	PrefixEvaluation      = "evl"
	PrefixRule            = "rul"
	PrefixAlert           = "alr"
	PrefixCase            = "cas"
	PrefixCaseNote        = "cnt"
	PrefixActivity        = "act"
	PrefixWebhook         = "whk"
	PrefixWebhookEvent    = "wev"
	PrefixWebhookDelivery = "whd"
	PrefixUser            = "usr"
	PrefixInvite          = "inv"
	PrefixPasskey         = "pky"
	PrefixRuleChange      = "rch"
	PrefixDenyEntry       = "dny"
)

// ErrInvalid is returned when an external ID is malformed or has the wrong prefix.
var ErrInvalid = errors.New("invalid identifier")

// NewV7 returns a new time-ordered UUIDv7.
func NewV7() uuid.UUID {
	// uuid.NewV7 only fails if the system random source fails, which crypto/rand treats as fatal.
	return uuid.Must(uuid.NewV7())
}

// Time returns the creation time embedded in a UUIDv7 (millisecond precision, UTC). Tables that
// are partitioned by time use it as the partition key, so a lookup by id selects one partition.
func Time(id uuid.UUID) time.Time {
	sec, nsec := id.Time().UnixTime()
	return time.Unix(sec, nsec).UTC()
}

// Format renders id as "<prefix>_<base32>". Lexicographic order matches UUIDv7 time order.
func Format(prefix string, id uuid.UUID) string {
	out := make([]byte, 0, len(prefix)+1+encodedLen)
	out = append(out, prefix...)
	out = append(out, '_')
	return string(appendBase32(out, id[:]))
}

// Parse is the inverse of Format. It rejects a wrong prefix, characters outside the alphabet
// (including uppercase), the wrong length and non-canonical encodings (non-zero padding bits),
// so every accepted string maps to exactly one UUID.
func Parse(prefix string, s string) (uuid.UUID, error) {
	body, ok := strings.CutPrefix(s, prefix+"_")
	if !ok || len(body) != encodedLen {
		return uuid.Nil, ErrInvalid
	}
	b, err := base32.NewEncoding(alphabet).WithPadding(base32.NoPadding).DecodeString(body)
	if err != nil || len(b) != len(uuid.UUID{}) {
		return uuid.Nil, ErrInvalid
	}
	id := uuid.UUID(b)
	// Re-encoding must reproduce the input exactly; this rejects non-zero padding bits.
	if Format(prefix, id) != s {
		return uuid.Nil, ErrInvalid
	}
	return id, nil
}

func appendBase32(out, b []byte) []byte {
	var acc uint32
	bits := 0
	for _, v := range b {
		acc = acc<<8 | uint32(v)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out = append(out, alphabet[(acc>>bits)&31])
		}
	}
	if bits > 0 {
		out = append(out, alphabet[(acc<<(5-bits))&31])
	}
	return out
}

// IsSafeExternalID reports whether a caller-supplied correlation ID may be echoed and logged:
// 1–64 characters drawn from [A-Za-z0-9._-]. This prevents log injection and header abuse.
func IsSafeExternalID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
