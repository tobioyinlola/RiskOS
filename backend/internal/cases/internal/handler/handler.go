// Package handler adapts HTTP requests for the cases module.
package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/present"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// Handlers implements the cases module's operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *service.Service
}

// New returns the cases HTTP handlers.
func New(svc *service.Service) *Handlers { return &Handlers{svc: svc} }

// ListCases implements GET /api/v1/cases.
func (h *Handlers) ListCases(ctx context.Context, req apigen.ListCasesRequestObject) (apigen.ListCasesResponseObject, error) {
	page, err := httpx.ParsePage(req.Params.Limit, req.Params.Cursor, ids.PrefixCase)
	if err != nil {
		return nil, err
	}
	f := domain.Filter{Limit: page.Limit + 1, Before: page.Before}
	if req.Params.Status != nil {
		f.Status = new(domain.Status(*req.Params.Status))
	}
	rows, err := h.svc.List(ctx, f)
	if err != nil {
		return nil, mapError(err)
	}
	rows, next := httpx.NextCursor(rows, page.Limit, ids.PrefixCase, func(c domain.Case) uuid.UUID { return c.ID })
	data := make([]apigen.Case, len(rows))
	for i, c := range rows {
		data[i] = present.ToAPI(c)
	}
	return apigen.ListCases200JSONResponse{Data: data, NextCursor: next}, nil
}

// CreateCase implements POST /api/v1/cases.
func (h *Handlers) CreateCase(ctx context.Context, req apigen.CreateCaseRequestObject) (apigen.CreateCaseResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in := domain.CreateInput{Title: req.Body.Title}
	if req.Body.Description != nil {
		in.Description = *req.Body.Description
	}
	if req.Body.Priority != nil {
		in.Priority = string(*req.Body.Priority)
	}
	if req.Body.Assignee != nil {
		in.Assignee = *req.Body.Assignee
	}
	if req.Body.AlertIds != nil {
		for _, raw := range *req.Body.AlertIds {
			id, err := ids.Parse(ids.PrefixAlert, raw)
			if err != nil {
				// Like any other unknown alert: indistinguishable from another tenant's.
				return nil, httpx.NotFound(err)
			}
			in.AlertIDs = append(in.AlertIDs, id)
		}
	}
	c, err := h.svc.Create(ctx, in)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.CreateCase201JSONResponse(present.ToAPI(c)), nil
}

// GetCase implements GET /api/v1/cases/{case_id}.
func (h *Handlers) GetCase(ctx context.Context, req apigen.GetCaseRequestObject) (apigen.GetCaseResponseObject, error) {
	id, err := ids.Parse(ids.PrefixCase, req.CaseId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	d, err := h.svc.Get(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	as := make([]apigen.Alert, len(d.Alerts))
	for i, a := range d.Alerts {
		as[i] = alerts.ToAPI(a)
	}
	notes := make([]apigen.CaseNote, len(d.Notes))
	for i, n := range d.Notes {
		notes[i] = noteToAPI(n)
	}
	activity := make([]alerts.Activity, len(d.Activity))
	for i, a := range d.Activity {
		activity[i] = alerts.Activity(a)
	}
	return apigen.GetCase200JSONResponse{Case: present.ToAPI(d.Case), Alerts: as, Notes: notes, Activity: alerts.ActivityToAPI(activity)}, nil
}

// UpdateCase implements PATCH /api/v1/cases/{case_id}.
func (h *Handlers) UpdateCase(ctx context.Context, req apigen.UpdateCaseRequestObject) (apigen.UpdateCaseResponseObject, error) {
	id, err := ids.Parse(ids.PrefixCase, req.CaseId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	b := req.Body
	p := domain.Patch{Title: b.Title, Description: b.Description, Assignee: b.Assignee, ResolutionNote: b.ResolutionNote}
	if b.Priority != nil {
		p.Priority = new(string(*b.Priority))
	}
	if b.Status != nil {
		p.Status = new(domain.Status(*b.Status))
	}
	if b.Resolution != nil {
		p.Resolution = new(domain.Resolution(*b.Resolution))
	}
	c, err := h.svc.Update(ctx, id, p)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.UpdateCase200JSONResponse(present.ToAPI(c)), nil
}

// AddCaseNote implements POST /api/v1/cases/{case_id}/notes.
func (h *Handlers) AddCaseNote(ctx context.Context, req apigen.AddCaseNoteRequestObject) (apigen.AddCaseNoteResponseObject, error) {
	id, err := ids.Parse(ids.PrefixCase, req.CaseId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	n, err := h.svc.AddNote(ctx, id, req.Body.Body)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.AddCaseNote201JSONResponse(noteToAPI(n)), nil
}

func noteToAPI(n domain.Note) apigen.CaseNote {
	return apigen.CaseNote{Id: ids.Format(ids.PrefixCaseNote, n.ID), Body: n.Body, Author: n.Author, CreatedAt: n.CreatedAt}
}

func mapError(err error) error {
	detail := func(sentinel error) string {
		d := strings.TrimPrefix(strings.TrimPrefix(err.Error(), sentinel.Error()), ": ")
		if d == "" {
			return "The request is invalid."
		}
		return strings.ToUpper(d[:1]) + d[1:] + "."
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, domain.ErrInvalid):
		return httpx.InvalidRequest(detail(domain.ErrInvalid), err)
	case errors.Is(err, domain.ErrConflict):
		return httpx.Conflict(detail(domain.ErrConflict), err)
	default:
		// Alert errors raised while escalating (unknown alert, alert not escalatable).
		return alerts.MapError(err)
	}
}
