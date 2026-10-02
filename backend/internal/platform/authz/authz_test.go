package authz_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

func TestRolePermissions(t *testing.T) {
	assert.ElementsMatch(t, authz.PersonPermissions(), authz.RolePermissions(authz.RoleCustomerAdmin))
	assert.False(t, authz.IsCustomerPermission(authz.RulesApprove), "API keys can never approve rule changes")
	assert.NotContains(t, authz.RolePermissions(authz.RoleDeveloper), authz.AuditLogsRead)
	assert.False(t, authz.IsCustomerPermission(authz.ComplianceWrite), "API keys can never set the compliance profile")

	// Auditors only read; only engineers and admins see the engineering logic.
	for _, p := range authz.RolePermissions(authz.RoleAuditor) {
		assert.Regexp(t, `:read$`, p)
	}
	for _, r := range authz.CustomerRoles() {
		want := r == authz.RoleCustomerAdmin || r == authz.RoleDeveloper
		assert.Equal(t, want, slices.Contains(authz.RolePermissions(r), authz.ComplianceEngineering), "role %s", r)
		assert.Contains(t, authz.RolePermissions(r), authz.ComplianceRead, "role %s", r)
	}

	// Internal roles never grant tenant data access; unknown roles grant nothing (deny by default).
	for _, r := range []authz.Role{authz.RoleOperationsAdmin, authz.RoleSecurityAdmin, "root", ""} {
		assert.Empty(t, authz.RolePermissions(r), "role %q", r)
	}
	// Every role grants only permissions people can hold.
	for _, r := range authz.CustomerRoles() {
		for _, p := range authz.RolePermissions(r) {
			assert.Contains(t, authz.PersonPermissions(), p, "%s grants unknown %s", r, p)
		}
	}
}

func TestRequire(t *testing.T) {
	ctx := tenancy.WithScope(context.Background(), tenancy.Scope{
		TenantID:  uuid.New(),
		Principal: tenancy.Principal{Permissions: []string{authz.APIKeysRead}},
	})
	assert.NoError(t, authz.Require(ctx, authz.APIKeysRead))
	assert.ErrorIs(t, authz.Require(ctx, authz.APIKeysWrite), authz.ErrForbidden)
	assert.ErrorIs(t, authz.Require(context.Background(), authz.APIKeysRead), authz.ErrForbidden, "no scope is a denial")
}

func TestScopeIsImmutable(t *testing.T) {
	perms := []string{authz.APIKeysRead}
	ctx := tenancy.WithScope(context.Background(), tenancy.Scope{TenantID: uuid.New(), Principal: tenancy.Principal{Permissions: perms}})
	perms[0] = authz.APIKeysWrite // the caller mutates its slice afterwards

	assert.ErrorIs(t, authz.Require(ctx, authz.APIKeysWrite), authz.ErrForbidden)
	s, _ := tenancy.FromContext(ctx)
	s.Principal.Permissions[0] = authz.AuditLogsRead // a reader mutates its copy
	assert.ErrorIs(t, authz.Require(ctx, authz.AuditLogsRead), authz.ErrForbidden)
}
