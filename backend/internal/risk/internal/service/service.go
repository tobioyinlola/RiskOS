// Package service runs evaluations (features, signals, rules, score, policy, persistence) and
// manages rules and risk settings.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/engine"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
)

// ErrNotFound is returned when an evaluation does not exist in the caller's environment.
var ErrNotFound = store.ErrNotFound

// FeatureStore reads and updates behavioral features.
type FeatureStore interface {
	engine.FeatureSource
	RecordAmount(ctx context.Context, e features.Event) error
}

// Service evaluates events and manages rules and settings.
type Service struct {
	engine   engine.Engine
	features FeatureStore
	store    store.Store
	runner   txn.Runner
	audit    audit.Recorder
	clock    clock.Clock
	logger   *slog.Logger
	config   *configCache
}

// New returns a Service.
func New(fs FeatureStore, runner txn.Runner, rec audit.Recorder, clk clock.Clock, logger *slog.Logger) *Service {
	return &Service{
		engine: engine.New(fs), features: fs, runner: runner, audit: rec, clock: clk, logger: logger,
		config: newConfigCache(configTTL),
	}
}

// storedFeatures is what an evaluation records next to its result so it can be reproduced:
// every fact the signals and rules were computed from and, for dry runs, the normalized event
// (an ingested event is stored in events and linked by event_id).
type storedFeatures struct {
	Event json.RawMessage `json:"event,omitempty"`
	Facts map[string]any  `json:"facts"`
}

// Evaluate scores an event and stores the evaluation, joining the caller's transaction when
// there is one. Dry runs (in.EventID == nil) read features without updating them. Authorization
// is the caller's: ingestion requires events:write, dry runs risk:evaluate.
func (s *Service) Evaluate(ctx context.Context, in domain.Input) (domain.Result, error) {
	start := s.clock.Now()
	ev, err := features.ParseEvent(in.Features.Normalized)
	if err != nil {
		return domain.Result{}, err
	}
	write := in.EventID != nil

	var res domain.Result
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cfg, err := s.config.get(ctx, s.load)
		if err != nil {
			return err
		}
		out := s.engine.Evaluate(ctx, ev, write, cfg.settings, cfg.rules)
		score := domain.Score(out.Signals)
		level := domain.LevelForScore(score, cfg.settings.Thresholds)
		policy := domain.Decide(domain.PolicyInput{
			EventType: ev.Type, Level: level, Signals: out.Signals, Degraded: out.Degraded,
			HighValue: highValue(ev, cfg.settings), Settings: cfg.settings,
		})
		id := ids.NewV7()
		res = domain.Result{
			ID: id, CreatedAt: ids.Time(id), EventID: in.EventID, EngineVersion: engine.Version,
			Score: score, Level: level, Decision: policy.Decision,
			Signals: nonNil(out.Signals), DegradedSignals: nonNilStrings(out.Degraded), RequestID: in.RequestID,
			PolicyReasons: nonNilStrings(policy.Reasons), ShadowSignals: nonNil(out.Shadow),
			RuleVersions: nonNilStrings(out.RuleVersions),
			RaiseAlert:   write && cfg.settings.RaisesAlert(policy.Decision),
		}
		if len(out.Degraded) > 0 {
			// Degradation is a security event (ARCHITECTURE.md §16.8): it may be induced.
			s.logger.WarnContext(ctx, "risk evaluation degraded",
				slog.Any("degraded_signals", out.Degraded), slog.String("decision", string(policy.Decision)))
		}
		if write && ev.IsDebit() && level != domain.LevelHigh && level != domain.LevelCritical {
			// High-risk events never enter the baseline, so it cannot be poisoned (§16.8).
			if err := s.features.RecordAmount(ctx, ev); err != nil {
				s.logger.WarnContext(ctx, "amount baseline not updated", slog.Any("error", err))
			}
		}
		record := storedFeatures{Facts: out.Facts}
		if !write {
			// A dry run's event is stored nowhere else; an ingested event is in events.
			record.Event = in.Features.Normalized
		}
		stored, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("encode features: %w", err)
		}
		return s.store.Insert(ctx, res, in.EventKey, stored, s.clock.Now().Sub(start))
	})
	if err != nil {
		return domain.Result{}, err
	}
	return res, nil
}

func highValue(e features.Event, s domain.Settings) bool {
	return e.Transaction != nil && e.Transaction.Amount >= s.HighValueThreshold(e.Transaction.Currency)
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

// load reads the environment's settings and current rules and compiles the rules.
func (s *Service) load(ctx context.Context) (config, error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return config{}, err
	}
	all, err := s.store.ListRules(ctx)
	if err != nil {
		return config{}, err
	}
	compiled := make([]engine.CompiledRule, 0, len(all))
	for _, r := range all {
		c, err := compileCondition(r.Current.Condition)
		if err != nil {
			// Conditions are validated when saved; a stored rule that no longer compiles is
			// skipped loudly rather than failing every evaluation.
			s.logger.ErrorContext(ctx, "stored rule does not compile; skipped", slog.String("rule", r.VersionRef()), slog.Any("error", err))
			continue
		}
		compiled = append(compiled, engine.CompiledRule{Rule: r, Condition: c})
	}
	return config{settings: settings, rules: compiled}, nil
}

// scopeKey identifies an environment in caches.
func scopeKey(ctx context.Context) (string, error) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return "", err
	}
	return scope.TenantID.String() + "/" + scope.EnvironmentID.String(), nil
}

func nonNil(s []domain.Signal) []domain.Signal {
	if s == nil {
		return []domain.Signal{}
	}
	return s
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// TopSignals counts how often each signal fired in events since since. Requires events:read.
func (s *Service) TopSignals(ctx context.Context, since time.Time) ([]domain.SignalCount, error) {
	if err := authz.Require(ctx, authz.EventsRead); err != nil {
		return nil, err
	}
	var out []domain.SignalCount
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.TopSignals(ctx, since)
		return err
	})
	return out, err
}
