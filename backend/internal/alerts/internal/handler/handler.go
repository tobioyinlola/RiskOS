// Package handler adapts HTTP requests for the alerts module.
package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/present"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// Handlers implements the alerts module's operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *service.Service
}

// New returns the alerts HTTP handlers.
func New(svc *service.Service) *Handlers { return &Handlers{svc: svc} }

// ListAlerts implements GET /api/v1/alerts.
func (h *Handlers) ListAlerts(ctx context.Context, req apigen.ListAlertsRequestObject) (apigen.ListAlertsResponseObject, error) {
	page, err := httpx.ParsePage(req.Params.Limit, req.Params.Cursor, ids.PrefixAlert)
	if err != nil {
		return nil, err
	}
	f := domain.Filter{Limit: page.Limit + 1, Before: page.Before}
	if req.Params.Status != nil {
		f.Status = new(domain.Status(*req.Params.Status))
	}
	if req.Params.Kind != nil {
		f.Kind = new(domain.Kind(*req.Params.Kind))
	}
	rows, err := h.svc.List(ctx, f)
	if err != nil {
		return nil, MapError(err)
	}
	rows, next := httpx.NextCursor(rows, page.Limit, ids.PrefixAlert, func(a domain.Alert) uuid.UUID { return a.ID })
	data := make([]apigen.Alert, len(rows))
	for i, a := range rows {
		data[i] = present.ToAPI(a)
	}
	return apigen.ListAlerts200JSONResponse{Data: data, NextCursor: next}, nil
}

// GetAlert implements GET /api/v1/alerts/{alert_id}.
func (h *Handlers) GetAlert(ctx context.Context, req apigen.GetAlertRequestObject) (apigen.GetAlertResponseObject, error) {
	id, err := ids.Parse(ids.PrefixAlert, req.AlertId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	d, err := h.svc.Get(ctx, id)
	if err != nil {
		return nil, MapError(err)
	}
	evals := make([]apigen.AlertEvaluation, len(d.Evaluations))
	for i, e := range d.Evaluations {
		signals := make([]apigen.Signal, len(e.Signals))
		for j, s := range e.Signals {
			signals[j] = apigen.Signal{Code: s.Code, Label: s.Label, Weight: s.Weight}
		}
		evals[i] = apigen.AlertEvaluation{
			EvaluationId: ids.Format(ids.PrefixEvaluation, e.EvaluationID), EventId: ids.Format(ids.PrefixEvent, e.EventID),
			EventType: apigen.EventType(e.EventType), RiskScore: e.Score, RiskLevel: apigen.RiskLevel(e.Level),
			Decision: apigen.Decision(e.Decision), Signals: signals, DegradedSignals: e.Degraded, EvaluatedAt: e.EvaluatedAt,
		}
	}
	return apigen.GetAlert200JSONResponse{Alert: present.ToAPI(d.Alert), Evaluations: evals, Activity: present.ActivityToAPI(d.Activity)}, nil
}

// UpdateAlert implements PATCH /api/v1/alerts/{alert_id}.
func (h *Handlers) UpdateAlert(ctx context.Context, req apigen.UpdateAlertRequestObject) (apigen.UpdateAlertResponseObject, error) {
	id, err := ids.Parse(ids.PrefixAlert, req.AlertId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	p := domain.Patch{Assignee: req.Body.Assignee, Note: req.Body.Note}
	if req.Body.Status != nil {
		p.Status = new(domain.Status(*req.Body.Status))
	}
	a, err := h.svc.Update(ctx, id, p)
	if err != nil {
		return nil, MapError(err)
	}
	return apigen.UpdateAlert200JSONResponse(present.ToAPI(a)), nil
}

// MapError translates the module's errors to the error contract.
func MapError(err error) error {
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
		return err
	}
}
