package tenants

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/service"
)

// Public types and errors.
type (
	// Tenant is a created tenant with its environments.
	Tenant = service.Tenant
	// CreateParams describes a new tenant.
	CreateParams = service.CreateParams
	// Onboarding is a tenant's compliance onboarding.
	Onboarding = service.OnboardingView
	// Answers are onboarding answers.
	Answers = service.Answers
)

// Errors.
var (
	ErrInvalidInput = service.ErrInvalidInput
	ErrNotFound     = service.ErrNotFound
	ErrNotActive    = service.ErrNotActive
	// ErrNoOnboarding means the tenant has no onboarding (created by an operator).
	ErrNoOnboarding = service.ErrNoOnboarding
	// ErrOnboardingState means the onboarding cannot change in its current state.
	ErrOnboardingState = service.ErrOnboardingState
)

// OnboardingHandlers are the onboarding operations of the public API.
type OnboardingHandlers interface {
	GetOnboarding(ctx context.Context, req apigen.GetOnboardingRequestObject) (apigen.GetOnboardingResponseObject, error)
	SaveOnboarding(ctx context.Context, req apigen.SaveOnboardingRequestObject) (apigen.SaveOnboardingResponseObject, error)
	SubmitOnboarding(ctx context.Context, req apigen.SubmitOnboardingRequestObject) (apigen.SubmitOnboardingResponseObject, error)
}

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

// StaffTenantScope returns ctx scoped to the tenant itself, whatever its status, with no
// environment and no permissions, so a staff member can record a tenant status change in the
// tenant's audit log. For the admin module only.
func (m *Module) StaffTenantScope(ctx context.Context, tenantID uuid.UUID, staffRef string) context.Context {
	return m.svc.StaffTenantScope(ctx, tenantID, staffRef)
}

// OperatorScope returns ctx scoped to an environment of an active tenant, acting as the named
// operator tool. For operator tooling only.
func (m *Module) OperatorScope(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, actor string) (context.Context, error) {
	return m.svc.OperatorScope(ctx, tenantID, kind, actor)
}

// HTTPHandlers returns the onboarding handlers of the public API.
func (m *Module) HTTPHandlers(clk clock.Clock) OnboardingHandlers { return handler.New(m.svc, clk) }

// StaffOnboarding returns a tenant's onboarding for RiskOS staff, recorded in the tenant's audit
// log. ctx must be a tenant scope from StaffScope. For the admin module only.
func (m *Module) StaffOnboarding(ctx context.Context) (Onboarding, error) {
	return m.svc.StaffOnboarding(ctx)
}

// ReviewOnboarding approves (opening production) or rejects (with a note for the customer) a
// submitted onboarding. ctx must be a tenant scope from StaffScope. For the admin module only.
func (m *Module) ReviewOnboarding(ctx context.Context, approve bool, note string, now time.Time) (Onboarding, error) {
	return m.svc.ReviewOnboarding(ctx, approve, note, now)
}
