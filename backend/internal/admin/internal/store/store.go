// Package store is the only code that touches the admin realm's tables (sqlc queries). They are
// platform tables with no tenant, so the store works on the pool directly, not through the
// tenant-scoped transaction runner.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Tx is a database transaction, for work that must enqueue a job in the same one.
type Tx = pgx.Tx

// TxWith runs fn in a transaction like Tx, and also hands it the transaction itself.
func (s Store) TxWith(ctx context.Context, fn func(tx Tx, q *sqlcgen.Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return fn(tx, sqlcgen.New(tx)) })
}

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

// OnboardingRow is a self-serve workspace's onboarding in the review queue: identity, status
// and dates, never the answers.
type OnboardingRow struct {
	TenantID        [16]byte
	TenantName      string
	TenantStatus    string
	TenantCreatedAt time.Time
	Status          string
	SubmittedAt     *time.Time
	ReviewedAt      *time.Time
}

// OnboardingQueue lists every self-serve workspace's onboarding, submitted ones first (through
// the admin_onboarding_queue function; hand-written because its dates are nullable).
func (s Store) OnboardingQueue(ctx context.Context) ([]OnboardingRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT tenant_id, tenant_name, tenant_status, created_at, status, submitted_at, reviewed_at FROM admin_onboarding_queue()`)
	if err != nil {
		return nil, Classify(err, "list onboarding")
	}
	defer rows.Close()
	var out []OnboardingRow
	for rows.Next() {
		var r OnboardingRow
		if err := rows.Scan(&r.TenantID, &r.TenantName, &r.TenantStatus, &r.TenantCreatedAt, &r.Status, &r.SubmittedAt, &r.ReviewedAt); err != nil {
			return nil, Classify(err, "list onboarding")
		}
		out = append(out, r)
	}
	return out, Classify(rows.Err(), "list onboarding")
}
