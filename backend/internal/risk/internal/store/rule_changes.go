package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store/sqlcgen"
)

// maxChanges bounds a listing of rule changes.
const maxChanges = 200

func int32Ptr(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p) //nolint:gosec // versions are small
	return &v
}

func intPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

// InsertChange records a proposed rule change. A second pending change for the same rule or
// code is refused (domain.ErrChangePending).
func (Store) InsertChange(ctx context.Context, c domain.RuleChange) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	v := c.Proposed
	err = q.InsertRuleChange(ctx, sqlcgen.InsertRuleChangeParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: c.ID, RuleID: c.RuleID, Code: c.Code,
		Kind: string(c.Kind), BaseVersion: int32Ptr(c.BaseVersion), Name: v.Name, Description: v.Description,
		Severity: v.Severity, Weight: int16(v.Weight), Mode: string(v.Mode), Enabled: v.Enabled, //nolint:gosec // 0..100
		Condition: v.Condition, RestoredFrom: int32Ptr(v.RestoredFrom), ProposedBy: c.ProposedBy, ProposedAt: c.ProposedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrChangePending
	}
	if err != nil {
		return fmt.Errorf("insert rule change: %w", err)
	}
	return nil
}

// GetChange returns a rule change; with lock, the row is locked until the transaction ends.
func (Store) GetChange(ctx context.Context, id uuid.UUID, lock bool) (domain.RuleChange, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.RuleChange{}, err
	}
	var row sqlcgen.RuleChange
	if lock {
		row, err = q.LockRuleChange(ctx, sqlcgen.LockRuleChangeParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	} else {
		row, err = q.GetRuleChange(ctx, sqlcgen.GetRuleChangeParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RuleChange{}, domain.ErrChangeNotFound
	}
	if err != nil {
		return domain.RuleChange{}, fmt.Errorf("get rule change: %w", err)
	}
	return toChange(row), nil
}

// ListChanges returns the newest rule changes, optionally only those in status.
func (Store) ListChanges(ctx context.Context, status *domain.ChangeStatus) ([]domain.RuleChange, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	var st *string
	if status != nil {
		st = new(string(*status))
	}
	rows, err := q.ListRuleChanges(ctx, sqlcgen.ListRuleChangesParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Status: st, Lim: maxChanges,
	})
	if err != nil {
		return nil, fmt.Errorf("list rule changes: %w", err)
	}
	out := make([]domain.RuleChange, len(rows))
	for i, r := range rows {
		out[i] = toChange(r)
	}
	return out, nil
}

// PendingChanges maps rule IDs to the ID of their pending change.
func (Store) PendingChanges(ctx context.Context) (map[uuid.UUID]uuid.UUID, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPendingRuleChanges(ctx, sqlcgen.ListPendingRuleChangesParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID})
	if err != nil {
		return nil, fmt.Errorf("list pending rule changes: %w", err)
	}
	out := make(map[uuid.UUID]uuid.UUID, len(rows))
	for _, r := range rows {
		out[r.RuleID] = r.ID
	}
	return out, nil
}

// DecideChange records a decision on a pending change.
func (Store) DecideChange(ctx context.Context, id uuid.UUID, status domain.ChangeStatus, by string, at time.Time, reason *string, applied *int) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	n, err := q.DecideRuleChange(ctx, sqlcgen.DecideRuleChangeParams{
		Status: string(status), DecidedBy: &by, DecidedAt: &at, Reason: reason, AppliedVersion: int32Ptr(applied),
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id,
	})
	if err != nil {
		return fmt.Errorf("decide rule change: %w", err)
	}
	if n != 1 {
		return domain.ErrChangeDecided
	}
	return nil
}

// CodeExists reports whether a rule with code exists in the environment.
func (Store) CodeExists(ctx context.Context, code string) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	ok, err := q.RuleCodeExists(ctx, sqlcgen.RuleCodeExistsParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Code: code})
	if err != nil {
		return false, fmt.Errorf("check rule code: %w", err)
	}
	return ok, nil
}

func toChange(r sqlcgen.RuleChange) domain.RuleChange {
	return domain.RuleChange{
		ID: r.ID, RuleID: r.RuleID, Code: r.Code, Kind: domain.ChangeKind(r.Kind), BaseVersion: intPtr(r.BaseVersion),
		Proposed: domain.RuleVersion{
			Name: r.Name, Description: r.Description, Severity: r.Severity, Weight: int(r.Weight),
			Mode: domain.RuleMode(r.Mode), Enabled: r.Enabled, Condition: r.Condition, RestoredFrom: intPtr(r.RestoredFrom),
		},
		Status: domain.ChangeStatus(r.Status), ProposedBy: r.ProposedBy, ProposedAt: r.ProposedAt,
		DecidedBy: r.DecidedBy, DecidedAt: r.DecidedAt, Reason: r.Reason, AppliedVersion: intPtr(r.AppliedVersion),
	}
}
