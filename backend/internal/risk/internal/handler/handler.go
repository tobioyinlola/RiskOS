// Package handler adapts HTTP requests for the risk module: rules, risk settings and the catalog.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/engine"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/service"
)

// Handlers implements the risk module's operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *service.Service
}

// New returns the risk HTTP handlers.
func New(svc *service.Service) *Handlers { return &Handlers{svc: svc} }

// GetRiskCatalog implements GET /api/v1/risk/catalog.
func (h *Handlers) GetRiskCatalog(ctx context.Context, _ apigen.GetRiskCatalogRequestObject) (apigen.GetRiskCatalogResponseObject, error) {
	settings, err := h.svc.CatalogSettings(ctx)
	if err != nil {
		return nil, MapError(err)
	}
	var out apigen.GetRiskCatalog200JSONResponse
	out.EngineVersion = engine.Version
	for _, s := range domain.BuiltinSignals() {
		out.Signals = append(out.Signals, struct {
			Code           string                          `json:"code"`
			DefaultWeight  int                             `json:"default_weight"`
			Label          string                          `json:"label"`
			Source         apigen.RiskCatalogSignalsSource `json:"source"`
			Strengthenable bool                            `json:"strengthenable"`
			Weight         int                             `json:"weight"`
		}{s.Code, s.DefaultWeight, s.Label, apigen.RiskCatalogSignalsSource(s.Source), s.Strengthenable, settings.Weight(s)})
	}
	fields := rules.Fields()
	names := make([]string, 0, len(fields))
	for n := range fields {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		out.Fields = append(out.Fields, struct {
			Name      string                       `json:"name"`
			Operators []string                     `json:"operators"`
			Type      apigen.RiskCatalogFieldsType `json:"type"`
		}{n, rules.Operators(fields[n]), apigen.RiskCatalogFieldsType(fields[n].String())})
	}
	return out, nil
}

// GetRiskSettings implements GET /api/v1/risk/settings.
func (h *Handlers) GetRiskSettings(ctx context.Context, _ apigen.GetRiskSettingsRequestObject) (apigen.GetRiskSettingsResponseObject, error) {
	s, err := h.svc.Settings(ctx)
	if err != nil {
		return nil, MapError(err)
	}
	return apigen.GetRiskSettings200JSONResponse(settingsToAPI(s)), nil
}

// PutRiskSettings implements PUT /api/v1/risk/settings.
func (h *Handlers) PutRiskSettings(ctx context.Context, req apigen.PutRiskSettingsRequestObject) (apigen.PutRiskSettingsResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in := domain.Settings{
		Weights:             req.Body.Weights,
		HighValueDefault:    req.Body.HighValueDefault,
		HighValueByCurrency: req.Body.HighValueByCurrency,
		FailPolicies:        map[string]domain.FailPolicy{},
		AlertMinDecision:    domain.DecisionReview,
	}
	if req.Body.AlertMinDecision != nil {
		in.AlertMinDecision = domain.Decision(*req.Body.AlertMinDecision)
	}
	for k, v := range req.Body.FailPolicies {
		in.FailPolicies[k] = domain.FailPolicy(v)
	}
	s, err := h.svc.PutSettings(ctx, in)
	if err != nil {
		return nil, MapError(err)
	}
	return apigen.PutRiskSettings200JSONResponse(settingsToAPI(s)), nil
}

// ListRules implements GET /api/v1/rules.
func (h *Handlers) ListRules(ctx context.Context, _ apigen.ListRulesRequestObject) (apigen.ListRulesResponseObject, error) {
	rs, err := h.svc.ListRules(ctx)
	if err != nil {
		return nil, MapError(err)
	}
	data := make([]apigen.Rule, len(rs))
	for i, r := range rs {
		if data[i], err = RuleToAPI(r); err != nil {
			return nil, err
		}
	}
	return apigen.ListRules200JSONResponse{Data: data}, nil
}

// CreateRule implements POST /api/v1/rules.
func (h *Handlers) CreateRule(ctx context.Context, req apigen.CreateRuleRequestObject) (apigen.CreateRuleResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	b := req.Body
	condition, err := json.Marshal(b.Condition)
	if err != nil {
		return nil, fmt.Errorf("encode condition: %w", err)
	}
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	description := ""
	if b.Description != nil {
		description = *b.Description
	}
	r, err := h.svc.CreateRule(ctx, service.RuleInput{
		Code: b.Code, Name: b.Name, Description: description, Severity: string(b.Severity),
		Weight: b.Weight, Mode: domain.RuleMode(b.Mode), Enabled: enabled, Condition: condition,
	})
	if err != nil {
		return nil, MapError(err)
	}
	out, err := RuleToAPI(r)
	if err != nil {
		return nil, err
	}
	return apigen.CreateRule201JSONResponse(out), nil
}

// GetRule implements GET /api/v1/rules/{rule_id}.
func (h *Handlers) GetRule(ctx context.Context, req apigen.GetRuleRequestObject) (apigen.GetRuleResponseObject, error) {
	id, err := ParseRuleID(req.RuleId)
	if err != nil {
		return nil, err
	}
	r, err := h.svc.GetRule(ctx, id)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := RuleToAPI(r)
	if err != nil {
		return nil, err
	}
	return apigen.GetRule200JSONResponse(out), nil
}

// UpdateRule implements PATCH /api/v1/rules/{rule_id}.
func (h *Handlers) UpdateRule(ctx context.Context, req apigen.UpdateRuleRequestObject) (apigen.UpdateRuleResponseObject, error) {
	id, err := ParseRuleID(req.RuleId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	b := req.Body
	patch := service.RulePatch{Name: b.Name, Description: b.Description, Weight: b.Weight, Enabled: b.Enabled}
	if b.Severity != nil {
		patch.Severity = new(string(*b.Severity))
	}
	if b.Mode != nil {
		patch.Mode = new(domain.RuleMode(*b.Mode))
	}
	if b.Condition != nil {
		if patch.Condition, err = json.Marshal(b.Condition); err != nil {
			return nil, fmt.Errorf("encode condition: %w", err)
		}
	}
	r, err := h.svc.UpdateRule(ctx, id, patch)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := RuleToAPI(r)
	if err != nil {
		return nil, err
	}
	return apigen.UpdateRule200JSONResponse(out), nil
}

// ListRuleVersions implements GET /api/v1/rules/{rule_id}/versions.
func (h *Handlers) ListRuleVersions(ctx context.Context, req apigen.ListRuleVersionsRequestObject) (apigen.ListRuleVersionsResponseObject, error) {
	id, err := ParseRuleID(req.RuleId)
	if err != nil {
		return nil, err
	}
	r, versions, err := h.svc.RuleVersions(ctx, id)
	if err != nil {
		return nil, MapError(err)
	}
	data := make([]apigen.RuleVersion, len(versions))
	for i, v := range versions {
		cond, err := conditionToAPI(v.Condition)
		if err != nil {
			return nil, err
		}
		data[i] = apigen.RuleVersion{
			Version: v.Version, Name: v.Name, Description: v.Description, Severity: apigen.Severity(v.Severity),
			Weight: v.Weight, Mode: apigen.RuleMode(v.Mode), Enabled: v.Enabled, Condition: cond,
			CreatedAt: v.CreatedAt, CreatedBy: v.CreatedBy, RestoredFrom: v.RestoredFrom,
		}
	}
	return apigen.ListRuleVersions200JSONResponse{RuleId: ids.Format(ids.PrefixRule, id), CurrentVersion: r.Current.Version, Data: data}, nil
}

// RollbackRule implements POST /api/v1/rules/{rule_id}/rollback.
func (h *Handlers) RollbackRule(ctx context.Context, req apigen.RollbackRuleRequestObject) (apigen.RollbackRuleResponseObject, error) {
	id, err := ParseRuleID(req.RuleId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	r, err := h.svc.RollbackRule(ctx, id, req.Body.Version)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := RuleToAPI(r)
	if err != nil {
		return nil, err
	}
	return apigen.RollbackRule200JSONResponse(out), nil
}

// ParseRuleID parses a rul_ ID; a malformed ID is indistinguishable from a missing rule.
func ParseRuleID(s string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixRule, s)
	if err != nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

// RuleToAPI renders a rule.
func RuleToAPI(r domain.Rule) (apigen.Rule, error) {
	cond, err := conditionToAPI(r.Current.Condition)
	if err != nil {
		return apigen.Rule{}, err
	}
	v := r.Current
	return apigen.Rule{
		Id: ids.Format(ids.PrefixRule, r.ID), Code: r.Code, Name: v.Name, Description: v.Description,
		Severity: apigen.Severity(v.Severity), Weight: v.Weight, Mode: apigen.RuleMode(v.Mode), Enabled: v.Enabled,
		Condition: cond, Version: v.Version, RestoredFrom: v.RestoredFrom, CreatedAt: r.CreatedAt, UpdatedAt: v.CreatedAt,
	}, nil
}

func conditionToAPI(raw json.RawMessage) (apigen.RuleCondition, error) {
	var c apigen.RuleCondition
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("decode stored condition: %w", err)
	}
	return c, nil
}

func settingsToAPI(s domain.Settings) apigen.RiskSettings {
	out := apigen.RiskSettings{
		Weights: s.Weights, HighValueDefault: s.HighValueDefault, HighValueByCurrency: s.HighValueByCurrency,
		FailPolicies:     map[string]apigen.RiskSettingsFailPolicies{},
		AlertMinDecision: new(apigen.RiskSettingsAlertMinDecision(s.AlertMinDecision)),
	}
	for k, v := range s.FailPolicies {
		out.FailPolicies[k] = apigen.RiskSettingsFailPolicies(v)
	}
	if out.Weights == nil {
		out.Weights = map[string]int{}
	}
	if out.HighValueByCurrency == nil {
		out.HighValueByCurrency = map[string]int64{}
	}
	return out
}

// MapError translates the risk module's errors to the error contract.
func MapError(err error) error {
	detail := func(sentinel error) string {
		return strings.TrimPrefix(strings.TrimPrefix(err.Error(), sentinel.Error()), ": ")
	}
	switch {
	case errors.Is(err, domain.ErrRuleNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, domain.ErrRuleInvalid):
		return httpx.InvalidRequest(capitalizeSentence(detail(domain.ErrRuleInvalid)), err)
	case errors.Is(err, domain.ErrSettingsInvalid):
		return httpx.InvalidRequest(capitalizeSentence(detail(domain.ErrSettingsInvalid)), err)
	case errors.Is(err, domain.ErrRuleLimit):
		return httpx.Conflict(capitalizeSentence(detail(domain.ErrRuleLimit)), err)
	case errors.Is(err, domain.ErrRuleCodeTaken):
		return httpx.Conflict("A rule with this code already exists in this environment.", err)
	case errors.Is(err, domain.ErrRuleConflict):
		return httpx.Conflict("The rule was changed by another request; reload it and try again.", err)
	default:
		return err
	}
}

func capitalizeSentence(s string) string {
	if s == "" {
		return "The request is invalid."
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}
