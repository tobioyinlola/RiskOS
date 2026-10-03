// Package store is the only code that touches the cases module's tables. Every query runs in the
// caller's tenant transaction and names tenant and environment explicitly.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Store accesses cases.
type Store struct{}

func scoped(ctx context.Context) (*sqlcgen.Queries, tenancy.Scope, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	return sqlcgen.New(tx), scope, nil
}

// Insert creates an open case.
func (Store) Insert(ctx context.Context, c domain.Case) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertCase(ctx, sqlcgen.InsertCaseParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: c.ID, Title: c.Title,
		Description: c.Description, Priority: c.Priority, Assignee: c.Assignee, Now: c.CreatedAt, CreatedBy: c.CreatedBy,
	}); err != nil {
		return fmt.Errorf("insert case: %w", err)
	}
	return nil
}

// Get returns a case; forUpdate locks it for the rest of the transaction.
func (Store) Get(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Case, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Case{}, err
	}
	var row sqlcgen.Case
	if forUpdate {
		row, err = q.GetCaseForUpdate(ctx, sqlcgen.GetCaseForUpdateParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	} else {
		row, err = q.GetCase(ctx, sqlcgen.GetCaseParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Case{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Case{}, fmt.Errorf("get case: %w", err)
	}
	return toDomain(row), nil
}

// List returns a newest-first page of cases.
func (Store) List(ctx context.Context, f domain.Filter) ([]domain.Case, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	p := sqlcgen.ListCasesParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, BeforeID: f.Before, RowLimit: int32(f.Limit), //nolint:gosec // bounded page size
	}
	if f.Status != nil {
		p.Status = new(string(*f.Status))
	}
	if f.Resolution != nil {
		p.Resolution = new(string(*f.Resolution))
	}
	rows, err := q.ListCases(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list cases: %w", err)
	}
	out := make([]domain.Case, len(rows))
	for i, r := range rows {
		out[i] = toDomain(r)
	}
	return out, nil
}

// Update stores every mutable field of c.
func (Store) Update(ctx context.Context, c domain.Case) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	var resolution *string
	if c.Resolution != nil {
		resolution = new(string(*c.Resolution))
	}
	if err := q.UpdateCase(ctx, sqlcgen.UpdateCaseParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: c.ID, Title: c.Title,
		Description: c.Description, Status: string(c.Status), Priority: c.Priority, Assignee: c.Assignee,
		Resolution: resolution, ResolutionNote: c.ResolutionNote, ResolvedAt: c.ResolvedAt, ResolvedBy: c.ResolvedBy,
		Now: c.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("update case: %w", err)
	}
	return nil
}

// AddNote appends a note.
func (Store) AddNote(ctx context.Context, caseID uuid.UUID, n domain.Note) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertCaseNote(ctx, sqlcgen.InsertCaseNoteParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: n.ID, CaseID: caseID,
		Body: n.Body, Author: n.Author, CreatedAt: n.CreatedAt,
	}); err != nil {
		return fmt.Errorf("insert case note: %w", err)
	}
	return nil
}

// Notes returns a case's notes, oldest first.
func (Store) Notes(ctx context.Context, caseID uuid.UUID) ([]domain.Note, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCaseNotes(ctx, sqlcgen.ListCaseNotesParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, CaseID: caseID})
	if err != nil {
		return nil, fmt.Errorf("list case notes: %w", err)
	}
	out := make([]domain.Note, len(rows))
	for i, r := range rows {
		out[i] = domain.Note{ID: r.ID, Body: r.Body, Author: r.Author, CreatedAt: r.CreatedAt}
	}
	return out, nil
}

// AddActivity appends to a case's timeline.
func (Store) AddActivity(ctx context.Context, caseID uuid.UUID, action, actor string, detail map[string]any, at time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if detail == nil {
		detail = map[string]any{}
	}
	enc, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("encode activity: %w", err)
	}
	if err := q.InsertCaseActivity(ctx, sqlcgen.InsertCaseActivityParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: ids.NewV7(), CaseID: caseID,
		Action: action, Actor: actor, Detail: enc, CreatedAt: at,
	}); err != nil {
		return fmt.Errorf("insert case activity: %w", err)
	}
	return nil
}

// Activity returns a case's timeline, oldest first.
func (Store) Activity(ctx context.Context, caseID uuid.UUID) ([]domain.Activity, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCaseActivity(ctx, sqlcgen.ListCaseActivityParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, CaseID: caseID})
	if err != nil {
		return nil, fmt.Errorf("list case activity: %w", err)
	}
	out := make([]domain.Activity, len(rows))
	for i, r := range rows {
		out[i] = domain.Activity{ID: r.ID, Action: r.Action, Actor: r.Actor, CreatedAt: r.CreatedAt}
		if err := json.Unmarshal(r.Detail, &out[i].Detail); err != nil {
			return nil, fmt.Errorf("decode activity: %w", err)
		}
	}
	return out, nil
}

func toDomain(r sqlcgen.Case) domain.Case {
	c := domain.Case{
		ID: r.ID, Title: r.Title, Description: r.Description, Status: domain.Status(r.Status), Priority: r.Priority,
		Assignee: r.Assignee, ResolutionNote: r.ResolutionNote, ResolvedAt: r.ResolvedAt, ResolvedBy: r.ResolvedBy,
		CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt,
	}
	if r.Resolution != nil {
		c.Resolution = new(domain.Resolution(*r.Resolution))
	}
	return c
}
