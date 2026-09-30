// Package handler adapts HTTP requests for the audit module to its service.
package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Handlers implements the audit operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *service.Service
}

// New returns the audit HTTP handlers.
func New(svc *service.Service) *Handlers { return &Handlers{svc: svc} }

// ListAuditLogs implements GET /api/v1/audit-logs.
func (h *Handlers) ListAuditLogs(ctx context.Context, req apigen.ListAuditLogsRequestObject) (apigen.ListAuditLogsResponseObject, error) {
	page, err := httpx.ParsePage(req.Params.Limit, req.Params.Cursor, ids.PrefixAudit)
	if err != nil {
		return nil, err
	}
	records, err := h.svc.List(ctx, domain.Page{Limit: page.Limit + 1, Before: page.Before})
	if err != nil {
		return nil, err
	}
	records, next := httpx.NextCursor(records, page.Limit, ids.PrefixAudit, func(r domain.Record) uuid.UUID { return r.ID })

	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	data := make([]apigen.AuditLogEntry, len(records))
	for i, r := range records {
		if data[i], err = toAPI(r, scope); err != nil {
			return nil, err
		}
	}
	return apigen.ListAuditLogs200JSONResponse{Data: data, NextCursor: next}, nil
}

func toAPI(r domain.Record, scope tenancy.Scope) (apigen.AuditLogEntry, error) {
	var meta map[string]any
	if err := json.Unmarshal(r.Metadata, &meta); err != nil {
		return apigen.AuditLogEntry{}, fmt.Errorf("decode audit metadata %d: %w", r.Seq, err)
	}
	e := apigen.AuditLogEntry{
		Id:         ids.Format(ids.PrefixAudit, r.ID),
		Sequence:   r.Seq,
		OccurredAt: r.OccurredAt,
		Action:     r.Action,
		Outcome:    apigen.AuditLogEntryOutcome(r.Outcome),
		RequestId:  r.RequestID,
		Metadata:   meta,
		Hash:       hex.EncodeToString(r.Hash),
		PrevHash:   hex.EncodeToString(r.PrevHash),
	}
	e.Actor.Type = apigen.AuditLogEntryActorType(r.ActorType)
	e.Actor.Id = r.ActorID
	e.Resource.Type = r.ResourceType
	e.Resource.Id = r.ResourceID
	if r.EnvironmentID != nil {
		// Row-level security only returns entries of the caller's own environment or tenant-level
		// ones, so a non-null environment is always the caller's.
		env := apigen.Environment(scope.EnvironmentKind)
		e.Environment = &env
	}
	return e, nil
}
