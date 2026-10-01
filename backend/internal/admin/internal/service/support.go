package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// SupportPermissions are what approved support access may do: read, never write. Risk settings
// (risk:configure) and anything else that changes state are out of reach.
func SupportPermissions() []string {
	return []string{
		authz.EventsRead, authz.AlertsRead, authz.CasesRead, authz.RulesRead, authz.AuditLogsRead,
		authz.WebhooksRead, authz.APIKeysRead, authz.TeamRead,
	}
}

// SupportAccess is a request with display names.
type SupportAccess struct {
	sqlcgen.SupportAccess
	TenantName    string
	RequesterName string
	DeciderName   string
}

// Status is the request's status, with "expired" for approved access past its end.
func (a SupportAccess) Status(now time.Time) string {
	if a.SupportAccess.Status == "approved" && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return "expired"
	}
	return a.SupportAccess.Status
}

// Active reports whether the access can be used now.
func (a SupportAccess) Active(now time.Time) bool { return a.Status(now) == "approved" }

func accessRef(id uuid.UUID) string { return ids.Format(ids.PrefixSupportAccess, id) }

// tenantRecord writes an entry in the tenant's own audit log, in the transaction of tctx.
func (s *Service) tenantRecord(ctx context.Context, action string, a uuid.UUID, meta map[string]any) error {
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "support_access", ResourceID: accessRef(a), Outcome: audit.OutcomeSuccess,
		Metadata: meta, TenantLevel: true,
	})
}

// staffTenant returns a tenant-scoped context acting as the staff member, or ErrNotFound /
// ErrTenantNotActive.
func (s *Service) staffTenant(ctx context.Context, tenantID uuid.UUID, kind tenancy.EnvironmentKind, ref string, perms []string) (context.Context, error) {
	tctx, err := s.tenants.StaffScope(ctx, tenantID, kind, ref, perms)
	switch {
	case err == nil:
		return tctx, nil
	case errors.Is(err, s.tenantErr.NotFound):
		return nil, ErrNotFound
	case errors.Is(err, s.tenantErr.NotActive):
		return nil, ErrTenantNotActive
	default:
		return nil, err
	}
}

// RequestSupport asks for read-only access to a tenant for minutes (15 to 240). Recorded in the
// staff audit log and the tenant's.
func (s *Service) RequestSupport(ctx context.Context, sess Session, tenantID uuid.UUID, reason string, minutes int) (SupportAccess, error) {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 10 || n > 500 {
		return SupportAccess{}, fmt.Errorf("%w: give a reason of 10 to 500 characters (the customer sees it)", ErrInvalid)
	}
	if minutes < 15 || minutes > 240 {
		return SupportAccess{}, fmt.Errorf("%w: access lasts 15 to 240 minutes", ErrInvalid)
	}
	tctx, err := s.staffTenant(ctx, tenantID, tenancy.EnvironmentTest, sess.Ref(), nil)
	if err != nil {
		return SupportAccess{}, err
	}
	id := ids.NewV7()
	err = s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		if err := q.InsertSupportAccess(ctx, sqlcgen.InsertSupportAccessParams{
			ID: id, TenantID: tenantID, RequestedBy: sess.StaffID, Reason: reason, DurationMinutes: int32(minutes), Now: s.clock.Now(), //nolint:gosec // 15..240
		}); err != nil {
			return store.Classify(err, "insert support access")
		}
		if err := s.record(ctx, q, sess.Ref(), "support_access.requested", &tenantID, accessRef(id), "success",
			map[string]any{"reason": reason, "duration_minutes": minutes}); err != nil {
			return err
		}
		return s.tenantRecord(ctx, "support_access.requested", id, map[string]any{
			"staff_name": sess.Name, "reason": reason, "duration_minutes": minutes,
		})
	})
	if err != nil {
		return SupportAccess{}, err
	}
	return s.GetSupport(ctx, id)
}

// decide applies an approval, rejection or revocation in one transaction with both audit logs.
func (s *Service) decide(ctx context.Context, sess Session, id uuid.UUID, action string) (SupportAccess, error) {
	a, err := s.store.Q().GetSupportAccess(ctx, id)
	if err := store.Classify(err, "get support access"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return SupportAccess{}, ErrNotFound
		}
		return SupportAccess{}, err
	}
	// Recording works even for a tenant suspended since: test environment, no permissions.
	tctx, err := s.staffTenant(ctx, a.TenantID, tenancy.EnvironmentTest, sess.Ref(), nil)
	if errors.Is(err, ErrTenantNotActive) && action == "revoke" {
		// Revoking must always work; the tenant's log is unreachable while it is suspended.
		return s.revokeWithoutTenant(ctx, sess, id)
	}
	if err != nil {
		return SupportAccess{}, err
	}
	err = s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		cur, err := q.GetSupportAccessForUpdate(ctx, id)
		if err != nil {
			return store.Classify(err, "lock support access")
		}
		now := s.clock.Now()
		meta := map[string]any{"staff_name": sess.Name}
		switch action {
		case "approve", "reject":
			if cur.Status != "pending" {
				return fmt.Errorf("%w: the request is %s", ErrConflict, cur.Status)
			}
			if cur.RequestedBy == sess.StaffID {
				return fmt.Errorf("%w: another staff member must decide on your request", ErrForbidden)
			}
			status := "rejected"
			var expires *time.Time
			if action == "approve" {
				status = "approved"
				e := now.Add(time.Duration(cur.DurationMinutes) * time.Minute)
				expires = &e
				meta["expires_at"] = e.UTC().Format(time.RFC3339)
			}
			if err := q.DecideSupportAccess(ctx, sqlcgen.DecideSupportAccessParams{
				ID: id, Status: status, DecidedBy: &sess.StaffID, Now: now, ExpiresAt: expires,
			}); err != nil {
				return store.Classify(err, "decide support access")
			}
		case "revoke":
			if (SupportAccess{SupportAccess: cur}).Status(now) != "approved" && cur.Status != "pending" {
				return fmt.Errorf("%w: the request is not pending or active", ErrConflict)
			}
			if err := q.RevokeSupportAccess(ctx, sqlcgen.RevokeSupportAccessParams{ID: id, RevokedBy: &sess.StaffID, Now: now}); err != nil {
				return store.Classify(err, "revoke support access")
			}
		}
		past := map[string]string{"approve": "approved", "reject": "rejected", "revoke": "revoked"}[action]
		if err := s.record(ctx, q, sess.Ref(), "support_access."+past, &cur.TenantID, accessRef(id), "success", meta); err != nil {
			return err
		}
		return s.tenantRecord(ctx, "support_access."+past, id, meta)
	})
	if err != nil {
		return SupportAccess{}, err
	}
	return s.GetSupport(ctx, id)
}

func (s *Service) revokeWithoutTenant(ctx context.Context, sess Session, id uuid.UUID) (SupportAccess, error) {
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		cur, err := q.GetSupportAccessForUpdate(ctx, id)
		if err != nil {
			return store.Classify(err, "lock support access")
		}
		if err := q.RevokeSupportAccess(ctx, sqlcgen.RevokeSupportAccessParams{ID: id, RevokedBy: &sess.StaffID, Now: s.clock.Now()}); err != nil {
			return store.Classify(err, "revoke support access")
		}
		return s.record(ctx, q, sess.Ref(), "support_access.revoked", &cur.TenantID, accessRef(id), "success",
			map[string]any{"tenant_inactive": true})
	})
	if err != nil {
		return SupportAccess{}, err
	}
	return s.GetSupport(ctx, id)
}

// ApproveSupport approves another staff member's pending request; access starts now.
func (s *Service) ApproveSupport(ctx context.Context, sess Session, id uuid.UUID) (SupportAccess, error) {
	return s.decide(ctx, sess, id, "approve")
}

// RejectSupport rejects another staff member's pending request.
func (s *Service) RejectSupport(ctx context.Context, sess Session, id uuid.UUID) (SupportAccess, error) {
	return s.decide(ctx, sess, id, "reject")
}

// RevokeSupport ends active access early or withdraws a pending request. Any staff member.
func (s *Service) RevokeSupport(ctx context.Context, sess Session, id uuid.UUID) (SupportAccess, error) {
	return s.decide(ctx, sess, id, "revoke")
}

// names loads display names for staff and tenants.
func (s *Service) names(ctx context.Context) (map[uuid.UUID]string, map[uuid.UUID]string, error) {
	q := s.store.Q()
	staff, err := q.ListStaff(ctx)
	if err != nil {
		return nil, nil, store.Classify(err, "list staff")
	}
	tenants, err := q.AdminTenants(ctx)
	if err != nil {
		return nil, nil, store.Classify(err, "list tenants")
	}
	sn := make(map[uuid.UUID]string, len(staff))
	for _, st := range staff {
		sn[st.ID] = st.Name
	}
	tn := make(map[uuid.UUID]string, len(tenants))
	for _, t := range tenants {
		tn[t.ID] = t.Name
	}
	return sn, tn, nil
}

func enrich(a sqlcgen.SupportAccess, sn, tn map[uuid.UUID]string) SupportAccess {
	out := SupportAccess{SupportAccess: a, TenantName: tn[a.TenantID], RequesterName: sn[a.RequestedBy]}
	if a.DecidedBy != nil {
		out.DeciderName = sn[*a.DecidedBy]
	}
	return out
}

// GetSupport returns one request.
func (s *Service) GetSupport(ctx context.Context, id uuid.UUID) (SupportAccess, error) {
	a, err := s.store.Q().GetSupportAccess(ctx, id)
	if err := store.Classify(err, "get support access"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return SupportAccess{}, ErrNotFound
		}
		return SupportAccess{}, err
	}
	sn, tn, err := s.names(ctx)
	if err != nil {
		return SupportAccess{}, err
	}
	return enrich(a, sn, tn), nil
}

// ListSupport returns recent requests, newest first.
func (s *Service) ListSupport(ctx context.Context) ([]SupportAccess, error) {
	rows, err := s.store.Q().ListSupportAccess(ctx)
	if err != nil {
		return nil, store.Classify(err, "list support access")
	}
	sn, tn, err := s.names(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SupportAccess, len(rows))
	for i, r := range rows {
		out[i] = enrich(r, sn, tn)
	}
	return out, nil
}

// SupportScope authorizes one customer API request made with approved support access: the
// staff session must be valid, the access approved, unexpired and the caller's own. It returns
// a read-only scope in the access's tenant and records the request in the tenant's audit log
// and the staff audit log.
func (s *Service) SupportScope(ctx context.Context, staffToken, grant string, kind tenancy.EnvironmentKind, request string) (tenancy.Scope, error) {
	sess, err := s.ResolveSession(ctx, staffToken)
	if err != nil {
		return tenancy.Scope{}, err
	}
	id, err := ids.Parse(ids.PrefixSupportAccess, grant)
	if err != nil {
		return tenancy.Scope{}, ErrForbidden
	}
	a, err := s.store.Q().GetSupportAccess(ctx, id)
	if err := store.Classify(err, "get support access"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return tenancy.Scope{}, ErrForbidden
		}
		return tenancy.Scope{}, err
	}
	if a.RequestedBy != sess.StaffID || !(SupportAccess{SupportAccess: a}).Active(s.clock.Now()) {
		return tenancy.Scope{}, ErrForbidden
	}
	tctx, err := s.staffTenant(ctx, a.TenantID, kind, sess.Ref(), SupportPermissions())
	if err != nil {
		return tenancy.Scope{}, ErrForbidden
	}
	err = s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		meta := map[string]any{"request": request, "environment": string(kind)}
		if err := s.record(ctx, q, sess.Ref(), "support_access.viewed", &a.TenantID, accessRef(id), "success", meta); err != nil {
			return err
		}
		meta["staff_name"] = sess.Name
		return s.tenantRecord(ctx, "support_access.viewed", id, meta)
	})
	if err != nil {
		return tenancy.Scope{}, err
	}
	return tenancy.FromContext(tctx)
}
