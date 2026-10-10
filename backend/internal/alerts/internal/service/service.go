// Package service holds the alerts module's business logic: raising and grouping alerts from
// evaluations, triage, and the case linkage used by the cases module.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/present"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/assignee"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

// Bounds of free-text fields.
const (
	maxAssignee = 254
	maxNote     = 2000
)

// Service raises and triages alerts.
type Service struct {
	store   store.Store
	runner  txn.Runner
	audit   audit.Recorder
	clock   clock.Clock
	webhook webhooks.Publisher
	// assignees validates who an alert is assigned to; nil accepts any reference.
	assignees assignee.Resolver
}

// New returns a Service.
func New(runner txn.Runner, rec audit.Recorder, clk clock.Clock, pub webhooks.Publisher) *Service {
	return &Service{runner: runner, audit: rec, clock: clk, webhook: pub}
}

// SetAssignees makes assignment require an active dashboard user of the workspace, found by r.
func (s *Service) SetAssignees(r assignee.Resolver) { s.assignees = r }

// resolveAssignee returns the canonical reference of the user ref names, or ErrInvalid when the
// workspace has no such active user. It runs in the caller's transaction.
func (s *Service) resolveAssignee(ctx context.Context, ref string) (string, error) {
	if s.assignees == nil {
		return ref, nil
	}
	canonical, ok, err := s.assignees.ResolveAssignee(ctx, ref)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%w: assignee must be the email of an active dashboard user of this workspace", domain.ErrInvalid)
	}
	return canonical, nil
}

// publish sends an alert event to subscribed webhook endpoints, in the caller's transaction.
func (s *Service) publish(ctx context.Context, eventType webhooks.EventType, id uuid.UUID, previous *domain.Status) error {
	a, err := s.store.Get(ctx, id, false)
	if err != nil {
		return err
	}
	data := map[string]any{"alert": present.ToAPI(a)}
	if previous != nil {
		data["previous_status"] = string(*previous)
	}
	return s.webhook.Publish(ctx, eventType, data)
}

// Raise creates or extends the alerts an evaluation triggers. It is idempotent per evaluation
// and alert kind, so a retried job changes nothing. Requires the system permission
// authz.AlertsRaise (background jobs only).
func (s *Service) Raise(ctx context.Context, t domain.Trigger) error {
	if err := authz.Require(ctx, authz.AlertsRaise); err != nil {
		return err
	}
	return s.runner.Run(ctx, func(ctx context.Context) error {
		if t.Risk {
			subject := t.UserRef
			if subject == "" {
				subject = "unknown"
			}
			if err := s.raise(ctx, domain.KindRisk, domain.SubjectUser, subject, t.Level, t); err != nil {
				return err
			}
		}
		if t.Security {
			if err := s.raise(ctx, domain.KindSecurity, domain.SubjectEnvironment, domain.SubjectDegraded, "high", t); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) raise(ctx context.Context, kind domain.Kind, subjectType, subjectRef, severity string, t domain.Trigger) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	if err := s.store.LockSubject(ctx, kind, subjectRef); err != nil {
		return err
	}
	done, err := s.store.Alerted(ctx, t.EvaluationID, kind)
	if err != nil || done {
		return err
	}
	now := s.clock.Now()
	group, err := s.store.FindGrouping(ctx, kind, subjectRef, t.EvaluatedAt.Add(-domain.GroupWindow))
	if err != nil {
		return err
	}
	var alertID uuid.UUID
	created := group == nil
	if group != nil {
		alertID = group.ID
		sev := domain.MaxSeverity(group.Severity, severity)
		if err := s.store.AddToAlert(ctx, alertID, sev, domain.Title(kind, sev), t.Score, t.EvaluatedAt, now); err != nil {
			return err
		}
		if err := s.store.AddActivity(ctx, alertID, "evaluation_added", scope.Principal.ID, map[string]any{
			"evaluation_id": ids.Format(ids.PrefixEvaluation, t.EvaluationID), "risk_score": t.Score, "risk_level": t.Level,
		}, now); err != nil {
			return err
		}
	} else {
		alertID = ids.NewV7()
		a := domain.Alert{
			ID: alertID, Kind: kind, SubjectType: subjectType, SubjectRef: subjectRef,
			Title: domain.Title(kind, severity), Severity: severity, MaxScore: t.Score, EvaluationCount: 1,
			FirstSeenAt: t.EvaluatedAt, CreatedAt: now,
		}
		if err := s.store.InsertAlert(ctx, a); err != nil {
			return err
		}
		if err := s.store.AddActivity(ctx, alertID, "created", scope.Principal.ID, map[string]any{
			"evaluation_id": ids.Format(ids.PrefixEvaluation, t.EvaluationID), "risk_score": t.Score, "risk_level": t.Level,
		}, now); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "alert.created", ResourceType: "alert", ResourceID: ids.Format(ids.PrefixAlert, alertID),
			Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{
				"kind": string(kind), "severity": severity, "evaluation_id": ids.Format(ids.PrefixEvaluation, t.EvaluationID),
			},
		}); err != nil {
			return err
		}
	}
	if err := s.store.LinkEvaluation(ctx, alertID, kind, domain.Evaluation{
		EvaluationID: t.EvaluationID, EventID: t.EventID, EventType: t.EventType, Score: t.Score,
		Level: t.Level, Decision: t.Decision, Signals: nonNil(t.Signals), Degraded: nonNilStrings(t.Degraded),
		EvaluatedAt: t.EvaluatedAt,
	}); err != nil {
		return err
	}
	if created {
		return s.publish(ctx, webhooks.EventAlertCreated, alertID, nil)
	}
	return nil
}

// RaiseWebhookDisabled raises a security alert for a webhook endpoint that was disabled after
// failing, unless one is already open for it. Runs in the caller's transaction; requires
// authz.AlertsRaise.
func (s *Service) RaiseWebhookDisabled(ctx context.Context, endpointID uuid.UUID) error {
	if err := authz.Require(ctx, authz.AlertsRaise); err != nil {
		return err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	ref := ids.Format(ids.PrefixWebhook, endpointID)
	return s.runner.Run(ctx, func(ctx context.Context) error {
		if err := s.store.LockSubject(ctx, domain.KindSecurity, ref); err != nil {
			return err
		}
		now := s.clock.Now()
		open, err := s.store.FindGrouping(ctx, domain.KindSecurity, ref, time.Time{})
		if err != nil || open != nil {
			return err
		}
		id := ids.NewV7()
		if err := s.store.InsertAlert(ctx, domain.Alert{
			ID: id, Kind: domain.KindSecurity, SubjectType: domain.SubjectWebhook, SubjectRef: ref,
			Title: "Webhook endpoint disabled after repeated failed deliveries", Severity: "elevated",
			FirstSeenAt: now, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := s.store.AddActivity(ctx, id, "created", scope.Principal.ID, map[string]any{"webhook_id": ref}, now); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "alert.created", ResourceType: "alert", ResourceID: ids.Format(ids.PrefixAlert, id),
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{"kind": "security", "webhook_id": ref},
		}); err != nil {
			return err
		}
		return s.publish(ctx, webhooks.EventAlertCreated, id, nil)
	})
}

// List returns a newest-first page of alerts. Requires alerts:read.
func (s *Service) List(ctx context.Context, f domain.Filter) ([]domain.Alert, error) {
	if err := authz.Require(ctx, authz.AlertsRead); err != nil {
		return nil, err
	}
	var out []domain.Alert
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.List(ctx, f)
		return err
	})
	return out, err
}

// Get returns an alert with its newest evaluations and its timeline. Requires alerts:read.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Detail, error) {
	if err := authz.Require(ctx, authz.AlertsRead); err != nil {
		return domain.Detail{}, err
	}
	var d domain.Detail
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if d.Alert, err = s.store.Get(ctx, id, false); err != nil {
			return err
		}
		if d.Evaluations, err = s.store.Evaluations(ctx, id); err != nil {
			return err
		}
		d.Activity, err = s.store.Activity(ctx, id)
		return err
	})
	return d, err
}

// Update applies a triage change (status, assignee, with an optional note). A change that alters
// nothing is a no-op. Requires alerts:write; audited.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p domain.Patch) (domain.Alert, error) {
	if err := authz.Require(ctx, authz.AlertsWrite); err != nil {
		return domain.Alert{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Alert{}, err
	}
	if p.Assignee != nil {
		trimmed := strings.TrimSpace(*p.Assignee)
		if utf8.RuneCountInString(trimmed) > maxAssignee {
			return domain.Alert{}, fmt.Errorf("%w: assignee must be at most %d characters", domain.ErrInvalid, maxAssignee)
		}
		p.Assignee = &trimmed
	}
	if p.Note != nil && utf8.RuneCountInString(*p.Note) > maxNote {
		return domain.Alert{}, fmt.Errorf("%w: note must be at most %d characters", domain.ErrInvalid, maxNote)
	}
	var out domain.Alert
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.store.Get(ctx, id, true)
		if err != nil {
			return err
		}
		status, assignee := cur.Status, cur.Assignee
		if p.Status != nil {
			if err := domain.CheckTransition(cur.Status, *p.Status); err != nil {
				return err
			}
			status = *p.Status
		}
		if p.Assignee != nil {
			if cur.Status.Terminal() || cur.Status == domain.StatusEscalated {
				return fmt.Errorf("%w: the alert is %s; assign its case instead", domain.ErrConflict, cur.Status)
			}
			assignee = nil
			if *p.Assignee != "" {
				assignee = p.Assignee
				// Only a change of assignee is checked: a legacy value kept as it is stays valid.
				if *p.Assignee != deref(cur.Assignee) {
					canonical, err := s.resolveAssignee(ctx, *p.Assignee)
					if err != nil {
						return err
					}
					assignee = &canonical
				}
			}
		}
		statusChanged := status != cur.Status
		assigneeChanged := deref(assignee) != deref(cur.Assignee)
		noted := p.Note != nil && strings.TrimSpace(*p.Note) != ""
		if !statusChanged && !assigneeChanged && !noted {
			out = cur
			return nil
		}
		now := s.clock.Now()
		if statusChanged || assigneeChanged {
			if err := s.store.SetTriage(ctx, id, status, assignee, now); err != nil {
				return err
			}
		}
		detail := map[string]any{}
		if statusChanged {
			detail["status_before"], detail["status_after"] = string(cur.Status), string(status)
		}
		if assigneeChanged {
			detail["assignee_before"], detail["assignee_after"] = deref(cur.Assignee), deref(assignee)
		}
		activity := detail
		if noted {
			activity = map[string]any{"note": strings.TrimSpace(*p.Note)}
			for k, v := range detail {
				activity[k] = v
			}
		}
		if err := s.store.AddActivity(ctx, id, "updated", scope.Principal.ID, activity, now); err != nil {
			return err
		}
		// The note text stays in the alert's timeline; the audit log records only that it exists.
		detail["note_added"] = noted
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "alert.updated", ResourceType: "alert", ResourceID: ids.Format(ids.PrefixAlert, id),
			Outcome: audit.OutcomeSuccess, Metadata: detail,
		}); err != nil {
			return err
		}
		if statusChanged {
			if err := s.publish(ctx, webhooks.EventAlertUpdated, id, &cur.Status); err != nil {
				return err
			}
		}
		out, err = s.store.Get(ctx, id, false)
		return err
	})
	return out, err
}

// Escalate attaches open or investigating alerts to a case, inside the caller's transaction.
// Requires alerts:write (and the caller's own cases:write check).
func (s *Service) Escalate(ctx context.Context, alertIDs []uuid.UUID, caseID uuid.UUID) ([]domain.Alert, error) {
	if err := authz.Require(ctx, authz.AlertsWrite); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Alert, 0, len(alertIDs))
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		for _, id := range alertIDs {
			cur, err := s.store.Get(ctx, id, true)
			if err != nil {
				return err
			}
			if !cur.Status.Groupable() {
				return fmt.Errorf("%w: alert %s is %s and cannot be added to a case",
					domain.ErrConflict, ids.Format(ids.PrefixAlert, id), cur.Status)
			}
			if err := s.store.Escalate(ctx, id, caseID, now); err != nil {
				return err
			}
			if err := s.store.AddActivity(ctx, id, "escalated", scope.Principal.ID, map[string]any{
				"case_id": ids.Format(ids.PrefixCase, caseID), "status_before": string(cur.Status),
			}, now); err != nil {
				return err
			}
			if err := s.publish(ctx, webhooks.EventAlertUpdated, id, &cur.Status); err != nil {
				return err
			}
			cur.Status, cur.CaseID = domain.StatusEscalated, &caseID
			out = append(out, cur)
		}
		return nil
	})
	return out, err
}

// CloseForCase closes the escalated alerts of a resolved case: false_positive when the case was
// a false positive, resolved otherwise. Runs in the caller's transaction; the caller authorizes.
func (s *Service) CloseForCase(ctx context.Context, caseID uuid.UUID, falsePositive bool) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	status := domain.StatusResolved
	if falsePositive {
		status = domain.StatusFalsePositive
	}
	return s.runner.Run(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		closed, err := s.store.CloseForCase(ctx, caseID, status, now)
		if err != nil {
			return err
		}
		escalated := domain.StatusEscalated
		for _, id := range closed {
			if err := s.store.AddActivity(ctx, id, "closed_with_case", scope.Principal.ID, map[string]any{
				"case_id": ids.Format(ids.PrefixCase, caseID), "status_after": string(status),
			}, now); err != nil {
				return err
			}
			if err := s.publish(ctx, webhooks.EventAlertUpdated, id, &escalated); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListByCase returns a case's alerts, in the caller's transaction; the caller authorizes.
func (s *Service) ListByCase(ctx context.Context, caseID uuid.UUID) ([]domain.Alert, error) {
	var out []domain.Alert
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.ListByCase(ctx, caseID)
		return err
	})
	return out, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
