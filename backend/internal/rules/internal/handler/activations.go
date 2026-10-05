package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

func activationToAPI(a store.Activation) apigen.JurisdictionActivation {
	day := int(a.DayStartsAt)
	return apigen.JurisdictionActivation{
		Id: ids.Format(ids.PrefixActivation, a.ID), Jurisdiction: a.Jurisdiction, BookingEntity: a.BookingEntity,
		Status: apigen.JurisdictionActivationStatus(a.Status), Fallback: a.Fallback, Pack: a.PackID + "@" + a.PackVersion,
		LicenceType: a.LicenceType, Capabilities: list(a.Capabilities), EffectiveFrom: a.EffectiveFrom,
		ActivatedAt: a.ActivatedAt, ActivatedBy: a.ActivatedBy, ApprovedBy: a.ApprovedBy, Rationale: a.Rationale,
		DayStartsAt: &day, UpdatedAt: a.UpdatedAt,
	}
}

func activationChangeToAPI(c store.ActivationChange) apigen.ActivationChange {
	out := apigen.ActivationChange{
		Id: ids.Format(ids.PrefixActivationChange, c.ID), ActivationId: ids.Format(ids.PrefixActivation, c.ActivationID),
		Action: apigen.ActivationChangeAction(c.Action), Details: c.Details, Rationale: c.Rationale,
		State: apigen.ActivationChangeState(c.State), ProposedBy: c.ProposedBy, ProposedAt: c.ProposedAt,
		DecidedBy: c.DecidedBy, DecidedAt: c.DecidedAt, DecisionNote: c.DecisionNote,
	}
	if c.StatusFrom != nil {
		s := apigen.ActivationChangeStatusFrom(*c.StatusFrom)
		out.StatusFrom = &s
	}
	if c.StatusTo != nil {
		s := apigen.ActivationChangeStatusTo(*c.StatusTo)
		out.StatusTo = &s
	}
	return out
}

func gapVerificationToAPI(v store.GapVerification) apigen.GapVerification {
	return apigen.GapVerification{Gap: v.GapID, VerifiedBy: v.VerifiedBy, VerifiedAt: v.VerifiedAt, Note: v.Note, Link: v.Link}
}

func activationID(raw string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixActivation, raw)
	if err != nil {
		return uuid.UUID{}, httpx.NotFound(err)
	}
	return id, nil
}

func changeID(raw string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixActivationChange, raw)
	if err != nil {
		return uuid.UUID{}, httpx.NotFound(err)
	}
	return id, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// CreateComplianceActivation implements POST /api/v1/compliance/activations.
func (h *Handlers) CreateComplianceActivation(ctx context.Context, req apigen.CreateComplianceActivationRequestObject) (apigen.CreateComplianceActivationResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	a, err := h.svc.CreateActivation(ctx, service.ActivationInput{
		Jurisdiction: req.Body.Jurisdiction, BookingEntity: req.Body.BookingEntity, LicenceType: req.Body.LicenceType,
		Capabilities: deref(req.Body.Capabilities), DayStartsAt: deref(req.Body.DayStartsAt), Rationale: req.Body.Rationale,
	}, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.CreateComplianceActivation201JSONResponse(activationToAPI(a)), nil
}

// UpdateComplianceActivation implements PATCH /api/v1/compliance/activations/{activation_id}.
func (h *Handlers) UpdateComplianceActivation(ctx context.Context, req apigen.UpdateComplianceActivationRequestObject) (apigen.UpdateComplianceActivationResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	id, err := activationID(req.ActivationId)
	if err != nil {
		return nil, err
	}
	res, err := h.svc.UpdateActivation(ctx, id, service.ActivationInput{
		LicenceType: req.Body.LicenceType, Capabilities: req.Body.Capabilities, DayStartsAt: req.Body.DayStartsAt,
		Rationale: req.Body.Rationale,
	}, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	if res.Proposed != nil {
		return apigen.UpdateComplianceActivation202JSONResponse(activationChangeToAPI(*res.Proposed)), nil
	}
	return apigen.UpdateComplianceActivation200JSONResponse(activationToAPI(*res.Applied)), nil
}

// SetComplianceActivationStatus implements POST /api/v1/compliance/activations/{activation_id}/status.
func (h *Handlers) SetComplianceActivationStatus(ctx context.Context, req apigen.SetComplianceActivationStatusRequestObject) (apigen.SetComplianceActivationStatusResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	id, err := activationID(req.ActivationId)
	if err != nil {
		return nil, err
	}
	res, err := h.svc.SetActivationStatus(ctx, id, string(req.Body.Status), req.Body.Rationale, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	if res.Proposed != nil {
		return apigen.SetComplianceActivationStatus202JSONResponse(activationChangeToAPI(*res.Proposed)), nil
	}
	return apigen.SetComplianceActivationStatus200JSONResponse(activationToAPI(*res.Applied)), nil
}

// MakeComplianceActivationFallback implements POST /api/v1/compliance/activations/{activation_id}/fallback.
func (h *Handlers) MakeComplianceActivationFallback(ctx context.Context, req apigen.MakeComplianceActivationFallbackRequestObject) (apigen.MakeComplianceActivationFallbackResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	id, err := activationID(req.ActivationId)
	if err != nil {
		return nil, err
	}
	res, err := h.svc.MakeFallback(ctx, id, req.Body.Rationale, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	if res.Proposed != nil {
		return apigen.MakeComplianceActivationFallback202JSONResponse(activationChangeToAPI(*res.Proposed)), nil
	}
	return apigen.MakeComplianceActivationFallback200JSONResponse(activationToAPI(*res.Applied)), nil
}

// GetComplianceActivationReadiness implements GET /api/v1/compliance/activations/{activation_id}/readiness.
func (h *Handlers) GetComplianceActivationReadiness(ctx context.Context, req apigen.GetComplianceActivationReadinessRequestObject) (apigen.GetComplianceActivationReadinessResponseObject, error) {
	id, err := activationID(req.ActivationId)
	if err != nil {
		return nil, err
	}
	r, err := h.svc.ActivationReadiness(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	out := apigen.ActivationReadiness{Ready: r.Ready, Items: make([]struct {
		Blocking bool                                `json:"blocking"`
		Kind     apigen.ActivationReadinessItemsKind `json:"kind"`
		Message  string                              `json:"message"`
		Rule     *string                             `json:"rule,omitempty"`
	}, len(r.Items))}
	for i, it := range r.Items {
		out.Items[i].Blocking, out.Items[i].Kind, out.Items[i].Message = it.Blocking, apigen.ActivationReadinessItemsKind(it.Kind), it.Message
		out.Items[i].Rule = optional(it.Rule)
	}
	return apigen.GetComplianceActivationReadiness200JSONResponse(out), nil
}

// ListComplianceActivationChanges implements GET /api/v1/compliance/activation-changes.
func (h *Handlers) ListComplianceActivationChanges(ctx context.Context, req apigen.ListComplianceActivationChangesRequestObject) (apigen.ListComplianceActivationChangesResponseObject, error) {
	var activation *uuid.UUID
	if req.Params.ActivationId != nil {
		id, err := ids.Parse(ids.PrefixActivation, *req.Params.ActivationId)
		if err != nil {
			return nil, httpx.InvalidRequest("activation_id is not an activation ID.", err)
		}
		activation = &id
	}
	changes, err := h.svc.ActivationChanges(ctx, activation, string(deref(req.Params.State)))
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ActivationChange, len(changes))
	for i, c := range changes {
		data[i] = activationChangeToAPI(c)
	}
	return apigen.ListComplianceActivationChanges200JSONResponse{Data: data}, nil
}

func noteText(n *apigen.DecisionNote) string {
	if n == nil || n.Note == nil {
		return ""
	}
	return *n.Note
}

// ApproveComplianceActivationChange implements POST /api/v1/compliance/activation-changes/{change_id}/approve.
func (h *Handlers) ApproveComplianceActivationChange(ctx context.Context, req apigen.ApproveComplianceActivationChangeRequestObject) (apigen.ApproveComplianceActivationChangeResponseObject, error) {
	id, err := changeID(req.ChangeId)
	if err != nil {
		return nil, err
	}
	c, err := h.svc.DecideActivationChange(ctx, id, true, noteText(req.Body), h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.ApproveComplianceActivationChange200JSONResponse(activationChangeToAPI(c)), nil
}

// RejectComplianceActivationChange implements POST /api/v1/compliance/activation-changes/{change_id}/reject.
func (h *Handlers) RejectComplianceActivationChange(ctx context.Context, req apigen.RejectComplianceActivationChangeRequestObject) (apigen.RejectComplianceActivationChangeResponseObject, error) {
	id, err := changeID(req.ChangeId)
	if err != nil {
		return nil, err
	}
	c, err := h.svc.DecideActivationChange(ctx, id, false, noteText(req.Body), h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.RejectComplianceActivationChange200JSONResponse(activationChangeToAPI(c)), nil
}

// VerifyComplianceGap implements POST /api/v1/compliance/gaps/{gap_id}/verify.
func (h *Handlers) VerifyComplianceGap(ctx context.Context, req apigen.VerifyComplianceGapRequestObject) (apigen.VerifyComplianceGapResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	v, err := h.svc.VerifyGap(ctx, jurisdictionOf(req.Params.Jurisdiction), req.GapId, req.Body.Note, deref(req.Body.Link), h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.VerifyComplianceGap201JSONResponse(gapVerificationToAPI(v)), nil
}

func verificationsToAPI(vs []store.GapVerification) *[]apigen.GapVerification {
	if len(vs) == 0 {
		return nil
	}
	out := make([]apigen.GapVerification, len(vs))
	for i, v := range vs {
		out[i] = gapVerificationToAPI(v)
	}
	return &out
}
