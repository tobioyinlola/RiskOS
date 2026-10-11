// Package service holds the cases module's business logic.
package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/present"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/assignee"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

// Service manages cases.
type Service struct {
	store   store.Store
	runner  txn.Runner
	alerts  alerts.Linker
	audit   audit.Recorder
	clock   clock.Clock
	webhook webhooks.Publisher
	// assignees validates who a case is assigned to; nil accepts any reference.
	assignees assignee.Resolver
}

// New returns a Service.
func New(runner txn.Runner, linker alerts.Linker, rec audit.Recorder, clk clock.Clock, pub webhooks.Publisher) *Service {
	return &Service{runner: runner, alerts: linker, audit: rec, clock: clk, webhook: pub}
}

// SetAssignees makes assignment require an active dashboard user of the workspace, found by r.
func (s *Service) SetAssignees(r assignee.Resolver) { s.assignees = r }

// resolveAssignee returns the canonical reference of the user ref names, or an invalid-input
// error when the workspace has no such active user. It runs in the caller's transaction.
func (s *Service) resolveAssignee(ctx context.Context, ref string) (string, error) {
	if s.assignees == nil {
		return ref, nil
	}
	canonical, ok, err := s.assignees.ResolveAssignee(ctx, ref)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", invalid("assignee must be the email of an active dashboard user of this workspace")
	}
	return canonical, nil
}

// Detail is a case with its alerts, notes and timeline.
type Detail struct {
	Case     domain.Case
	Alerts   []alerts.Alert
	Notes    []domain.Note
	Activity []domain.Activity
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalid, fmt.Sprintf(format, args...))
}

func checkLen(field, v string, maxLen int) error {
	if utf8.RuneCountInString(v) > maxLen {
		return invalid("%s must be at most %d characters", field, maxLen)
	}
	return nil
}

// Create opens a case and escalates the given alerts into it, in one transaction. Requires
// cases:write, plus alerts:write when alerts are given (checked by the alerts module). Audited.
func (s *Service) Create(ctx context.Context, in domain.CreateInput) (domain.Case, error) {
	if err := authz.Require(ctx, authz.CasesWrite); err != nil {
		return domain.Case{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Case{}, err
	}
	in.Title, in.Assignee = strings.TrimSpace(in.Title), strings.TrimSpace(in.Assignee)
	if in.Title == "" {
		return domain.Case{}, invalid("title is required")
	}
	for _, f := range []struct {
		name, v string
		max     int
	}{{"title", in.Title, domain.MaxTitle}, {"description", in.Description, domain.MaxDescription}, {"assignee", in.Assignee, domain.MaxAssignee}} {
		if err := checkLen(f.name, f.v, f.max); err != nil {
			return domain.Case{}, err
		}
	}
	if in.Priority != "" && !domain.ValidPriority(in.Priority) {
		return domain.Case{}, invalid("priority must be minimal, low, elevated, high or critical")
	}
	if len(in.AlertIDs) > domain.MaxAlertsOnCreate {
		return domain.Case{}, invalid("at most %d alerts can be added at once", domain.MaxAlertsOnCreate)
	}
	now := s.clock.Now()
	c := domain.Case{
		ID: ids.NewV7(), Title: in.Title, Description: in.Description, Status: domain.StatusOpen,
		Priority: in.Priority, CreatedAt: now, CreatedBy: scope.Principal.ID, UpdatedAt: now,
	}
	if in.Assignee != "" {
		c.Assignee = &in.Assignee
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if c.Assignee != nil {
			canonical, err := s.resolveAssignee(ctx, *c.Assignee)
			if err != nil {
				return err
			}
			c.Assignee, in.Assignee = &canonical, canonical
		}
		derive := c.Priority == ""
		if derive {
			c.Priority = domain.DefaultPriority
		}
		// The case row must exist before alerts reference it.
		if err := s.store.Insert(ctx, c); err != nil {
			return err
		}
		var escalated []alerts.Alert
		if len(in.AlertIDs) > 0 {
			var err error
			if escalated, err = s.alerts.Escalate(ctx, in.AlertIDs, c.ID); err != nil {
				return err
			}
		}
		if derive && len(escalated) > 0 {
			c.Priority = escalated[0].Severity
			for _, a := range escalated[1:] {
				if severityRank(a.Severity) > severityRank(c.Priority) {
					c.Priority = a.Severity
				}
			}
			if err := s.store.Update(ctx, c); err != nil {
				return err
			}
		}
		alertIDs := make([]string, len(escalated))
		for i, a := range escalated {
			alertIDs[i] = ids.Format(ids.PrefixAlert, a.ID)
		}
		if err := s.store.AddActivity(ctx, c.ID, "created", scope.Principal.ID, map[string]any{
			"priority": c.Priority, "assignee": in.Assignee, "alert_ids": alertIDs,
		}, now); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "case.created", ResourceType: "case", ResourceID: ids.Format(ids.PrefixCase, c.ID),
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{"priority": c.Priority, "alert_ids": alertIDs},
		}); err != nil {
			return err
		}
		return s.webhook.Publish(ctx, webhooks.EventCaseCreated, map[string]any{"case": present.ToAPI(c), "alert_ids": alertIDs})
	})
	if err != nil {
		return domain.Case{}, err
	}
	return c, nil
}

// List returns a newest-first page of cases. Requires cases:read.
func (s *Service) List(ctx context.Context, f domain.Filter) ([]domain.Case, error) {
	if err := authz.Require(ctx, authz.CasesRead); err != nil {
		return nil, err
	}
	var out []domain.Case
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.List(ctx, f)
		return err
	})
	return out, err
}

// Get returns a case with its alerts, notes and timeline. Requires cases:read: a case's alerts
// are part of the investigation, so they are shown without alerts:read.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Detail, error) {
	if err := authz.Require(ctx, authz.CasesRead); err != nil {
		return Detail{}, err
	}
	var d Detail
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if d.Case, err = s.store.Get(ctx, id, false); err != nil {
			return err
		}
		if d.Alerts, err = s.alerts.ListByCase(ctx, id); err != nil {
			return err
		}
		if d.Notes, err = s.store.Notes(ctx, id); err != nil {
			return err
		}
		d.Activity, err = s.store.Activity(ctx, id)
		return err
	})
	return d, err
}

// Update changes a case. Resolving requires a resolution and closes the case's alerts; a
// resolved case is final. A change that alters nothing is a no-op. Requires cases:write; audited.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p domain.Patch) (domain.Case, error) {
	if err := authz.Require(ctx, authz.CasesWrite); err != nil {
		return domain.Case{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Case{}, err
	}
	if err := validatePatch(&p); err != nil {
		return domain.Case{}, err
	}
	var out domain.Case
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.store.Get(ctx, id, true)
		if err != nil {
			return err
		}
		if cur.Status == domain.StatusResolved {
			return fmt.Errorf("%w: the case is resolved; add a note instead", domain.ErrConflict)
		}
		// Only a change of assignee is checked: a legacy value kept as it is stays valid.
		if p.Assignee != nil && *p.Assignee != "" && *p.Assignee != deref(cur.Assignee) {
			canonical, err := s.resolveAssignee(ctx, *p.Assignee)
			if err != nil {
				return err
			}
			p.Assignee = &canonical
		}
		next, changes := apply(cur, p)
		if len(changes) == 0 {
			out = cur
			return nil
		}
		now := s.clock.Now()
		next.UpdatedAt = now
		resolving := next.Status == domain.StatusResolved
		if resolving {
			next.ResolvedAt, next.ResolvedBy = &now, &scope.Principal.ID
		}
		if err := s.store.Update(ctx, next); err != nil {
			return err
		}
		action := "updated"
		if resolving {
			action = "resolved"
			if err := s.alerts.CloseForCase(ctx, id, *next.Resolution == domain.ResolutionFalsePositive); err != nil {
				return err
			}
		}
		if err := s.store.AddActivity(ctx, id, action, scope.Principal.ID, changes, now); err != nil {
			return err
		}
		// Free text (descriptions, resolution notes) stays in the case; the audit log records
		// which fields changed and the state transitions.
		if err := s.audit.Record(ctx, audit.Entry{
			Action: "case." + action, ResourceType: "case", ResourceID: ids.Format(ids.PrefixCase, id),
			Outcome: audit.OutcomeSuccess, Metadata: auditSafe(changes),
		}); err != nil {
			return err
		}
		out = next
		if resolving {
			return s.webhook.Publish(ctx, webhooks.EventCaseResolved, map[string]any{"case": present.ToAPI(next)})
		}
		return nil
	})
	return out, err
}

// AddNote appends a note, also to resolved cases. Requires cases:write; audited (without the
// note's text).
func (s *Service) AddNote(ctx context.Context, caseID uuid.UUID, body string) (domain.Note, error) {
	if err := authz.Require(ctx, authz.CasesWrite); err != nil {
		return domain.Note{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Note{}, err
	}
	if strings.TrimSpace(body) == "" {
		return domain.Note{}, invalid("note body is required")
	}
	if err := checkLen("note body", body, domain.MaxNote); err != nil {
		return domain.Note{}, err
	}
	n := domain.Note{ID: ids.NewV7(), Body: body, Author: scope.Principal.ID, CreatedAt: s.clock.Now()}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if _, err := s.store.Get(ctx, caseID, true); err != nil {
			return err
		}
		if err := s.store.AddNote(ctx, caseID, n); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "case.note_added", ResourceType: "case", ResourceID: ids.Format(ids.PrefixCase, caseID),
			Outcome:  audit.OutcomeSuccess,
			Metadata: map[string]any{"note_id": ids.Format(ids.PrefixCaseNote, n.ID), "length": utf8.RuneCountInString(body)},
		})
	})
	if err != nil {
		return domain.Note{}, err
	}
	return n, nil
}

func validatePatch(p *domain.Patch) error {
	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		if t == "" {
			return invalid("title cannot be empty")
		}
		p.Title = &t
		if err := checkLen("title", t, domain.MaxTitle); err != nil {
			return err
		}
	}
	if p.Description != nil {
		if err := checkLen("description", *p.Description, domain.MaxDescription); err != nil {
			return err
		}
	}
	if p.Assignee != nil {
		a := strings.TrimSpace(*p.Assignee)
		p.Assignee = &a
		if err := checkLen("assignee", a, domain.MaxAssignee); err != nil {
			return err
		}
	}
	if p.Priority != nil && !domain.ValidPriority(*p.Priority) {
		return invalid("priority must be minimal, low, elevated, high or critical")
	}
	if p.ResolutionNote != nil {
		if err := checkLen("resolution_note", *p.ResolutionNote, domain.MaxResolutionNote); err != nil {
			return err
		}
	}
	if p.Status != nil {
		switch *p.Status {
		case domain.StatusOpen, domain.StatusInvestigating, domain.StatusResolved:
		default:
			return invalid("status must be open, investigating or resolved")
		}
	}
	resolving := p.Status != nil && *p.Status == domain.StatusResolved
	if p.Resolution != nil && !p.Resolution.Valid() {
		return invalid("resolution must be confirmed_fraud, false_positive, legitimate or inconclusive")
	}
	if resolving && p.Resolution == nil {
		return invalid("a resolution is required to resolve a case")
	}
	if !resolving && (p.Resolution != nil || p.ResolutionNote != nil) {
		return invalid("resolution and resolution_note can only be set when resolving (status: resolved)")
	}
	return nil
}

// apply returns cur with the patch applied and the changed fields (before and after).
func apply(cur domain.Case, p domain.Patch) (domain.Case, map[string]any) {
	next := cur
	changes := map[string]any{}
	set := func(field string, before, after any) {
		changes[field] = map[string]any{"before": before, "after": after}
	}
	if p.Title != nil && *p.Title != cur.Title {
		next.Title = *p.Title
		set("title", cur.Title, next.Title)
	}
	if p.Description != nil && *p.Description != cur.Description {
		next.Description = *p.Description
		set("description", nil, nil) // free text: recorded as changed, not copied
	}
	if p.Priority != nil && *p.Priority != cur.Priority {
		next.Priority = *p.Priority
		set("priority", cur.Priority, next.Priority)
	}
	if p.Assignee != nil && *p.Assignee != deref(cur.Assignee) {
		next.Assignee = nil
		if *p.Assignee != "" {
			next.Assignee = p.Assignee
		}
		set("assignee", deref(cur.Assignee), *p.Assignee)
	}
	if p.Status != nil && *p.Status != cur.Status {
		next.Status = *p.Status
		set("status", string(cur.Status), string(next.Status))
	}
	if p.Resolution != nil {
		next.Resolution = p.Resolution
		set("resolution", nil, string(*p.Resolution))
	}
	if p.ResolutionNote != nil {
		next.ResolutionNote = p.ResolutionNote
	}
	return next, changes
}

// auditSafe drops free-text values from a change set.
func auditSafe(changes map[string]any) map[string]any {
	out := make(map[string]any, len(changes))
	for k, v := range changes {
		if k == "title" || k == "description" {
			out[k] = "changed"
			continue
		}
		out[k] = v
	}
	return out
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "elevated":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
