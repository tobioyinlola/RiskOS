// Command migrate applies database migrations.
//
// Usage: migrate [up|down|status]
//
//	up      apply all pending migrations (default)
//	down    roll back the most recent application migration
//	status  print applied/pending application migrations
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tobioyinlola/RiskOS/backend/internal/dbmigrate"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
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

	pool, err := db.NewPool(ctx, cfg.Postgres)
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
		return nil
	}
}
