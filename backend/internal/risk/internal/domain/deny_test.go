package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

func TestNormalizeDenyValue(t *testing.T) {
	for _, tc := range []struct {
		kind      domain.DenyKind
		in, want  string
		wantError bool
	}{
		{kind: domain.DenyIP, in: " 203.0.113.7 ", want: "203.0.113.7"},
		{kind: domain.DenyIP, in: "::ffff:203.0.113.7", want: "203.0.113.7"},
		{kind: domain.DenyIP, in: "2001:DB8::1", want: "2001:db8::1"},
		{kind: domain.DenyIP, in: "not-an-ip", wantError: true},
		{kind: domain.DenyCountry, in: "ng", want: "NG"},
		{kind: domain.DenyCountry, in: "NGA", wantError: true},
		{kind: domain.DenyUser, in: " usr_1 ", want: "usr_1"},
		{kind: domain.DenyDevice, in: "", wantError: true},
		{kind: "card", in: "x", wantError: true},
	} {
		got, err := domain.NormalizeDenyValue(tc.kind, tc.in)
		if tc.wantError {
			assert.ErrorIs(t, err, domain.ErrDenyInvalid, "%s %q", tc.kind, tc.in)
			continue
		}
		require.NoError(t, err, "%s %q", tc.kind, tc.in)
		assert.Equal(t, tc.want, got)
	}
}

func TestDenyListMatch(t *testing.T) {
	d := domain.DenyList{}
	d.Add(domain.DenyDevice, "dev_bad")
	d.Add(domain.DenyCountry, "KP")
	refs := []domain.DenyMatch{
		{Kind: domain.DenyUser, Value: "dev_bad"}, // same value, another kind: no match
		{Kind: domain.DenyDevice, Value: "dev_bad"},
		{Kind: domain.DenyCountry, Value: ""},
	}
	got := d.Match(refs)
	require.Len(t, got, 1)
	assert.Equal(t, domain.DenyDevice, got[0].Kind)
	assert.Equal(t, domain.Signal{Code: "deny_list", Label: "On your deny list: device dev_bad", Weight: 0}, got[0].Signal())
}

func TestDenyListedBlocksWhateverTheScore(t *testing.T) {
	for _, l := range []domain.Level{domain.LevelMinimal, domain.LevelHigh} {
		d := domain.Decide(domain.PolicyInput{EventType: "login", Level: l, Settings: domain.DefaultSettings(), DenyListed: true})
		assert.Equal(t, domain.DecisionBlock, d.Decision)
		assert.Contains(t, d.Reasons, "deny_list")
	}
}
