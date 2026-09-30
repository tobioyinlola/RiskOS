// Package present converts alerts to the API contract, for responses and webhook payloads.
package present

import (
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// ToAPI converts an alert to the contract.
func ToAPI(a domain.Alert) apigen.Alert {
	out := apigen.Alert{
		Id: ids.Format(ids.PrefixAlert, a.ID), Kind: apigen.AlertKind(a.Kind), Title: a.Title,
		Severity: apigen.RiskLevel(a.Severity), Status: apigen.AlertStatus(a.Status), Assignee: a.Assignee,
		MaxScore: a.MaxScore, EvaluationCount: a.EvaluationCount, FirstSeenAt: a.FirstSeenAt,
		LastSeenAt: a.LastSeenAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
	out.Subject.Type, out.Subject.Ref = apigen.AlertSubjectType(a.SubjectType), a.SubjectRef
	if a.CaseID != nil {
		out.CaseId = new(ids.Format(ids.PrefixCase, *a.CaseID))
	}
	return out
}

// ActivityToAPI converts a timeline to the contract.
func ActivityToAPI(entries []domain.Activity) []apigen.ActivityEntry {
	out := make([]apigen.ActivityEntry, len(entries))
	for i, e := range entries {
		out[i] = apigen.ActivityEntry{
			Id: ids.Format(ids.PrefixActivity, e.ID), Action: e.Action, Actor: e.Actor, Detail: e.Detail, CreatedAt: e.CreatedAt,
		}
		if out[i].Detail == nil {
			out[i].Detail = map[string]any{}
		}
	}
	return out
}
