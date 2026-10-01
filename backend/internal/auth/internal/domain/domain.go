// Package domain holds the auth module's types and the API key format. It has no I/O.
package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Errors of the auth module.
var (
	// ErrUnauthenticated covers missing, malformed, unknown, revoked and expired keys alike, so
	// responses never reveal which check failed.
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrTenantSuspended = errors.New("tenant is not active")
	ErrNotFound        = errors.New("api key not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrConflict        = errors.New("api key cannot be changed in its current state")
)

// Errors of the dashboard realm.
var (
	// ErrInvalidCredentials covers unknown emails, wrong passwords and disabled users alike.
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrMFAInvalid         = errors.New("invalid verification code")
	// ErrSessionState means the session is not at the step this operation needs.
	ErrSessionState   = errors.New("session is not in the right state")
	ErrReauthRequired = errors.New("recent verification required")
	ErrInviteInvalid  = errors.New("invite is invalid or expired")
	ErrEmailTaken     = errors.New("email already has an account")
	ErrLastAdmin      = errors.New("a tenant must keep at least one active administrator")
	ErrSelfChange     = errors.New("you cannot change your own role or status")
	ErrUserNotFound   = errors.New("user not found")
	// Single sign-on.
	ErrSSONotConfigured = errors.New("single sign-on is not set up for this email")
	ErrSSOFailed        = errors.New("single sign-on failed")
	ErrSSOProvider      = errors.New("identity provider unavailable")
	ErrSSONotFound      = errors.New("no single sign-on connection")
	// Support access by RiskOS staff (the admin realm resolves it).
	ErrStaffUnauthenticated = errors.New("staff session required")
	ErrSupportDenied        = errors.New("support access is not approved, has ended or is not yours")
)

// RateLimitedError is returned when sign-in or verification attempts are throttled.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "too many attempts" }

// API key format (ARCHITECTURE.md §9, §16.7):
//
//	rsk_<env>_<lookup>_<secret>
//
// env is "test" or "live" (production); lookup is 12 base32 characters (60 random bits), public
// and indexed; secret is 32 base32 characters (160 random bits). The recognizable "rsk_live_" and
// "rsk_test_" prefixes let secret scanners find leaked keys.
const (
	keyScheme    = "rsk"
	lookupLength = 12
	secretLength = 32
	alphabet     = "0123456789abcdefghjkmnpqrstvwxyz"
)

func envTag(kind tenancy.EnvironmentKind) string {
	if kind == tenancy.EnvironmentProduction {
		return "live"
	}
	return "test"
}

// NewToken returns a new API key for an environment and its public lookup value.
func NewToken(kind tenancy.EnvironmentKind) (token, lookup string) {
	lookup = randomString(lookupLength)
	return DisplayPrefix(kind, lookup) + "_" + randomString(secretLength), lookup
}

// DisplayPrefix is the non-secret start of a key, safe to show and log.
func DisplayPrefix(kind tenancy.EnvironmentKind, lookup string) string {
	return keyScheme + "_" + envTag(kind) + "_" + lookup
}

// ParseToken validates a key's shape and returns its lookup value and environment kind. It does
// not authenticate the key.
func ParseToken(token string) (lookup string, kind tenancy.EnvironmentKind, ok bool) {
	parts := strings.Split(token, "_")
	if len(parts) != 4 || parts[0] != keyScheme || len(parts[2]) != lookupLength || len(parts[3]) != secretLength {
		return "", "", false
	}
	switch parts[1] {
	case "test":
		kind = tenancy.EnvironmentTest
	case "live":
		kind = tenancy.EnvironmentProduction
	default:
		return "", "", false
	}
	if !inAlphabet(parts[2]) || !inAlphabet(parts[3]) {
		return "", "", false
	}
	return parts[2], kind, true
}

// HashToken returns HMAC-SHA256(pepper, token). Only this value is stored; without the pepper
// (held in the secrets manager, never in the database) a stolen table cannot be used to verify
// guesses offline.
func HashToken(pepper []byte, token string) []byte {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte(token))
	return m.Sum(nil)
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // never fails; see crypto/rand
	for i := range b {
		b[i] = alphabet[b[i]&31] // 256 is a multiple of 32, so the mapping is uniform
	}
	return string(b)
}

func inAlphabet(s string) bool {
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(alphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}

// APIKey is a stored API key (never with its secret).
type APIKey struct {
	ID          uuid.UUID
	Name        string
	Lookup      string
	Environment tenancy.EnvironmentKind
	Scopes      []string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	ReplacedBy  *uuid.UUID
}

// Status values.
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
)

// Status returns the key's status at now.
func (k APIKey) Status(now time.Time) string {
	switch {
	case k.RevokedAt != nil && !now.Before(*k.RevokedAt):
		return StatusRevoked
	case k.ExpiresAt != nil && !now.Before(*k.ExpiresAt):
		return StatusExpired
	default:
		return StatusActive
	}
}

// CreatedKey is a newly created key with the only copy of its secret.
type CreatedKey struct {
	Key    APIKey
	Secret string
}

// Page selects a page of a newest-first listing.
type Page struct {
	Limit  int
	Before *uuid.UUID
}
