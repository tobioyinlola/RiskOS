package handler

import (
	"context"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strEnum[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

// listFilter reads the listing filters of GET /api/v1/events.
func listFilter(p apigen.ListEventsParams) (domain.Filter, error) {
	f := domain.Filter{
		Type: strEnum(p.Type), Decision: strEnum(p.Decision), Level: strEnum(p.RiskLevel),
		User: str(p.User), Account: str(p.Account), Device: str(p.Device),
	}
	if p.After != nil {
		id, err := ids.Parse(ids.PrefixEvent, *p.After)
		if err != nil {
			return domain.Filter{}, httpx.InvalidRequest("after must be an event ID (evt_...).", err)
		}
		f.After = &id
	}
	return f, nil
}

func summaryToAPI(s domain.Summary) apigen.EventSummary {
	out := apigen.EventSummary{
		Id: ids.Format(ids.PrefixEvent, s.ID), EventKey: s.Key, Type: apigen.EventType(s.Type),
		OccurredAt: s.OccurredAt, ReceivedAt: s.ReceivedAt, RiskScore: s.Score,
		RiskLevel: apigen.RiskLevel(s.Level), Decision: apigen.Decision(s.Decision),
		Actor:   &apigen.EventActorRefs{UserId: s.User, AccountId: s.Account, DeviceId: s.Device},
		Country: s.Country,
	}
	if t := s.Txn; t != nil {
		out.Transaction = &apigen.TransactionSummary{
			Amount: t.Amount, Currency: t.Currency, Direction: t.Direction,
			BeneficiaryId: nonEmpty(t.Beneficiary), Channel: nonEmpty(t.Channel),
		}
	}
	return out
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func linkedToAPI(ls []domain.Linked) []apigen.LinkedEntity {
	out := make([]apigen.LinkedEntity, len(ls))
	for i, l := range ls {
		out[i] = apigen.LinkedEntity{Ref: l.Ref, Events: l.Events, LastSeen: l.LastSeen}
	}
	return out
}

// GetEntityProfile implements GET /api/v1/entities/{kind}/{ref}.
func (h *Handlers) GetEntityProfile(ctx context.Context, req apigen.GetEntityProfileRequestObject) (apigen.GetEntityProfileResponseObject, error) {
	p, err := h.svc.Profile(ctx, domain.EntityKind(req.Kind), req.Ref)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.GetEntityProfile200JSONResponse{
		Kind: apigen.EntityProfileKind(p.Kind), Ref: p.Ref, EventCount: p.Events,
		FirstSeen: p.FirstSeen, LastSeen: p.LastSeen, MaxScore: p.MaxScore, MeanScore: p.MeanScore,
		Decisions: apigen.DecisionCounts{Allow: p.Allowed, StepUp: p.SteppedUp, Review: p.Reviewed, Block: p.Blocked},
		Linked: apigen.EntityLinks{
			Users: linkedToAPI(p.Users), Accounts: linkedToAPI(p.Accounts), Devices: linkedToAPI(p.Devices),
			Countries: linkedToAPI(p.Countries), Ips: linkedToAPI(p.IPs),
		},
	}, nil
}

// GetAnalytics implements GET /api/v1/analytics.
func (h *Handlers) GetAnalytics(ctx context.Context, req apigen.GetAnalyticsRequestObject) (apigen.GetAnalyticsResponseObject, error) {
	name := "24h"
	if req.Params.Window != nil {
		name = string(*req.Params.Window)
	}
	w, ok := domain.AnalyticsWindows()[name]
	if !ok {
		return nil, httpx.InvalidRequest("window must be 24h, 7d or 30d.", nil)
	}
	a, err := h.svc.Analytics(ctx, w)
	if err != nil {
		return nil, mapError(err)
	}
	out := apigen.GetAnalytics200JSONResponse{
		Window: apigen.AnalyticsWindow(w.Name), Bucket: apigen.AnalyticsBucket(w.BucketName), Since: a.Since,
		Series: make([]apigen.AnalyticsPoint, len(a.Series)), TopUsers: make([]apigen.TopUser, len(a.TopUsers)),
		TopSignals: make([]apigen.SignalCount, len(a.TopSignals)),
	}
	for i, p := range a.Series {
		out.Series[i] = apigen.AnalyticsPoint{
			At: p.At, Events: p.Events, MeanScore: p.MeanScore,
			Decisions: apigen.DecisionCounts{Allow: p.Allowed, StepUp: p.SteppedUp, Review: p.Reviewed, Block: p.Blocked},
			Bands:     apigen.BandCounts{Minimal: p.Minimal, Low: p.Low, Elevated: p.Elevated, High: p.High, Critical: p.Critical},
		}
	}
	for i, u := range a.TopUsers {
		out.TopUsers[i] = apigen.TopUser{UserId: u.User, MaxScore: u.MaxScore, Events: u.Events, Flagged: u.Flagged, LastSeen: u.LastSeen}
	}
	for i, s := range a.TopSignals {
		out.TopSignals[i] = apigen.SignalCount{Code: s.Code, Label: s.Label, Hits: s.Hits}
	}
	return out, nil
}
