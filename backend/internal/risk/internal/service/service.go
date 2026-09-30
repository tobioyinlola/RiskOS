// Package service runs evaluations: engine, score, level, decision, persistence.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
)

// ErrNotFound is returned when an evaluation does not exist in the caller's environment.
var ErrNotFound = store.ErrNotFound

// Service evaluates events.
type Service struct {
	engine domain.Engine
	store  store.Store
	runner txn.Runner
	clock  clock.Clock
}

// New returns a Service using engine.
func New(engine domain.Engine, runner txn.Runner, clk clock.Clock) *Service {
	return &Service{engine: engine, runner: runner, clock: clk}
}

// Evaluate scores an event and stores the evaluation. It joins the caller's transaction when
// there is one (ingestion stores the event and its evaluation atomically) and opens its own
// otherwise. Authorization is the caller's responsibility: ingestion requires events:write,
// dry runs require risk:evaluate.
func (s *Service) Evaluate(ctx context.Context, in domain.Input) (domain.Result, error) {
	start := s.clock.Now()
	signals, degraded := s.engine.Signals(ctx, in.Features)
	score := domain.Score(signals)
	level := domain.LevelForScore(score)
	id := ids.NewV7()
	res := domain.Result{
		ID:              id,
		CreatedAt:       ids.Time(id),
		EventID:         in.EventID,
		EngineVersion:   s.engine.Version(),
		Score:           score,
		Level:           level,
		Decision:        domain.DefaultDecision(level),
		Signals:         signals,
		DegradedSignals: degraded,
		RequestID:       in.RequestID,
	}
	duration := s.clock.Now().Sub(start)
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		return s.store.Insert(ctx, res, in, duration)
	})
	if err != nil {
		return domain.Result{}, err
	}
	if res.Signals == nil {
		res.Signals = []domain.Signal{}
	}
	if res.DegradedSignals == nil {
		res.DegradedSignals = []string{}
	}
	return res, nil
}

// Get returns a stored evaluation.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Result, error) {
	var res domain.Result
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.store.Get(ctx, id, ids.Time(id))
		return err
	})
	if errors.Is(err, store.ErrNotFound) {
		return domain.Result{}, ErrNotFound
	}
	return res, err
}
