// Package authz is the permission catalog and role model (RBAC, ARCHITECTURE.md §9). It is
// vocabulary shared by every module: modules check permissions with Require; the auth module
// decides which permissions a credential holds.
//
// Deny by default: a principal holds exactly the permissions it was granted, and anything not
// listed here does not exist. Customer and internal roles are disjoint sets.
package authz

import (
	"context"
	"errors"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Permission is an action on a resource type, written "<resource>:<action>". API key scopes are
// permissions.
type Permission = string

// Customer permissions. Every value must also appear in the Scope enum of api/openapi.yaml.
//
//nolint:gosec // G101 false positive: permission names, not credentials
const (
	APIKeysRead   Permission = "api_keys:read"
	APIKeysWrite  Permission = "api_keys:write"
	AuditLogsRead Permission = "audit_logs:read"
	EventsWrite   Permission = "events:write"
	EventsRead    Permission = "events:read"
	RiskEvaluate  Permission = "risk:evaluate"
	RulesRead     Permission = "rules:read"
	RulesWrite    Permission = "rules:write"
	RiskConfigure Permission = "risk:configure"
	AlertsRead    Permission = "alerts:read"
	AlertsWrite   Permission = "alerts:write"
	CasesRead     Permission = "cases:read"
	CasesWrite    Permission = "cases:write"
	WebhooksRead  Permission = "webhooks:read"
	WebhooksWrite Permission = "webhooks:write"
)

// System permissions are held only by RiskOS's own background jobs, never by a customer
// credential or role.
const (
	// AlertsRaise creates and groups alerts from evaluations.
	AlertsRaise Permission = "system:alerts_raise"
	// WebhooksDeliver attempts webhook deliveries.
	WebhooksDeliver Permission = "system:webhooks_deliver"
)

// CustomerPermissions returns every permission that can be granted to a customer credential.
func CustomerPermissions() []Permission {
	return []Permission{
		APIKeysRead, APIKeysWrite, AuditLogsRead, EventsWrite, EventsRead, RiskEvaluate,
		RulesRead, RulesWrite, RiskConfigure, AlertsRead, AlertsWrite, CasesRead, CasesWrite,
		WebhooksRead, WebhooksWrite,
	}
}

// IsCustomerPermission reports whether p exists and may be granted to a customer credential.
func IsCustomerPermission(p string) bool { return slices.Contains(CustomerPermissions(), p) }

// Role is a named bundle of permissions assigned to a person.
type Role string

// Customer roles (spec §3).
const (
	RoleCustomerAdmin Role = "customer_admin"
	RoleDeveloper     Role = "developer"
	RoleRiskManager   Role = "risk_manager"
	RoleAnalyst       Role = "analyst"
)

// Internal RiskOS roles. They grant no access to tenant data: support access to a tenant is a
// separate, time-boxed, approved grant (ARCHITECTURE.md §8).
const (
	RoleOperationsAdmin Role = "operations_admin"
	RoleSecurityAdmin   Role = "security_admin"
)

// RolePermissions returns the permissions of a customer role. Internal roles and unknown roles
// have none.
func RolePermissions(r Role) []Permission {
	switch r {
	case RoleCustomerAdmin:
		return CustomerPermissions()
	case RoleDeveloper:
		return []Permission{
			APIKeysRead, APIKeysWrite, EventsWrite, EventsRead, RiskEvaluate, RulesRead, WebhooksRead, WebhooksWrite,
		}
	case RoleRiskManager:
		return []Permission{
			EventsRead, RiskEvaluate, RulesRead, RulesWrite, RiskConfigure,
			AlertsRead, AlertsWrite, CasesRead, CasesWrite,
		}
	case RoleAnalyst:
		return []Permission{EventsRead, RulesRead, AlertsRead, AlertsWrite, CasesRead, CasesWrite}
	case RoleOperationsAdmin, RoleSecurityAdmin:
		return nil
	default:
		return nil
	}
}

// RequireAny returns ErrForbidden unless the principal in ctx holds at least one of ps.
func RequireAny(ctx context.Context, ps ...Permission) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return errors.Join(ErrForbidden, err)
	}
	for _, p := range ps {
		if scope.Principal.Can(p) {
			return nil
		}
	}
	return ErrForbidden
}

// ErrForbidden means the caller is authenticated but lacks a permission.
var ErrForbidden = errors.New("forbidden")

// Require returns ErrForbidden unless the principal in ctx holds p. A missing scope is also a
// denial (tenancy.ErrNoScope is wrapped).
func Require(ctx context.Context, p Permission) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return errors.Join(ErrForbidden, err)
	}
	if !scope.Principal.Can(p) {
		return ErrForbidden
	}
	return nil
}
