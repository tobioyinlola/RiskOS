// Package db owns the PostgreSQL connection pool.
//
// Only module stores (internal/<module>/internal/store) and platform code may use the pool
// directly; this is enforced by depguard (see .golangci.yml).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db/sqlcgen"
)

// NewPool creates a connection pool and verifies connectivity before returning, so a
// misconfigured process fails at startup rather than on its first request.
func NewPool(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		// pgx parse errors can echo the connection string, which may contain a password.
		return nil, fmt.Errorf("parse postgres config: invalid RISKOS_POSTGRES_URL")
	}
	pc.MaxConns = cfg.MaxConns
	pc.MinConns = cfg.MinConns
	pc.MaxConnLifetime = cfg.MaxConnLifetime
	pc.MaxConnLifetimeJitter = cfg.MaxConnLifetime / 10
	pc.MaxConnIdleTime = cfg.MaxConnIdleTime
	pc.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	// All timestamps are handled in UTC regardless of server or database defaults.
	pc.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pc.ConnConfig.RuntimeParams["application_name"] = "riskos"

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := Ping(pingCtx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return pool, nil
}

// PartitionMonthsAhead is how many months of events/evaluations partitions are kept ready
// beyond the current one. A daily job maintains it, so inserts never hit a missing partition
// even if the worker is down for weeks.
const PartitionMonthsAhead = 3

// EnsureTimePartitions creates missing monthly partitions of the time-partitioned tables through
// the ensure_time_partitions function (which runs with the owner's rights, so the application
// role can call it). It returns how many partitions were created.
func EnsureTimePartitions(ctx context.Context, pool *pgxpool.Pool, monthsAhead int) (int, error) {
	var created int
	if err := pool.QueryRow(ctx, `SELECT ensure_time_partitions($1)`, monthsAhead).Scan(&created); err != nil {
		return 0, fmt.Errorf("ensure time partitions: %w", err)
	}
	return created, nil
}

// Ping executes a trivial statement on a pooled connection.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := sqlcgen.New(pool).Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	return nil
}
