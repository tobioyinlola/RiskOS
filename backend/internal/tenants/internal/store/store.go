// Package store is the only code that reads or writes the tenants and environments tables.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/store/sqlcgen"
)

// ErrNotFound is returned when a tenant or environment does not exist in the current scope.
var ErrNotFound = errors.New("not found")

// Store operates inside the tenant transaction carried by the context.
type Store struct{}

func queries(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, err
	}
	return sqlcgen.New(tx), nil
}

// InsertTenant creates a tenant row.
func (Store) InsertTenant(ctx context.Context, id uuid.UUID, name, region string) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertTenant(ctx, sqlcgen.InsertTenantParams{ID: id, Name: name, Region: region}); err != nil {
		return fmt.Errorf("insert tenant: %w", err)
	}
	return nil
}

// InsertEnvironment creates an environment of tenantID.
func (Store) InsertEnvironment(ctx context.Context, id, tenantID uuid.UUID, kind string) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertEnvironment(ctx, sqlcgen.InsertEnvironmentParams{ID: id, TenantID: tenantID, Kind: kind}); err != nil {
		return fmt.Errorf("insert environment: %w", err)
	}
	return nil
}

// Environment returns an environment's kind and its tenant's status.
func (Store) Environment(ctx context.Context, tenantID, id uuid.UUID) (kind, tenantStatus string, err error) {
	q, err := queries(ctx)
	if err != nil {
		return "", "", err
	}
	row, err := q.GetEnvironment(ctx, sqlcgen.GetEnvironmentParams{TenantID: tenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("get environment: %w", err)
	}
	return row.Kind, row.TenantStatus, nil
}

// EnvironmentByKind returns the environment ID of kind and the tenant's status.
func (Store) EnvironmentByKind(ctx context.Context, tenantID uuid.UUID, kind string) (uuid.UUID, string, error) {
	q, err := queries(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}
	row, err := q.GetEnvironmentByKind(ctx, sqlcgen.GetEnvironmentByKindParams{TenantID: tenantID, Kind: kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("get environment: %w", err)
	}
	return row.ID, row.TenantStatus, nil
}
