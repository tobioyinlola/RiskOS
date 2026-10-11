package handler

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func supportToAPI(a service.SupportAccess, now time.Time) adminapi.SupportAccess {
	out := adminapi.SupportAccess{
		Id: ids.Format(ids.PrefixSupportAccess, a.ID), TenantId: ids.Format(ids.PrefixTenant, a.TenantID), TenantName: a.TenantName,
		RequestedBy:     adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, a.RequestedBy), Name: a.RequesterName},
		Reason:          a.Reason,
		DurationMinutes: int(a.DurationMinutes),
		Status:          adminapi.SupportAccessStatus(a.Status(now)),
		Active:          a.Active(now),
		CreatedAt:       a.CreatedAt, DecidedAt: a.DecidedAt, ExpiresAt: a.ExpiresAt, RevokedAt: a.RevokedAt,
	}
	if a.DecidedBy != nil {
		out.DecidedBy = &adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, *a.DecidedBy), Name: a.DeciderName}
	}
	return out
}

// ListSupportAccess implements GET /api/admin/v1/support-access.
func (h *Handlers) ListSupportAccess(ctx context.Context, _ adminapi.ListSupportAccessRequestObject) (adminapi.ListSupportAccessResponseObject, error) {
	rows, err := h.svc.ListSupport(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	now := time.Now()
	out := adminapi.ListSupportAccess200JSONResponse{Data: make([]adminapi.SupportAccess, len(rows))}
	for i, r := range rows {
		out.Data[i] = supportToAPI(r, now)
	}
	return out, nil
}

// RequestSupportAccess implements POST /api/admin/v1/support-access.
func (h *Handlers) RequestSupportAccess(ctx context.Context, req adminapi.RequestSupportAccessRequestObject) (adminapi.RequestSupportAccessResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	tenantID, err := ids.Parse(ids.PrefixTenant, req.Body.TenantId)
	if err != nil || tenantID == uuid.Nil {
		return nil, httpx.NotFound(err)
	}
	a, err := h.svc.RequestSupport(ctx, s, tenantID, req.Body.Reason, req.Body.DurationMinutes)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.RequestSupportAccess201JSONResponse(supportToAPI(a, time.Now())), nil
}

func (h *Handlers) decide(ctx context.Context, ref string, fn func(context.Context, service.Session, [16]byte) (service.SupportAccess, error)) (adminapi.SupportAccess, error) {
	s, err := session(ctx)
	if err != nil {
		return adminapi.SupportAccess{}, err
	}
	id, err := ids.Parse(ids.PrefixSupportAccess, ref)
	if err != nil {
		return adminapi.SupportAccess{}, httpx.NotFound(err)
	}
	a, err := fn(ctx, s, id)
	if err != nil {
		return adminapi.SupportAccess{}, mapError(err)
	}
	return supportToAPI(a, time.Now()), nil
}

// ApproveSupportAccess implements POST /api/admin/v1/support-access/{access_id}/approve.
func (h *Handlers) ApproveSupportAccess(ctx context.Context, req adminapi.ApproveSupportAccessRequestObject) (adminapi.ApproveSupportAccessResponseObject, error) {
	a, err := h.decide(ctx, req.AccessId, func(ctx context.Context, s service.Session, id [16]byte) (service.SupportAccess, error) {
		return h.svc.ApproveSupport(ctx, s, id)
	})
	if err != nil {
		return nil, err
	}
	return adminapi.ApproveSupportAccess200JSONResponse(a), nil
}

// RejectSupportAccess implements POST /api/admin/v1/support-access/{access_id}/reject.
func (h *Handlers) RejectSupportAccess(ctx context.Context, req adminapi.RejectSupportAccessRequestObject) (adminapi.RejectSupportAccessResponseObject, error) {
	a, err := h.decide(ctx, req.AccessId, func(ctx context.Context, s service.Session, id [16]byte) (service.SupportAccess, error) {
		return h.svc.RejectSupport(ctx, s, id)
	})
	if err != nil {
		return nil, err
	}
	return adminapi.RejectSupportAccess200JSONResponse(a), nil
}

// RevokeSupportAccess implements POST /api/admin/v1/support-access/{access_id}/revoke.
func (h *Handlers) RevokeSupportAccess(ctx context.Context, req adminapi.RevokeSupportAccessRequestObject) (adminapi.RevokeSupportAccessResponseObject, error) {
	a, err := h.decide(ctx, req.AccessId, func(ctx context.Context, s service.Session, id [16]byte) (service.SupportAccess, error) {
		return h.svc.RevokeSupport(ctx, s, id)
	})
	if err != nil {
		return nil, err
	}
	return adminapi.RevokeSupportAccess200JSONResponse(a), nil
}

// ListStaffAudit implements GET /api/admin/v1/audit-log.
func (h *Handlers) ListStaffAudit(ctx context.Context, req adminapi.ListStaffAuditRequestObject) (adminapi.ListStaffAuditResponseObject, error) {
	limit := 100
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httpx.InvalidRequest("limit must be between 1 and 200.", nil)
	}
	rows, err := h.svc.StaffAudit(ctx, limit)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.ListStaffAudit200JSONResponse{Data: make([]adminapi.StaffAuditEntry, len(rows))}
	for i, r := range rows {
		out.Data[i] = auditToAPI(r)
	}
	return out, nil
}
