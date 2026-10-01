// Package service holds the tenants module's business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/store"
)

// Errors returned by the service.
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("tenant or environment not found")
	ErrNotActive    = errors.New("tenant is not active")
)

var regionPattern = regexp.MustCompile(`^[a-z0-9-]{2,32}$`) //nolint:gochecknoglobals // compiled once, never mutated

// Tenant is a created tenant.
type Tenant struct {
	ID            uuid.UUID
	Name          string
	Region        string
	TestEnvID     uuid.UUID
	ProductionEnv uuid.UUID
}

// CreateParams describes a new tenant.
type CreateParams struct {
	Name   string
	Region string
	// Actor names the operator tool or person creating the tenant, for the audit log.
	Actor string
}

// Service creates tenants and resolves environments.
type Service struct {
	store  store.Store
	runner txn.Runner
	audit  audit.Recorder
}

// New returns a Service.
func New(runner txn.Runner, rec audit.Recorder) *Service {
	return &Service{runner: runner, audit: rec}
}

// Create creates a tenant with its test and production environments and audits it, all in one
// transaction. then, if not nil, runs inside the same transaction with a context scoped to the
// new tenant's test environment (used to issue the first API key atomically).
func (s *Service) Create(ctx context.Context, p CreateParams, then func(ctx context.Context, t Tenant) error) (Tenant, error) {
	p.Name = strings.TrimSpace(p.Name)
	if n := utf8.RuneCountInString(p.Name); n < 1 || n > 200 {
		return Tenant{}, fmt.Errorf("%w: name must be 1-200 characters", ErrInvalidInput)
	}
	if !regionPattern.MatchString(p.Region) {
		return Tenant{}, fmt.Errorf("%w: region must match %s", ErrInvalidInput, regionPattern)
	}
	if strings.TrimSpace(p.Actor) == "" {
		return Tenant{}, fmt.Errorf("%w: actor is required", ErrInvalidInput)
	}

	t := Tenant{ID: ids.NewV7(), Name: p.Name, Region: p.Region, TestEnvID: ids.NewV7(), ProductionEnv: ids.NewV7()}
	ctx = tenancy.WithScope(ctx, systemScope(t.ID, t.TestEnvID, tenancy.EnvironmentTest, p.Actor))

	err := s.runner.Run(ctx, func(ctx context.Context) error {
		if err := s.store.InsertTenant(ctx, t.ID, t.Name, t.Region); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "tenant.created", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, t.ID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true,
			Metadata: map[string]any{"name": t.Name, "region": t.Region},
		}); err != nil {
			return err
		}
		for _, env := range []struct {
			id   uuid.UUID
			kind tenancy.EnvironmentKind
		}{{t.TestEnvID, tenancy.EnvironmentTest}, {t.ProductionEnv, tenancy.EnvironmentProduction}} {
			if err := s.store.InsertEnvironment(ctx, env.id, t.ID, string(env.kind)); err != nil {
				return err
			}
			if err := s.audit.Record(ctx, audit.Entry{
				Action: "environment.created", ResourceType: "environment", ResourceID: ids.Format(ids.PrefixEnvironment, env.id),
				Outcome: audit.OutcomeSuccess, TenantLevel: true,
				Metadata: map[string]any{"kind": string(env.kind)},
			}); err != nil {
				return err
			}
		}
		if then != nil {
			return then(ctx, t)
		}
		return nil
	})
	if err != nil {
		return Tenant{}, err
	}
	return t, nil
}

// OperatorScope returns a context scoped to an environment of an existing, active tenant, acting
// as the named operator tool with every customer permission. It is used by operator tooling
// (riskosctl) only; customer requests get their scope from their credential.
func (s *Service) OperatorScope(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, actor string) (context.Context, error) {
	if kind != tenancy.EnvironmentTest && kind != tenancy.EnvironmentProduction {
		return nil, fmt.Errorf("%w: environment must be test or production", ErrInvalidInput)
	}
	lookupCtx := tenancy.WithScope(ctx, systemScope(tenantID, uuid.Nil, kind, actor))
	var envID uuid.UUID
	var status string
	err := s.runner.Run(lookupCtx, func(ctx context.Context) error {
		var err error
		envID, status, err = s.store.EnvironmentByKind(ctx, tenantID, string(kind))
		return err
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrNotActive
	}
	return tenancy.WithScope(ctx, systemScope(tenantID, envID, kind, actor)), nil
}

// StaffScope returns ctx scoped to an environment of an active tenant for RiskOS staff holding
// approved support access, with exactly permissions. Only the admin module calls it, after
// checking the access grant.
func (s *Service) StaffScope(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, staffRef string, permissions []string) (context.Context, error) {
	ctx, err := s.OperatorScope(ctx, tenantID, kind, staffRef)
	if err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	scope.Principal = tenancy.Principal{Type: tenancy.PrincipalStaff, ID: staffRef, Permissions: permissions}
	return tenancy.WithScope(ctx, scope), nil
}

// JobScope returns ctx scoped to an environment of an active tenant for a background job, acting
// as actor with exactly permissions. Jobs carry tenant and environment IDs that the server wrote
// when it enqueued them; this re-checks that both exist and that the tenant is still active.
func (s *Service) JobScope(ctx context.Context, tenantID, envID uuid.UUID, actor string, permissions []string) (context.Context, error) {
	lookupCtx := tenancy.WithScope(ctx, systemScope(tenantID, envID, "", actor))
	var kind, status string
	err := s.runner.Run(lookupCtx, func(ctx context.Context) error {
		var err error
		kind, status, err = s.store.Environment(ctx, tenantID, envID)
		return err
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrNotActive
	}
	scope := systemScope(tenantID, envID, tenancy.EnvironmentKind(kind), actor)
	scope.Principal.Permissions = permissions
	return tenancy.WithScope(ctx, scope), nil
}

func systemScope(tenantID, envID uuid.UUID, kind tenancy.EnvironmentKind, actor string) tenancy.Scope {
	return tenancy.Scope{
		TenantID:        tenantID,
		EnvironmentID:   envID,
		EnvironmentKind: kind,
		Principal: tenancy.Principal{
			Type:        tenancy.PrincipalSystem,
			ID:          actor,
			Permissions: authz.CustomerPermissions(),
		},
	}
}
