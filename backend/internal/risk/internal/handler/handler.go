// Package handler adapts HTTP requests for the risk module: rules, risk settings and the catalog.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/engine"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/service"
	rulepack "github.com/tobioyinlola/RiskOS/backend/internal/rules"
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
	info := rules.Info()
	out.Fields = slices.Grow(out.Fields, len(names))[:len(names)]
	for i, n := range names {
		fi, f := info[n], &out.Fields[i]
		f.Name, f.Operators, f.Type = n, rules.Operators(fields[n]), apigen.RiskCatalogFieldsType(fields[n].String())
		f.Label, f.Group, f.Events = fi.Label, fi.Group, append([]string{}, fi.Events...)
		if fi.Unit != "" {
			f.Unit = new(apigen.RiskCatalogFieldsUnit(fi.Unit))
		}
		if fi.Kind != "" {
			f.Kind = new(apigen.RiskCatalogFieldsKind(fi.Kind))
		}
		if len(fi.Values) > 0 {
			values := make([]struct {
				Label string `json:"label"`
				Value string `json:"value"`
			}, len(fi.Values))
			for j, c := range fi.Values {
				values[j].Label, values[j].Value = c.Label, c.Value
			}
			f.Values = &values
		}
	}
	return out, nil
}

// GetRiskPolicy implements GET /api/v1/risk/policy.
func (h *Handlers) GetRiskPolicy(ctx context.Context, _ apigen.GetRiskPolicyRequestObject) (apigen.GetRiskPolicyResponseObject, error) {
	view, err := h.svc.Policy(ctx)
	if err != nil {
		return nil, MapError(err)
	}
	s := view.Settings
	t := s.Thresholds
	bounds := []struct {
		level    domain.Level
		min, max int
	}{
		{domain.LevelMinimal, 0, t.Low - 1}, {domain.LevelLow, t.Low, t.Elevated - 1},
		{domain.LevelElevated, t.Elevated, t.High - 1}, {domain.LevelHigh, t.High, t.Critical - 1},
		{domain.LevelCritical, t.Critical, domain.MaxScore},
	}
	out := apigen.GetRiskPolicy200JSONResponse{
		EngineVersion: engine.Version, CustomCalibration: t != domain.DefaultThresholds(),
		AlertMinDecision: apigen.RiskPolicyAlertMinDecision(s.AlertMinDecision),
		Thresholds:       apigen.RiskThresholds{Low: t.Low, Elevated: t.Elevated, High: t.High, Critical: t.Critical},
		AutoBlock: apigen.AutoBlockStatus{
			Mode: apigen.AutoBlockMode(s.AutoBlock), ShadowSince: s.AutoBlockSince, EligibleAt: s.AutoBlockEligibleAt(),
			ShadowDaysRequired: int(domain.AutoBlockShadowPeriod.Hours() / 24), WouldBlockLast14Days: int(view.WouldBlock),
		},
	}
	for _, b := range bounds {
		out.Bands = append(out.Bands, struct {
			Band            apigen.RiskLevel `json:"band"`
			DefaultDecision apigen.Decision  `json:"default_decision"`
			MaxScore        int              `json:"max_score"`
			MinScore        int              `json:"min_score"`
		}{Band: apigen.RiskLevel(b.level), DefaultDecision: apigen.Decision(s.DecisionFor(b.level)), MaxScore: b.max, MinScore: b.min})
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
		Thresholds:          domain.DefaultThresholds(),
	}
	if t := req.Body.Thresholds; t != nil {
		in.Thresholds = domain.Thresholds{Low: t.Low, Elevated: t.Elevated, High: t.High, Critical: t.Critical}
	}
	if req.Body.AlertMinDecision != nil {
		in.AlertMinDecision = domain.Decision(*req.Body.AlertMinDecision)
	}
	if req.Body.AutoBlock != nil {
		in.AutoBlock = domain.AutoBlockMode(req.Body.AutoBlock.Mode)
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
	res, err := h.svc.CreateRule(ctx, service.RuleInput{
		Code: b.Code, Name: b.Name, Description: description, Severity: string(b.Severity),
		Weight: b.Weight, Mode: domain.RuleMode(b.Mode), Enabled: enabled, Condition: condition,
	})
	if err != nil {
		return nil, MapError(err)
	}
	if res.Change != nil {
		c, err := ChangeToAPI(*res.Change)
		return apigen.CreateRule202JSONResponse(c), err
	}
	out, err := RuleToAPI(res.Rule)
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
	res, err := h.svc.UpdateRule(ctx, id, patch)
	if err != nil {
		return nil, MapError(err)
	}
	if res.Change != nil {
		c, err := ChangeToAPI(*res.Change)
		return apigen.UpdateRule202JSONResponse(c), err
	}
	out, err := RuleToAPI(res.Rule)
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
	res, err := h.svc.RollbackRule(ctx, id, req.Body.Version)
	if err != nil {
		return nil, MapError(err)
	}
	if res.Change != nil {
		c, err := ChangeToAPI(*res.Change)
		return apigen.RollbackRule202JSONResponse(c), err
	}
	out, err := RuleToAPI(res.Rule)
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
		PendingChangeId: formatID(ids.PrefixRuleChange, r.PendingChange),
	}, nil
}

func formatID(prefix string, id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	return new(ids.Format(prefix, *id))
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
		AutoBlock:        &apigen.AutoBlock{Mode: apigen.AutoBlockMode(autoBlockMode(s.AutoBlock)), ShadowSince: s.AutoBlockSince},
		Thresholds: &apigen.RiskThresholds{
			Low: s.Thresholds.Low, Elevated: s.Thresholds.Elevated, High: s.Thresholds.High, Critical: s.Thresholds.Critical,
		},
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

func autoBlockMode(m domain.AutoBlockMode) domain.AutoBlockMode {
	if m == "" {
		return domain.AutoBlockOff
	}
	return m
}

// MapError translates the risk module's errors to the error contract.
func MapError(err error) error {
	detail := func(sentinel error) string {
		return strings.TrimPrefix(strings.TrimPrefix(err.Error(), sentinel.Error()), ": ")
	}
	switch {
	case errors.Is(err, domain.ErrRuleNotFound), errors.Is(err, rulepack.ErrNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, domain.ErrBacktestInvalid):
		return httpx.InvalidRequest(capitalizeSentence(detail(domain.ErrBacktestInvalid)), err)
	case errors.Is(err, rulepack.ErrInvalidInput):
		return httpx.InvalidRequest(capitalizeSentence(detail(rulepack.ErrInvalidInput)), err)
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
	case errors.Is(err, domain.ErrChangeNotFound), errors.Is(err, domain.ErrDenyNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, domain.ErrDenyInvalid):
		return httpx.InvalidRequest(capitalizeSentence(detail(domain.ErrDenyInvalid)), err)
	case errors.Is(err, domain.ErrDenyDuplicate):
		return httpx.Conflict("This identifier is already on the deny list.", err)
	case errors.Is(err, domain.ErrDenyLimit):
		return httpx.Conflict(capitalizeSentence(detail(domain.ErrDenyLimit)), err)
	case errors.Is(err, domain.ErrChangePending), errors.Is(err, domain.ErrChangeDecided), errors.Is(err, domain.ErrChangeStale):
		return httpx.Conflict(capitalizeSentence(err.Error()), err)
	case errors.Is(err, domain.ErrSelfApproval), errors.Is(err, domain.ErrApproverNotPerson):
		return httpx.NewError(http.StatusForbidden, httpx.CodeForbidden, capitalizeSentence(err.Error()), err)
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

func tallyToAPI(t service.BacktestTally) apigen.BacktestTally {
	samples := make([]string, len(t.Samples))
	for i, id := range t.Samples {
		samples[i] = ids.Format(ids.PrefixEvent, id)
	}
	missing := make([]apigen.BacktestMissing, 0, len(t.Missing))
	for _, item := range slices.Sorted(maps.Keys(t.Missing)) {
		missing = append(missing, apigen.BacktestMissing{Item: item, Events: t.Missing[item]})
	}
	slices.SortStableFunc(missing, func(a, b apigen.BacktestMissing) int { return b.Events - a.Events })
	return apigen.BacktestTally{
		Triggered: t.Triggered, Fired: t.Fired, Clear: t.Clear, NotEvaluated: t.NotEvaluated, Missing: missing,
		FiredSamples: samples,
	}
}

// BacktestPackRule implements POST /api/v1/compliance/rules/{rule_id}/backtest.
func (h *Handlers) BacktestPackRule(ctx context.Context, req apigen.BacktestPackRuleRequestObject) (apigen.BacktestPackRuleResponseObject, error) {
	in := service.BacktestInput{RuleID: req.RuleId, Days: 7}
	if b := req.Body; b != nil {
		if b.Days != nil {
			in.Days = *b.Days
		}
		if b.Settings != nil {
			in.Settings = *b.Settings
		}
	}
	res, err := h.svc.Backtest(ctx, in)
	if err != nil {
		return nil, MapError(err)
	}
	return apigen.BacktestPackRule200JSONResponse{
		Rule: res.RuleID, From: res.From, To: res.To, Replayed: res.Replayed, Truncated: res.Truncated,
		Candidate: tallyToAPI(res.Candidate), Current: tallyToAPI(res.Current),
	}, nil
}

// ListRestrictedSignals implements GET /api/v1/compliance/restricted-signals.
func (h *Handlers) ListRestrictedSignals(ctx context.Context, req apigen.ListRestrictedSignalsRequestObject) (apigen.ListRestrictedSignalsResponseObject, error) {
	days := 7
	if req.Params.Days != nil {
		days = *req.Params.Days
	}
	rows, err := h.svc.RestrictedSignals(ctx, days)
	if err != nil {
		return nil, MapError(err)
	}
	data := make([]apigen.RestrictedEvaluation, len(rows))
	for i, r := range rows {
		signals := make([]apigen.RestrictedSignal, len(r.Signals))
		for j, s := range r.Signals {
			signals[j] = apigen.RestrictedSignal{Rule: s.Code, Name: s.Label, Weight: s.Weight, Mode: apigen.PackRuleMode(s.Mode)}
		}
		data[i] = apigen.RestrictedEvaluation{
			EvaluationId: ids.Format(ids.PrefixEvaluation, r.ID), CreatedAt: r.CreatedAt, Signals: signals,
		}
		if r.EventID != nil {
			data[i].EventId = new(ids.Format(ids.PrefixEvent, *r.EventID))
		}
	}
	return apigen.ListRestrictedSignals200JSONResponse{Data: data}, nil
}

// GetRuleStats implements GET /api/v1/rules/stats.
func (h *Handlers) GetRuleStats(ctx context.Context, req apigen.GetRuleStatsRequestObject) (apigen.GetRuleStatsResponseObject, error) {
	n := 0
	if req.Params.Days != nil {
		n = *req.Params.Days
	}
	st, err := h.svc.RuleStats(ctx, n)
	if err != nil {
		return nil, MapError(err)
	}
	out := apigen.GetRuleStats200JSONResponse{Since: st.Since, Days: st.Days, Evaluations: st.Evaluations}
	out.Rules = slices.Grow(out.Rules, len(st.Rules))[:len(st.Rules)]
	for i, r := range st.Rules {
		o := &out.Rules[i]
		o.RuleId, o.Matches, o.LastMatchedAt = ids.Format(ids.PrefixRule, r.RuleID), r.Matches, r.LastMatchedAt
		o.Daily = slices.Grow(o.Daily, len(r.Daily))[:len(r.Daily)]
		for j, d := range r.Daily {
			o.Daily[j].Day, o.Daily[j].Matches = openapi_types.Date{Time: d.Day}, d.Matches
		}
	}
	return out, nil
}

// TestRuleOnRecentEvents implements POST /api/v1/rules/test-recent.
func (h *Handlers) TestRuleOnRecentEvents(ctx context.Context, req apigen.TestRuleOnRecentEventsRequestObject) (apigen.TestRuleOnRecentEventsResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	condition, err := json.Marshal(req.Body.Condition)
	if err != nil {
		return nil, httpx.InvalidRequest("The condition is not valid JSON.", err)
	}
	n := 0
	if req.Body.Days != nil {
		n = *req.Body.Days
	}
	res, err := h.svc.TestRecent(ctx, condition, n)
	if err != nil {
		return nil, MapError(err)
	}
	out := apigen.TestRuleOnRecentEvents200JSONResponse{
		Since: res.Since, Checked: res.Checked, Matched: res.Matched, Truncated: res.Truncated,
	}
	out.Sample = slices.Grow(out.Sample, len(res.Sample))[:len(res.Sample)]
	for i, m := range res.Sample {
		o := &out.Sample[i]
		o.EvaluationId, o.At, o.EventType = ids.Format(ids.PrefixEvaluation, m.EvaluationID), m.At, m.EventType
		o.Amount, o.Currency, o.Decision, o.RiskScore = m.Amount, m.Currency, m.Decision, m.Score
		if m.EventID != nil {
			o.EventId = ids.Format(ids.PrefixEvent, *m.EventID)
		}
	}
	return out, nil
}
