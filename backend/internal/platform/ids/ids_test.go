package ids_test

import (
	"regexp"
	"strings"
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func TestNewRequestID(t *testing.T) {
	re := regexp.MustCompile(`^req_[0-9a-hjkmnp-tv-z]{26}$`)
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		id := ids.NewRequestID()
		assert.Regexp(t, re, id)
		_, dup := seen[id]
		assert.False(t, dup, "duplicate id %s", id)
		seen[id] = struct{}{}
	}
}

func TestIsSafeExternalID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"abc-123_DEF.x", true},
		{strings.Repeat("a", 64), true},
		{"", false},
		{strings.Repeat("a", 65), false},
		{"has space", false},
		{"new\nline", false},
		{`quote"`, false},
		{"unicodé", false},
		{"semi;colon", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ids.IsSafeExternalID(tt.in), "input %q", tt.in)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	for range 1000 {
		id := ids.NewV7()
		s := ids.Format(ids.PrefixAPIKey, id)
		assert.Regexp(t, `^key_[0-9a-hjkmnp-tv-z]{26}$`, s)
		got, err := ids.Parse(ids.PrefixAPIKey, s)
		require.NoError(t, err)
		assert.Equal(t, id, got)
	}
}

func TestFormatPreservesTimeOrder(t *testing.T) {
	a := ids.Format(ids.PrefixAudit, ids.NewV7())
	time.Sleep(2 * time.Millisecond)
	b := ids.Format(ids.PrefixAudit, ids.NewV7())
	assert.Less(t, a, b)
}

func TestParseRejects(t *testing.T) {
	valid := ids.Format(ids.PrefixAPIKey, ids.NewV7())
	nonCanonical := valid[:len(valid)-1] + "z" // sets the zero padding bits
	tests := map[string]string{
		"wrong prefix":  ids.Format(ids.PrefixTenant, ids.NewV7()),
		"no prefix":     valid[4:],
		"too short":     valid[:len(valid)-1],
		"too long":      valid + "0",
		"bad character": valid[:10] + "u" + valid[11:],
		"uppercase":     strings.ToUpper(valid),
		"non-canonical": nonCanonical,
		"empty":         "",
		"sql injection": "key_' OR '1'='1",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ids.Parse(ids.PrefixAPIKey, in)
			assert.ErrorIs(t, err, ids.ErrInvalid)
		})
	}
}

func BenchmarkNewRequestID(b *testing.B) {
	for b.Loop() {
		_ = ids.NewRequestID()
	}
}
