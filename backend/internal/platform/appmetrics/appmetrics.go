// Package appmetrics holds the product-level metrics behind the service level objectives in
// docs/OBSERVABILITY.md: ingestion outcomes, risk evaluation latency, rate-limit rejections,
// authentication failures, webhook delivery and job execution.
//
// A *Metrics is injected where it is recorded, like the telemetry providers. A nil *Metrics
// records nothing, so modules and tests that have none need no branches. Attribute values are
// from small fixed sets (never tenant, key, user or address), keeping cardinality bounded.
package appmetrics

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const scope = "github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"

// Ingest results.
const (
	IngestAccepted = "accepted" // stored and decided
	IngestReplayed = "replayed" // same key and payload: the stored outcome was returned
	IngestRejected = "rejected" // refused by validation, policy or authorization (a caller error)
	IngestError    = "error"    // failed for a reason on our side
)

// Rate limit kinds.
const (
	LimitKey         = "key"
	LimitTenant      = "tenant"
	LimitOperation   = "operation"
	LimitAuthFailure = "auth_failure"
)

// Webhook delivery results.
const (
	DeliverySucceeded = "succeeded"
	DeliveryRetrying  = "retrying" // failed; another attempt is scheduled
	DeliveryFailed    = "failed"   // failed; the schedule is exhausted or the endpoint was disabled
)

// Job outcomes.
const (
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobSnoozed   = "snoozed"
)

// Metrics records the application metrics. The zero value and nil record nothing.
type Metrics struct {
	ingest          metric.Int64Counter
	evalDuration    metric.Float64Histogram
	rateRejected    metric.Int64Counter
	limiterErrors   metric.Int64Counter
	authFailures    metric.Int64Counter
	deliveries      metric.Int64Counter
	deliveryTime    metric.Float64Histogram
	jobDuration     metric.Float64Histogram
	queueDepthGauge metric.Int64ObservableGauge
	meter           metric.Meter
}

// New creates the instruments on mp.
func New(mp metric.MeterProvider) (*Metrics, error) {
	m := mp.Meter(scope)
	out := &Metrics{meter: m}
	var err error
	fail := func(name string, e error) error { return fmt.Errorf("create metric %s: %w", name, e) }

	if out.ingest, err = m.Int64Counter("riskos.events.ingested",
		metric.WithDescription("Ingest requests by result."), metric.WithUnit("{event}")); err != nil {
		return nil, fail("riskos.events.ingested", err)
	}
	if out.evalDuration, err = m.Float64Histogram("riskos.risk.evaluation.duration",
		metric.WithDescription("Time to evaluate an event, by decision."), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.5, 1, 2.5)); err != nil {
		return nil, fail("riskos.risk.evaluation.duration", err)
	}
	if out.rateRejected, err = m.Int64Counter("riskos.ratelimit.rejected",
		metric.WithDescription("Requests refused by a rate limit, by limit kind."), metric.WithUnit("{request}")); err != nil {
		return nil, fail("riskos.ratelimit.rejected", err)
	}
	if out.limiterErrors, err = m.Int64Counter("riskos.ratelimit.errors",
		metric.WithDescription("Rate limit checks that failed open because the limiter was unavailable."),
		metric.WithUnit("{check}")); err != nil {
		return nil, fail("riskos.ratelimit.errors", err)
	}
	if out.authFailures, err = m.Int64Counter("riskos.auth.failures",
		metric.WithDescription("API requests that failed authentication."), metric.WithUnit("{request}")); err != nil {
		return nil, fail("riskos.auth.failures", err)
	}
	if out.deliveries, err = m.Int64Counter("riskos.webhooks.deliveries",
		metric.WithDescription("Webhook delivery attempts by result."), metric.WithUnit("{attempt}")); err != nil {
		return nil, fail("riskos.webhooks.deliveries", err)
	}
	if out.deliveryTime, err = m.Float64Histogram("riskos.webhooks.delivery.duration",
		metric.WithDescription("Duration of webhook delivery attempts."), metric.WithUnit("s")); err != nil {
		return nil, fail("riskos.webhooks.delivery.duration", err)
	}
	if out.jobDuration, err = m.Float64Histogram("riskos.jobs.duration",
		metric.WithDescription("Time a worker spent executing a job, by kind and outcome."), metric.WithUnit("s")); err != nil {
		return nil, fail("riskos.jobs.duration", err)
	}
	return out, nil
}

func (m *Metrics) ok() bool { return m != nil && m.ingest != nil }

// Ingested counts one ingest request.
func (m *Metrics) Ingested(ctx context.Context, result string) {
	if !m.ok() {
		return
	}
	m.ingest.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

// Evaluated records how long one evaluation took and its decision.
func (m *Metrics) Evaluated(ctx context.Context, d time.Duration, decision string) {
	if !m.ok() {
		return
	}
	m.evalDuration.Record(ctx, d.Seconds(), metric.WithAttributes(attribute.String("decision", decision)))
}

// RateLimited counts a request refused by the given limit kind.
func (m *Metrics) RateLimited(ctx context.Context, kind string) {
	if !m.ok() {
		return
	}
	m.rateRejected.Add(ctx, 1, metric.WithAttributes(attribute.String("limit", kind)))
}

// LimiterUnavailable counts a rate limit check that failed open.
func (m *Metrics) LimiterUnavailable(ctx context.Context) {
	if !m.ok() {
		return
	}
	m.limiterErrors.Add(ctx, 1)
}

// AuthFailed counts a request that failed authentication.
func (m *Metrics) AuthFailed(ctx context.Context) {
	if !m.ok() {
		return
	}
	m.authFailures.Add(ctx, 1)
}

// Delivered records one webhook delivery attempt.
func (m *Metrics) Delivered(ctx context.Context, result string, d time.Duration) {
	if !m.ok() {
		return
	}
	attrs := metric.WithAttributes(attribute.String("result", result))
	m.deliveries.Add(ctx, 1, attrs)
	m.deliveryTime.Record(ctx, d.Seconds(), attrs)
}

// JobRan records one job execution.
func (m *Metrics) JobRan(ctx context.Context, kind, outcome string, d time.Duration) {
	if !m.ok() {
		return
	}
	m.jobDuration.Record(ctx, d.Seconds(), metric.WithAttributes(
		attribute.String("kind", kind), attribute.String("outcome", outcome)))
}

// QueueDepth is one observation of waiting jobs.
type QueueDepth struct {
	Queue string
	State string
	Count int64
}

// ObserveQueueDepth registers a gauge, read at each collection, of jobs by queue and state.
// read must be cheap and bounded; its failure leaves the previous collection without a value.
func (m *Metrics) ObserveQueueDepth(read func(context.Context) ([]QueueDepth, error)) error {
	if !m.ok() {
		return nil
	}
	g, err := m.meter.Int64ObservableGauge("riskos.jobs.queue.depth",
		metric.WithDescription("Jobs waiting or scheduled, by queue and state."), metric.WithUnit("{job}"))
	if err != nil {
		return fmt.Errorf("create metric riskos.jobs.queue.depth: %w", err)
	}
	m.queueDepthGauge = g
	_, err = m.meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		rows, err := read(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			o.ObserveInt64(g, r.Count, metric.WithAttributes(
				attribute.String("queue", r.Queue), attribute.String("state", r.State)))
		}
		return nil
	}, g)
	if err != nil {
		return fmt.Errorf("register queue depth callback: %w", err)
	}
	return nil
}
