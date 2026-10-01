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

// maxDenyListing bounds one listing of deny-list entries.
const maxDenyListing = 1000

// ActiveDenyList returns the entries evaluation applies at now.
func (Store) ActiveDenyList(ctx context.Context, now time.Time) (domain.DenyList, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListActiveDenyEntries(ctx, sqlcgen.ListActiveDenyEntriesParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Now: &now,
	})
	if err != nil {
		return nil, fmt.Errorf("load deny list: %w", err)
	}
	d := domain.DenyList{}
	for _, r := range rows {
		d.Add(domain.DenyKind(r.Kind), r.Value)
	}
	return d, nil
}

// ListDeny returns the newest entries, optionally of one kind.
func (Store) ListDeny(ctx context.Context, kind *domain.DenyKind) ([]domain.DenyEntry, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	var k *string
	if kind != nil {
		k = new(string(*kind))
	}
	rows, err := q.ListDenyEntries(ctx, sqlcgen.ListDenyEntriesParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Kind: k, Lim: maxDenyListing,
	})
	if err != nil {
		return nil, fmt.Errorf("list deny entries: %w", err)
	}
	out := make([]domain.DenyEntry, len(rows))
	for i, r := range rows {
		out[i] = domain.DenyEntry{
			ID: r.ID, Kind: domain.DenyKind(r.Kind), Value: r.Value, Reason: r.Reason, ExpiresAt: r.ExpiresAt,
			CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		}
	}
	return out, nil
}

// CountDeny returns how many entries the environment has.
func (Store) CountDeny(ctx context.Context) (int64, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountDenyEntries(ctx, sqlcgen.CountDenyEntriesParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID})
	if err != nil {
		return 0, fmt.Errorf("count deny entries: %w", err)
	}
	return n, nil
}

// InsertDeny adds an entry; domain.ErrDenyDuplicate when the identifier is already listed.
func (Store) InsertDeny(ctx context.Context, e domain.DenyEntry) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	err = q.InsertDenyEntry(ctx, sqlcgen.InsertDenyEntryParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: e.ID, Kind: string(e.Kind), Value: e.Value,
		Reason: e.Reason, ExpiresAt: e.ExpiresAt, CreatedAt: e.CreatedAt, CreatedBy: e.CreatedBy,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrDenyDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert deny entry: %w", err)
	}
	return nil
}

// DeleteDeny removes an entry and returns what it listed.
func (Store) DeleteDeny(ctx context.Context, id uuid.UUID) (domain.DenyKind, string, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return "", "", err
	}
	row, err := q.DeleteDenyEntry(ctx, sqlcgen.DeleteDenyEntryParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", domain.ErrDenyNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("delete deny entry: %w", err)
	}
	return domain.DenyKind(row.Kind), row.Value, nil
}
