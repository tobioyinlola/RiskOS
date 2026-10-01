// Package service implements event ingestion, dry-run evaluation and the event explorer.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"

	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

// Service ingests and serves events.
type Service struct {
	store  store.Store
	runner txn.Runner
	risk   risk.Evaluator
	tester risk.RuleTester
	jobs   jobs.Enqueuer
	clock  clock.Clock
}

// New returns a Service.
func New(runner txn.Runner, evaluator risk.Evaluator, tester risk.RuleTester, enq jobs.Enqueuer, clk clock.Clock) *Service {
	return &Service{runner: runner, risk: evaluator, tester: tester, jobs: enq, clock: clk}
}

// TestRule normalizes an event and tests a rule against it without storing anything. The time
// window is not applied, so rules can be tested against historical events; card numbers are
// still rejected. Authorization (rules:read) is enforced by the risk module.
func (s *Service) TestRule(ctx context.Context, ruleID uuid.UUID, raw []byte, e domain.Event, draft json.RawMessage) (risk.TestResult, error) {
	if domain.ContainsCardNumber(raw) {
		return risk.TestResult{}, domain.ErrSensitiveData
	}
	n, _, err := domain.Normalize(e)
	if err != nil {
		return risk.TestResult{}, err
	}
	encoded, err := n.Encode()
	if err != nil {
		return risk.TestResult{}, fmt.Errorf("encode normalized event: %w", err)
	}
	return s.tester.TestRule(ctx, ruleID, encoded, draft)
}

// prepared is an event that passed every pre-storage check.
type prepared struct {
	normalized domain.Normalized
	encoded    []byte
	hash       []byte
	ip         *netip.Addr
}

// prepare applies the checks the contract cannot express and normalizes the event.
func (s *Service) prepare(raw []byte, e domain.Event) (prepared, error) {
	if domain.ContainsCardNumber(raw) {
		return prepared{}, domain.ErrSensitiveData
	}
	if err := domain.CheckWindow(e.OccurredAt, s.clock.Now()); err != nil {
		return prepared{}, err
	}
	n, ip, err := domain.Normalize(e)
	if err != nil {
		return prepared{}, err
	}
	encoded, err := n.Encode()
	if err != nil {
		return prepared{}, fmt.Errorf("encode normalized event: %w", err)
	}
	return prepared{normalized: n, encoded: encoded, hash: domain.PayloadHash(encoded), ip: ip}, nil
}

// Ingest validates, normalizes and stores an event, evaluates it and returns the decision, all in
// one transaction: nothing is visible until the decision is durable. It is idempotent on the
// event key: a retry with the same payload returns the stored outcome (replay = true); the same
// key with a different payload returns domain.ErrIdempotencyConflict. Requires events:write.
func (s *Service) Ingest(ctx context.Context, raw []byte, e domain.Event) (outcome domain.Outcome, replay bool, err error) {
	if err := authz.Require(ctx, authz.EventsWrite); err != nil {
		return domain.Outcome{}, false, err
	}
	p, err := s.prepare(raw, e)
	if err != nil {
		return domain.Outcome{}, false, err
	}

	eventID := ids.NewV7()
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		prev, err := s.store.Claim(ctx, e.Key, p.hash, eventID)
		if err != nil {
			return err
		}
		if prev != nil {
			if !bytes.Equal(prev.PayloadHash, p.hash) {
				return domain.ErrIdempotencyConflict
			}
			if prev.Response == nil {
				return errors.New("idempotency record has no response")
			}
			replay = true
			return json.Unmarshal(prev.Response, &outcome)
		}

		res, err := s.risk.Evaluate(ctx, risk.Input{
			Features:  risk.Features{EventType: p.normalized.Type, Normalized: p.encoded},
			EventKey:  e.Key,
			EventID:   &eventID,
			RequestID: requestctx.RequestID(ctx),
		})
		if err != nil {
			return err
		}
		if err := s.raiseAlerts(ctx, eventID, p.normalized, res); err != nil {
			return err
		}
		if err := s.store.Insert(ctx, store.InsertParams{
			ID: eventID, ReceivedAt: ids.Time(eventID), Event: withNormalized(e, p.normalized), Normalized: p.encoded,
			PayloadHash: p.hash, RawPayload: raw, IP: p.ip, EvaluationID: res.ID,
			Score: res.Score, Level: string(res.Level), Decision: string(res.Decision), RequestID: res.RequestID,
		}); err != nil {
			return err
		}
		outcome = toOutcome(res)
		stored, err := json.Marshal(outcome)
		if err != nil {
			return fmt.Errorf("encode outcome: %w", err)
		}
		return s.store.SetResponse(ctx, e.Key, stored)
	})
	if err != nil {
		return domain.Outcome{}, false, err
	}
	return outcome, replay, nil
}

// raiseAlerts enqueues the alert job for a decision that reached the alert threshold or ran with
// degraded signals. It runs in the decision transaction, so the job exists exactly when the
// decision does (transactional outbox, ARCHITECTURE.md §6).
func (s *Service) raiseAlerts(ctx context.Context, eventID uuid.UUID, n domain.Normalized, res risk.Result) error {
	security := len(res.DegradedSignals) > 0
	if !res.RaiseAlert && !security {
		return nil
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	signals := make([]alerts.RaiseSignal, len(res.Signals))
	for i, sig := range res.Signals {
		signals[i] = alerts.RaiseSignal{Code: sig.Code, Label: sig.Label, Weight: sig.Weight}
	}
	return s.jobs.Enqueue(ctx, alerts.RaiseArgs{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, EvaluationID: res.ID, EventID: eventID,
		EventType: n.Type, UserRef: n.Actor.UserID, Score: res.Score, Level: string(res.Level),
		Decision: string(res.Decision), Signals: signals, Degraded: res.DegradedSignals, EvaluatedAt: res.CreatedAt,
		Risk: res.RaiseAlert, Security: security,
	})
}

// Evaluate runs the same checks and evaluation as Ingest without storing an event (dry run). The
// evaluation is recorded, flagged as a dry run, and never raises alerts or webhooks. Not
// idempotent. Requires risk:evaluate.
func (s *Service) Evaluate(ctx context.Context, raw []byte, e domain.Event) (domain.Outcome, error) {
	if err := authz.Require(ctx, authz.RiskEvaluate); err != nil {
		return domain.Outcome{}, err
	}
	p, err := s.prepare(raw, e)
	if err != nil {
		return domain.Outcome{}, err
	}
	res, err := s.risk.Evaluate(ctx, risk.Input{
		Features:  risk.Features{EventType: p.normalized.Type, Normalized: p.encoded},
		EventKey:  e.Key,
		RequestID: requestctx.RequestID(ctx),
	})
	if err != nil {
		return domain.Outcome{}, err
	}
	return toOutcome(res), nil
}

// List returns a newest-first page of events. Requires events:read.
func (s *Service) List(ctx context.Context, p domain.Page) ([]domain.Summary, error) {
	if err := authz.Require(ctx, authz.EventsRead); err != nil {
		return nil, err
	}
	var out []domain.Summary
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.List(ctx, p)
		return err
	})
	return out, err
}

// Get returns an event with its payloads and evaluation. Requires events:read.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Detail, error) {
	if err := authz.Require(ctx, authz.EventsRead); err != nil {
		return domain.Detail{}, err
	}
	var d domain.Detail
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if d, err = s.store.Get(ctx, id, ids.Time(id)); err != nil {
			return err
		}
		res, err := s.risk.Get(ctx, d.EvaluationID)
		if err != nil {
			return fmt.Errorf("evaluation of event: %w", err)
		}
		d.Evaluation = toOutcome(res)
		return nil
	})
	return d, err
}

func withNormalized(e domain.Event, n domain.Normalized) domain.Event {
	e.OccurredAt = n.OccurredAt
	e.Context = n.Context
	return e
}

func toOutcome(r risk.Result) domain.Outcome {
	signals := make([]domain.Signal, len(r.Signals))
	for i, sg := range r.Signals {
		signals[i] = domain.Signal{Code: sg.Code, Label: sg.Label, Weight: sg.Weight}
	}
	degraded := r.DegradedSignals
	if degraded == nil {
		degraded = []string{}
	}
	return domain.Outcome{
		RequestID: r.RequestID, EventID: r.EventID, EvaluationID: r.ID,
		Score: r.Score, Level: string(r.Level), Decision: string(r.Decision),
		EngineVersion: r.EngineVersion, Signals: signals, DegradedSignals: degraded,
	}
}

// Profile returns an entity's profile. Requires events:read.
func (s *Service) Profile(ctx context.Context, kind domain.EntityKind, ref string) (domain.Profile, error) {
	if err := authz.Require(ctx, authz.EventsRead); err != nil {
		return domain.Profile{}, err
	}
	var p domain.Profile
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		p, err = s.store.Profile(ctx, kind, ref)
		return err
	})
	return p, err
}

// Analytics is the event activity of a window.
type Analytics struct {
	Window     domain.Window
	Since      time.Time
	Series     []domain.Point
	TopUsers   []domain.TopUser
	TopSignals []risk.SignalCount
}

// Analytics returns events per bucket, the highest-risk users and the most frequent signals of
// a window. Requires events:read.
func (s *Service) Analytics(ctx context.Context, w domain.Window) (Analytics, error) {
	if err := authz.Require(ctx, authz.EventsRead); err != nil {
		return Analytics{}, err
	}
	// Buckets align to whole hours or days (UTC), so the series reads cleanly.
	since := s.clock.Now().UTC().Add(-w.Length).Truncate(w.Bucket)
	out := Analytics{Window: w, Since: since}
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if out.Series, err = s.store.Timeline(ctx, since, w.Bucket); err != nil {
			return err
		}
		if out.TopUsers, err = s.store.TopUsers(ctx, since, 10); err != nil {
			return err
		}
		out.TopSignals, err = s.risk.TopSignals(ctx, since)
		return err
	})
	return out, err
}
