package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

// IngestHandlers are the event operations of the public API, including the dry-run evaluation,
// which shares the event contract and checks.
type IngestHandlers interface {
	IngestEvent(ctx context.Context, req apigen.IngestEventRequestObject) (apigen.IngestEventResponseObject, error)
	EvaluateRisk(ctx context.Context, req apigen.EvaluateRiskRequestObject) (apigen.EvaluateRiskResponseObject, error)
	ListEvents(ctx context.Context, req apigen.ListEventsRequestObject) (apigen.ListEventsResponseObject, error)
	GetEvent(ctx context.Context, req apigen.GetEventRequestObject) (apigen.GetEventResponseObject, error)
	TestRule(ctx context.Context, req apigen.TestRuleRequestObject) (apigen.TestRuleResponseObject, error)
	GetEntityProfile(ctx context.Context, req apigen.GetEntityProfileRequestObject) (apigen.GetEntityProfileResponseObject, error)
	GetAnalytics(ctx context.Context, req apigen.GetAnalyticsRequestObject) (apigen.GetAnalyticsResponseObject, error)
}

// Module is the events module.
type Module struct {
	svc *service.Service
}

// New returns the events module.
// enq receives the alert jobs of decisions; it must join the caller's transaction. metrics may
// be nil.
func New(runner txn.Runner, evaluator risk.Evaluator, tester risk.RuleTester, enq jobs.Enqueuer, clk clock.Clock, metrics *appmetrics.Metrics) *Module {
	return &Module{svc: service.New(runner, evaluator, tester, enq, clk, metrics)}
}

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() IngestHandlers { return handler.New(m.svc) }

// RollupArgs is the hourly job that keeps the analytics rollups up to date. It carries nothing:
// the job visits every environment of every active tenant.
type RollupArgs struct{}

// Kind implements jobs.Args. Stable across releases.
func (RollupArgs) Kind() string { return "analytics_rollup" }

// rollupActor is who the job's scope is recorded as.
const rollupActor = "system:analytics-rollup"

// RollupJob returns the handler of RollupArgs. It lists the environments on db, outside any tenant
// scope (identifiers only), then rolls each up inside its own tenant scope. One failing
// environment does not stop the others; the job fails afterwards so it is retried and visible.
func (m *Module) RollupJob(db sqlcgen.DBTX, scopes *tenants.Module, signals service.SignalRoller, logger *slog.Logger) func(context.Context, RollupArgs) error {
	return func(ctx context.Context, _ RollupArgs) error {
		envs, err := store.RollupEnvironments(ctx, db)
		if err != nil {
			return err
		}
		var errs []error
		for _, e := range envs {
			scoped, err := scopes.JobScope(ctx, e.TenantID, e.EnvironmentID, rollupActor, nil)
			if errors.Is(err, tenants.ErrNotFound) || errors.Is(err, tenants.ErrNotActive) {
				continue
			}
			if err == nil {
				err = m.svc.Rollup(scoped, signals)
			}
			if err != nil {
				logger.WarnContext(ctx, "analytics rollup failed", slog.String("tenant_id", e.TenantID.String()), slog.Any("error", err))
				errs = append(errs, fmt.Errorf("rollup of tenant %s: %w", e.TenantID, err))
			}
		}
		return errors.Join(errs...)
	}
}
