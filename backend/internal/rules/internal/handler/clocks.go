package handler

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

func holidayToAPI(h store.Holiday) apigen.Holiday {
	return apigen.Holiday{
		Jurisdiction: h.Jurisdiction, Date: openapi_types.Date{Time: h.Day}, Name: h.Name,
		CreatedAt: h.CreatedAt, CreatedBy: h.CreatedBy,
	}
}

// ListComplianceHolidays implements GET /api/v1/compliance/jurisdictions/{jurisdiction}/holidays.
func (h *Handlers) ListComplianceHolidays(ctx context.Context, req apigen.ListComplianceHolidaysRequestObject) (apigen.ListComplianceHolidaysResponseObject, error) {
	list, err := h.svc.Holidays(ctx, req.Jurisdiction)
	if err != nil {
		return nil, configError(err)
	}
	data := make([]apigen.Holiday, len(list))
	for i, d := range list {
		data[i] = holidayToAPI(d)
	}
	return apigen.ListComplianceHolidays200JSONResponse{Data: data}, nil
}

// AddComplianceHoliday implements POST /api/v1/compliance/jurisdictions/{jurisdiction}/holidays.
func (h *Handlers) AddComplianceHoliday(ctx context.Context, req apigen.AddComplianceHolidayRequestObject) (apigen.AddComplianceHolidayResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	d, err := h.svc.AddHoliday(ctx, req.Jurisdiction, req.Body.Date.Time, req.Body.Name, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.AddComplianceHoliday201JSONResponse(holidayToAPI(d)), nil
}

// DeleteComplianceHoliday implements DELETE /api/v1/compliance/jurisdictions/{jurisdiction}/holidays/{date}.
func (h *Handlers) DeleteComplianceHoliday(ctx context.Context, req apigen.DeleteComplianceHolidayRequestObject) (apigen.DeleteComplianceHolidayResponseObject, error) {
	if err := h.svc.DeleteHoliday(ctx, req.Jurisdiction, req.Date.Time); err != nil {
		return nil, configError(err)
	}
	return apigen.DeleteComplianceHoliday204Response{}, nil
}

func loadToAPI(l store.Load) apigen.RuleLoad {
	return apigen.RuleLoad{Jurisdiction: l.Jurisdiction, Rule: l.RuleID, Running: l.Running, Overdue: l.Overdue, DueSoon: l.DueSoon}
}

// GetComplianceClockLoad implements GET /api/v1/compliance/clock-load.
func (h *Handlers) GetComplianceClockLoad(ctx context.Context, _ apigen.GetComplianceClockLoadRequestObject) (apigen.GetComplianceClockLoadResponseObject, error) {
	l, err := h.svc.ClockLoad(ctx, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	out := apigen.GetComplianceClockLoad200JSONResponse{
		Data: make([]apigen.ClockLoadRow, len(l.Clocks)), Unclocked: make([]apigen.RuleLoad, len(l.Unclocked)),
		SensitiveHidden: l.SensitiveHidden,
	}
	for i, c := range l.Clocks {
		out.Data[i] = apigen.ClockLoadRow{
			Jurisdiction: c.Jurisdiction, Clock: clockToAPI(c.Clock), Running: c.Running, Overdue: c.Overdue, DueSoon: c.DueSoon,
		}
	}
	for i, u := range l.Unclocked {
		out.Unclocked[i] = loadToAPI(u)
	}
	return out, nil
}

// ListComplianceCrossBorder implements GET /api/v1/compliance/cross-border.
func (h *Handlers) ListComplianceCrossBorder(ctx context.Context, _ apigen.ListComplianceCrossBorderRequestObject) (apigen.ListComplianceCrossBorderResponseObject, error) {
	events, err := h.svc.CrossBorder(ctx)
	if err != nil {
		return nil, configError(err)
	}
	now := h.clock.Now()
	data := make([]apigen.CrossBorderEvent, len(events))
	for i, e := range events {
		data[i] = apigen.CrossBorderEvent{
			EventId: ids.Format(ids.PrefixEvent, e.EventID), BookedIn: e.BookedIn, Countries: list(e.Countries),
			Tasks: make([]apigen.ComplianceTask, len(e.Tasks)),
		}
		for j, t := range e.Tasks {
			data[i].Tasks[j] = taskToAPI(t, now)
		}
	}
	return apigen.ListComplianceCrossBorder200JSONResponse{Data: data}, nil
}

func days(d time.Duration) int { return int(d / (24 * time.Hour)) }

func retentionRuleToAPI(r service.RetentionRule) apigen.RetentionRule {
	return apigen.RetentionRule{Jurisdiction: r.Jurisdiction, Rule: r.Rule, Name: r.Name, Setting: r.Setting, Days: days(r.Period)}
}

func periodToAPI(p service.RetentionPeriod) apigen.RetentionPeriod {
	out := apigen.RetentionPeriod{
		RecordType: apigen.RecordType(p.RecordType), Days: days(p.Period), Considered: make([]apigen.RetentionRule, len(p.Considered)),
	}
	if p.Winner != nil {
		out.Rule = new(retentionRuleToAPI(*p.Winner))
	}
	for i, r := range p.Considered {
		out.Considered[i] = retentionRuleToAPI(r)
	}
	return out
}

// GetComplianceRetention implements GET /api/v1/compliance/retention.
func (h *Handlers) GetComplianceRetention(ctx context.Context, _ apigen.GetComplianceRetentionRequestObject) (apigen.GetComplianceRetentionResponseObject, error) {
	list, err := h.svc.Retention(ctx)
	if err != nil {
		return nil, configError(err)
	}
	data := make([]apigen.RetentionPeriod, len(list))
	for i, p := range list {
		data[i] = periodToAPI(p)
	}
	return apigen.GetComplianceRetention200JSONResponse{Data: data}, nil
}

// CheckComplianceDeletion implements POST /api/v1/compliance/retention/check.
func (h *Handlers) CheckComplianceDeletion(ctx context.Context, req apigen.CheckComplianceDeletionRequestObject) (apigen.CheckComplianceDeletionResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	v, err := h.svc.CheckDeletion(ctx, service.DeletionRequest{
		RecordType: string(req.Body.RecordType), RecordRef: req.Body.RecordRef, Since: req.Body.Since,
		Reason: string(req.Body.Reason),
	}, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.CheckComplianceDeletion200JSONResponse{Allowed: v.Allowed, RetainUntil: v.RetainUntil, Period: periodToAPI(v.Period)}, nil
}
