// Command migrate applies database migrations.
//
// Usage: migrate [up|down|status]
//
//	up      apply all pending migrations (default), then load new rule pack versions
//	down    roll back the most recent application migration
//	status  print applied/pending application migrations
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tobioyinlola/RiskOS/backend/internal/dbmigrate"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
)

const usage = "usage: migrate [up|down|status]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments %q; %s", args[1:], usage)
	}
	switch command {
	case "up", "down", "status":
	default:
		return fmt.Errorf("unknown command %q; %s", command, usage)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.ServiceName+"-migrate")

	if cfg.Postgres.MigrationURL == "" {
		return errors.New("RISKOS_POSTGRES_MIGRATION_URL is required: migrations run as the schema owner")
	}
	ownerCfg := cfg.Postgres
	ownerCfg.URL = cfg.Postgres.MigrationURL
	pool, err := db.NewPool(ctx, ownerCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch command {
	case "down":
		return dbmigrate.Down(ctx, pool, logger)
	case "status":
		return dbmigrate.Status(ctx, pool, logger)
	default:
		version, err := dbmigrate.Up(ctx, pool, logger)
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "migrations complete", slog.Int64("version", version))
		// Rule packs are reference data that ships with the code: load any new version.
		return rules.LoadPacks(ctx, pool, rules.BuiltInPacks(), logger)
	}
}
