package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

func TestTokenRoundTrip(t *testing.T) {
	for _, kind := range []tenancy.EnvironmentKind{tenancy.EnvironmentTest, tenancy.EnvironmentProduction} {
		token, lookup := domain.NewToken(kind)
		prefix := map[tenancy.EnvironmentKind]string{tenancy.EnvironmentTest: "rsk_test_", tenancy.EnvironmentProduction: "rsk_live_"}[kind]
		assert.True(t, strings.HasPrefix(token, prefix+lookup+"_"), token)
		assert.Len(t, token, len(prefix)+12+1+32)

		gotLookup, gotKind, ok := domain.ParseToken(token)
		require.True(t, ok)
		assert.Equal(t, lookup, gotLookup)
		assert.Equal(t, kind, gotKind)
	}
}

func TestTokensAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		token, _ := domain.NewToken(tenancy.EnvironmentTest)
		require.False(t, seen[token])
		seen[token] = true
	}
}

func TestParseTokenRejects(t *testing.T) {
	valid, _ := domain.NewToken(tenancy.EnvironmentTest)
	tests := map[string]string{
		"empty":          "",
		"wrong scheme":   "sk" + valid[3:],
		"unknown env":    strings.Replace(valid, "_test_", "_prod_", 1),
		"short lookup":   "rsk_test_abc_" + strings.Repeat("a", 32),
		"short secret":   "rsk_test_" + strings.Repeat("a", 12) + "_abc",
		"bad character":  "rsk_test_" + strings.Repeat("a", 11) + "U_" + strings.Repeat("a", 32),
		"extra segment":  valid + "_x",
		"whitespace":     " " + valid,
		"another vendor": "sk_live_" + strings.Repeat("a", 24),
		"sql in lookup":  "rsk_test_' OR 1=1 --_" + strings.Repeat("a", 32),
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, ok := domain.ParseToken(in)
			assert.False(t, ok)
		})
	}
}

func TestHashTokenDependsOnPepperAndToken(t *testing.T) {
	token, _ := domain.NewToken(tenancy.EnvironmentTest)
	pepperA := []byte(strings.Repeat("a", 32))
	pepperB := []byte(strings.Repeat("b", 32))

	h := domain.HashToken(pepperA, token)
	assert.Len(t, h, 32)
	assert.Equal(t, h, domain.HashToken(pepperA, token), "deterministic")
	assert.NotEqual(t, h, domain.HashToken(pepperB, token), "a different pepper gives a different hash")
	other, _ := domain.NewToken(tenancy.EnvironmentTest)
	assert.NotEqual(t, h, domain.HashToken(pepperA, other))
	assert.NotContains(t, string(h), token)
}

func TestStatus(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	tests := []struct {
		name string
		key  domain.APIKey
		want string
	}{
		{"active", domain.APIKey{}, domain.StatusActive},
		{"expires later", domain.APIKey{ExpiresAt: &future}, domain.StatusActive},
		{"expired", domain.APIKey{ExpiresAt: &past}, domain.StatusExpired},
		{"expires exactly now", domain.APIKey{ExpiresAt: &now}, domain.StatusExpired},
		{"revoked", domain.APIKey{RevokedAt: &past}, domain.StatusRevoked},
		{"revoked beats expired", domain.APIKey{RevokedAt: &past, ExpiresAt: &past}, domain.StatusRevoked},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.key.Status(now), tt.name)
	}
}
