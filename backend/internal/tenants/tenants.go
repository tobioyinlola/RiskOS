package tenants

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/service"
)

// Public types and errors.
type (
	// Tenant is a created tenant with its environments.
	Tenant = service.Tenant
	// CreateParams describes a new tenant.
	CreateParams = service.CreateParams
)

// Errors.
var (
	ErrInvalidInput = service.ErrInvalidInput
	ErrNotFound     = service.ErrNotFound
	ErrNotActive    = service.ErrNotActive
)

// Module is the tenants module.
type Module struct {
	svc *service.Service
}

// New returns the tenants module.
func New(runner txn.Runner, rec audit.Recorder) *Module {
	return &Module{svc: service.New(runner, rec)}
}

// Create creates a tenant with test and production environments in one audited transaction.
// then, if not nil, runs in the same transaction scoped to the new test environment.
func (m *Module) Create(ctx context.Context, p CreateParams, then func(ctx context.Context, t Tenant) error) (Tenant, error) {
	return m.svc.Create(ctx, p, then)
}

// JobScope returns ctx scoped to an environment of an active tenant for a background job, holding
// only permissions. It returns ErrNotFound or ErrNotActive when the job should not run.
func (m *Module) JobScope(ctx context.Context, tenantID, envID uuid.UUID, actor string, permissions []string) (context.Context, error) {
	return m.svc.JobScope(ctx, tenantID, envID, actor, permissions)
}

// StaffScope returns ctx scoped to an environment of an active tenant for RiskOS staff with
// approved support access, holding only permissions. For the admin module only.
func (m *Module) StaffScope(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, staffRef string, permissions []string) (context.Context, error) {
	return m.svc.StaffScope(ctx, tenantID, kind, staffRef, permissions)
}

// OperatorScope returns ctx scoped to an environment of an active tenant, acting as the named
// operator tool. For operator tooling only.
func (m *Module) OperatorScope(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, actor string) (context.Context, error) {
	return m.svc.OperatorScope(ctx, tenantID, kind, actor)
}
