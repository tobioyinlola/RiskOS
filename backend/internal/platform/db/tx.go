package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// ErrNoTx is returned by stores called outside a tenant transaction.
var ErrNoTx = errors.New("no tenant transaction in context")

type txKey struct{}

// TxRunner runs tenant-scoped transactions with row-level security applied (TENANCY.md).
type TxRunner struct {
	pool *pgxpool.Pool
}

var _ txn.Runner = (*TxRunner)(nil)

// NewTxRunner returns a runner on pool. The pool must connect as the application role, which
// is subject to row-level security (no BYPASSRLS, not the table owner).
func NewTxRunner(pool *pgxpool.Pool) *TxRunner { return &TxRunner{pool: pool} }

// Run begins a transaction, binds it to the scope in ctx with transaction-local settings
// (set_config(..., true) == SET LOCAL), runs fn with the transaction in its context, and commits
// only if fn succeeds. Nested calls reuse the outer transaction.
func (r *TxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			// Rollback after a failed commit or with a canceled context is expected to fail;
			// the transaction is discarded either way.
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()

	if _, err = tx.Exec(ctx,
		`SELECT set_config('app.tenant_id', $1, true), set_config('app.environment_id', $2, true)`,
		scope.TenantID.String(), scope.EnvironmentID.String(),
	); err != nil {
		return fmt.Errorf("apply tenant scope: %w", err)
	}

	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// TxFrom returns the tenant transaction in ctx. Stores call it for every query, so a query can
// never run outside a scoped transaction.
func TxFrom(ctx context.Context) (pgx.Tx, error) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return nil, ErrNoTx
	}
	return tx, nil
}
