package events

import (
	"context"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

// IngestHandlers are the event operations of the public API, including the dry-run evaluation,
// which shares the event contract and checks.
type IngestHandlers interface {
	IngestEvent(ctx context.Context, req apigen.IngestEventRequestObject) (apigen.IngestEventResponseObject, error)
	EvaluateRisk(ctx context.Context, req apigen.EvaluateRiskRequestObject) (apigen.EvaluateRiskResponseObject, error)
	ListEvents(ctx context.Context, req apigen.ListEventsRequestObject) (apigen.ListEventsResponseObject, error)
	GetEvent(ctx context.Context, req apigen.GetEventRequestObject) (apigen.GetEventResponseObject, error)
	TestRule(ctx context.Context, req apigen.TestRuleRequestObject) (apigen.TestRuleResponseObject, error)
}

// Module is the events module.
type Module struct {
	svc *service.Service
}

// New returns the events module.
// enq receives the alert jobs of decisions; it must join the caller's transaction.
func New(runner txn.Runner, evaluator risk.Evaluator, tester risk.RuleTester, enq jobs.Enqueuer, clk clock.Clock) *Module {
	return &Module{svc: service.New(runner, evaluator, tester, enq, clk)}
}

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() IngestHandlers { return handler.New(m.svc) }
