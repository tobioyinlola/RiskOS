package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
)

// Handlers implements the compliance library operations of the public API.
type Handlers struct {
	svc   *service.Service
	clock clock.Clock
}

// New returns the library handlers.
func New(svc *service.Service, clk clock.Clock) *Handlers {
	return &Handlers{svc: svc, clock: clk}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, service.ErrInvalidInput):
		msg := strings.TrimPrefix(err.Error(), service.ErrInvalidInput.Error()+": ")
		return httpx.InvalidRequest(strings.ToUpper(msg[:1])+msg[1:]+".", err)
	default:
		return err
	}
}

// list returns a non-nil copy of in, so required arrays encode as [] rather than null.
func list[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func options(in []pack.Licence) []apigen.ComplianceOption {
	out := make([]apigen.ComplianceOption, len(in))
	for i, o := range in {
		out[i] = apigen.ComplianceOption{Id: o.ID, Label: o.Label}
	}
	return out
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetCompliancePack implements GET /api/v1/compliance/pack.
func (h *Handlers) GetCompliancePack(ctx context.Context, req apigen.GetCompliancePackRequestObject) (apigen.GetCompliancePackResponseObject, error) {
	jurisdiction := ""
	if req.Params.Jurisdiction != nil {
		jurisdiction = *req.Params.Jurisdiction
	}
	p, err := h.svc.Pack(ctx, jurisdiction)
	if err != nil {
		return nil, mapError(err)
	}
	counts := map[pack.Kind]int{}
	perGroup := map[string]int{}
	for _, r := range p.Rules {
		counts[r.Kind]++
		perGroup[r.Group]++
	}
	groups := make([]apigen.ComplianceGroup, len(p.Groups))
	for i, g := range p.Groups {
		groups[i] = apigen.ComplianceGroup{Id: g.ID, Label: g.Label, Rules: perGroup[g.ID]}
	}
	sources := make([]apigen.ComplianceSource, len(p.Sources))
	for i, s := range p.Sources {
		links := make([]apigen.ComplianceLink, len(s.Links))
		for j, l := range s.Links {
			links[j] = apigen.ComplianceLink{Title: l.Title, Url: l.URL}
		}
		sources[i] = apigen.ComplianceSource{Id: s.ID, Title: s.Title, Links: links}
	}
	penalties := make([]apigen.CompliancePenalty, len(p.Penalties))
	for i, pen := range p.Penalties {
		penalties[i] = apigen.CompliancePenalty{
			Provision: pen.Provision, Penalty: pen.Penalty, Estimate: optional(pen.Estimate), Inference: pen.Inference,
			Sources: list(pen.Sources),
		}
	}
	locale := p.Locale()
	return apigen.GetCompliancePack200JSONResponse{
		Currency: optional(locale.Currency), Timezone: optional(locale.Timezone), Penalties: &penalties,
		Id: p.ID, Jurisdiction: p.Jurisdiction, Name: p.Name, Version: p.Version, LatestVersion: p.Latest, Checksum: p.Checksum,
		LoadedAt: p.LoadedAt, Groups: groups, LicenceTypes: options(p.LicenceTypes),
		Capabilities: options(p.Capabilities), Sources: sources,
		Counts: apigen.ComplianceCounts{
			Controls: counts[pack.KindControl], Signals: counts[pack.KindSignal], Governance: counts[pack.KindGovernance],
		},
	}, nil
}

func ruleToAPI(v *service.RuleView) (apigen.ComplianceRule, error) {
	var events []string
	if v.Trigger != nil {
		events = v.Trigger.Events
	}
	tasks := make([]string, len(v.Outcome.Tasks))
	for i, t := range v.Outcome.Tasks {
		tasks[i] = t.Type
	}
	params := make([]apigen.ComplianceParameter, len(v.Parameters))
	for i, p := range v.Parameters {
		params[i] = apigen.ComplianceParameter{
			Name: p.Name, Label: p.Label, Type: apigen.ComplianceParameterType(p.Type), Currency: optional(p.Currency),
			Default: p.Default, Value: v.Value(p), Min: p.Min, Max: p.Max,
		}
	}
	out := apigen.ComplianceRule{
		Id: v.ID, Name: v.Text.Name, Description: v.Text.Description, Action: optional(v.Text.Action),
		Points: optional(v.Text.Points), Group: v.Group, Kind: apigen.ComplianceRuleKind(v.Kind),
		Status: apigen.ComplianceRuleStatus(v.Status), Regulators: list(v.Regulators), AppliesTo: list(v.AppliesTo),
		Requires: list(v.Requires), Sources: list(v.Sources),
		Effective:    apigen.ComplianceEffective{Date: optional(v.Effective.Date), Text: v.Effective.Text},
		Verification: apigen.ComplianceVerification(v.Verification), Caveat: optional(v.Caveat),
		Defaults: apigen.ComplianceDefaults{
			Enabled: v.Defaults.Enabled, Mode: apigen.ComplianceDefaultsMode(v.Defaults.Mode),
		},
		Inputs: list(v.Inputs), Events: list(events),
		Outcome: apigen.ComplianceOutcome{
			Decision: optional(v.Outcome.Decision), Severity: optional(v.Outcome.Severity), Tasks: tasks,
		},
		Parameters: params, Applicable: v.Applicable,
		Mode: apigen.PackRuleMode(v.Config.Mode), ConfigVersion: v.Config.Version, Verifications: verificationsToAPI(v.Verified),
		Jurisdiction: optional(v.Jurisdiction), InheritedFrom: optional(v.InheritedFrom), Basis: optional(v.Text.Basis),
	}
	if v.Provenance != "" {
		p := apigen.ComplianceRuleProvenance(v.Provenance)
		out.Provenance = &p
	}
	if v.OffReason != "" {
		r := apigen.ComplianceRuleOffReason(v.OffReason)
		out.OffReason = &r
	}
	if v.Lineage != nil {
		out.Lineage = &apigen.ComplianceLineage{Kind: apigen.ComplianceLineageKind(v.Lineage.Kind), Of: list(v.Lineage.Of)}
	}
	if v.Template != nil {
		out.Template = &v.Template.Rule
	}
	if len(v.Overrides) > 0 {
		changes := slices.Sorted(maps.Keys(v.Overrides))
		out.Changes = &changes
	}
	if v.Engineering {
		raw, err := json.Marshal(v.Rule)
		if err != nil {
			return apigen.ComplianceRule{}, fmt.Errorf("encode rule %s: %w", v.ID, err)
		}
		var def map[string]any
		if err := json.Unmarshal(raw, &def); err != nil {
			return apigen.ComplianceRule{}, fmt.Errorf("encode rule %s: %w", v.ID, err)
		}
		out.Definition = &def
		out.EngineeringLogic = &v.Text.EngineeringLogic
	}
	return out, nil
}

// ListComplianceRules implements GET /api/v1/compliance/rules.
func (h *Handlers) ListComplianceRules(ctx context.Context, req apigen.ListComplianceRulesRequestObject) (apigen.ListComplianceRulesResponseObject, error) {
	f := service.Filter{Applicable: req.Params.Applicable}
	for _, p := range []struct {
		dst *string
		src *string
	}{
		{&f.Group, req.Params.Group}, {&f.Regulator, req.Params.Regulator}, {&f.Query, req.Params.Q},
		{&f.Kind, (*string)(req.Params.Kind)}, {&f.Verification, (*string)(req.Params.Verification)},
		{&f.Jurisdiction, req.Params.Jurisdiction},
	} {
		if p.src != nil {
			*p.dst = *p.src
		}
	}
	rules, err := h.svc.Rules(ctx, f)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ComplianceRule, len(rules))
	for i := range rules {
		if data[i], err = ruleToAPI(&rules[i]); err != nil {
			return nil, err
		}
	}
	return apigen.ListComplianceRules200JSONResponse{Data: data}, nil
}

// GetComplianceRule implements GET /api/v1/compliance/rules/{rule_id}.
func (h *Handlers) GetComplianceRule(ctx context.Context, req apigen.GetComplianceRuleRequestObject) (apigen.GetComplianceRuleResponseObject, error) {
	jurisdiction := ""
	if req.Params.Jurisdiction != nil {
		jurisdiction = *req.Params.Jurisdiction
	}
	r, err := h.svc.Rule(ctx, jurisdiction, req.RuleId)
	if err != nil {
		return nil, mapError(err)
	}
	out, err := ruleToAPI(&r)
	if err != nil {
		return nil, err
	}
	return apigen.GetComplianceRule200JSONResponse(out), nil
}

// jurisdictionOf reads an optional jurisdiction parameter.
func jurisdictionOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ListComplianceClocks implements GET /api/v1/compliance/clocks.
func (h *Handlers) ListComplianceClocks(ctx context.Context, req apigen.ListComplianceClocksRequestObject) (apigen.ListComplianceClocksResponseObject, error) {
	p, err := h.svc.Pack(ctx, jurisdictionOf(req.Params.Jurisdiction))
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ComplianceClock, len(p.Clocks))
	for i, c := range p.Clocks {
		data[i] = apigen.ComplianceClock{
			Id: c.ID, Duration: c.Duration, Duty: c.Duty, AppliesTo: c.AppliesTo, Requirement: c.Requirement,
			Hours: c.Hours, Kind: apigen.ComplianceClockKind(c.ClockKindOf()), Authority: optional(c.Authority),
			Sources: list(c.Sources), Rules: list(c.Rules),
		}
	}
	return apigen.ListComplianceClocks200JSONResponse{Data: data}, nil
}

// ListComplianceGaps implements GET /api/v1/compliance/gaps.
func (h *Handlers) ListComplianceGaps(ctx context.Context, req apigen.ListComplianceGapsRequestObject) (apigen.ListComplianceGapsResponseObject, error) {
	p, err := h.svc.Pack(ctx, jurisdictionOf(req.Params.Jurisdiction))
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ComplianceGap, len(p.Gaps))
	for i, g := range p.Gaps {
		data[i] = apigen.ComplianceGap{
			Id: g.ID, Title: g.Title, Detail: g.Detail, Treatment: optional(g.Treatment),
			Rules: list(g.Rules), Sources: list(g.Sources),
		}
		if g.Kind != "" {
			k := apigen.ComplianceGapKind(g.Kind)
			data[i].Kind = &k
		}
		if v, ok := p.Verified[g.ID]; ok {
			gv := gapVerificationToAPI(v)
			data[i].Verification = &gv
		}
	}
	return apigen.ListComplianceGaps200JSONResponse{Data: data}, nil
}

// ListComplianceGlossary implements GET /api/v1/compliance/glossary.
func (h *Handlers) ListComplianceGlossary(ctx context.Context, req apigen.ListComplianceGlossaryRequestObject) (apigen.ListComplianceGlossaryResponseObject, error) {
	p, err := h.svc.Pack(ctx, jurisdictionOf(req.Params.Jurisdiction))
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ComplianceTerm, len(p.Glossary))
	for i, t := range p.Glossary {
		data[i] = apigen.ComplianceTerm{Term: t.Term, Expansion: t.Expansion, Definition: t.Definition}
	}
	return apigen.ListComplianceGlossary200JSONResponse{Data: data}, nil
}

func profileToAPI(p service.Profile) apigen.ComplianceProfile {
	return apigen.ComplianceProfile{
		Jurisdiction: p.Jurisdiction, LicenceType: p.LicenceType, Capabilities: list(p.Capabilities),
		PackId: p.PackID, PackVersion: p.PackVersion, UpdatedAt: p.UpdatedAt, UpdatedBy: p.UpdatedBy,
		Confirmed: p.Confirmed,
	}
}

// GetComplianceProfile implements GET /api/v1/compliance/profile.
func (h *Handlers) GetComplianceProfile(ctx context.Context, _ apigen.GetComplianceProfileRequestObject) (apigen.GetComplianceProfileResponseObject, error) {
	p, err := h.svc.Profile(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.GetComplianceProfile200JSONResponse(profileToAPI(p)), nil
}

// PutComplianceProfile implements PUT /api/v1/compliance/profile.
func (h *Handlers) PutComplianceProfile(ctx context.Context, req apigen.PutComplianceProfileRequestObject) (apigen.PutComplianceProfileResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	p, err := h.svc.SaveProfile(ctx, service.ProfileInput{
		Jurisdiction: req.Body.Jurisdiction, LicenceType: req.Body.LicenceType, Capabilities: req.Body.Capabilities,
	}, h.clock.Now())
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.PutComplianceProfile200JSONResponse(profileToAPI(p)), nil
}

// ListCompliancePacks implements GET /api/v1/compliance/packs.
func (h *Handlers) ListCompliancePacks(ctx context.Context, _ apigen.ListCompliancePacksRequestObject) (apigen.ListCompliancePacksResponseObject, error) {
	packs, err := h.svc.Packs(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.CompliancePackSummary, len(packs))
	for i, p := range packs {
		data[i] = apigen.CompliancePackSummary{
			Id: p.ID, Jurisdiction: p.Jurisdiction, Name: p.Name, Version: p.Version, Current: p.Current, Activated: &p.Activated,
			Controls: p.Counts[pack.KindControl], Signals: p.Counts[pack.KindSignal], Governance: p.Counts[pack.KindGovernance],
		}
	}
	return apigen.ListCompliancePacks200JSONResponse{Data: data}, nil
}

// ListComplianceJurisdictions implements GET /api/v1/compliance/jurisdictions.
func (h *Handlers) ListComplianceJurisdictions(ctx context.Context, _ apigen.ListComplianceJurisdictionsRequestObject) (apigen.ListComplianceJurisdictionsResponseObject, error) {
	js, err := h.svc.Jurisdictions(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.Jurisdiction, len(js))
	for i, j := range js {
		data[i] = apigen.Jurisdiction{
			Code: j.Code, Name: j.Name, Currency: j.Currency, MinorUnit: int(j.MinorUnit), Timezone: j.Timezone,
			Regulators: list(j.Regulators), Pack: j.PackID, Activated: j.Activated,
		}
	}
	return apigen.ListComplianceJurisdictions200JSONResponse{Data: data}, nil
}

// ListComplianceActivations implements GET /api/v1/compliance/activations.
func (h *Handlers) ListComplianceActivations(ctx context.Context, _ apigen.ListComplianceActivationsRequestObject) (apigen.ListComplianceActivationsResponseObject, error) {
	acts, err := h.svc.Activations(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.JurisdictionActivation, len(acts))
	for i, a := range acts {
		data[i] = activationToAPI(a)
	}
	return apigen.ListComplianceActivations200JSONResponse{Data: data}, nil
}

// ListActiveComplianceRules implements GET /api/v1/compliance/active-rules.
func (h *Handlers) ListActiveComplianceRules(ctx context.Context, req apigen.ListActiveComplianceRulesRequestObject) (apigen.ListActiveComplianceRulesResponseObject, error) {
	jurisdiction := ""
	if req.Params.Jurisdiction != nil {
		jurisdiction = *req.Params.Jurisdiction
	}
	rules, err := h.svc.ActiveRules(ctx, jurisdiction)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ActiveComplianceRule, len(rules))
	for i, r := range rules {
		data[i] = apigen.ActiveComplianceRule{
			Rule: r.Rule.ID, Name: r.Rule.Text.Name, Kind: apigen.ComplianceRuleKind(r.Rule.Kind),
			Jurisdiction: r.Jurisdiction, Pack: r.Pack, BookingEntity: r.BookingEntity,
			Mode: apigen.ActiveComplianceRuleMode(r.Mode), Events: list(r.Rule.Trigger.Events),
			Facts: list(r.Facts), Refs: list(r.Refs), UnsetSettings: list(r.UnsetSettings),
		}
		if r.Rule.Provenance != "" {
			p := apigen.ActiveComplianceRuleProvenance(r.Rule.Provenance)
			data[i].Provenance = &p
		}
	}
	return apigen.ListActiveComplianceRules200JSONResponse{Data: data}, nil
}

// GetComplianceClockMatrix implements GET /api/v1/compliance/clock-matrix.
func (h *Handlers) GetComplianceClockMatrix(ctx context.Context, _ apigen.GetComplianceClockMatrixRequestObject) (apigen.GetComplianceClockMatrixResponseObject, error) {
	rows, err := h.svc.ClockMatrix(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	data := make([]apigen.ClockMatrixRow, len(rows))
	for i, r := range rows {
		data[i] = apigen.ClockMatrixRow{
			Item: r.Item, Nigeria: r.Nigeria, Ghana: r.Ghana, Rwanda: r.Rwanda, WorkingDefault: r.WorkingDefault,
		}
	}
	return apigen.GetComplianceClockMatrix200JSONResponse{Data: data}, nil
}

// ExportComplianceLibrary implements GET /api/v1/compliance/export.
func (h *Handlers) ExportComplianceLibrary(ctx context.Context, req apigen.ExportComplianceLibraryRequestObject) (apigen.ExportComplianceLibraryResponseObject, error) {
	format := string(req.Params.Format)
	data, name, err := h.svc.Export(ctx, jurisdictionOf(req.Params.Jurisdiction), format, h.clock.Now())
	if err != nil {
		return nil, mapError(err)
	}
	headers := apigen.ExportComplianceLibrary200ResponseHeaders{ContentDisposition: new(`attachment; filename="` + name + `"`)}
	if format == service.ExportPDF {
		return apigen.ExportComplianceLibrary200ApplicationpdfResponse{
			Body: bytes.NewReader(data), ContentLength: int64(len(data)), Headers: headers,
		}, nil
	}
	return apigen.ExportComplianceLibrary200TextcsvResponse{
		Body: bytes.NewReader(data), ContentLength: int64(len(data)), Headers: headers,
	}, nil
}
