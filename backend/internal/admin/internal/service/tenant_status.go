package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// Tenant status actions.
const (
	StatusActionSuspend    = "suspend"
	StatusActionReactivate = "reactivate"
)

// TenantStatusRequest is a request to suspend or reactivate a tenant, with display names.
type TenantStatusRequest struct {
	sqlcgen.TenantStatusRequest
	TenantName    string
	RequesterName string
	DeciderName   string
	ReviewerName  string
}

func statusRequestRef(id uuid.UUID) string { return ids.Format(ids.PrefixTenantStatusRequest, id) }

func enrichStatus(r sqlcgen.TenantStatusRequest, sn, tn map[uuid.UUID]string) TenantStatusRequest {
	out := TenantStatusRequest{TenantStatusRequest: r, TenantName: tn[r.TenantID], RequesterName: sn[r.RequestedBy]}
	if r.DecidedBy != nil {
		out.DeciderName = sn[*r.DecidedBy]
	}
	if r.ReviewedBy != nil {
		out.ReviewerName = sn[*r.ReviewedBy]
	}
	return out
}

// RequestTenantStatus asks for a tenant to be suspended or reactivated. A different staff member
// decides. Recorded in the staff audit log.
func (s *Service) RequestTenantStatus(ctx context.Context, sess Session, tenantID uuid.UUID, action, reason string) (TenantStatusRequest, error) {
	if action != StatusActionSuspend && action != StatusActionReactivate {
		return TenantStatusRequest{}, fmt.Errorf("%w: action must be suspend or reactivate", ErrInvalid)
	}
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 10 || n > 500 {
		return TenantStatusRequest{}, fmt.Errorf("%w: give a reason of 10 to 500 characters", ErrInvalid)
	}
	rows, err := s.store.Q().AdminTenantDirectory(ctx)
	if err != nil {
		return TenantStatusRequest{}, store.Classify(err, "get tenant")
	}
	var status string
	found := false
	for _, r := range rows {
		if r.ID == tenantID {
			status, found = r.Status, true
		}
	}
	switch {
	case !found:
		return TenantStatusRequest{}, ErrNotFound
	case action == StatusActionSuspend && status != "active":
		return TenantStatusRequest{}, fmt.Errorf("%w: the tenant is %s, not active", ErrConflict, status)
	case action == StatusActionReactivate && status != "suspended":
		return TenantStatusRequest{}, fmt.Errorf("%w: the tenant is %s, not suspended", ErrConflict, status)
	}
	id := ids.NewV7()
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		if err := q.InsertTenantStatusRequest(ctx, sqlcgen.InsertTenantStatusRequestParams{
			ID: id, TenantID: tenantID, Action: action, Reason: reason, RequestedBy: sess.StaffID, Now: s.clock.Now(),
		}); err != nil {
			err = store.Classify(err, "insert tenant status request")
			if errors.Is(err, store.ErrDuplicate) {
				return fmt.Errorf("%w: a request for this tenant is already waiting for a decision", ErrConflict)
			}
			return err
		}
		return s.record(ctx, q, sess.Ref(), "tenant_status.requested", &tenantID, statusRequestRef(id), "success",
			map[string]any{"action": action, "reason": reason})
	})
	if err != nil {
		return TenantStatusRequest{}, err
	}
	return s.GetTenantStatusRequest(ctx, id)
}

// GetTenantStatusRequest returns one request.
func (s *Service) GetTenantStatusRequest(ctx context.Context, id uuid.UUID) (TenantStatusRequest, error) {
	r, err := s.store.Q().GetTenantStatusRequest(ctx, id)
	if err := store.Classify(err, "get tenant status request"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return TenantStatusRequest{}, ErrNotFound
		}
		return TenantStatusRequest{}, err
	}
	sn, tn, err := s.names(ctx)
	if err != nil {
		return TenantStatusRequest{}, err
	}
	return enrichStatus(r, sn, tn), nil
}

// ListTenantStatusRequests returns recent requests, newest first.
func (s *Service) ListTenantStatusRequests(ctx context.Context) ([]TenantStatusRequest, error) {
	rows, err := s.store.Q().ListTenantStatusRequests(ctx)
	if err != nil {
		return nil, store.Classify(err, "list tenant status requests")
	}
	sn, tn, err := s.names(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TenantStatusRequest, len(rows))
	for i, r := range rows {
		out[i] = enrichStatus(r, sn, tn)
	}
	return out, nil
}

// DecideTenantStatus approves or rejects another staff member's request. Approval changes the
// tenant's status in the same transaction as both audit entries: the database function applies
// the request and the change together and refuses a decision on one's own request. A tenant sees
// "tenant.suspended" or "tenant.reactivated" in its own audit log, without the reason.
func (s *Service) DecideTenantStatus(ctx context.Context, sess Session, id uuid.UUID, approve bool, note string) (TenantStatusRequest, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 500 {
		return TenantStatusRequest{}, fmt.Errorf("%w: a note is at most 500 characters", ErrInvalid)
	}
	cur, err := s.store.Q().GetTenantStatusRequest(ctx, id)
	if err := store.Classify(err, "get tenant status request"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return TenantStatusRequest{}, ErrNotFound
		}
		return TenantStatusRequest{}, err
	}
	// The tenant-level scope works for a suspended tenant too: reactivating one is the point.
	tctx := s.tenants.StaffTenantScope(ctx, cur.TenantID, sess.Ref())
	err = s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		res, err := q.DecideTenantStatusRequest(ctx, sqlcgen.DecideTenantStatusRequestParams{
			RequestID: id, DeciderID: sess.StaffID, Approve: approve, Note: note, Now: s.clock.Now(),
		})
		if err != nil {
			return store.Classify(err, "decide tenant status request")
		}
		switch res.Outcome {
		case "not_found":
			return ErrNotFound
		case "not_pending":
			return fmt.Errorf("%w: the request is no longer pending", ErrConflict)
		case "own_request":
			return fmt.Errorf("%w: another staff member must decide on your request", ErrForbidden)
		case "not_applicable":
			return fmt.Errorf("%w: the tenant is no longer in a state this request applies to; reject it", ErrConflict)
		case "rejected":
			return s.record(ctx, q, sess.Ref(), "tenant_status.rejected", &cur.TenantID, statusRequestRef(id), "success",
				map[string]any{"action": res.Action, "note": note})
		case "applied":
			past := map[string]string{StatusActionSuspend: "tenant.suspended", StatusActionReactivate: "tenant.reactivated"}[res.Action]
			if err := s.record(ctx, q, sess.Ref(), "tenant_status.approved", &cur.TenantID, statusRequestRef(id), "success",
				map[string]any{"action": res.Action, "note": note}); err != nil {
				return err
			}
			return s.audit.Record(ctx, audit.Entry{
				Action: past, ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, cur.TenantID),
				Outcome: audit.OutcomeSuccess, TenantLevel: true,
				Metadata: map[string]any{"staff_name": sess.Name, "request": statusRequestRef(id)},
			})
		}
		return fmt.Errorf("unexpected outcome %q", res.Outcome)
	})
	if err != nil {
		return TenantStatusRequest{}, err
	}
	return s.GetTenantStatusRequest(ctx, id)
}

// EmergencySuspend suspends an active tenant at once on one security administrator's authority
// (break-glass). It is never silent: the request is recorded as an emergency in both audit logs,
// flagged for review, and shown on every other staff member's work queue until one of them
// reviews it. Reactivation has no emergency path, and a waiting request must be decided first.
func (s *Service) EmergencySuspend(ctx context.Context, sess Session, tenantID uuid.UUID, reason string) (TenantStatusRequest, error) {
	if err := requireSecurity(sess); err != nil {
		return TenantStatusRequest{}, err
	}
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 10 || n > 500 {
		return TenantStatusRequest{}, fmt.Errorf("%w: give a reason of 10 to 500 characters", ErrInvalid)
	}
	tctx := s.tenants.StaffTenantScope(ctx, tenantID, sess.Ref())
	var id uuid.UUID
	err := s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		res, err := q.EmergencySuspendTenant(ctx, sqlcgen.EmergencySuspendTenantParams{
			TenantID: tenantID, StaffID: sess.StaffID, Reason: reason, Now: s.clock.Now(),
		})
		if err != nil {
			return store.Classify(err, "emergency suspend tenant")
		}
		switch res.Outcome {
		case "forbidden":
			return fmt.Errorf("%w: only an active security administrator may suspend in an emergency", ErrForbidden)
		case "not_found":
			return ErrNotFound
		case "not_applicable":
			return fmt.Errorf("%w: the tenant is not active", ErrConflict)
		case "pending_exists":
			return fmt.Errorf("%w: a request for this tenant is waiting; decide or withdraw it first", ErrConflict)
		case "applied":
			id = res.RequestID
		default:
			return fmt.Errorf("unexpected outcome %q", res.Outcome)
		}
		if err := s.record(ctx, q, sess.Ref(), "tenant_status.emergency_suspended", &tenantID, statusRequestRef(id), "success",
			map[string]any{"action": StatusActionSuspend, "reason": reason, "needs_review": true}); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "tenant.suspended", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, tenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true,
			Metadata: map[string]any{"staff_name": sess.Name, "request": statusRequestRef(id), "emergency": true},
		})
	})
	if err != nil {
		return TenantStatusRequest{}, err
	}
	return s.GetTenantStatusRequest(ctx, id)
}

// ReviewEmergencySuspension records that a staff member other than the one who suspended has
// looked at an emergency suspension. It does not undo it: reactivation is an ordinary request.
func (s *Service) ReviewEmergencySuspension(ctx context.Context, sess Session, id uuid.UUID, note string) (TenantStatusRequest, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 500 {
		return TenantStatusRequest{}, fmt.Errorf("%w: a note is at most 500 characters", ErrInvalid)
	}
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		res, err := q.ReviewEmergencySuspension(ctx, sqlcgen.ReviewEmergencySuspensionParams{
			RequestID: id, ReviewerID: sess.StaffID, Note: note, Now: s.clock.Now(),
		})
		if err != nil {
			return store.Classify(err, "review emergency suspension")
		}
		switch res.Outcome {
		case "not_found":
			return ErrNotFound
		case "not_emergency":
			return fmt.Errorf("%w: only emergency suspensions are reviewed", ErrConflict)
		case "already_reviewed":
			return fmt.Errorf("%w: it has been reviewed already", ErrConflict)
		case "own_request":
			return fmt.Errorf("%w: another staff member must review your emergency suspension", ErrForbidden)
		case "reviewed":
			return s.record(ctx, q, sess.Ref(), "tenant_status.emergency_reviewed", &res.TenantID, statusRequestRef(id), "success",
				map[string]any{"note": note})
		}
		return fmt.Errorf("unexpected outcome %q", res.Outcome)
	})
	if err != nil {
		return TenantStatusRequest{}, err
	}
	return s.GetTenantStatusRequest(ctx, id)
}

// WithdrawTenantStatus lets the requester take back their own pending request.
func (s *Service) WithdrawTenantStatus(ctx context.Context, sess Session, id uuid.UUID) (TenantStatusRequest, error) {
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		cur, err := q.GetTenantStatusRequestForUpdate(ctx, id)
		if err != nil {
			return store.Classify(err, "lock tenant status request")
		}
		if cur.RequestedBy != sess.StaffID {
			return fmt.Errorf("%w: only the requester can withdraw a request", ErrForbidden)
		}
		n, err := q.WithdrawTenantStatusRequest(ctx, sqlcgen.WithdrawTenantStatusRequestParams{
			ID: id, StaffID: sess.StaffID, Now: s.clock.Now(),
		})
		if err != nil {
			return store.Classify(err, "withdraw tenant status request")
		}
		if n == 0 {
			return fmt.Errorf("%w: the request is no longer pending", ErrConflict)
		}
		return s.record(ctx, q, sess.Ref(), "tenant_status.withdrawn", &cur.TenantID, statusRequestRef(id), "success",
			map[string]any{"action": cur.Action})
	})
	if errors.Is(err, store.ErrNotFound) {
		return TenantStatusRequest{}, ErrNotFound
	}
	if err != nil {
		return TenantStatusRequest{}, err
	}
	return s.GetTenantStatusRequest(ctx, id)
}
