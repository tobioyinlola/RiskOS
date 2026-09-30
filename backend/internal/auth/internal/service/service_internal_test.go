package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

func TestValidateScopes(t *testing.T) {
	admin := tenancy.Principal{Permissions: authz.RolePermissions(authz.RoleCustomerAdmin)}
	developer := tenancy.Principal{Permissions: authz.RolePermissions(authz.RoleDeveloper)}
	tests := []struct {
		name    string
		scopes  []string
		caller  tenancy.Principal
		want    []string
		wantErr error
	}{
		{"subset, sorted", []string{authz.APIKeysWrite, authz.APIKeysRead}, admin, []string{authz.APIKeysRead, authz.APIKeysWrite}, nil},
		{"none", nil, admin, nil, domain.ErrInvalidInput},
		{"duplicate", []string{authz.APIKeysRead, authz.APIKeysRead}, admin, nil, domain.ErrInvalidInput},
		{"unknown scope", []string{"events:delete_everything"}, admin, nil, domain.ErrInvalidInput},
		{"internal-looking scope", []string{"tenants:manage"}, admin, nil, domain.ErrInvalidInput},
		{"escalation is forbidden", []string{authz.AuditLogsRead}, developer, nil, authz.ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateScopes(tt.scopes, tt.caller)
			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr), "got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateName(t *testing.T) {
	got, err := validateName("  ci deploy key  ")
	require.NoError(t, err)
	assert.Equal(t, "ci deploy key", got)

	for _, bad := range []string{"", "   ", string(make([]rune, 101))} {
		_, err := validateName(bad)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	}
}

func TestLookupCache(t *testing.T) {
	now := time.Now()
	res := &store.LookupResult{TenantStatus: "active"}

	t.Run("hit within ttl, miss after", func(t *testing.T) {
		c := newLookupCache(5*time.Second, 10)
		c.put("a", res, now)
		got, ok := c.get("a", now.Add(4*time.Second))
		assert.True(t, ok)
		assert.Same(t, res, got)
		_, ok = c.get("a", now.Add(5*time.Second))
		assert.False(t, ok, "revocation staleness is bounded by the ttl")
	})
	t.Run("negative results are cached", func(t *testing.T) {
		c := newLookupCache(time.Second, 10)
		c.put("unknown", nil, now)
		got, ok := c.get("unknown", now)
		assert.True(t, ok)
		assert.Nil(t, got)
	})
	t.Run("delete evicts immediately", func(t *testing.T) {
		c := newLookupCache(time.Minute, 10)
		c.put("a", res, now)
		c.delete("a")
		_, ok := c.get("a", now)
		assert.False(t, ok)
	})
	t.Run("bounded size", func(t *testing.T) {
		c := newLookupCache(time.Minute, 3)
		for _, k := range []string{"a", "b", "c", "d"} {
			c.put(k, res, now)
		}
		assert.LessOrEqual(t, len(c.entries), 3)
		_, ok := c.get("d", now)
		assert.True(t, ok, "the newest entry survives a reset")
	})
	t.Run("zero ttl disables caching", func(t *testing.T) {
		c := newLookupCache(0, 10)
		c.put("a", res, now)
		_, ok := c.get("a", now)
		assert.False(t, ok)
	})
}
