package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/domain"
)

func event() domain.Event {
	return domain.Event{
		Key:        "txn_1",
		Type:       "transaction",
		OccurredAt: time.Date(2026, 9, 30, 12, 0, 0, 123456789, time.FixedZone("WAT", 3600)),
		Actor:      domain.Actor{UserID: "u_1"},
		Context:    domain.Context{IP: "::ffff:203.0.113.7", Country: "NG"},
		Data:       json.RawMessage(`{ "amount": 5000,  "currency": "NGN" }`),
	}
}

func TestNormalize(t *testing.T) {
	n, ip, err := domain.Normalize(event())
	require.NoError(t, err)
	assert.Equal(t, time.UTC, n.OccurredAt.Location())
	assert.Equal(t, 123456000, n.OccurredAt.Nanosecond(), "microsecond precision, as stored")
	assert.Equal(t, "203.0.113.7", n.Context.IP, "IPv4-mapped IPv6 is unmapped")
	require.NotNil(t, ip)
	assert.True(t, ip.Is4())
	assert.JSONEq(t, `{"amount":5000,"currency":"NGN"}`, string(n.Data))
	assert.NotContains(t, string(n.Data), " ")
}

func TestNormalizeRejectsBadIP(t *testing.T) {
	for _, bad := range []string{"not-an-ip", "300.1.1.1", "fe80::1%eth0"} {
		e := event()
		e.Context.IP = bad
		_, _, err := domain.Normalize(e)
		assert.ErrorIs(t, err, domain.ErrInvalidEvent, bad)
	}
}

func TestPayloadHashIgnoresFormattingButNotContent(t *testing.T) {
	encode := func(e domain.Event) []byte {
		n, _, err := domain.Normalize(e)
		require.NoError(t, err)
		b, err := n.Encode()
		require.NoError(t, err)
		return domain.PayloadHash(b)
	}
	a := event()
	b := event()
	b.Data = json.RawMessage(`{"amount":5000,"currency":"NGN"}`) // different whitespace
	b.OccurredAt = a.OccurredAt.UTC()                            // same instant, other zone
	assert.Equal(t, encode(a), encode(b))

	c := event()
	c.Data = json.RawMessage(`{"amount":5001,"currency":"NGN"}`)
	assert.NotEqual(t, encode(a), encode(c))
}

func TestCheckWindow(t *testing.T) {
	now := time.Now()
	assert.NoError(t, domain.CheckWindow(now, now))
	assert.NoError(t, domain.CheckWindow(now.Add(4*time.Minute), now))
	assert.NoError(t, domain.CheckWindow(now.Add(-23*time.Hour), now))
	assert.ErrorIs(t, domain.CheckWindow(now.Add(6*time.Minute), now), domain.ErrInvalidEvent)
	assert.ErrorIs(t, domain.CheckWindow(now.Add(-25*time.Hour), now), domain.ErrStaleEvent)
}

func TestContainsCardNumber(t *testing.T) {
	tests := []struct {
		doc  string
		want bool
	}{
		{`{"actor":{"user_id":"4111111111111111"}}`, true},          // Visa test number
		{`{"data":{"note":"card 5555-5555-5555-4444 used"}}`, true}, // Mastercard, dashes
		{`{"x":"378282246310005"}`, true},                           // Amex, 15 digits
		{`{"x":"4111 1111 1111 1111"}`, true},                       // spaces
		{`{"x":"prefix4111111111111111suffix"}`, true},              // embedded
		{`{"x":"4111111111111112"}`, false},                         // fails Luhn
		{`{"x":"1111111111111117"}`, false},                         // no card-network prefix
		{`{"x":"411111111111"}`, false},                             // too short
		{`{"amount":4111111111111111}`, false},                      // numbers are amounts, not strings
		{`{"transaction_id":"txn_2026_000123","user_id":"u_42"}`, false},
		{`{"ip":"2001:db8::1","ua":"Mozilla/5.0 (X11; Linux x86_64)"}`, false},
		{`not json`, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, domain.ContainsCardNumber([]byte(tt.doc)), tt.doc)
	}
}

func TestContainsCardNumberIsBounded(t *testing.T) {
	long := `{"x":"` + strings.Repeat("7", 60000) + `"}`
	start := time.Now()
	domain.ContainsCardNumber([]byte(long))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}
