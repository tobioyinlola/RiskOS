package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// configError maps configuration errors, then the library's.
func configError(err error) error {
	switch {
	case errors.Is(err, service.ErrConflict):
		msg := strings.TrimPrefix(err.Error(), service.ErrConflict.Error()+": ")
		return httpx.Conflict(strings.ToUpper(msg[:1])+msg[1:]+".", err)
	case errors.Is(err, service.ErrOwnChange):
		return httpx.NewError(http.StatusForbidden, httpx.CodeForbidden, "A change needs another person's approval.", err)
	default:
		return mapError(err)
	}
}

func settingsToAPI(s map[string]float64) apigen.PackRuleSettings {
	out := apigen.PackRuleSettings{}
	for k, v := range s {
		out[k] = v
	}
	return out
}

func changeToAPI(c store.RuleChange) apigen.PackRuleChange {
	return apigen.PackRuleChange{
		Id: ids.Format(ids.PrefixPackChange, c.ID), Rule: c.RuleID, BaseVersion: c.BaseVersion,
		Mode: apigen.PackRuleMode(c.Mode), Settings: settingsToAPI(c.Settings), Reason: c.Reason,
		RestoredFrom: c.RestoredFrom, Status: apigen.PackRuleChangeStatus(c.Status), ProposedBy: c.ProposedBy,
		ProposedAt: c.ProposedAt, DecidedBy: c.DecidedBy, DecidedAt: c.DecidedAt, DecisionNote: c.DecisionNote,
		AppliedVersion: c.AppliedVersion,
	}
}

func configToAPI(c service.RuleConfig) apigen.PackRuleConfig {
	out := apigen.PackRuleConfig{
		Rule: c.RuleID, Version: c.Version, Mode: apigen.PackRuleMode(c.Mode), Settings: settingsToAPI(c.Settings),
		Reason: c.Reason, UpdatedAt: c.UpdatedAt,
	}
	if c.UpdatedBy != "" {
		out.UpdatedBy = &c.UpdatedBy
	}
	if c.Pending != nil {
		p := changeToAPI(*c.Pending)
		out.Pending = &p
	}
	return out
}

// GetPackRuleConfig implements GET /api/v1/compliance/rules/{rule_id}/config.
func (h *Handlers) GetPackRuleConfig(ctx context.Context, req apigen.GetPackRuleConfigRequestObject) (apigen.GetPackRuleConfigResponseObject, error) {
	c, err := h.svc.Config(ctx, req.RuleId)
	if err != nil {
		return nil, configError(err)
	}
	return apigen.GetPackRuleConfig200JSONResponse(configToAPI(c)), nil
}

// PutPackRuleConfig implements PUT /api/v1/compliance/rules/{rule_id}/config.
func (h *Handlers) PutPackRuleConfig(ctx context.Context, req apigen.PutPackRuleConfigRequestObject) (apigen.PutPackRuleConfigResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	res, err := h.svc.Configure(ctx, req.RuleId, service.ConfigInput{
		Mode: string(req.Body.Mode), Settings: req.Body.Settings, Reason: req.Body.Reason,
	}, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	if res.Proposed != nil {
		return apigen.PutPackRuleConfig202JSONResponse(changeToAPI(*res.Proposed)), nil
	}
	return apigen.PutPackRuleConfig200JSONResponse(configToAPI(*res.Applied)), nil
}

// ListPackRuleVersions implements GET /api/v1/compliance/rules/{rule_id}/history.
func (h *Handlers) ListPackRuleVersions(ctx context.Context, req apigen.ListPackRuleVersionsRequestObject) (apigen.ListPackRuleVersionsResponseObject, error) {
	vs, err := h.svc.History(ctx, req.RuleId)
	if err != nil {
		return nil, configError(err)
	}
	data := make([]apigen.PackRuleVersion, len(vs))
	for i, v := range vs {
		data[i] = apigen.PackRuleVersion{
			Version: v.Version, Mode: apigen.PackRuleMode(v.Mode), Settings: settingsToAPI(v.Settings), Reason: v.Reason,
			CreatedAt: v.CreatedAt, CreatedBy: v.CreatedBy, RestoredFrom: v.RestoredFrom,
		}
		if v.ChangeID != nil {
			data[i].ChangeId = new(ids.Format(ids.PrefixPackChange, *v.ChangeID))
		}
	}
	return apigen.ListPackRuleVersions200JSONResponse{Data: data}, nil
}

// RollbackPackRule implements POST /api/v1/compliance/rules/{rule_id}/rollback.
func (h *Handlers) RollbackPackRule(ctx context.Context, req apigen.RollbackPackRuleRequestObject) (apigen.RollbackPackRuleResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	res, err := h.svc.Rollback(ctx, req.RuleId, req.Body.Version, req.Body.Reason, h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	if res.Proposed != nil {
		return apigen.RollbackPackRule202JSONResponse(changeToAPI(*res.Proposed)), nil
	}
	return apigen.RollbackPackRule200JSONResponse(configToAPI(*res.Applied)), nil
}

// ListPackRuleChanges implements GET /api/v1/compliance/changes.
func (h *Handlers) ListPackRuleChanges(ctx context.Context, req apigen.ListPackRuleChangesRequestObject) (apigen.ListPackRuleChangesResponseObject, error) {
	status := ""
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	cs, err := h.svc.Changes(ctx, status)
	if err != nil {
		return nil, configError(err)
	}
	data := make([]apigen.PackRuleChange, len(cs))
	for i, c := range cs {
		data[i] = changeToAPI(c)
	}
	return apigen.ListPackRuleChanges200JSONResponse{Data: data}, nil
}

func note(n *apigen.DecisionNote) string {
	if n == nil || n.Note == nil {
		return ""
	}
	return *n.Note
}

// ApprovePackRuleChange implements POST /api/v1/compliance/changes/{change_id}/approve.
func (h *Handlers) ApprovePackRuleChange(ctx context.Context, req apigen.ApprovePackRuleChangeRequestObject) (apigen.ApprovePackRuleChangeResponseObject, error) {
	id, err := ids.Parse(ids.PrefixPackChange, req.ChangeId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	c, err := h.svc.Approve(ctx, id, note(req.Body), h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.ApprovePackRuleChange200JSONResponse(changeToAPI(c)), nil
}

// RejectPackRuleChange implements POST /api/v1/compliance/changes/{change_id}/reject.
func (h *Handlers) RejectPackRuleChange(ctx context.Context, req apigen.RejectPackRuleChangeRequestObject) (apigen.RejectPackRuleChangeResponseObject, error) {
	id, err := ids.Parse(ids.PrefixPackChange, req.ChangeId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	c, err := h.svc.Reject(ctx, id, note(req.Body), h.clock.Now())
	if err != nil {
		return nil, configError(err)
	}
	return apigen.RejectPackRuleChange200JSONResponse(changeToAPI(c)), nil
}
