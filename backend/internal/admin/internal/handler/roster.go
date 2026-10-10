package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func rosterToAPI(r service.StaffRosterRow) adminapi.StaffRosterEntry {
	out := adminapi.StaffRosterEntry{
		Id: ids.Format(ids.PrefixStaff, r.ID), Email: r.Email, Name: r.Name, Role: adminapi.StaffRole(r.Role),
		Status: adminapi.StaffRosterEntryStatus(r.Status), Passkeys: r.Passkeys,
		EnrollmentPending: r.EnrollmentPending, CreatedAt: r.CreatedAt,
	}
	if r.LastSignIn.Unix() > 0 { // the query reports "never" as the epoch
		out.LastSignIn = &r.LastSignIn
	}
	return out
}

// ListStaffRoster implements GET /api/admin/v1/staff.
func (h *Handlers) ListStaffRoster(ctx context.Context, _ adminapi.ListStaffRosterRequestObject) (adminapi.ListStaffRosterResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.svc.Roster(ctx, s)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.ListStaffRoster200JSONResponse{Data: make([]adminapi.StaffRosterEntry, len(rows))}
	for i, r := range rows {
		out.Data[i] = rosterToAPI(r)
	}
	return out, nil
}

func (h *Handlers) setStaffAccess(ctx context.Context, rawID string, active bool) (adminapi.StaffRosterEntry, error) {
	s, err := session(ctx)
	if err != nil {
		return adminapi.StaffRosterEntry{}, err
	}
	id, err := ids.Parse(ids.PrefixStaff, rawID)
	if err != nil || id == uuid.Nil {
		return adminapi.StaffRosterEntry{}, httpx.NotFound(err)
	}
	r, err := h.svc.SetStaffAccess(ctx, s, id, active)
	if err != nil {
		return adminapi.StaffRosterEntry{}, mapError(err)
	}
	return rosterToAPI(r), nil
}

// DisableStaff implements POST /api/admin/v1/staff/{staff_id}/disable.
func (h *Handlers) DisableStaff(ctx context.Context, req adminapi.DisableStaffRequestObject) (adminapi.DisableStaffResponseObject, error) {
	r, err := h.setStaffAccess(ctx, req.StaffId, false)
	if err != nil {
		return nil, err
	}
	return adminapi.DisableStaff200JSONResponse(r), nil
}

// EnableStaff implements POST /api/admin/v1/staff/{staff_id}/enable.
func (h *Handlers) EnableStaff(ctx context.Context, req adminapi.EnableStaffRequestObject) (adminapi.EnableStaffResponseObject, error) {
	r, err := h.setStaffAccess(ctx, req.StaffId, true)
	if err != nil {
		return nil, err
	}
	return adminapi.EnableStaff200JSONResponse(r), nil
}
