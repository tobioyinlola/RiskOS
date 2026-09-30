package domain_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/domain"
)

func TestValidateURL(t *testing.T) {
	strict := domain.URLPolicy{}
	for _, u := range []string{"https://hooks.example.com/riskos", "https://example.com:8443/x?y=1"} {
		assert.NoError(t, domain.ValidateURL(u, strict), u)
	}
	for _, u := range []string{
		"http://hooks.example.com", "ftp://example.com", "https://", "/relative", "https://user:pw@example.com",
		"https://example.com/#frag", "https://localhost/x", "https://api.localhost", "https://127.0.0.1/",
		"https://10.0.0.5/", "https://169.254.169.254/latest/meta-data", "https://[::1]/", "https://[fd00::1]/",
		"https://[::ffff:10.0.0.1]/", "https://100.64.0.1/", "https://0.0.0.0/",
	} {
		assert.ErrorIs(t, domain.ValidateURL(u, strict), domain.ErrInvalid, u)
	}
	dev := domain.URLPolicy{AllowPrivate: true}
	assert.NoError(t, domain.ValidateURL("http://127.0.0.1:9000/hook", dev))
	assert.NoError(t, domain.ValidateURL("http://localhost:9000/hook", dev))
	assert.ErrorIs(t, domain.ValidateURL("ftp://127.0.0.1/", dev), domain.ErrInvalid)
}

func TestBlocked(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "::1", "fe80::1", "::ffff:192.168.0.1", "64:ff9b::a00:1"} {
		assert.True(t, domain.Blocked(netip.MustParseAddr(s)), s)
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		assert.False(t, domain.Blocked(netip.MustParseAddr(s)), s)
	}
}

func TestSecretsAndSignatures(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	id := uuid.MustParse("01920000-0000-7000-8000-000000000001")
	s1 := domain.Secret(key, id, 1)
	assert.Equal(t, s1, domain.Secret(key, id, 1), "deterministic")
	assert.NotEqual(t, s1, domain.Secret(key, id, 2), "rotation changes the secret")
	assert.NotEqual(t, s1, domain.Secret(key, uuid.New(), 1))
	assert.Regexp(t, `^whsec_[A-Za-z0-9_-]{43}$`, s1)

	// A receiver's independent computation (what docs/WEBHOOKS.md tells customers to do).
	body := []byte(`{"id":"wev_1","type":"alert.created"}`)
	ts := time.Unix(1_790_000_000, 0).Unix()
	m := hmac.New(sha256.New, []byte(s1))
	m.Write([]byte("wev_1.1790000000." + string(body)))
	assert.Equal(t, "v1="+hex.EncodeToString(m.Sum(nil)), domain.Sign(s1, "wev_1", ts, body))
}

func TestRetrySchedule(t *testing.T) {
	var total time.Duration
	for _, d := range domain.RetryDelays() {
		total += d
	}
	assert.Len(t, domain.RetryDelays(), 8, "9 attempts")
	assert.InDelta(t, time.Hour.Minutes(), total.Minutes(), 2, "about an hour")
}
