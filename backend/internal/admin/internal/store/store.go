// Package store is the only code that touches the admin realm's tables (sqlc queries). They are
// platform tables with no tenant, so the store works on the pool directly, not through the
// tenant-scoped transaction runner.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when a unique value already exists.
var ErrDuplicate = errors.New("already exists")

// Pool is the database pool (*pgxpool.Pool), connected as the application role.
type Pool interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store runs the admin realm's queries.
type Store struct {
	pool Pool
}

// New returns a store on pool.
func New(pool Pool) Store { return Store{pool: pool} }

// Q returns queries outside a transaction.
func (s Store) Q() *sqlcgen.Queries { return sqlcgen.New(s.pool) }

// Tx runs fn in a transaction, committing only if it succeeds.
func (s Store) Tx(ctx context.Context, fn func(q *sqlcgen.Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return fn(sqlcgen.New(tx)) })
}

// Classify maps driver errors to the store's.
func Classify(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicate
	}
	return fmt.Errorf("%s: %w", what, err)
}

// Scoped returns queries on the transaction in ctx (the tenant-scoped runner's), so admin rows
// and the tenant's audit entry about them commit together.
func (s Store) Scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, err
	}
	return sqlcgen.New(tx), nil
}
