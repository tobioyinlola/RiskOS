package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

// ListRuleChanges implements GET /api/v1/rule-changes.
func (h *Handlers) ListRuleChanges(ctx context.Context, req apigen.ListRuleChangesRequestObject) (apigen.ListRuleChangesResponseObject, error) {
	status := new(domain.ChangePending)
	if req.Params.Status != nil {
		if *req.Params.Status == "all" {
			status = nil
		} else {
			status = new(domain.ChangeStatus(*req.Params.Status))
		}
	}
	cs, err := h.svc.ListChanges(ctx, status)
	if err != nil {
		return nil, MapError(err)
	}
	data := make([]apigen.RuleChange, len(cs))
	for i, c := range cs {
		if data[i], err = ChangeToAPI(c); err != nil {
			return nil, err
		}
	}
	return apigen.ListRuleChanges200JSONResponse{Data: data}, nil
}

// GetRuleChange implements GET /api/v1/rule-changes/{change_id}.
func (h *Handlers) GetRuleChange(ctx context.Context, req apigen.GetRuleChangeRequestObject) (apigen.GetRuleChangeResponseObject, error) {
	id, err := parseChangeID(req.ChangeId)
	if err != nil {
		return nil, err
	}
	c, err := h.svc.GetChange(ctx, id)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := ChangeToAPI(c)
	return apigen.GetRuleChange200JSONResponse(out), err
}

// ApproveRuleChange implements POST /api/v1/rule-changes/{change_id}/approve.
func (h *Handlers) ApproveRuleChange(ctx context.Context, req apigen.ApproveRuleChangeRequestObject) (apigen.ApproveRuleChangeResponseObject, error) {
	id, err := parseChangeID(req.ChangeId)
	if err != nil {
		return nil, err
	}
	c, err := h.svc.ApproveChange(ctx, id)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := ChangeToAPI(c)
	return apigen.ApproveRuleChange200JSONResponse(out), err
}

// RejectRuleChange implements POST /api/v1/rule-changes/{change_id}/reject.
func (h *Handlers) RejectRuleChange(ctx context.Context, req apigen.RejectRuleChangeRequestObject) (apigen.RejectRuleChangeResponseObject, error) {
	id, err := parseChangeID(req.ChangeId)
	if err != nil {
		return nil, err
	}
	reason := ""
	if req.Body != nil && req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	c, err := h.svc.RejectChange(ctx, id, reason)
	if err != nil {
		return nil, MapError(err)
	}
	out, err := ChangeToAPI(c)
	return apigen.RejectRuleChange200JSONResponse(out), err
}

func parseChangeID(s string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixRuleChange, s)
	if err != nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

// ChangeToAPI renders a proposed rule change.
func ChangeToAPI(c domain.RuleChange) (apigen.RuleChange, error) {
	cond, err := conditionToAPI(c.Proposed.Condition)
	if err != nil {
		return apigen.RuleChange{}, err
	}
	v := c.Proposed
	return apigen.RuleChange{
		Id: ids.Format(ids.PrefixRuleChange, c.ID), RuleId: ids.Format(ids.PrefixRule, c.RuleID), Code: c.Code,
		Kind: apigen.RuleChangeKind(c.Kind), BaseVersion: c.BaseVersion, Status: apigen.RuleChangeStatus(c.Status),
		Proposed: apigen.RuleContent{
			Name: v.Name, Description: v.Description, Severity: apigen.Severity(v.Severity), Weight: v.Weight,
			Mode: apigen.RuleMode(v.Mode), Enabled: v.Enabled, Condition: cond, RestoredFrom: v.RestoredFrom,
		},
		ProposedBy: c.ProposedBy, ProposedAt: c.ProposedAt, DecidedBy: c.DecidedBy, DecidedAt: c.DecidedAt,
		Reason: c.Reason, AppliedVersion: c.AppliedVersion,
	}, nil
}
