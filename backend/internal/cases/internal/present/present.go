// Package present converts cases to the API contract, for responses and webhook payloads.
package present

import (
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// ToAPI converts a case to the contract.
func ToAPI(c domain.Case) apigen.Case {
	out := apigen.Case{
		Id: ids.Format(ids.PrefixCase, c.ID), Title: c.Title, Description: c.Description,
		Status: apigen.CaseStatus(c.Status), Priority: apigen.Severity(c.Priority), Assignee: c.Assignee,
		ResolutionNote: c.ResolutionNote, ResolvedAt: c.ResolvedAt, ResolvedBy: c.ResolvedBy,
		CreatedAt: c.CreatedAt, CreatedBy: c.CreatedBy, UpdatedAt: c.UpdatedAt,
	}
	if c.Resolution != nil {
		out.Resolution = new(apigen.CaseResolution(*c.Resolution))
	}
	return out
}
