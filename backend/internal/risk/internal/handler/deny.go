package handler

import (
	"context"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/service"
)

func denyToAPI(e domain.DenyEntry) apigen.DenyEntry {
	return apigen.DenyEntry{
		Id: ids.Format(ids.PrefixDenyEntry, e.ID), Kind: apigen.DenyKind(e.Kind), Value: e.Value, Reason: e.Reason,
		ExpiresAt: e.ExpiresAt, CreatedAt: e.CreatedAt, CreatedBy: e.CreatedBy,
	}
}

// ListDenyEntries implements GET /api/v1/deny-list.
func (h *Handlers) ListDenyEntries(ctx context.Context, req apigen.ListDenyEntriesRequestObject) (apigen.ListDenyEntriesResponseObject, error) {
	var kind *domain.DenyKind
	if req.Params.Kind != nil {
		kind = new(domain.DenyKind(*req.Params.Kind))
	}
	l, err := h.svc.ListDeny(ctx, kind)
	if err != nil {
		return nil, MapError(err)
	}
	data := make([]apigen.DenyEntry, len(l.Entries))
	for i, e := range l.Entries {
		data[i] = denyToAPI(e)
	}
	return apigen.ListDenyEntries200JSONResponse{Data: data, Total: int(l.Total), Limit: domain.MaxDenyEntries}, nil
}

// CreateDenyEntry implements POST /api/v1/deny-list.
func (h *Handlers) CreateDenyEntry(ctx context.Context, req apigen.CreateDenyEntryRequestObject) (apigen.CreateDenyEntryResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in := service.DenyInput{Kind: domain.DenyKind(req.Body.Kind), Value: req.Body.Value, ExpiresAt: req.Body.ExpiresAt}
	if req.Body.Reason != nil {
		in.Reason = *req.Body.Reason
	}
	e, err := h.svc.AddDeny(ctx, in)
	if err != nil {
		return nil, MapError(err)
	}
	return apigen.CreateDenyEntry201JSONResponse(denyToAPI(e)), nil
}

// DeleteDenyEntry implements DELETE /api/v1/deny-list/{entry_id}.
func (h *Handlers) DeleteDenyEntry(ctx context.Context, req apigen.DeleteDenyEntryRequestObject) (apigen.DeleteDenyEntryResponseObject, error) {
	id, err := ids.Parse(ids.PrefixDenyEntry, req.EntryId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	if err := h.svc.RemoveDeny(ctx, id); err != nil {
		return nil, MapError(err)
	}
	return apigen.DeleteDenyEntry204Response{}, nil
}
