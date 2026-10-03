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
	TeamRead      Permission = "team:read"
	TeamWrite     Permission = "team:write"
	// ComplianceRead reads the workspace's rule pack: rules, sources, clocks and gaps.
	ComplianceRead Permission = "compliance:read"
	// ComplianceEngineering also shows each rule's engineering logic and machine definition.
	ComplianceEngineering Permission = "compliance:engineering"
)

// RulesApprove approves rule changes proposed for a production environment (two-person rule).
// Only people hold it, through their role: an API key can never be granted it, so an approval
// is always a second person's decision.
//
//nolint:gosec // G101 false positive: a permission name
const RulesApprove Permission = "rules:approve"

// ComplianceWrite sets the workspace's compliance profile (jurisdiction, licence type and
// capabilities), which decides the regulatory controls that apply. Only people hold it.
//
//nolint:gosec // G101 false positive: a permission name
const ComplianceWrite Permission = "compliance:write"

// ComplianceConfigure sets a rule pack rule's mode and settings in an environment. Only people
// hold it; in production, a change that could loosen a regulatory control also needs a second
// person with RulesApprove.
//
//nolint:gosec // G101 false positive: a permission name
const ComplianceConfigure Permission = "compliance:configure"

// ComplianceSensitive sees what must never tip off a customer (GOV-09): suspicious transaction
// reports, confirmed suspicion and compliance-only signals. Only compliance officers hold it,
// not even administrators, and never an API key.
//
//nolint:gosec // G101 false positive: a permission name
const ComplianceSensitive Permission = "compliance:sensitive"

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
		WebhooksRead, WebhooksWrite, TeamRead, TeamWrite, ComplianceRead, ComplianceEngineering,
	}
}

// PersonPermissions returns every permission a customer role can hold: the customer
// permissions plus those reserved for people.
func PersonPermissions() []Permission {
	return append(CustomerPermissions(), RulesApprove, ComplianceWrite, ComplianceConfigure, ComplianceSensitive)
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
	// RoleComplianceOfficer runs the compliance programme: the rule pack, approvals, alerts and
	// cases.
	RoleComplianceOfficer Role = "compliance_officer"
	// RoleAuditor reads, and changes nothing.
	RoleAuditor Role = "auditor"
)

// CustomerRoles returns every customer role.
func CustomerRoles() []Role {
	return []Role{RoleCustomerAdmin, RoleDeveloper, RoleRiskManager, RoleAnalyst, RoleComplianceOfficer, RoleAuditor}
}

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
		// Everything but compliance-sensitive data, which only compliance roles see (GOV-09).
		return slices.DeleteFunc(PersonPermissions(), func(p Permission) bool { return p == ComplianceSensitive })
	case RoleDeveloper:
		return []Permission{
			APIKeysRead, APIKeysWrite, EventsWrite, EventsRead, RiskEvaluate, RulesRead, WebhooksRead, WebhooksWrite,
			ComplianceRead, ComplianceEngineering,
		}
	case RoleRiskManager:
		return []Permission{
			EventsRead, RiskEvaluate, RulesRead, RulesWrite, RulesApprove, RiskConfigure,
			AlertsRead, AlertsWrite, CasesRead, CasesWrite, ComplianceRead, ComplianceConfigure,
		}
	case RoleAnalyst:
		return []Permission{EventsRead, RulesRead, AlertsRead, AlertsWrite, CasesRead, CasesWrite, ComplianceRead}
	case RoleComplianceOfficer:
		return []Permission{
			EventsRead, RulesRead, RulesApprove, AlertsRead, AlertsWrite, CasesRead, CasesWrite, AuditLogsRead,
			TeamRead, ComplianceRead, ComplianceWrite, ComplianceConfigure, ComplianceSensitive,
		}
	case RoleAuditor:
		return []Permission{EventsRead, RulesRead, AlertsRead, CasesRead, AuditLogsRead, TeamRead, ComplianceRead}
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
