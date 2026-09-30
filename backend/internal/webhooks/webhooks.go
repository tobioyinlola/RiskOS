// Package webhooks is the public interface of the webhooks module: customer endpoints, signed
// event notifications, delivery with retries, and delivery logs (docs/WEBHOOKS.md).
//
// Modules publish events with Publisher.Publish inside the transaction of the change that caused
// them; the worker delivers them from their own queue, so slow customer endpoints delay nothing
// else.
package webhooks

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/service"
)

// Public types.
type (
	// EventType is a kind of event endpoints can subscribe to.
	EventType = domain.EventType
	// Config configures signing and delivery.
	Config = service.Config
	// Notifier is told when an endpoint is disabled automatically.
	Notifier = service.Notifier
	// DeliverArgs is the delivery job.
	DeliverArgs = service.DeliverArgs
)

// Event types.
const (
	EventAlertCreated = domain.EventAlertCreated
	EventAlertUpdated = domain.EventAlertUpdated
	EventCaseCreated  = domain.EventCaseCreated
	EventCaseResolved = domain.EventCaseResolved
)

// NotifierFunc adapts a function to Notifier.
type NotifierFunc func(ctx context.Context, endpointID uuid.UUID) error

// WebhookEndpointDisabled implements Notifier.
func (f NotifierFunc) WebhookEndpointDisabled(ctx context.Context, endpointID uuid.UUID) error {
	return f(ctx, endpointID)
}

// Publisher records events for delivery. Publish joins the caller's transaction; data is the
// event's "data" object (JSON-encoded as is).
type Publisher interface {
	Publish(ctx context.Context, eventType EventType, data any) error
}

// WebhookHandlers are the webhooks operations of the public API.
type WebhookHandlers interface {
	ListWebhooks(ctx context.Context, req apigen.ListWebhooksRequestObject) (apigen.ListWebhooksResponseObject, error)
	CreateWebhook(ctx context.Context, req apigen.CreateWebhookRequestObject) (apigen.CreateWebhookResponseObject, error)
	GetWebhook(ctx context.Context, req apigen.GetWebhookRequestObject) (apigen.GetWebhookResponseObject, error)
	UpdateWebhook(ctx context.Context, req apigen.UpdateWebhookRequestObject) (apigen.UpdateWebhookResponseObject, error)
	RotateWebhookSecret(ctx context.Context, req apigen.RotateWebhookSecretRequestObject) (apigen.RotateWebhookSecretResponseObject, error)
	TestWebhook(ctx context.Context, req apigen.TestWebhookRequestObject) (apigen.TestWebhookResponseObject, error)
	ListWebhookDeliveries(ctx context.Context, req apigen.ListWebhookDeliveriesRequestObject) (apigen.ListWebhookDeliveriesResponseObject, error)
	GetWebhookDelivery(ctx context.Context, req apigen.GetWebhookDeliveryRequestObject) (apigen.GetWebhookDeliveryResponseObject, error)
	RedeliverWebhook(ctx context.Context, req apigen.RedeliverWebhookRequestObject) (apigen.RedeliverWebhookResponseObject, error)
}

// Module is the webhooks module.
type Module struct {
	svc    *service.Service
	logger *slog.Logger
}

// New returns the webhooks module. enq must join the caller's transaction. notifier may be nil
// in processes that do not deliver.
func New(runner txn.Runner, rec audit.Recorder, clk clock.Clock, enq jobs.Enqueuer, cfg Config, notifier Notifier, logger *slog.Logger) *Module {
	return &Module{svc: service.New(runner, rec, clk, enq, cfg, notifier, logger), logger: logger}
}

// Publisher returns the module's publisher.
func (m *Module) Publisher() Publisher { return m.svc }

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() WebhookHandlers { return handler.New(m.svc) }

const jobActor = "system:webhooks"

// DeliverJob returns the worker handler for DeliverArgs. The job's scope holds the delivery
// permission and the alert permission (an endpoint disabled for failing raises a security
// alert). Jobs of deleted or suspended tenants are dropped.
func (m *Module) DeliverJob(scopes *tenants.Module) func(context.Context, DeliverArgs) error {
	return func(ctx context.Context, a DeliverArgs) error {
		ctx, err := scopes.JobScope(ctx, a.TenantID, a.EnvironmentID, jobActor,
			[]string{authz.WebhooksDeliver, authz.AlertsRaise})
		if errors.Is(err, tenants.ErrNotFound) || errors.Is(err, tenants.ErrNotActive) {
			m.logger.WarnContext(ctx, "webhook job dropped: tenant not active",
				slog.String("tenant_id", a.TenantID.String()), slog.String("delivery_id", a.DeliveryID.String()))
			return nil
		}
		if err != nil {
			return err
		}
		return m.svc.Deliver(ctx, a.DeliveryID)
	}
}

// Deliver runs one delivery attempt directly (tests and tooling); ctx must hold a job scope.
func (m *Module) Deliver(ctx context.Context, deliveryID uuid.UUID) error {
	return m.svc.Deliver(ctx, deliveryID)
}
