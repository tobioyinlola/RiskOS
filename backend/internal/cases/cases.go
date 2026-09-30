// Package cases is the public interface of the cases module: investigation cases grouping
// alerts, with assignment, append-only notes and a resolution (docs/ALERTS_AND_CASES.md).
package cases

import (
	"context"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// CaseHandlers are the cases operations of the public API.
type CaseHandlers interface {
	ListCases(ctx context.Context, req apigen.ListCasesRequestObject) (apigen.ListCasesResponseObject, error)
	CreateCase(ctx context.Context, req apigen.CreateCaseRequestObject) (apigen.CreateCaseResponseObject, error)
	GetCase(ctx context.Context, req apigen.GetCaseRequestObject) (apigen.GetCaseResponseObject, error)
	UpdateCase(ctx context.Context, req apigen.UpdateCaseRequestObject) (apigen.UpdateCaseResponseObject, error)
	AddCaseNote(ctx context.Context, req apigen.AddCaseNoteRequestObject) (apigen.AddCaseNoteResponseObject, error)
}

// Module is the cases module.
type Module struct {
	svc *service.Service
}

// New returns the cases module.
func New(runner txn.Runner, linker alerts.Linker, rec audit.Recorder, clk clock.Clock) *Module {
	return &Module{svc: service.New(runner, linker, rec, clk)}
}

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() CaseHandlers { return handler.New(m.svc) }
