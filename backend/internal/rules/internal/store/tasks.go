package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store/sqlcgen"
)

// Task is a piece of compliance work: a report to prepare or an action a control requires.
type Task struct {
	ID                            uuid.UUID
	Kind, RuleID                  string
	EvaluationID, EventID, CaseID *uuid.UUID
	SubjectRef                    *string
	Sensitive                     bool
	Status                        string
	DueAt                         *time.Time
	Report                        map[string]any
	CreatedAt                     time.Time
	CreatedBy                     string
	UpdatedAt                     time.Time
	ApprovedBy, ClosedBy          *string
	ApprovedAt, ClosedAt          *time.Time
	CloseNote, FilingRef          *string
}

func taskOf(r sqlcgen.ComplianceTask) (Task, error) {
	t := Task{
		ID: r.ID, Kind: r.Kind, RuleID: r.RuleID, EvaluationID: r.EvaluationID, EventID: r.EventID, CaseID: r.CaseID,
		SubjectRef: r.SubjectRef, Sensitive: r.Sensitive, Status: r.Status, DueAt: r.DueAt, CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, ApprovedBy: r.ApprovedBy, ApprovedAt: r.ApprovedAt,
		ClosedBy: r.ClosedBy, ClosedAt: r.ClosedAt, CloseNote: r.CloseNote, FilingRef: r.FilingRef,
	}
	if err := json.Unmarshal(r.Report, &t.Report); err != nil {
		return Task{}, fmt.Errorf("decode report: %w", err)
	}
	return t, nil
}

// InsertTask stores a new task; created is false when the task already exists (a retried job).
func (Store) InsertTask(ctx context.Context, t Task) (created bool, err error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	report, err := json.Marshal(t.Report)
	if err != nil {
		return false, err
	}
	n, err := q.InsertTask(ctx, sqlcgen.InsertTaskParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: t.ID, Kind: t.Kind, RuleID: t.RuleID,
		EvaluationID: t.EvaluationID, EventID: t.EventID, CaseID: t.CaseID, SubjectRef: t.SubjectRef,
		Sensitive: t.Sensitive, DueAt: t.DueAt, Report: report, CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy,
	})
	if err != nil {
		return false, fmt.Errorf("insert compliance task: %w", err)
	}
	return n == 1, nil
}

// Task returns a task, locked until the transaction ends.
func (Store) Task(ctx context.Context, id uuid.UUID) (Task, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return Task{}, err
	}
	r, err := q.GetTask(ctx, sqlcgen.GetTaskParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("get compliance task: %w", err)
	}
	return taskOf(r)
}

// Tasks lists tasks, open ones first by deadline; sensitive ones only when asked.
func (Store) Tasks(ctx context.Context, status string, includeSensitive bool) ([]Task, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	p := sqlcgen.ListTasksParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, IncludeSensitive: includeSensitive}
	if status != "" {
		p.Status = &status
	}
	rows, err := q.ListTasks(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list compliance tasks: %w", err)
	}
	out := make([]Task, len(rows))
	for i, r := range rows {
		if out[i], err = taskOf(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// UpdateTask writes a task's status, report and decisions.
func (Store) UpdateTask(ctx context.Context, t Task) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	report, err := json.Marshal(t.Report)
	if err != nil {
		return err
	}
	if err := q.UpdateTask(ctx, sqlcgen.UpdateTaskParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: t.ID, Status: t.Status, Report: report,
		UpdatedAt: t.UpdatedAt, ApprovedBy: t.ApprovedBy, ApprovedAt: t.ApprovedAt, ClosedBy: t.ClosedBy,
		ClosedAt: t.ClosedAt, CloseNote: t.CloseNote, FilingRef: t.FilingRef,
	}); err != nil {
		return fmt.Errorf("update compliance task: %w", err)
	}
	return nil
}
