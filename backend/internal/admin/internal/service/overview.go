package service

import (
	"context"
	"time"

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
