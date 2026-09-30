package ids_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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

func BenchmarkNewRequestID(b *testing.B) {
	for b.Loop() {
		_ = ids.NewRequestID()
	}
}
