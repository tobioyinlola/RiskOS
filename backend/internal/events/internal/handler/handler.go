// Package handler adapts HTTP requests for the events module to its service.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

// Error codes specific to events (EVENTS.md).
const (
	CodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	CodeStaleEvent          = "STALE_EVENT"
	CodeInvalidEvent        = "INVALID_EVENT"
	CodeSensitiveData       = "SENSITIVE_DATA"
)

// Handlers implements the event operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *service.Service
}

// New returns the events HTTP handlers.
func New(svc *service.Service) *Handlers { return &Handlers{svc: svc} }

// IngestEvent implements POST /api/v1/events.
func (h *Handlers) IngestEvent(ctx context.Context, req apigen.IngestEventRequestObject) (apigen.IngestEventResponseObject, error) {
	e, raw, err := decode(ctx, req.Body)
	if err != nil {
		return nil, err
	}
	outcome, replay, err := h.svc.Ingest(ctx, raw, e)
	if err != nil {
		return nil, mapError(err)
	}
	resp := apigen.IngestEvent200JSONResponse{Body: toAPI(outcome)}
	if replay {
		resp.Headers.IdempotentReplay = new("true")
	}
	return resp, nil
}

// EvaluateRisk implements POST /api/v1/risk/evaluate (dry run).
func (h *Handlers) EvaluateRisk(ctx context.Context, req apigen.EvaluateRiskRequestObject) (apigen.EvaluateRiskResponseObject, error) {
	e, raw, err := decode(ctx, req.Body)
	if err != nil {
		return nil, err
	}
	outcome, err := h.svc.Evaluate(ctx, raw, e)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.EvaluateRisk200JSONResponse(toAPI(outcome)), nil
}

// ListEvents implements GET /api/v1/events.
func (h *Handlers) ListEvents(ctx context.Context, req apigen.ListEventsRequestObject) (apigen.ListEventsResponseObject, error) {
	page, err := httpx.ParsePage(req.Params.Limit, req.Params.Cursor, ids.PrefixEvent)
	if err != nil {
		return nil, err
	}
	filter, err := listFilter(req.Params)
	if err != nil {
		return nil, err
	}
	rows, err := h.svc.List(ctx, domain.Page{Limit: page.Limit + 1, Before: page.Before, Filter: filter})
	if err != nil {
		return nil, mapError(err)
	}
	rows, next := httpx.NextCursor(rows, page.Limit, ids.PrefixEvent, func(s domain.Summary) uuid.UUID { return s.ID })
	data := make([]apigen.EventSummary, len(rows))
	for i, s := range rows {
		data[i] = summaryToAPI(s)
	}
	return apigen.ListEvents200JSONResponse{Data: data, NextCursor: next}, nil
}

// GetEvent implements GET /api/v1/events/{event_id}.
func (h *Handlers) GetEvent(ctx context.Context, req apigen.GetEventRequestObject) (apigen.GetEventResponseObject, error) {
	id, err := ids.Parse(ids.PrefixEvent, req.EventId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	d, err := h.svc.Get(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var raw, normalized map[string]any
	if err := json.Unmarshal(d.RawPayload, &raw); err != nil {
		return nil, fmt.Errorf("decode raw payload: %w", err)
	}
	if err := json.Unmarshal(d.Normalized, &normalized); err != nil {
		return nil, fmt.Errorf("decode normalized payload: %w", err)
	}
	return apigen.GetEvent200JSONResponse{
		Id: ids.Format(ids.PrefixEvent, d.ID), EventKey: d.Key, Type: apigen.EventType(d.Type),
		Environment: apigen.Environment(scope.EnvironmentKind), OccurredAt: d.OccurredAt, ReceivedAt: d.ReceivedAt,
		RawPayload: raw, NormalizedPayload: normalized, Evaluation: toAPI(d.Evaluation),
	}, nil
}

// TestRule implements POST /api/v1/rules/{rule_id}/test. It lives here because the events module
// owns event validation and normalization; the rule itself is evaluated by the risk module.
func (h *Handlers) TestRule(ctx context.Context, req apigen.TestRuleRequestObject) (apigen.TestRuleResponseObject, error) {
	id, err := ids.Parse(ids.PrefixRule, req.RuleId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	body, ok := httpx.RawBody(ctx)
	if !ok {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	var envelope struct {
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, httpx.InvalidRequest("The request body is invalid.", err)
	}
	e, err := toDomain(&req.Body.Event)
	if err != nil {
		return nil, err
	}
	var draft json.RawMessage
	if req.Body.Condition != nil {
		if draft, err = json.Marshal(req.Body.Condition); err != nil {
			return nil, fmt.Errorf("encode condition: %w", err)
		}
	}
	res, err := h.svc.TestRule(ctx, id, envelope.Event, e, draft)
	if err != nil {
		return nil, risk.MapError(mapError(err))
	}
	out := apigen.TestRule200JSONResponse{
		RuleId: ids.Format(ids.PrefixRule, res.Rule.ID), Draft: res.Draft, Matched: res.Matched, Facts: res.Facts,
	}
	if !res.Draft {
		out.Version = new(res.Rule.Current.Version)
	}
	return out, nil
}

// decode converts the contract-validated body into a domain event and returns the raw bytes the
// validation middleware captured.
func decode(ctx context.Context, body *apigen.Event) (domain.Event, []byte, error) {
	raw, ok := httpx.RawBody(ctx)
	if body == nil || !ok {
		return domain.Event{}, nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	e, err := toDomain(body)
	if err != nil {
		return domain.Event{}, nil, err
	}
	return e, raw, nil
}

// toDomain converts a contract-validated event into the domain event.
func toDomain(body *apigen.Event) (domain.Event, error) {
	v, err := body.ValueByDiscriminator()
	if err != nil {
		return domain.Event{}, httpx.InvalidRequest("Unknown event type.", err)
	}
	var e domain.Event
	var data any
	switch ev := v.(type) {
	case apigen.LoginEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), ev.Data
	case apigen.TransactionEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), ev.Data
	case apigen.BeneficiaryAddedEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), ev.Data
	case apigen.DeviceChangeEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), ev.Data
	case apigen.SessionChangeEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), ev.Data
	case apigen.ProfileChangeEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), ev.Data
	case apigen.ActivityEvent:
		e, data = common(ev.EventId, string(ev.Type), ev.OccurredAt, ev.Actor, ev.Context), struct{}{}
	default:
		return domain.Event{}, httpx.InvalidRequest("Unknown event type.", fmt.Errorf("unexpected %T", v))
	}
	if e.Data, err = json.Marshal(data); err != nil {
		return domain.Event{}, fmt.Errorf("encode event data: %w", err)
	}
	e.Compliance = complianceFromAPI(v)
	return e, nil
}

// complianceFromAPI reads the compliance block every event type may carry.
func complianceFromAPI(v any) *rules.EventContext {
	var c *apigen.ComplianceContext
	switch ev := v.(type) {
	case apigen.LoginEvent:
		c = ev.Compliance
	case apigen.TransactionEvent:
		c = ev.Compliance
	case apigen.BeneficiaryAddedEvent:
		c = ev.Compliance
	case apigen.DeviceChangeEvent:
		c = ev.Compliance
	case apigen.SessionChangeEvent:
		c = ev.Compliance
	case apigen.ProfileChangeEvent:
		c = ev.Compliance
	case apigen.ActivityEvent:
		c = &ev.Compliance
	}
	if c == nil {
		return nil
	}
	out := &rules.EventContext{}
	if c.Kind != nil {
		out.Kind = string(*c.Kind)
	}
	if c.Facts != nil {
		out.Facts = *c.Facts
	}
	if r := c.Refs; r != nil {
		out.Refs = map[string]string{}
		for k, ref := range map[string]*string{
			"card": r.Card, "bvn": r.Bvn, "phone": r.Phone, "address": r.Address, "loan": r.Loan, "inflow": r.Inflow,
			"merchant": r.Merchant, "api_key": r.ApiKey, "sender": r.Sender, "reporter": r.Reporter,
			"funding_source": r.FundingSource,
		} {
			if ref != nil {
				out.Refs[k] = *ref
			}
		}
	}
	return out
}

func common(key, typ string, occurredAt time.Time, a apigen.Actor, c *apigen.EventContext) domain.Event {
	e := domain.Event{
		Key: key, Type: typ, OccurredAt: occurredAt,
		Actor: domain.Actor{UserID: a.UserId, AccountID: deref(a.AccountId), DeviceID: deref(a.DeviceId), SessionID: deref(a.SessionId)},
	}
	if c != nil {
		e.Context = domain.Context{IP: deref(c.Ip), UserAgent: deref(c.UserAgent), Country: deref(c.Country)}
	}
	return e
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mapError(err error) error {
	detail := func(sentinel error) string {
		return strings.TrimPrefix(strings.TrimPrefix(err.Error(), sentinel.Error()), ": ")
	}
	switch {
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return httpx.NewError(http.StatusConflict, CodeIdempotencyConflict,
			"This event_id was already used with a different payload.", err)
	case errors.Is(err, domain.ErrStaleEvent):
		return httpx.NewError(http.StatusUnprocessableEntity, CodeStaleEvent, detail(domain.ErrStaleEvent)+".", err)
	case errors.Is(err, domain.ErrInvalidEvent):
		return httpx.NewError(http.StatusUnprocessableEntity, CodeInvalidEvent, detail(domain.ErrInvalidEvent)+".", err)
	case errors.Is(err, rules.ErrSensitiveData):
		return httpx.NewError(http.StatusUnprocessableEntity, CodeSensitiveData, detail(rules.ErrSensitiveData)+".", err)
	case errors.Is(err, domain.ErrSensitiveData):
		return httpx.NewError(http.StatusUnprocessableEntity, CodeSensitiveData,
			"The event contains a value that looks like a card number. Never send card numbers, CVVs, PINs or passwords; use opaque references.", err)
	case errors.Is(err, domain.ErrNotFound):
		return httpx.NotFound(err)
	default:
		return err
	}
}

func toAPI(o domain.Outcome) apigen.RiskDecision {
	signals := make([]apigen.Signal, len(o.Signals))
	for i, s := range o.Signals {
		signals[i] = apigen.Signal{Code: s.Code, Label: s.Label, Weight: s.Weight}
	}
	d := apigen.RiskDecision{
		RequestId: o.RequestID, EvaluationId: ids.Format(ids.PrefixEvaluation, o.EvaluationID),
		RiskScore: o.Score, RiskLevel: apigen.RiskLevel(o.Level), Decision: apigen.Decision(o.Decision),
		EngineVersion: o.EngineVersion, Signals: signals, DegradedSignals: o.DegradedSignals,
		WarmingSignals: o.WarmingSignals,
	}
	if d.WarmingSignals == nil {
		d.WarmingSignals = []string{}
	}
	d.Controls = make([]apigen.ControlResult, len(o.Controls))
	for i, c := range o.Controls {
		d.Controls[i] = apigen.ControlResult{
			Rule: c.Rule, Name: c.Name, Mode: apigen.ControlResultMode(c.Mode), Reason: c.Reason, Tasks: c.Tasks,
			ExemptionUnknown: c.ExemptionUnknown,
		}
		if c.Decision != "" {
			d.Controls[i].Decision = new(apigen.Decision(c.Decision))
		}
		if c.FeePercent != 0 {
			d.Controls[i].FeePercent = new(c.FeePercent)
		}
		if c.HoldSeconds != 0 {
			d.Controls[i].HoldSeconds = new(c.HoldSeconds)
		}
		if c.Branch != "" {
			d.Controls[i].Branch = new(c.Branch)
		}
	}
	d.NotEvaluated = make([]apigen.NotEvaluatedRule, len(o.NotEvaluated))
	for i, n := range o.NotEvaluated {
		d.NotEvaluated[i] = apigen.NotEvaluatedRule{Rule: n.Rule, Missing: n.Missing}
	}
	if o.EventID != nil {
		d.EventId = new(ids.Format(ids.PrefixEvent, *o.EventID))
	}
	return d
}
