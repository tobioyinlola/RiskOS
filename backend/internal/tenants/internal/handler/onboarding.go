package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/service"
)

// Handlers implements the onboarding operations of the public API.
type Handlers struct {
	svc   *service.Service
	clock clock.Clock
}

// New returns the onboarding handlers.
func New(svc *service.Service, clk clock.Clock) *Handlers {
	return &Handlers{svc: svc, clock: clk}
}

// AnswersFromAPI converts the API's answers (the same JSON field names) to the service's.
func AnswersFromAPI(in apigen.OnboardingAnswers) (service.Answers, error) {
	var out service.Answers
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

func answersToAPI(in service.Answers) (apigen.OnboardingAnswers, error) {
	var out apigen.OnboardingAnswers
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

func toAPI(v service.OnboardingView) (apigen.Onboarding, error) {
	a, err := answersToAPI(v.Answers)
	if err != nil {
		return apigen.Onboarding{}, fmt.Errorf("encode onboarding: %w", err)
	}
	return apigen.Onboarding{
		Status: apigen.OnboardingStatus(v.Status), Answers: a, Missing: v.Missing, ProductionEnabled: v.ProductionEnabled,
		SubmittedAt: v.SubmittedAt, ReviewedAt: v.ReviewedAt, ReviewNote: v.ReviewNote, UpdatedAt: v.UpdatedAt,
	}, nil
}

// MapError maps the onboarding errors to API errors.
func MapError(err error) error {
	switch {
	case errors.Is(err, service.ErrNoOnboarding):
		return httpx.NotFound(err)
	case errors.Is(err, service.ErrOnboardingState):
		return httpx.Conflict("The onboarding cannot change now: it has been submitted or approved.", err)
	case errors.Is(err, service.ErrInvalidInput):
		msg := strings.TrimPrefix(err.Error(), service.ErrInvalidInput.Error()+": ")
		return httpx.InvalidRequest(strings.ToUpper(msg[:1])+msg[1:]+".", err)
	default:
		return err
	}
}

// GetOnboarding implements GET /api/v1/onboarding.
func (h *Handlers) GetOnboarding(ctx context.Context, _ apigen.GetOnboardingRequestObject) (apigen.GetOnboardingResponseObject, error) {
	v, err := h.svc.Onboarding(ctx)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := toAPI(v)
	if err != nil {
		return nil, err
	}
	return apigen.GetOnboarding200JSONResponse(out), nil
}

// SaveOnboarding implements PUT /api/v1/onboarding.
func (h *Handlers) SaveOnboarding(ctx context.Context, req apigen.SaveOnboardingRequestObject) (apigen.SaveOnboardingResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	a, err := AnswersFromAPI(*req.Body)
	if err != nil {
		return nil, httpx.InvalidRequest("The answers are invalid.", err)
	}
	v, err := h.svc.SaveOnboarding(ctx, a, h.clock.Now())
	if err != nil {
		return nil, MapError(err)
	}
	out, err := toAPI(v)
	if err != nil {
		return nil, err
	}
	return apigen.SaveOnboarding200JSONResponse(out), nil
}

// SubmitOnboarding implements POST /api/v1/onboarding/submit.
func (h *Handlers) SubmitOnboarding(ctx context.Context, _ apigen.SubmitOnboardingRequestObject) (apigen.SubmitOnboardingResponseObject, error) {
	v, err := h.svc.SubmitOnboarding(ctx, h.clock.Now())
	if err != nil {
		return nil, MapError(err)
	}
	out, err := toAPI(v)
	if err != nil {
		return nil, err
	}
	return apigen.SubmitOnboarding200JSONResponse(out), nil
}
