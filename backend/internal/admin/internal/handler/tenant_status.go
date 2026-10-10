package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func statusRequestToAPI(r service.TenantStatusRequest) adminapi.TenantStatusRequest {
	out := adminapi.TenantStatusRequest{
		Id: ids.Format(ids.PrefixTenantStatusRequest, r.ID), TenantId: ids.Format(ids.PrefixTenant, r.TenantID),
		TenantName: r.TenantName, Action: adminapi.TenantStatusRequestAction(r.Action), Reason: r.Reason,
		Status:      adminapi.TenantStatusRequestStatus(r.Status),
		RequestedBy: adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, r.RequestedBy), Name: r.RequesterName},
		CreatedAt:   r.CreatedAt, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
	}
	if r.DecidedBy != nil {
		out.DecidedBy = &adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, *r.DecidedBy), Name: r.DeciderName}
	}
	return out
}

func requestID(raw string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixTenantStatusRequest, raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

// RequestTenantStatus implements POST /api/admin/v1/tenants/{tenant_id}/status-requests.
func (h *Handlers) RequestTenantStatus(ctx context.Context, req adminapi.RequestTenantStatusRequestObject) (adminapi.RequestTenantStatusResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	tenantID, err := ids.Parse(ids.PrefixTenant, req.TenantId)
	if err != nil || tenantID == uuid.Nil {
		return nil, httpx.NotFound(err)
	}
	r, err := h.svc.RequestTenantStatus(ctx, s, tenantID, string(req.Body.Action), req.Body.Reason)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.RequestTenantStatus201JSONResponse(statusRequestToAPI(r)), nil
}

// ListTenantStatusRequests implements GET /api/admin/v1/tenant-status-requests.
func (h *Handlers) ListTenantStatusRequests(ctx context.Context, _ adminapi.ListTenantStatusRequestsRequestObject) (adminapi.ListTenantStatusRequestsResponseObject, error) {
	rows, err := h.svc.ListTenantStatusRequests(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.ListTenantStatusRequests200JSONResponse{Data: make([]adminapi.TenantStatusRequest, len(rows))}
	for i, r := range rows {
		out.Data[i] = statusRequestToAPI(r)
	}
	return out, nil
}

func (h *Handlers) decideStatus(ctx context.Context, rawID string, approve bool, note *string) (adminapi.TenantStatusRequest, error) {
	s, err := session(ctx)
	if err != nil {
		return adminapi.TenantStatusRequest{}, err
	}
	id, err := requestID(rawID)
	if err != nil {
		return adminapi.TenantStatusRequest{}, err
	}
	n := ""
	if note != nil {
		n = *note
	}
	r, err := h.svc.DecideTenantStatus(ctx, s, id, approve, n)
	if err != nil {
		return adminapi.TenantStatusRequest{}, mapError(err)
	}
	return statusRequestToAPI(r), nil
}

// ApproveTenantStatus implements POST /api/admin/v1/tenant-status-requests/{request_id}/approve.
func (h *Handlers) ApproveTenantStatus(ctx context.Context, req adminapi.ApproveTenantStatusRequestObject) (adminapi.ApproveTenantStatusResponseObject, error) {
	var note *string
	if req.Body != nil {
		note = req.Body.Note
	}
	r, err := h.decideStatus(ctx, req.RequestId, true, note)
	if err != nil {
		return nil, err
	}
	return adminapi.ApproveTenantStatus200JSONResponse(r), nil
}

// RejectTenantStatus implements POST /api/admin/v1/tenant-status-requests/{request_id}/reject.
func (h *Handlers) RejectTenantStatus(ctx context.Context, req adminapi.RejectTenantStatusRequestObject) (adminapi.RejectTenantStatusResponseObject, error) {
	var note *string
	if req.Body != nil {
		note = req.Body.Note
	}
	r, err := h.decideStatus(ctx, req.RequestId, false, note)
	if err != nil {
		return nil, err
	}
	return adminapi.RejectTenantStatus200JSONResponse(r), nil
}

// WithdrawTenantStatus implements POST /api/admin/v1/tenant-status-requests/{request_id}/withdraw.
func (h *Handlers) WithdrawTenantStatus(ctx context.Context, req adminapi.WithdrawTenantStatusRequestObject) (adminapi.WithdrawTenantStatusResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	id, err := requestID(req.RequestId)
	if err != nil {
		return nil, err
	}
	r, err := h.svc.WithdrawTenantStatus(ctx, s, id)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.WithdrawTenantStatus200JSONResponse(statusRequestToAPI(r)), nil
}
