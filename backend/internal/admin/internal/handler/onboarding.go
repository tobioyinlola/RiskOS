package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

func reviewToAPI(o tenants.Onboarding) (adminapi.OnboardingReview, error) {
	raw, err := json.Marshal(o.Answers)
	if err != nil {
		return adminapi.OnboardingReview{}, fmt.Errorf("encode onboarding: %w", err)
	}
	answers := map[string]any{}
	if err := json.Unmarshal(raw, &answers); err != nil {
		return adminapi.OnboardingReview{}, fmt.Errorf("encode onboarding: %w", err)
	}
	return adminapi.OnboardingReview{
		TenantId: ids.Format(ids.PrefixTenant, o.TenantID), TenantName: o.TenantName,
		Status: adminapi.OnboardingStatus(o.Status), Answers: answers, Missing: o.Missing,
		ProductionEnabled: o.ProductionEnabled, SubmittedAt: o.SubmittedAt, SubmittedBy: o.SubmittedBy,
		ReviewedAt: o.ReviewedAt, ReviewedBy: o.ReviewedBy, ReviewNote: o.ReviewNote,
	}, nil
}

func tenantRef(ref string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixTenant, ref)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

// ListOnboardingReviews implements GET /api/admin/v1/onboarding.
func (h *Handlers) ListOnboardingReviews(ctx context.Context, _ adminapi.ListOnboardingReviewsRequestObject) (adminapi.ListOnboardingReviewsResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.svc.OnboardingQueue(ctx, s)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.ListOnboardingReviews200JSONResponse{Data: make([]adminapi.OnboardingQueueItem, len(rows))}
	for i, r := range rows {
		out.Data[i] = adminapi.OnboardingQueueItem{
			TenantId: ids.Format(ids.PrefixTenant, r.TenantID), TenantName: r.TenantName, TenantStatus: r.TenantStatus,
			TenantCreatedAt: r.TenantCreatedAt, Status: adminapi.OnboardingStatus(r.Status),
			SubmittedAt: r.SubmittedAt, ReviewedAt: r.ReviewedAt,
		}
	}
	return out, nil
}

// GetOnboardingReview implements GET /api/admin/v1/onboarding/{tenant_id}.
func (h *Handlers) GetOnboardingReview(ctx context.Context, req adminapi.GetOnboardingReviewRequestObject) (adminapi.GetOnboardingReviewResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	id, err := tenantRef(req.TenantId)
	if err != nil {
		return nil, err
	}
	o, err := h.svc.Onboarding(ctx, s, id)
	if err != nil {
		return nil, mapError(err)
	}
	out, err := reviewToAPI(o)
	if err != nil {
		return nil, err
	}
	return adminapi.GetOnboardingReview200JSONResponse(out), nil
}

func (h *Handlers) review(ctx context.Context, ref string, approve bool, body *adminapi.OnboardingDecision) (adminapi.OnboardingReview, error) {
	s, err := session(ctx)
	if err != nil {
		return adminapi.OnboardingReview{}, err
	}
	id, err := tenantRef(ref)
	if err != nil {
		return adminapi.OnboardingReview{}, err
	}
	var note string
	if body != nil && body.Note != nil {
		note = *body.Note
	}
	o, err := h.svc.ReviewOnboarding(ctx, s, id, approve, note)
	if err != nil {
		return adminapi.OnboardingReview{}, mapError(err)
	}
	return reviewToAPI(o)
}

// ApproveOnboarding implements POST /api/admin/v1/onboarding/{tenant_id}/approve.
func (h *Handlers) ApproveOnboarding(ctx context.Context, req adminapi.ApproveOnboardingRequestObject) (adminapi.ApproveOnboardingResponseObject, error) {
	out, err := h.review(ctx, req.TenantId, true, req.Body)
	if err != nil {
		return nil, err
	}
	return adminapi.ApproveOnboarding200JSONResponse(out), nil
}

// RejectOnboarding implements POST /api/admin/v1/onboarding/{tenant_id}/reject.
func (h *Handlers) RejectOnboarding(ctx context.Context, req adminapi.RejectOnboardingRequestObject) (adminapi.RejectOnboardingResponseObject, error) {
	out, err := h.review(ctx, req.TenantId, false, req.Body)
	if err != nil {
		return nil, err
	}
	return adminapi.RejectOnboarding200JSONResponse(out), nil
}
