package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDomain(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":           "example.com",
		"  Example.COM ":        "example.com",
		"mail.example.co.uk":    "mail.example.co.uk",
		"example.com.":          "example.com",
		"xn--bcher-kva.example": "xn--bcher-kva.example",
	} {
		got, err := normalizeDomain(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{
		"", "com", "a.b", "http://example.com", "example.com/path", "user@example.com", "example.com:443",
		"-bad.example.com", "bad-.example.com", "exa mple.com", "1.2.3.4", "example.123", "*.example.com",
		"a..example.com", ".example.com",
	} {
		_, err := normalizeDomain(in)
		assert.Error(t, err, in)
	}
}

func TestDomainTokenIsPerWorkspaceAndDomain(t *testing.T) {
	c := &Console{cfg: ConsoleConfig{Secret: []byte("0123456789abcdef0123456789abcdef")}}
	a, b := uuid.New(), uuid.New()
	tok := c.domainToken(a, "example.com")
	assert.Equal(t, tok, c.domainToken(a, "example.com"), "stable")
	assert.NotEqual(t, tok, c.domainToken(b, "example.com"), "another workspace cannot reuse it")
	assert.NotEqual(t, tok, c.domainToken(a, "example.org"), "another domain cannot reuse it")
	assert.Regexp(t, `^riskos-verification=[a-z2-7]{32}$`, tok)

	other := &Console{cfg: ConsoleConfig{Secret: []byte("fedcba9876543210fedcba9876543210")}}
	assert.NotEqual(t, tok, other.domainToken(a, "example.com"), "it depends on the console secret")
	assert.Equal(t, "_riskos-challenge.example.com", txtHost("example.com"))
}
