package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func accountRequestToAPI(r service.StaffAccountRequest) adminapi.StaffAccountRequest {
	out := adminapi.StaffAccountRequest{
		Id: ids.Format(ids.PrefixStaffAccountRequest, r.ID), Email: r.Email, Name: r.Name,
		Role: adminapi.StaffRole(r.Role), Reason: r.Reason,
		Status:      adminapi.StaffAccountRequestStatus(r.Status),
		RequestedBy: adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, r.RequestedBy), Name: r.RequesterName},
		CreatedAt:   r.CreatedAt, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
	}
	if r.DecidedBy != nil {
		out.DecidedBy = &adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, *r.DecidedBy), Name: r.DeciderName}
	}
	if r.StaffID != nil {
		id := ids.Format(ids.PrefixStaff, *r.StaffID)
		out.StaffId = &id
	}
	return out
}

func accountRequestID(raw string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixStaffAccountRequest, raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

// ListStaffAccountRequests implements GET /api/admin/v1/staff/account-requests.
func (h *Handlers) ListStaffAccountRequests(ctx context.Context, _ adminapi.ListStaffAccountRequestsRequestObject) (adminapi.ListStaffAccountRequestsResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.svc.ListStaffAccountRequests(ctx, s)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.ListStaffAccountRequests200JSONResponse{Data: make([]adminapi.StaffAccountRequest, len(rows))}
	for i, r := range rows {
		out.Data[i] = accountRequestToAPI(r)
	}
	return out, nil
}

// RequestStaffAccount implements POST /api/admin/v1/staff/account-requests.
func (h *Handlers) RequestStaffAccount(ctx context.Context, req adminapi.RequestStaffAccountRequestObject) (adminapi.RequestStaffAccountResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	r, err := h.svc.RequestStaffAccount(ctx, s, req.Body.Email, req.Body.Name, string(req.Body.Role), req.Body.Reason)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.RequestStaffAccount201JSONResponse(accountRequestToAPI(r)), nil
}

func (h *Handlers) decideAccount(ctx context.Context, rawID string, approve bool, note *string) (adminapi.StaffAccountRequest, error) {
	s, err := session(ctx)
	if err != nil {
		return adminapi.StaffAccountRequest{}, err
	}
	id, err := accountRequestID(rawID)
	if err != nil {
		return adminapi.StaffAccountRequest{}, err
	}
	n := ""
	if note != nil {
		n = *note
	}
	r, err := h.svc.DecideStaffAccount(ctx, s, id, approve, n)
	if err != nil {
		return adminapi.StaffAccountRequest{}, mapError(err)
	}
	return accountRequestToAPI(r), nil
}

// ApproveStaffAccount implements POST /api/admin/v1/staff/account-requests/{request_id}/approve.
func (h *Handlers) ApproveStaffAccount(ctx context.Context, req adminapi.ApproveStaffAccountRequestObject) (adminapi.ApproveStaffAccountResponseObject, error) {
	var note *string
	if req.Body != nil {
		note = req.Body.Note
	}
	r, err := h.decideAccount(ctx, req.RequestId, true, note)
	if err != nil {
		return nil, err
	}
	return adminapi.ApproveStaffAccount200JSONResponse(r), nil
}

// RejectStaffAccount implements POST /api/admin/v1/staff/account-requests/{request_id}/reject.
func (h *Handlers) RejectStaffAccount(ctx context.Context, req adminapi.RejectStaffAccountRequestObject) (adminapi.RejectStaffAccountResponseObject, error) {
	var note *string
	if req.Body != nil {
		note = req.Body.Note
	}
	r, err := h.decideAccount(ctx, req.RequestId, false, note)
	if err != nil {
		return nil, err
	}
	return adminapi.RejectStaffAccount200JSONResponse(r), nil
}

// WithdrawStaffAccount implements POST /api/admin/v1/staff/account-requests/{request_id}/withdraw.
func (h *Handlers) WithdrawStaffAccount(ctx context.Context, req adminapi.WithdrawStaffAccountRequestObject) (adminapi.WithdrawStaffAccountResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	id, err := accountRequestID(req.RequestId)
	if err != nil {
		return nil, err
	}
	r, err := h.svc.WithdrawStaffAccount(ctx, s, id)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.WithdrawStaffAccount200JSONResponse(accountRequestToAPI(r)), nil
}
