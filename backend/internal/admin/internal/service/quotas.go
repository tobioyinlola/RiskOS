package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// maxMonthlyEvents bounds a quota: the table's check, restated so the caller gets a message.
const maxMonthlyEvents = 1_000_000_000_000

// usageActor is who the worker's measurements are recorded as.
const usageActor = "system:usage-check"

// QuotaInput is the quota staff set for a tenant.
type QuotaInput struct {
	MonthlyEvents int64
	Reason        string
}

// QuotaView is a tenant's quota for the console: nil Quota means none is set.
type QuotaView struct {
	Quota         *sqlcgen.TenantQuota
	UpdatedByName string
}

// Exceeded reports whether the measured month is over the quota.
func (v QuotaView) Exceeded() bool {
	q := v.Quota
	return q != nil && q.UsageMonth != nil && q.UsageEvents > q.MonthlyEvents
}

// monthStart is the first instant of t's UTC calendar month.
func monthStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Quota returns a tenant's quota and what it has used.
func (s *Service) Quota(ctx context.Context, tenantID uuid.UUID) (QuotaView, error) {
	if err := s.tenantExists(ctx, tenantID); err != nil {
		return QuotaView{}, err
	}
	row, err := s.store.Q().GetTenantQuota(ctx, tenantID)
	switch {
	case err == nil:
		sn, _, err := s.names(ctx)
		if err != nil {
			return QuotaView{}, err
		}
		return QuotaView{Quota: &row, UpdatedByName: sn[row.UpdatedBy]}, nil
	case errors.Is(store.Classify(err, "get tenant quota"), store.ErrNotFound):
		return QuotaView{}, nil
	default:
		return QuotaView{}, store.Classify(err, "get tenant quota")
	}
}

// SetQuota sets a tenant's monthly event quota, with a reason. It starts a new measurement. Recorded
// in the staff audit log with the reason, and in the tenant's own audit log without it.
func (s *Service) SetQuota(ctx context.Context, sess Session, tenantID uuid.UUID, in QuotaInput) (QuotaView, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 10 || n > 500 {
		return QuotaView{}, fmt.Errorf("%w: give a reason of 10 to 500 characters", ErrInvalid)
	}
	if in.MonthlyEvents < 1 || in.MonthlyEvents > maxMonthlyEvents {
		return QuotaView{}, fmt.Errorf("%w: the quota must be between 1 and %d events a month", ErrInvalid, int64(maxMonthlyEvents))
	}
	if err := s.tenantExists(ctx, tenantID); err != nil {
		return QuotaView{}, err
	}
	tctx := s.tenants.StaffTenantScope(ctx, tenantID, sess.Ref())
	err := s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		if err := q.UpsertTenantQuota(ctx, sqlcgen.UpsertTenantQuotaParams{
			TenantID: tenantID, MonthlyEvents: in.MonthlyEvents, Reason: in.Reason, UpdatedBy: sess.StaffID, Now: s.clock.Now(),
		}); err != nil {
			return store.Classify(err, "set tenant quota")
		}
		if err := s.record(ctx, q, sess.Ref(), "tenant_quota.set", &tenantID, ids.Format(ids.PrefixTenant, tenantID), "success",
			map[string]any{"monthly_events": in.MonthlyEvents, "reason": in.Reason}); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "tenant.quota_changed", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, tenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true,
			Metadata: map[string]any{"monthly_events": in.MonthlyEvents, "staff_name": sess.Name},
		})
	})
	if err != nil {
		return QuotaView{}, err
	}
	return s.Quota(ctx, tenantID)
}

// ClearQuota removes a tenant's quota.
func (s *Service) ClearQuota(ctx context.Context, sess Session, tenantID uuid.UUID) (QuotaView, error) {
	if err := s.tenantExists(ctx, tenantID); err != nil {
		return QuotaView{}, err
	}
	tctx := s.tenants.StaffTenantScope(ctx, tenantID, sess.Ref())
	err := s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		n, err := q.DeleteTenantQuota(ctx, tenantID)
		if err != nil {
			return store.Classify(err, "clear tenant quota")
		}
		if n == 0 {
			return nil // nothing to clear is the state asked for
		}
		if err := s.record(ctx, q, sess.Ref(), "tenant_quota.cleared", &tenantID, ids.Format(ids.PrefixTenant, tenantID), "success", nil); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "tenant.quota_changed", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, tenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true,
			Metadata: map[string]any{"cleared": true, "staff_name": sess.Name},
		})
	})
	if err != nil {
		return QuotaView{}, err
	}
	return s.Quota(ctx, tenantID)
}

// CheckUsage counts each quota-holding tenant's events this month and announces, once a month, the
// ones over their quota: in the staff audit log and in the tenant's own. It refuses nothing. A
// tenant that fails is logged and the rest are still measured.
func (s *Service) CheckUsage(ctx context.Context, logger *slog.Logger) error {
	now := s.clock.Now()
	month := monthStart(now)
	quotas, err := s.store.Q().ListTenantQuotas(ctx)
	if err != nil {
		return store.Classify(err, "list tenant quotas")
	}
	var failed []error
	for _, q := range quotas {
		if err := s.checkOne(ctx, q, month, now); err != nil {
			logger.WarnContext(ctx, "usage check failed", slog.String("tenant", ids.Format(ids.PrefixTenant, q.TenantID)), slog.Any("error", err))
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}

func (s *Service) checkOne(ctx context.Context, q sqlcgen.TenantQuota, month, now time.Time) error {
	count, err := s.store.Q().CountTenantEvents(ctx, sqlcgen.CountTenantEventsParams{TenantID: q.TenantID, Since: month})
	if err != nil {
		return store.Classify(err, "count tenant events")
	}
	if err := s.store.Q().SetTenantQuotaUsage(ctx, sqlcgen.SetTenantQuotaUsageParams{
		TenantID: q.TenantID, UsageMonth: &month, UsageEvents: count, Now: &now,
	}); err != nil {
		return store.Classify(err, "set tenant quota usage")
	}
	if count <= q.MonthlyEvents {
		return nil
	}
	tctx := s.tenants.StaffTenantScope(ctx, q.TenantID, usageActor)
	return s.runner.Run(tctx, func(ctx context.Context) error {
		sq, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		claimed, err := sq.MarkTenantQuotaExceeded(ctx, sqlcgen.MarkTenantQuotaExceededParams{TenantID: q.TenantID, Month: &month})
		if err != nil {
			return store.Classify(err, "mark tenant quota exceeded")
		}
		if claimed == 0 {
			return nil // announced already this month
		}
		meta := map[string]any{
			"month": month.Format("2006-01"), "monthly_events": q.MonthlyEvents, "events": count,
		}
		if err := s.record(ctx, sq, usageActor, "tenant_quota.exceeded", &q.TenantID, ids.Format(ids.PrefixTenant, q.TenantID), "success", meta); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "tenant.quota_exceeded", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, q.TenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: meta,
		})
	})
}
