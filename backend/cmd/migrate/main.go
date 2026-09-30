// Command migrate applies database migrations: the application schema (goose, db/migrations)
// followed by the job queue's schema.
//
// Usage: migrate [up|down|status]
//
//	up      apply all pending migrations (default)
//	down    roll back the most recent application migration
//	status  print applied/pending application migrations
//
// A Postgres advisory lock serializes concurrent runs, so it is safe to start several replicas.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	dbmigrations "github.com/tobioyinlola/RiskOS/backend/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments %q; usage: migrate [up|down|status]", args[1:])
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.ServiceName+"-migrate")

	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()

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
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	switch command {
	case "up":
		results, err := provider.Up(ctx)
		if err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		for _, r := range results {
			logger.InfoContext(ctx, "applied migration", slog.String("source", r.Source.Path), slog.Duration("duration", r.Duration))
		}
		if err := jobs.Migrate(ctx, pool, logger); err != nil {
			return err
		}
		version, err := provider.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		logger.InfoContext(ctx, "migrations complete", slog.Int64("version", version))
	case "down":
		r, err := provider.Down(ctx)
		if err != nil {
			return fmt.Errorf("roll back migration: %w", err)
		}
		logger.InfoContext(ctx, "rolled back migration", slog.String("source", r.Source.Path))
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return fmt.Errorf("read migration status: %w", err)
		}
		for _, s := range statuses {
			logger.InfoContext(ctx, "migration", slog.String("source", s.Source.Path), slog.String("state", string(s.State)))
		}
	default:
		return fmt.Errorf("unknown command %q; usage: migrate [up|down|status]", command)
	}
	return nil
}
