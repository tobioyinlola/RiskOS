package service

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
)

// securityWindow is the period the security overview counts over.
const securityWindow = 24 * time.Hour

// Tenants lists tenants: identity and status only.
func (s *Service) Tenants(ctx context.Context) ([]sqlcgen.AdminTenantsRow, error) {
	out, err := s.store.Q().AdminTenants(ctx)
	return out, store.Classify(err, "list tenants")
}

// TenantDirectory lists tenants with the counts the console's directory shows: identity, status,
// origin, whether production is open, active users and keys, onboarding status. Never tenant data.
func (s *Service) TenantDirectory(ctx context.Context) ([]sqlcgen.AdminTenantDirectoryRow, error) {
	out, err := s.store.Q().AdminTenantDirectory(ctx)
	return out, store.Classify(err, "list tenant directory")
}

// TenantDetail is one tenant as the console's detail page shows it.
type TenantDetail struct {
	Tenant sqlcgen.AdminTenantDirectoryRow
	// Security counts the same signals as the security view, for this tenant, since Since.
	Security sqlcgen.SecurityOverviewRow
	Since    time.Time
	// Actions are the staff audit entries that name this tenant, newest first.
	Actions []sqlcgen.StaffAuditLog
}

// detailActions bounds the staff actions shown on a tenant's page.
const detailActions = 25

// TenantDetail returns one tenant, or ErrNotFound. Counts and staff activity only.
func (s *Service) TenantDetail(ctx context.Context, tenantID uuid.UUID) (TenantDetail, error) {
	rows, err := s.store.Q().AdminTenantDirectory(ctx)
	if err != nil {
		return TenantDetail{}, store.Classify(err, "get tenant")
	}
	i := slices.IndexFunc(rows, func(r sqlcgen.AdminTenantDirectoryRow) bool { return r.ID == tenantID })
	if i < 0 {
		return TenantDetail{}, ErrNotFound
	}
	out := TenantDetail{Tenant: rows[i], Since: s.clock.Now().Add(-securityWindow)}
	sec, err := s.store.Q().SecurityOverview(ctx, out.Since)
	if err != nil {
		return TenantDetail{}, store.Classify(err, "get tenant security")
	}
	if j := slices.IndexFunc(sec, func(r sqlcgen.SecurityOverviewRow) bool { return r.TenantID == tenantID }); j >= 0 {
		out.Security = sec[j]
	}
	out.Actions, err = s.store.Q().ListStaffAuditForTenant(ctx, sqlcgen.ListStaffAuditForTenantParams{
		TenantID: &tenantID, Lim: detailActions,
	})
	if err != nil {
		return TenantDetail{}, store.Classify(err, "get tenant activity")
	}
	return out, nil
}

// Overview returns the console's work queue for the staff member: what waits for them.
func (s *Service) Overview(ctx context.Context, staffID uuid.UUID) (sqlcgen.AdminOverviewRow, error) {
	row, err := s.store.Q().AdminOverview(ctx, staffID)
	return row, store.Classify(err, "get overview")
}

// Security returns security counts per tenant since the start of the window. Never tenant data.
func (s *Service) Security(ctx context.Context) (time.Time, []sqlcgen.SecurityOverviewRow, error) {
	since := s.clock.Now().Add(-securityWindow)
	rows, err := s.store.Q().SecurityOverview(ctx, since)
	return since, rows, store.Classify(err, "security overview")
}

// StaffAudit returns the staff audit log, newest first.
func (s *Service) StaffAudit(ctx context.Context, limit int) ([]sqlcgen.StaffAuditLog, error) {
	out, err := s.store.Q().ListStaffAudit(ctx, int32(limit)) //nolint:gosec // 1..200, validated by the handler
	return out, store.Classify(err, "list staff audit")
}
