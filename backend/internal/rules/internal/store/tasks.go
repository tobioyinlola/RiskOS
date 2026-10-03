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
	SubjectRef, AccountRef        *string
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
		SubjectRef: r.SubjectRef, AccountRef: r.AccountRef, Sensitive: r.Sensitive, Status: r.Status, DueAt: r.DueAt, CreatedAt: r.CreatedAt,
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
		AccountRef: t.AccountRef, Sensitive: t.Sensitive, DueAt: t.DueAt, Report: report, CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy,
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

// TaskCaseIDs returns the cases with a task of rule and kind that was not dismissed.
func (Store) TaskCaseIDs(ctx context.Context, rule, kind string) ([]uuid.UUID, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	out, err := q.TaskCaseIDs(ctx, sqlcgen.TaskCaseIDsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RuleID: rule, Kind: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("list task cases: %w", err)
	}
	return out, nil
}

// Account is what the register knows of an account's BVN (REG-16).
type Account struct {
	Ref         string
	SubjectRef  *string
	HasBVN      bool
	FirstSeenAt time.Time
}

// RecordAccount records an event's BVN fact about an account. The register keeps the earliest
// time the account was seen and the latest fact by event time.
func (Store) RecordAccount(ctx context.Context, ref string, subject *string, hasBVN bool, observedAt time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.UpsertComplianceAccount(ctx, sqlcgen.UpsertComplianceAccountParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, AccountRef: ref, SubjectRef: subject,
		HasBvn: hasBVN, ObservedAt: observedAt,
	}); err != nil {
		return fmt.Errorf("record compliance account: %w", err)
	}
	return nil
}

// AccountsWithoutTask returns accounts without a BVN that never had a task of rule and kind,
// oldest first, at most limit.
func (Store) AccountsWithoutTask(ctx context.Context, rule, kind string, limit int32) ([]Account, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.AccountsWithoutTask(ctx, sqlcgen.AccountsWithoutTaskParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RuleID: rule, Kind: kind, RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list accounts without a task: %w", err)
	}
	out := make([]Account, len(rows))
	for i, r := range rows {
		out[i] = Account{Ref: r.AccountRef, SubjectRef: r.SubjectRef, HasBVN: r.HasBvn, FirstSeenAt: r.FirstSeenAt}
	}
	return out, nil
}

// OpenTasksOfAccountsWithBVN returns open tasks of rule and kind whose account now has a BVN,
// locked until the transaction ends, at most limit.
func (Store) OpenTasksOfAccountsWithBVN(ctx context.Context, rule, kind string, limit int32) ([]Task, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.OpenTasksOfAccountsWithBVN(ctx, sqlcgen.OpenTasksOfAccountsWithBVNParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RuleID: rule, Kind: kind, RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks of accounts with a BVN: %w", err)
	}
	out := make([]Task, len(rows))
	for i, r := range rows {
		if out[i], err = taskOf(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DBTX runs queries outside a tenant transaction (a connection pool).
type DBTX = sqlcgen.DBTX

// Scope is a tenant environment.
type Scope struct{ TenantID, EnvironmentID uuid.UUID }

// AccountScopes returns every environment with accounts in the register. It runs outside any
// tenant scope, on db, and returns identifiers only.
func AccountScopes(ctx context.Context, db DBTX) ([]Scope, error) {
	rows, err := sqlcgen.New(db).AccountScopes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list account scopes: %w", err)
	}
	out := make([]Scope, len(rows))
	for i, r := range rows {
		out[i] = Scope{TenantID: r.TenantID, EnvironmentID: r.EnvironmentID}
	}
	return out, nil
}
