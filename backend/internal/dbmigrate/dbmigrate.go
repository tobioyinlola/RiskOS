// Package dbmigrate applies database migrations: the application schema (goose, embedded from
// db/migrations) followed by the job queue's schema. It is used by cmd/migrate and by
// integration tests, so both exercise exactly the same migration path.
//
// A Postgres advisory lock serializes concurrent runs, so starting several replicas is safe.
package dbmigrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	dbmigrations "github.com/tobioyinlola/RiskOS/backend/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
)

// AppRole is the database role the api and worker connect as. It owns nothing and has no
// BYPASSRLS, so row-level security always applies to it (TENANCY.md). Migrations create it
// (NOLOGIN) if missing and grant it the privileges it needs; login and password are provisioned
// by the platform.
const AppRole = "riskos_app"

// Up applies all pending migrations and returns the resulting application schema version. pool
// must connect as the schema owner.
func Up(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (version int64, err error) {
	err = withProvider(pool, func(p *goose.Provider) error {
		results, err := p.Up(ctx)
		if err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		for _, r := range results {
			logger.InfoContext(ctx, "applied migration", slog.String("source", r.Source.Path), slog.Duration("duration", r.Duration))
		}
		if err := jobs.Migrate(ctx, pool, AppRole, logger); err != nil {
			return err
		}
		version, err = p.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		return nil
	})
	return version, err
}

// UpTo applies pending application migrations up to and including version, and not the job
// queue's schema. It exists to rehearse a migration on populated data: stop one version short,
// seed, then Up.
func UpTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	return withProvider(pool, func(p *goose.Provider) error {
		if _, err := p.UpTo(ctx, version); err != nil {
			return fmt.Errorf("apply migrations to %d: %w", version, err)
		}
		return nil
	})
}

// Version returns the application schema version.
func Version(ctx context.Context, pool *pgxpool.Pool) (version int64, err error) {
	err = withProvider(pool, func(p *goose.Provider) error {
		version, err = p.GetDBVersion(ctx)
		return err
	})
	return version, err
}

// Down rolls back the most recent application migration.
func Down(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	return withProvider(pool, func(p *goose.Provider) error {
		r, err := p.Down(ctx)
		if err != nil {
			return fmt.Errorf("roll back migration: %w", err)
		}
		logger.InfoContext(ctx, "rolled back migration", slog.String("source", r.Source.Path))
		return nil
	})
}

// Status logs the state of every application migration.
func Status(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	return withProvider(pool, func(p *goose.Provider) error {
		statuses, err := p.Status(ctx)
		if err != nil {
			return fmt.Errorf("read migration status: %w", err)
		}
		for _, s := range statuses {
			logger.InfoContext(ctx, "migration", slog.String("source", s.Source.Path), slog.String("state", string(s.State)))
		}
		return nil
	})
}

func withProvider(pool *pgxpool.Pool, fn func(*goose.Provider) error) (err error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { err = errors.Join(err, sqlDB.Close()) }()

	migrations, err := fs.Sub(dbmigrations.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration lock: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	return fn(p)
}
