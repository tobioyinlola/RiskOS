// Package tenancy carries the authenticated tenant scope and principal through context.Context.
//
// The scope is the only source of tenant identity (ARCHITECTURE.md §8): it is derived from the
// authenticated credential by the auth module, never from client input. Stores and the audit log
// refuse to run without it. WithScope may only be called by authentication, by bootstrap code that
// creates a tenant, and by tests; a forbidigo rule enforces this.
package tenancy

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
)

// EnvironmentKind separates test from production data under one tenant.
type EnvironmentKind string

// Environment kinds.
const (
	EnvironmentTest       EnvironmentKind = "test"
	EnvironmentProduction EnvironmentKind = "production"
)

// PrincipalType identifies who is acting.
type PrincipalType string

// Principal types.
const (
	PrincipalAPIKey PrincipalType = "api_key"
	PrincipalSystem PrincipalType = "system" // operator tooling and background jobs
	PrincipalUser   PrincipalType = "user"   // a person signed in to the console
	PrincipalStaff  PrincipalType = "staff"  // RiskOS staff with approved, read-only support access
)

// Principal is the authenticated actor and what it may do.
type Principal struct {
	Type PrincipalType
	// ID is the external ID of the actor (e.g. "key_…") or a system name (e.g. "riskosctl").
	ID          string
	Permissions []string
}

// Can reports whether the principal holds permission. Deny by default.
func (p Principal) Can(permission string) bool {
	return slices.Contains(p.Permissions, permission)
}

// Scope is the immutable tenant context of a request or job.
type Scope struct {
	TenantID        uuid.UUID
	EnvironmentID   uuid.UUID
	EnvironmentKind EnvironmentKind
	Principal       Principal
}

// ErrNoScope is returned by code that requires a tenant scope when none is present.
var ErrNoScope = errors.New("no tenant scope in context")

type scopeKey struct{}

// WithScope returns ctx carrying s. The permissions slice is copied so later changes by the
// caller cannot alter an established scope.
func WithScope(ctx context.Context, s Scope) context.Context {
	s.Principal.Permissions = slices.Clone(s.Principal.Permissions)
	return context.WithValue(ctx, scopeKey{}, s)
}

// FromContext returns the scope in ctx, or ErrNoScope.
func FromContext(ctx context.Context) (Scope, error) {
	s, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok || s.TenantID == uuid.Nil {
		return Scope{}, ErrNoScope
	}
	s.Principal.Permissions = slices.Clone(s.Principal.Permissions)
	return s, nil
}
