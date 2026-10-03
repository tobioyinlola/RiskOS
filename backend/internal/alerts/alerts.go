// Package alerts is the public interface of the alerts module: alerts raised from risk decisions
// and degraded evaluations, grouped per subject, and their triage lifecycle
// (docs/ALERTS_AND_CASES.md).
//
// Alerts are raised off the decision path: the events module enqueues a RaiseArgs job in the
// decision transaction (a transactional outbox), and the worker runs RaiseJob.
package alerts

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/present"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

// Public types and errors.
type (
	// Alert is an alert's current state.
	Alert = domain.Alert
	// Activity is an entry of a timeline.
	Activity = domain.Activity
)

// Errors.
var (
	ErrNotFound = domain.ErrNotFound
	ErrConflict = domain.ErrConflict
)

// AlertHandlers are the alerts operations of the public API.
type AlertHandlers interface {
	ListAlerts(ctx context.Context, req apigen.ListAlertsRequestObject) (apigen.ListAlertsResponseObject, error)
	GetAlert(ctx context.Context, req apigen.GetAlertRequestObject) (apigen.GetAlertResponseObject, error)
	UpdateAlert(ctx context.Context, req apigen.UpdateAlertRequestObject) (apigen.UpdateAlertResponseObject, error)
}

// Linker is what the cases module uses to attach alerts to cases. Every method joins the
// caller's transaction.
type Linker interface {
	// Escalate attaches open or investigating alerts to a case. Requires alerts:write.
	Escalate(ctx context.Context, alertIDs []uuid.UUID, caseID uuid.UUID) ([]Alert, error)
	// CloseForCase closes a resolved case's escalated alerts. The caller authorizes.
	CloseForCase(ctx context.Context, caseID uuid.UUID, falsePositive bool) error
	// ListByCase returns a case's alerts. The caller authorizes.
	ListByCase(ctx context.Context, caseID uuid.UUID) ([]Alert, error)
}

// RaiseSignal is a signal of the evaluation that raised the job.
type RaiseSignal struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
}

// RaiseArgs is the job enqueued with a decision that may raise alerts. It carries identifiers
// and the decision snapshot only (no payloads, no secrets); the tenant and environment are the
// server's own values from the decision transaction.
type RaiseArgs struct {
	TenantID      uuid.UUID     `json:"tenant_id"`
	EnvironmentID uuid.UUID     `json:"environment_id"`
	EvaluationID  uuid.UUID     `json:"evaluation_id"`
	EventID       uuid.UUID     `json:"event_id"`
	EventType     string        `json:"event_type"`
	UserRef       string        `json:"user_ref"`
	Score         int           `json:"score"`
	Level         string        `json:"level"`
	Decision      string        `json:"decision"`
	Signals       []RaiseSignal `json:"signals"`
	Degraded      []string      `json:"degraded"`
	EvaluatedAt   time.Time     `json:"evaluated_at"`
	// Risk: the decision reached the alert threshold. Security: signals were degraded.
	Risk     bool `json:"risk"`
	Security bool `json:"security"`
}

// Kind implements jobs.Args. Stable across releases.
func (RaiseArgs) Kind() string { return "alerts_raise" }

// Module is the alerts module.
type Module struct {
	svc    *service.Service
	logger *slog.Logger
}

// New returns the alerts module.
// Alert events are published to pub in the transaction of each change.
func New(runner txn.Runner, rec audit.Recorder, clk clock.Clock, pub webhooks.Publisher, logger *slog.Logger) *Module {
	return &Module{svc: service.New(runner, rec, clk, pub), logger: logger}
}

// WebhookEndpointDisabled implements webhooks.Notifier: it raises a security alert for an
// endpoint the webhooks module disabled after it kept failing.
func (m *Module) WebhookEndpointDisabled(ctx context.Context, endpointID uuid.UUID) error {
	return m.svc.RaiseWebhookDisabled(ctx, endpointID)
}

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() AlertHandlers { return handler.New(m.svc) }

// Linker returns the case linkage.
func (m *Module) Linker() Linker { return m.svc }

// ToAPI converts an alert to the contract (used by the cases module's responses).
func ToAPI(a Alert) apigen.Alert { return present.ToAPI(a) }

// ActivityToAPI converts a timeline to the contract.
func ActivityToAPI(a []Activity) []apigen.ActivityEntry { return present.ActivityToAPI(a) }

// MapError translates the module's errors to the API error contract.
func MapError(err error) error { return handler.MapError(err) }

// jobActor names the raise job in timelines and the audit log.
const jobActor = "system:alerts"

// RaiseJob returns the worker handler for RaiseArgs. It re-establishes the job's tenant scope
// through the tenants module, holding only authz.AlertsRaise. Jobs of deleted or suspended
// tenants are dropped (logged), not retried.
func (m *Module) RaiseJob(scopes *tenants.Module) func(context.Context, RaiseArgs) error {
	return func(ctx context.Context, a RaiseArgs) error {
		ctx, err := scopes.JobScope(ctx, a.TenantID, a.EnvironmentID, jobActor, []string{authz.AlertsRaise})
		if errors.Is(err, tenants.ErrNotFound) || errors.Is(err, tenants.ErrNotActive) {
			m.logger.WarnContext(ctx, "alert job dropped: tenant not active",
				slog.String("tenant_id", a.TenantID.String()), slog.String("evaluation_id", a.EvaluationID.String()))
			return nil
		}
		if err != nil {
			return err
		}
		signals := make([]domain.Signal, len(a.Signals))
		for i, s := range a.Signals {
			signals[i] = domain.Signal(s)
		}
		return m.svc.Raise(ctx, domain.Trigger{
			EvaluationID: a.EvaluationID, EventID: a.EventID, EventType: a.EventType, UserRef: a.UserRef,
			Score: a.Score, Level: a.Level, Decision: a.Decision, Signals: signals, Degraded: a.Degraded,
			EvaluatedAt: a.EvaluatedAt, Risk: a.Risk, Security: a.Security,
		})
	}
}

// OpenBefore lists alerts created before before and still open, newest first, at most limit.
// Requires alerts:read.
func (m *Module) OpenBefore(ctx context.Context, before time.Time, limit int) ([]Alert, error) {
	status, bound := domain.StatusOpen, ids.V7At(before)
	return m.svc.List(ctx, domain.Filter{Status: &status, Before: &bound, Limit: limit})
}
