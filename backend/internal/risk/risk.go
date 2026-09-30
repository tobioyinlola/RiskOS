package risk

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/engine"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/service"
)

// Public types.
type (
	// Features is the engine's input.
	Features = domain.Features
	// Input describes an evaluation request.
	Input = domain.Input
	// Result is a stored evaluation.
	Result = domain.Result
	// Signal is one explained reason contributing to the score.
	Signal = domain.Signal
	// Level is the risk level.
	Level = domain.Level
	// Decision is the recommended action.
	Decision = domain.Decision
)

// ErrNotFound is returned when an evaluation does not exist in the caller's environment.
var ErrNotFound = service.ErrNotFound

// Evaluator evaluates events and reads stored evaluations. Evaluate joins the caller's
// transaction when there is one, so an event and its evaluation commit together.
type Evaluator interface {
	Evaluate(ctx context.Context, in Input) (Result, error)
	Get(ctx context.Context, id uuid.UUID) (Result, error)
}

// Module is the risk module.
type Module struct {
	svc *service.Service
}

// New returns the risk module. Build step 3 runs the stub engine (no signals; every event is
// LOW/ALLOW, labelled with its engine version); build step 4 replaces it.
func New(runner txn.Runner, clk clock.Clock) *Module {
	return &Module{svc: service.New(engine.Stub{}, runner, clk)}
}

// Evaluator returns the module's evaluator.
func (m *Module) Evaluator() Evaluator { return m.svc }
