package risk

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
)

// Public types.
type (
	// Features is the engine's input.
	Features = domain.Features
	// Input describes an evaluation request.
	Input = domain.Input
	// Result is a stored evaluation.
	Result = domain.Result
	// SignalCount is how often a signal fired over a window.
	SignalCount = domain.SignalCount
	// Signal is one explained reason contributing to the score.
	Signal = domain.Signal
	// Level is the risk level.
	Level = domain.Level
	// Decision is the recommended action.
	Decision = domain.Decision
	// Control is a rule pack control that applied to an event.
	Control = domain.Control
	// NotEvaluated is a rule pack rule an event triggered that could not be evaluated.
	NotEvaluated = domain.NotEvaluated
	// JurisdictionResolution is which country's pack judged an event.
	JurisdictionResolution = domain.JurisdictionResolution
	// TestResult is the outcome of testing a rule against an event.
	TestResult = service.TestResult
)

// ErrNotFound is returned when an evaluation does not exist in the caller's environment.
var ErrNotFound = service.ErrNotFound

// Evaluator evaluates events and reads stored evaluations. Evaluate joins the caller's
// transaction when there is one, so an event and its evaluation commit together.
type Evaluator interface {
	Evaluate(ctx context.Context, in Input) (Result, error)
	Get(ctx context.Context, id uuid.UUID) (Result, error)
	// TopSignals counts how often each signal fired in events since a time (analytics).
	TopSignals(ctx context.Context, since time.Time) ([]SignalCount, error)
}

// RuleTester evaluates a rule against a normalized event without side effects. The events
// module serves the test endpoint because it owns event validation and normalization.
type RuleTester interface {
	TestRule(ctx context.Context, id uuid.UUID, normalized []byte, draft json.RawMessage) (TestResult, error)
}

// RuleHandlers are the risk module's operations of the public API.
type RuleHandlers interface {
	GetRiskCatalog(ctx context.Context, req apigen.GetRiskCatalogRequestObject) (apigen.GetRiskCatalogResponseObject, error)
	GetRiskPolicy(ctx context.Context, req apigen.GetRiskPolicyRequestObject) (apigen.GetRiskPolicyResponseObject, error)
	GetRiskSettings(ctx context.Context, req apigen.GetRiskSettingsRequestObject) (apigen.GetRiskSettingsResponseObject, error)
	PutRiskSettings(ctx context.Context, req apigen.PutRiskSettingsRequestObject) (apigen.PutRiskSettingsResponseObject, error)
	ListRules(ctx context.Context, req apigen.ListRulesRequestObject) (apigen.ListRulesResponseObject, error)
	GetRuleStats(ctx context.Context, req apigen.GetRuleStatsRequestObject) (apigen.GetRuleStatsResponseObject, error)
	TestRuleOnRecentEvents(ctx context.Context, req apigen.TestRuleOnRecentEventsRequestObject) (apigen.TestRuleOnRecentEventsResponseObject, error)
	CreateRule(ctx context.Context, req apigen.CreateRuleRequestObject) (apigen.CreateRuleResponseObject, error)
	GetRule(ctx context.Context, req apigen.GetRuleRequestObject) (apigen.GetRuleResponseObject, error)
	UpdateRule(ctx context.Context, req apigen.UpdateRuleRequestObject) (apigen.UpdateRuleResponseObject, error)
	ListRuleVersions(ctx context.Context, req apigen.ListRuleVersionsRequestObject) (apigen.ListRuleVersionsResponseObject, error)
	RollbackRule(ctx context.Context, req apigen.RollbackRuleRequestObject) (apigen.RollbackRuleResponseObject, error)
	ListRuleChanges(ctx context.Context, req apigen.ListRuleChangesRequestObject) (apigen.ListRuleChangesResponseObject, error)
	GetRuleChange(ctx context.Context, req apigen.GetRuleChangeRequestObject) (apigen.GetRuleChangeResponseObject, error)
	ApproveRuleChange(ctx context.Context, req apigen.ApproveRuleChangeRequestObject) (apigen.ApproveRuleChangeResponseObject, error)
	RejectRuleChange(ctx context.Context, req apigen.RejectRuleChangeRequestObject) (apigen.RejectRuleChangeResponseObject, error)
	ListDenyEntries(ctx context.Context, req apigen.ListDenyEntriesRequestObject) (apigen.ListDenyEntriesResponseObject, error)
	CreateDenyEntry(ctx context.Context, req apigen.CreateDenyEntryRequestObject) (apigen.CreateDenyEntryResponseObject, error)
	DeleteDenyEntry(ctx context.Context, req apigen.DeleteDenyEntryRequestObject) (apigen.DeleteDenyEntryResponseObject, error)
	BacktestPackRule(ctx context.Context, req apigen.BacktestPackRuleRequestObject) (apigen.BacktestPackRuleResponseObject, error)
	ListRestrictedSignals(ctx context.Context, req apigen.ListRestrictedSignalsRequestObject) (apigen.ListRestrictedSignalsResponseObject, error)
}

// Config configures the risk module.
type Config struct {
	// FeatureHashKey keys the HMAC that pseudonymizes identifiers before they reach Redis.
	FeatureHashKey []byte
	// FeatureTimeout bounds the feature store round trip on the decision path; beyond it the
	// affected signals are reported as degraded.
	FeatureTimeout time.Duration
	// StateDB enables feature-store reset detection (DISASTER_RECOVERY.md): the application
	// pool, used for the platform-wide state row only. Nil disables it.
	StateDB StateDB
	// HistoryWarmup is how long history signals are held back after a reset.
	HistoryWarmup time.Duration
	// Packs gives each tenant's rule pack (the rules module); nil evaluates none.
	Packs PackSource
}

// PackSource gives the caller's tenant's rule pack compiled for evaluation.
type PackSource = service.PackSource

// StateDB runs the reset detector's statements; *pgxpool.Pool satisfies it.
type StateDB = store.StateDB

// Redis holds the behavioral feature store. *redis.Client connected to the shared instance
// satisfies it.
type Redis = store.Redis

// Module is the risk module.
type Module struct {
	svc   *service.Service
	state *store.StateWatcher
}

// New returns the risk module.
func New(rdb Redis, runner txn.Runner, rec audit.Recorder, clk clock.Clock, logger *slog.Logger, cfg Config) *Module {
	fs := store.NewFeatureStore(rdb, cfg.FeatureHashKey, cfg.FeatureTimeout)
	m := &Module{}
	if cfg.StateDB != nil {
		m.state = store.NewStateWatcher(rdb, cfg.StateDB, cfg.FeatureHashKey, cfg.HistoryWarmup, clk, logger)
		fs.WithState(m.state)
	}
	m.svc = service.New(fs, runner, rec, clk, logger, cfg.Packs)
	return m
}

// stateCheckInterval is how often the reset detector compares epochs.
const stateCheckInterval = 30 * time.Second

// WatchFeatureState runs reset detection in the background until ctx ends. A no-op without
// Config.StateDB.
func (m *Module) WatchFeatureState(ctx context.Context) {
	if m.state != nil {
		go m.state.Run(ctx, stateCheckInterval)
	}
}

// CheckFeatureState runs reset detection once (tests and tooling).
func (m *Module) CheckFeatureState(ctx context.Context) error {
	if m.state == nil {
		return nil
	}
	return m.state.Check(ctx)
}

// Evaluator returns the module's evaluator.
func (m *Module) Evaluator() Evaluator { return m.svc }

// RuleTester returns the module's rule tester.
func (m *Module) RuleTester() RuleTester { return m.svc }

// MapError translates the risk module's errors to the API error contract.
func MapError(err error) error { return handler.MapError(err) }

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() RuleHandlers { return handler.New(m.svc) }
