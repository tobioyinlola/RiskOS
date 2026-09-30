// Package handler adapts HTTP requests for the webhooks module.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/store"
)

// Handlers implements the webhooks module's operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *service.Service
}

// New returns the webhooks HTTP handlers.
func New(svc *service.Service) *Handlers { return &Handlers{svc: svc} }

func parse(raw string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixWebhook, raw)
	if err != nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

func types(in []apigen.WebhookEventType) []domain.EventType {
	out := make([]domain.EventType, len(in))
	for i, t := range in {
		out[i] = domain.EventType(t)
	}
	return out
}

// ListWebhooks implements GET /api/v1/webhooks.
func (h *Handlers) ListWebhooks(ctx context.Context, _ apigen.ListWebhooksRequestObject) (apigen.ListWebhooksResponseObject, error) {
	es, err := h.svc.List(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.Webhook, len(es))
	for i, e := range es {
		data[i] = toAPI(e)
	}
	return apigen.ListWebhooks200JSONResponse{Data: data}, nil
}

// CreateWebhook implements POST /api/v1/webhooks.
func (h *Handlers) CreateWebhook(ctx context.Context, req apigen.CreateWebhookRequestObject) (apigen.CreateWebhookResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in := service.Input{URL: req.Body.Url, EventTypes: types(req.Body.EventTypes)}
	if req.Body.Description != nil {
		in.Description = *req.Body.Description
	}
	e, secret, err := h.svc.Create(ctx, in)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.CreateWebhook201JSONResponse{Webhook: toAPI(e), Secret: secret}, nil
}

// GetWebhook implements GET /api/v1/webhooks/{webhook_id}.
func (h *Handlers) GetWebhook(ctx context.Context, req apigen.GetWebhookRequestObject) (apigen.GetWebhookResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	e, err := h.svc.Get(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.GetWebhook200JSONResponse(toAPI(e)), nil
}

// UpdateWebhook implements PATCH /api/v1/webhooks/{webhook_id}.
func (h *Handlers) UpdateWebhook(ctx context.Context, req apigen.UpdateWebhookRequestObject) (apigen.UpdateWebhookResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	p := service.Patch{URL: req.Body.Url, Description: req.Body.Description}
	if req.Body.EventTypes != nil {
		p.EventTypes = new(types(*req.Body.EventTypes))
	}
	if req.Body.Status != nil {
		p.Status = new(string(*req.Body.Status))
	}
	e, err := h.svc.Update(ctx, id, p)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.UpdateWebhook200JSONResponse(toAPI(e)), nil
}

// RotateWebhookSecret implements POST /api/v1/webhooks/{webhook_id}/rotate-secret.
func (h *Handlers) RotateWebhookSecret(ctx context.Context, req apigen.RotateWebhookSecretRequestObject) (apigen.RotateWebhookSecretResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	overlap := domain.DefaultOverlap
	if req.Body != nil && req.Body.OverlapHours != nil {
		overlap = time.Duration(*req.Body.OverlapHours) * time.Hour
	}
	e, secret, err := h.svc.RotateSecret(ctx, id, overlap)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.RotateWebhookSecret200JSONResponse{Webhook: toAPI(e), Secret: secret}, nil
}

// TestWebhook implements POST /api/v1/webhooks/{webhook_id}/test.
func (h *Handlers) TestWebhook(ctx context.Context, req apigen.TestWebhookRequestObject) (apigen.TestWebhookResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.SendTest(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.TestWebhook202JSONResponse(deliveryToAPI(d)), nil
}

// ListWebhookDeliveries implements GET /api/v1/webhooks/{webhook_id}/deliveries.
func (h *Handlers) ListWebhookDeliveries(ctx context.Context, req apigen.ListWebhookDeliveriesRequestObject) (apigen.ListWebhookDeliveriesResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	page, err := httpx.ParsePage(req.Params.Limit, req.Params.Cursor, ids.PrefixWebhookDelivery)
	if err != nil {
		return nil, err
	}
	f := store.DeliveryFilter{EndpointID: id, Limit: page.Limit + 1, Before: page.Before}
	if req.Params.Status != nil {
		f.Status = new(string(*req.Params.Status))
	}
	rows, err := h.svc.ListDeliveries(ctx, f)
	if err != nil {
		return nil, mapError(err)
	}
	rows, next := httpx.NextCursor(rows, page.Limit, ids.PrefixWebhookDelivery, func(d domain.Delivery) uuid.UUID { return d.ID })
	data := make([]apigen.WebhookDelivery, len(rows))
	for i, d := range rows {
		data[i] = deliveryToAPI(d)
	}
	return apigen.ListWebhookDeliveries200JSONResponse{Data: data, NextCursor: next}, nil
}

// GetWebhookDelivery implements GET /api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}.
func (h *Handlers) GetWebhookDelivery(ctx context.Context, req apigen.GetWebhookDeliveryRequestObject) (apigen.GetWebhookDeliveryResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	deliveryID, err := ids.Parse(ids.PrefixWebhookDelivery, req.DeliveryId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	d, attempts, err := h.svc.GetDelivery(ctx, id, deliveryID)
	if err != nil {
		return nil, mapError(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(d.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode webhook payload: %w", err)
	}
	out := apigen.GetWebhookDelivery200JSONResponse{Delivery: deliveryToAPI(d), Payload: payload, Attempts: make([]apigen.WebhookAttempt, len(attempts))}
	for i, a := range attempts {
		out.Attempts[i] = apigen.WebhookAttempt{Attempt: a.Attempt, StartedAt: a.StartedAt, DurationMs: a.DurationMS, StatusCode: a.StatusCode, Error: a.Error}
	}
	return out, nil
}

// RedeliverWebhook implements POST /api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}/redeliver.
func (h *Handlers) RedeliverWebhook(ctx context.Context, req apigen.RedeliverWebhookRequestObject) (apigen.RedeliverWebhookResponseObject, error) {
	id, err := parse(req.WebhookId)
	if err != nil {
		return nil, err
	}
	deliveryID, err := ids.Parse(ids.PrefixWebhookDelivery, req.DeliveryId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	d, err := h.svc.Redeliver(ctx, id, deliveryID)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.RedeliverWebhook202JSONResponse(deliveryToAPI(d)), nil
}

func toAPI(e domain.Endpoint) apigen.Webhook {
	out := apigen.Webhook{
		Id: ids.Format(ids.PrefixWebhook, e.ID), Url: e.URL, Description: e.Description,
		EventTypes: make([]apigen.WebhookEventType, len(e.EventTypes)), Status: apigen.WebhookStatus(e.Status),
		DisabledReason: e.DisabledReason, FailingSince: e.FailingSince, CreatedAt: e.CreatedAt,
		CreatedBy: e.CreatedBy, UpdatedAt: e.UpdatedAt,
	}
	for i, t := range e.EventTypes {
		out.EventTypes[i] = apigen.WebhookEventType(t)
	}
	if e.PreviousSecretValidUntil != nil && e.PreviousSecretValidUntil.After(time.Now()) {
		out.PreviousSecretValidUntil = e.PreviousSecretValidUntil
	}
	return out
}

func deliveryToAPI(d domain.Delivery) apigen.WebhookDelivery {
	return apigen.WebhookDelivery{
		Id: ids.Format(ids.PrefixWebhookDelivery, d.ID), WebhookId: ids.Format(ids.PrefixWebhook, d.EndpointID),
		EventId: ids.Format(ids.PrefixWebhookEvent, d.EventID), EventType: string(d.EventType),
		Status: apigen.WebhookDeliveryStatus(d.Status), Attempts: d.Attempts, NextAttemptAt: d.NextAttemptAt,
		LastAttemptAt: d.LastAttemptAt, LastStatusCode: d.LastStatusCode, LastError: d.LastError,
		CreatedAt: d.CreatedAt, CompletedAt: d.CompletedAt,
	}
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
	case errors.Is(err, domain.ErrLimit):
		return httpx.Conflict(detail(domain.ErrLimit), err)
	default:
		return err
	}
}
