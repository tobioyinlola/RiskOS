// Package domain holds the webhooks module's types and rules: event types, endpoint validation,
// secrets, signatures and the retry schedule.
package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrNotFound = errors.New("webhook not found")
	ErrInvalid  = errors.New("invalid webhook")
	ErrConflict = errors.New("webhook state conflict")
	ErrLimit    = errors.New("webhook limit reached")
)

// Bounds.
const (
	MaxEndpointsPerEnvironment = 20
	MaxURL                     = 2048
	MaxDescription             = 500
	MinOverlap                 = 0
	MaxOverlap                 = 7 * 24 * time.Hour
	DefaultOverlap             = 24 * time.Hour
)

// EventType is a kind of event endpoints can subscribe to.
type EventType string

// Event types.
const (
	EventAlertCreated EventType = "alert.created"
	EventAlertUpdated EventType = "alert.updated" // status changes
	EventCaseCreated  EventType = "case.created"
	EventCaseResolved EventType = "case.resolved"
	// EventTest is sent on request to one endpoint; it cannot be subscribed to.
	EventTest EventType = "webhook.test"
)

// SubscribableEvents returns the event types an endpoint can subscribe to.
func SubscribableEvents() []EventType {
	return []EventType{EventAlertCreated, EventAlertUpdated, EventCaseCreated, EventCaseResolved}
}

// Endpoint status.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Delivery status.
const (
	DeliveryPending   = "pending"
	DeliverySucceeded = "succeeded"
	DeliveryFailed    = "failed"
)

// Endpoint is a customer's webhook endpoint.
type Endpoint struct {
	ID                       uuid.UUID
	URL                      string
	Description              string
	EventTypes               []EventType
	Status                   string
	DisabledReason           *string
	SecretVersion            int
	PreviousSecretVersion    *int
	PreviousSecretValidUntil *time.Time
	FailingSince             *time.Time
	CreatedAt                time.Time
	CreatedBy                string
	UpdatedAt                time.Time
}

// Delivery is one event's delivery to one endpoint.
type Delivery struct {
	ID             uuid.UUID
	EndpointID     uuid.UUID
	EventID        uuid.UUID
	EventType      EventType
	Payload        []byte
	Status         string
	Attempts       int
	NextAttemptAt  *time.Time
	LastAttemptAt  *time.Time
	LastStatusCode *int
	LastError      *string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

// Attempt is one logged delivery attempt.
type Attempt struct {
	Attempt    int
	StartedAt  time.Time
	DurationMS int
	StatusCode *int
	Error      *string
}

// URLPolicy says which endpoint URLs are allowed.
type URLPolicy struct {
	// AllowPrivate permits http:// and private or loopback addresses. Only ever true for test
	// environments with the server flag set; production endpoints never get it.
	AllowPrivate bool
}

// ValidateURL checks an endpoint URL: absolute, https (unless AllowPrivate), a host, no
// credentials or fragment, and no literal private address or localhost (unless AllowPrivate).
// Hostnames are checked again, after DNS resolution, when a delivery connects.
func ValidateURL(raw string, p URLPolicy) error {
	invalid := func(msg string) error { return fmt.Errorf("%w: url %s", ErrInvalid, msg) }
	if len(raw) > MaxURL {
		return invalid(fmt.Sprintf("must be at most %d characters", MaxURL))
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return invalid("must be an absolute URL")
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && p.AllowPrivate:
	default:
		return invalid("must use https")
	}
	if u.User != nil {
		return invalid("must not contain credentials")
	}
	if u.Fragment != "" {
		return invalid("must not contain a fragment")
	}
	if p.AllowPrivate {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return invalid("must not point to localhost")
	}
	if ip, err := netip.ParseAddr(host); err == nil && Blocked(ip) {
		return invalid("must not point to a private, loopback or reserved address")
	}
	return nil
}

var blockedPrefixes = func() []netip.Prefix { //nolint:gochecknoglobals // constant table
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
		"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "2002::/16",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// Blocked reports whether a webhook may not connect to ip: loopback, private, link-local
// (including cloud metadata at 169.254.169.254), multicast, unspecified, carrier-grade NAT,
// documentation, benchmarking, reserved, and IPv6 translation ranges that could reach them.
func Blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	return slices.ContainsFunc(blockedPrefixes, func(p netip.Prefix) bool { return p.Contains(ip) })
}

// Secret derives an endpoint's signing secret. Secrets are never stored: they are recomputed
// from the server-side key, the endpoint ID and the secret version.
func Secret(key []byte, endpointID uuid.UUID, version int) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("riskos-webhook-secret\x00" + endpointID.String() + "\x00" + strconv.Itoa(version)))
	return "whsec_" + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Sign returns the v1 signature of a delivery: hex HMAC-SHA256, keyed by the secret, over
// "<event id>.<unix timestamp>.<body>".
func Sign(secret, eventID string, timestamp int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(eventID + "." + strconv.FormatInt(timestamp, 10) + "."))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

// RetryDelays is the production schedule after each failed attempt: 9 attempts over about an
// hour (10 s, 30 s, 1 m, 2 m, 5 m, 10 m, 20 m, 20 m between them), then the delivery fails.
func RetryDelays() []time.Duration {
	return []time.Duration{
		10 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute,
		5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 20 * time.Minute,
	}
}

// DisableAfter is how long an endpoint may fail every attempt before it is disabled.
const DisableAfter = 72 * time.Hour
