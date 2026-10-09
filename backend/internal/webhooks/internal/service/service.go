// Package service holds the webhooks module's business logic: endpoints, publishing events in
// the caller's transaction, and delivering them with retries.
package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/egress"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/store"
)

// Config configures delivery.
type Config struct {
	// SigningKey derives every endpoint's secret (at least 32 bytes, from the secrets manager).
	SigningKey []byte
	// Timeout bounds each attempt.
	Timeout time.Duration
	// AllowPrivateTest lets test-environment endpoints use http:// and private addresses (local
	// development only; refused by configuration in production).
	AllowPrivateTest bool
	// RetryDelays is the wait after each failed attempt; nil means domain.RetryDelays().
	RetryDelays []time.Duration
	// DisableAfter disables an endpoint that has failed every attempt for this long; zero means
	// domain.DisableAfter.
	DisableAfter time.Duration
	// Metrics records delivery outcomes; nil records nothing.
	Metrics *appmetrics.Metrics
}

// Notifier is told when an endpoint is disabled automatically (the alerts module raises a
// security alert). It runs in the delivery's transaction.
type Notifier interface {
	WebhookEndpointDisabled(ctx context.Context, endpointID uuid.UUID) error
}

// DeliverArgs is the delivery job. It carries identifiers only.
type DeliverArgs struct {
	TenantID      uuid.UUID `json:"tenant_id"`
	EnvironmentID uuid.UUID `json:"environment_id"`
	DeliveryID    uuid.UUID `json:"delivery_id"`
}

// Kind implements jobs.Args. Stable across releases.
func (DeliverArgs) Kind() string { return "webhooks_deliver" }

// Queue implements jobs.Queued: deliveries never share workers with alert creation.
func (DeliverArgs) Queue() string { return jobs.QueueWebhooks }

// Service manages endpoints and delivers events.
type Service struct {
	store    store.Store
	runner   txn.Runner
	audit    audit.Recorder
	clock    clock.Clock
	jobs     jobs.Enqueuer
	client   *egress.Client
	cfg      Config
	notifier Notifier
	logger   *slog.Logger
}

// New returns a Service. notifier may be nil (the API process never delivers).
func New(runner txn.Runner, rec audit.Recorder, clk clock.Clock, enq jobs.Enqueuer, cfg Config, notifier Notifier, logger *slog.Logger) *Service {
	if cfg.RetryDelays == nil {
		cfg.RetryDelays = domain.RetryDelays()
	}
	if cfg.DisableAfter == 0 {
		cfg.DisableAfter = domain.DisableAfter
	}
	return &Service{
		runner: runner, audit: rec, clock: clk, jobs: enq, client: egress.New(cfg.Timeout), cfg: cfg,
		notifier: notifier, logger: logger,
	}
}

// Input configures an endpoint.
type Input struct {
	URL         string
	Description string
	EventTypes  []domain.EventType
}

// Patch changes an endpoint. A nil field is unchanged.
type Patch struct {
	URL         *string
	Description *string
	EventTypes  *[]domain.EventType
	Status      *string
}

func (s *Service) policy(scope tenancy.Scope) domain.URLPolicy {
	return domain.URLPolicy{AllowPrivate: s.cfg.AllowPrivateTest && scope.EnvironmentKind == tenancy.EnvironmentTest}
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalid, fmt.Sprintf(format, args...))
}

func checkEventTypes(ts []domain.EventType) ([]domain.EventType, error) {
	if len(ts) == 0 {
		return nil, invalid("event_types must list at least one event type")
	}
	out := slices.Clone(ts)
	slices.Sort(out)
	out = slices.Compact(out)
	for _, t := range out {
		if !slices.Contains(domain.SubscribableEvents(), t) {
			return nil, invalid("unknown event type %q", t)
		}
	}
	return out, nil
}

// host returns the host of a URL for audit records: query strings and paths can carry tokens.
func host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// Create registers an endpoint and returns it with its secret, which is shown only now.
// Requires webhooks:write; audited.
func (s *Service) Create(ctx context.Context, in Input) (domain.Endpoint, string, error) {
	if err := authz.Require(ctx, authz.WebhooksWrite); err != nil {
		return domain.Endpoint{}, "", err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	if err := domain.ValidateURL(in.URL, s.policy(scope)); err != nil {
		return domain.Endpoint{}, "", err
	}
	if utf8.RuneCountInString(in.Description) > domain.MaxDescription {
		return domain.Endpoint{}, "", invalid("description must be at most %d characters", domain.MaxDescription)
	}
	types, err := checkEventTypes(in.EventTypes)
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	now := s.clock.Now()
	e := domain.Endpoint{
		ID: ids.NewV7(), URL: in.URL, Description: in.Description, EventTypes: types, Status: domain.StatusActive,
		SecretVersion: 1, CreatedAt: now, CreatedBy: scope.Principal.ID, UpdatedAt: now,
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		n, err := s.store.CountEndpoints(ctx)
		if err != nil {
			return err
		}
		if n >= domain.MaxEndpointsPerEnvironment {
			return fmt.Errorf("%w: an environment can have at most %d webhook endpoints", domain.ErrLimit, domain.MaxEndpointsPerEnvironment)
		}
		if err := s.store.InsertEndpoint(ctx, e); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "webhook.created", ResourceType: "webhook", ResourceID: ids.Format(ids.PrefixWebhook, e.ID),
			Outcome:  audit.OutcomeSuccess,
			Metadata: map[string]any{"host": host(e.URL), "event_types": types},
		})
	})
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	return e, domain.Secret(s.cfg.SigningKey, e.ID, e.SecretVersion), nil
}

// List returns the environment's endpoints. Requires webhooks:read.
func (s *Service) List(ctx context.Context) ([]domain.Endpoint, error) {
	if err := authz.Require(ctx, authz.WebhooksRead); err != nil {
		return nil, err
	}
	var out []domain.Endpoint
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.ListEndpoints(ctx)
		return err
	})
	return out, err
}

// Get returns an endpoint. Requires webhooks:read.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Endpoint, error) {
	if err := authz.Require(ctx, authz.WebhooksRead); err != nil {
		return domain.Endpoint{}, err
	}
	var out domain.Endpoint
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.GetEndpoint(ctx, id, false)
		return err
	})
	return out, err
}

// Update changes an endpoint. Enabling a disabled endpoint clears its failure state.
// Requires webhooks:write; audited.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p Patch) (domain.Endpoint, error) {
	if err := authz.Require(ctx, authz.WebhooksWrite); err != nil {
		return domain.Endpoint{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Endpoint{}, err
	}
	if p.URL != nil {
		if err := domain.ValidateURL(*p.URL, s.policy(scope)); err != nil {
			return domain.Endpoint{}, err
		}
	}
	if p.Description != nil && utf8.RuneCountInString(*p.Description) > domain.MaxDescription {
		return domain.Endpoint{}, invalid("description must be at most %d characters", domain.MaxDescription)
	}
	if p.EventTypes != nil {
		types, err := checkEventTypes(*p.EventTypes)
		if err != nil {
			return domain.Endpoint{}, err
		}
		p.EventTypes = &types
	}
	if p.Status != nil && *p.Status != domain.StatusActive && *p.Status != domain.StatusDisabled {
		return domain.Endpoint{}, invalid("status must be active or disabled")
	}
	var out domain.Endpoint
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.store.GetEndpoint(ctx, id, true)
		if err != nil {
			return err
		}
		next := cur
		changes := map[string]any{}
		if p.URL != nil && *p.URL != cur.URL {
			next.URL = *p.URL
			changes["host"] = map[string]any{"before": host(cur.URL), "after": host(next.URL)}
		}
		if p.Description != nil && *p.Description != cur.Description {
			next.Description = *p.Description
			changes["description"] = "changed"
		}
		if p.EventTypes != nil && !slices.Equal(*p.EventTypes, cur.EventTypes) {
			next.EventTypes = *p.EventTypes
			changes["event_types"] = map[string]any{"before": cur.EventTypes, "after": next.EventTypes}
		}
		if p.Status != nil && *p.Status != cur.Status {
			next.Status = *p.Status
			changes["status"] = map[string]any{"before": cur.Status, "after": next.Status}
			if next.Status == domain.StatusActive {
				next.DisabledReason, next.FailingSince = nil, nil
			} else {
				next.DisabledReason = new("disabled by " + scope.Principal.ID)
			}
		}
		if len(changes) == 0 {
			out = cur
			return nil
		}
		next.UpdatedAt = s.clock.Now()
		if err := s.store.UpdateEndpoint(ctx, next); err != nil {
			return err
		}
		out = next
		return s.audit.Record(ctx, audit.Entry{
			Action: "webhook.updated", ResourceType: "webhook", ResourceID: ids.Format(ids.PrefixWebhook, id),
			Outcome: audit.OutcomeSuccess, Metadata: changes,
		})
	})
	return out, err
}

// RotateSecret issues a new secret, shown only now. Deliveries are signed with both secrets
// until overlap has passed. Requires webhooks:write; audited.
func (s *Service) RotateSecret(ctx context.Context, id uuid.UUID, overlap time.Duration) (domain.Endpoint, string, error) {
	if err := authz.Require(ctx, authz.WebhooksWrite); err != nil {
		return domain.Endpoint{}, "", err
	}
	if overlap < domain.MinOverlap || overlap > domain.MaxOverlap {
		return domain.Endpoint{}, "", invalid("overlap must be between 0 and %d hours", int(domain.MaxOverlap.Hours()))
	}
	var out domain.Endpoint
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetEndpoint(ctx, id, true); err != nil {
			return err
		}
		now := s.clock.Now()
		if err := s.store.RotateSecret(ctx, id, now.Add(overlap), now); err != nil {
			return err
		}
		var err error
		if out, err = s.store.GetEndpoint(ctx, id, false); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "webhook.secret_rotated", ResourceType: "webhook", ResourceID: ids.Format(ids.PrefixWebhook, id),
			Outcome:  audit.OutcomeSuccess,
			Metadata: map[string]any{"secret_version": out.SecretVersion, "overlap_seconds": int(overlap.Seconds())},
		})
	})
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	return out, domain.Secret(s.cfg.SigningKey, out.ID, out.SecretVersion), nil
}

// envelope is the JSON body of every delivery.
type envelope struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	Environment string    `json:"environment"`
	Data        any       `json:"data"`
}

// Publish records an event for every active endpoint subscribed to it and enqueues the
// deliveries, in the caller's transaction: an event is delivered only if the change that caused
// it committed. The caller has already authorized the change.
func (s *Service) Publish(ctx context.Context, eventType domain.EventType, data any) error {
	return s.runner.Run(ctx, func(ctx context.Context) error {
		endpoints, err := s.store.Subscribed(ctx, eventType)
		if err != nil || len(endpoints) == 0 {
			return err
		}
		return s.publishTo(ctx, endpoints, eventType, data)
	})
}

func (s *Service) publishTo(ctx context.Context, endpoints []uuid.UUID, eventType domain.EventType, data any) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	eventID := ids.NewV7()
	body, err := json.Marshal(envelope{
		ID: ids.Format(ids.PrefixWebhookEvent, eventID), Type: string(eventType), CreatedAt: now,
		Environment: string(scope.EnvironmentKind), Data: data,
	})
	if err != nil {
		return fmt.Errorf("encode webhook event: %w", err)
	}
	for _, endpointID := range endpoints {
		d := domain.Delivery{
			ID: ids.NewV7(), EndpointID: endpointID, EventID: eventID, EventType: eventType, Payload: body, CreatedAt: now,
		}
		if err := s.store.InsertDelivery(ctx, d); err != nil {
			return err
		}
		if err := s.jobs.Enqueue(ctx, DeliverArgs{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, DeliveryID: d.ID}); err != nil {
			return err
		}
	}
	return nil
}

// SendTest delivers a webhook.test event to one active endpoint. Requires webhooks:write.
func (s *Service) SendTest(ctx context.Context, endpointID uuid.UUID) (domain.Delivery, error) {
	if err := authz.Require(ctx, authz.WebhooksWrite); err != nil {
		return domain.Delivery{}, err
	}
	var out domain.Delivery
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		e, err := s.store.GetEndpoint(ctx, endpointID, false)
		if err != nil {
			return err
		}
		if e.Status != domain.StatusActive {
			return fmt.Errorf("%w: the endpoint is disabled", domain.ErrConflict)
		}
		if err := s.publishTo(ctx, []uuid.UUID{endpointID}, domain.EventTest, map[string]any{
			"message": "Test event from RiskOS. Verify its signature, then respond with any 2xx status.",
		}); err != nil {
			return err
		}
		list, err := s.store.ListDeliveries(ctx, store.DeliveryFilter{EndpointID: endpointID, Limit: 1})
		if err != nil || len(list) == 0 {
			return errors.Join(err, errors.New("test delivery not found"))
		}
		out = list[0]
		return nil
	})
	return out, err
}

// ListDeliveries returns a newest-first page of an endpoint's deliveries. Requires
// webhooks:read.
func (s *Service) ListDeliveries(ctx context.Context, f store.DeliveryFilter) ([]domain.Delivery, error) {
	if err := authz.Require(ctx, authz.WebhooksRead); err != nil {
		return nil, err
	}
	var out []domain.Delivery
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetEndpoint(ctx, f.EndpointID, false); err != nil {
			return err
		}
		var err error
		out, err = s.store.ListDeliveries(ctx, f)
		return err
	})
	return out, err
}

// GetDelivery returns a delivery of an endpoint with its attempts. Requires webhooks:read.
func (s *Service) GetDelivery(ctx context.Context, endpointID, id uuid.UUID) (domain.Delivery, []domain.Attempt, error) {
	if err := authz.Require(ctx, authz.WebhooksRead); err != nil {
		return domain.Delivery{}, nil, err
	}
	var d domain.Delivery
	var attempts []domain.Attempt
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if d, err = s.store.GetDelivery(ctx, id, false); err != nil {
			return err
		}
		if d.EndpointID != endpointID {
			return domain.ErrNotFound
		}
		attempts, err = s.store.Attempts(ctx, id)
		return err
	})
	return d, attempts, err
}

// Redeliver starts a new delivery cycle for a failed delivery to an active endpoint. The event
// keeps its ID, so receivers that processed it already can recognize it. Requires
// webhooks:write; audited.
func (s *Service) Redeliver(ctx context.Context, endpointID, id uuid.UUID) (domain.Delivery, error) {
	if err := authz.Require(ctx, authz.WebhooksWrite); err != nil {
		return domain.Delivery{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Delivery{}, err
	}
	var out domain.Delivery
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		e, err := s.store.GetEndpoint(ctx, endpointID, false)
		if err != nil {
			return err
		}
		d, err := s.store.GetDelivery(ctx, id, true)
		if err != nil {
			return err
		}
		if d.EndpointID != endpointID {
			return domain.ErrNotFound
		}
		if d.Status != domain.DeliveryFailed {
			return fmt.Errorf("%w: only failed deliveries can be redelivered", domain.ErrConflict)
		}
		if e.Status != domain.StatusActive {
			return fmt.Errorf("%w: enable the endpoint first", domain.ErrConflict)
		}
		if err := s.store.RestartDelivery(ctx, id, s.clock.Now()); err != nil {
			return err
		}
		if err := s.jobs.Enqueue(ctx, DeliverArgs{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, DeliveryID: id}); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "webhook.redelivered", ResourceType: "webhook", ResourceID: ids.Format(ids.PrefixWebhook, endpointID),
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{"delivery_id": ids.Format(ids.PrefixWebhookDelivery, id)},
		}); err != nil {
			return err
		}
		out, err = s.store.GetDelivery(ctx, id, false)
		return err
	})
	return out, err
}

// Deliver makes one delivery attempt. The HTTP request runs outside any transaction. On failure
// it returns jobs.Snooze with the next delay until the schedule is exhausted, then marks the
// delivery failed. Requires the system permission authz.WebhooksDeliver (background jobs only).
func (s *Service) Deliver(ctx context.Context, deliveryID uuid.UUID) error {
	if err := authz.Require(ctx, authz.WebhooksDeliver); err != nil {
		return err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	var d domain.Delivery
	var e domain.Endpoint
	skip := false
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if d, err = s.store.GetDelivery(ctx, deliveryID, false); err != nil {
			return err
		}
		if d.Status != domain.DeliveryPending {
			skip = true
			return nil
		}
		if e, err = s.store.GetEndpoint(ctx, d.EndpointID, false); err != nil {
			return err
		}
		if e.Status != domain.StatusActive {
			skip = true
			return s.store.FailDelivery(ctx, deliveryID, "endpoint disabled", s.clock.Now())
		}
		return nil
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil // nothing to deliver
	}
	if err != nil || skip {
		return err
	}

	start := s.clock.Now()
	res := s.client.Send(ctx, egress.Request{
		URL: e.URL, Body: d.Payload, EventID: ids.Format(ids.PrefixWebhookEvent, d.EventID), EventType: string(d.EventType),
		DeliveryID: ids.Format(ids.PrefixWebhookDelivery, d.ID), Timestamp: start.Unix(),
		Signatures:   s.signatures(e, d, start),
		AllowPrivate: s.policy(scope).AllowPrivate,
	})
	duration := s.clock.Now().Sub(start)

	var retryIn time.Duration
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.store.GetEndpoint(ctx, d.EndpointID, true)
		if err != nil {
			return err
		}
		locked, err := s.store.GetDelivery(ctx, deliveryID, true)
		if err != nil {
			return err
		}
		if locked.Status != domain.DeliveryPending {
			return nil
		}
		o := store.AttemptOutcome{
			Attempt: locked.Attempts + 1, StartedAt: start, Duration: duration, StatusCode: res.StatusCode, Error: res.Error,
		}
		now := s.clock.Now()
		if res.OK() {
			o.Status = domain.DeliverySucceeded
			if cur.FailingSince != nil {
				if err := s.store.SetHealth(ctx, cur.ID, nil, cur.Status, cur.DisabledReason, now); err != nil {
					return err
				}
			}
			return s.store.RecordAttempt(ctx, deliveryID, o)
		}
		disabled, err := s.recordFailure(ctx, cur, now)
		if err != nil {
			return err
		}
		o.Status = domain.DeliveryFailed
		if !disabled && o.Attempt <= len(s.cfg.RetryDelays) {
			retryIn = jitter(s.cfg.RetryDelays[o.Attempt-1])
			next := now.Add(retryIn)
			o.Status, o.NextAttemptAt = domain.DeliveryPending, &next
		}
		return s.store.RecordAttempt(ctx, deliveryID, o)
	})
	if err != nil {
		return err
	}
	switch {
	case res.OK():
		s.cfg.Metrics.Delivered(ctx, appmetrics.DeliverySucceeded, duration)
	case retryIn > 0:
		s.cfg.Metrics.Delivered(ctx, appmetrics.DeliveryRetrying, duration)
	default:
		s.cfg.Metrics.Delivered(ctx, appmetrics.DeliveryFailed, duration)
	}
	if retryIn > 0 {
		return jobs.Snooze(retryIn)
	}
	return nil
}

// recordFailure updates an endpoint's failing state and disables it once it has failed for
// DisableAfter. It reports whether the endpoint is disabled now.
func (s *Service) recordFailure(ctx context.Context, e domain.Endpoint, now time.Time) (bool, error) {
	if e.Status != domain.StatusActive {
		return true, nil
	}
	since := now
	if e.FailingSince != nil {
		since = *e.FailingSince
	}
	if now.Sub(since) < s.cfg.DisableAfter {
		if e.FailingSince == nil {
			return false, s.store.SetHealth(ctx, e.ID, &now, e.Status, nil, now)
		}
		return false, nil
	}
	reason := fmt.Sprintf("every delivery failed since %s", since.UTC().Format(time.RFC3339))
	if err := s.store.SetHealth(ctx, e.ID, &since, domain.StatusDisabled, &reason, now); err != nil {
		return false, err
	}
	if err := s.audit.Record(ctx, audit.Entry{
		Action: "webhook.disabled", ResourceType: "webhook", ResourceID: ids.Format(ids.PrefixWebhook, e.ID),
		Outcome:  audit.OutcomeSuccess,
		Metadata: map[string]any{"reason": "failing", "failing_since": since.UTC().Format(time.RFC3339), "host": host(e.URL)},
	}); err != nil {
		return false, err
	}
	s.logger.WarnContext(ctx, "webhook endpoint disabled after repeated failures",
		slog.String("webhook_id", ids.Format(ids.PrefixWebhook, e.ID)))
	if s.notifier != nil {
		if err := s.notifier.WebhookEndpointDisabled(ctx, e.ID); err != nil {
			return false, err
		}
	}
	return true, nil
}

// signatures returns the current secret's signature and, during a rotation overlap, the
// previous one's.
func (s *Service) signatures(e domain.Endpoint, d domain.Delivery, at time.Time) []string {
	eventID := ids.Format(ids.PrefixWebhookEvent, d.EventID)
	out := []string{domain.Sign(domain.Secret(s.cfg.SigningKey, e.ID, e.SecretVersion), eventID, at.Unix(), d.Payload)}
	if e.PreviousSecretVersion != nil && e.PreviousSecretValidUntil != nil && at.Before(*e.PreviousSecretValidUntil) {
		prev := domain.Secret(s.cfg.SigningKey, e.ID, *e.PreviousSecretVersion)
		out = append(out, domain.Sign(prev, eventID, at.Unix(), d.Payload))
	}
	return out
}

// jitter spreads retries by ±20% so failing endpoints are not hit in lockstep.
func jitter(d time.Duration) time.Duration {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand never fails on supported platforms
	f := float64(binary.BigEndian.Uint64(b[:])>>11) / (1 << 53)
	spread := float64(d) * 0.2
	return time.Duration(float64(d) - spread + f*2*spread)
}

// ParseEventTypes converts contract values, rejecting unknown ones.
func ParseEventTypes(raw []string) []domain.EventType {
	out := make([]domain.EventType, len(raw))
	for i, r := range raw {
		out[i] = domain.EventType(strings.TrimSpace(r))
	}
	return out
}
