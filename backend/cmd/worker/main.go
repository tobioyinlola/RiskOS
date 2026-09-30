// Command worker executes background jobs. It shares configuration, logging and telemetry with
// the API but serves no HTTP traffic.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.ServiceName+"-worker")

	tel, err := telemetry.Setup(ctx, cfg.Telemetry, cfg.ServiceName+"-worker", cfg.Env)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tel.Shutdown(context.WithoutCancel(ctx))) }()

	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()

	clk := clock.System{}
	runner := db.NewTxRunner(pool)
	auditModule := audit.New(runner, clk)
	tenantsModule := tenants.New(runner, auditModule.Recorder())
	alertsModule := alerts.New(runner, auditModule.Recorder(), clk, logger)

	registry := jobs.NewRegistry()
	jobs.Register(registry, jobs.NoopHandler(logger))
	jobs.Register(registry, alertsModule.RaiseJob(tenantsModule))
	jobs.RegisterPeriodic(registry, 24*time.Hour, jobs.EnsurePartitionsArgs{}, func(ctx context.Context, _ jobs.EnsurePartitionsArgs) error {
		created, err := db.EnsureTimePartitions(ctx, pool, db.PartitionMonthsAhead)
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "time partitions ensured", slog.Int("created", created), slog.Int("months_ahead", db.PartitionMonthsAhead))
		return nil
	})

	queue, err := jobs.NewQueue(pool, registry, jobs.QueueConfig{
		Concurrency: cfg.Worker.Concurrency,
		JobTimeout:  cfg.Worker.JobTimeout,
	}, logger)
	if err != nil {
		return err
	}
	if err := queue.Start(ctx); err != nil {
		return err
	}
	logger.InfoContext(ctx, "worker started", slog.Int("concurrency", cfg.Worker.Concurrency))

	// Startup self-check: proves enqueue → fetch → execute against the live database.
	if err := queue.Enqueue(ctx, jobs.NoopArgs{Reason: "worker startup self-check"}); err != nil {
		return errors.Join(err, queue.Stop(context.WithoutCancel(ctx)))
	}

	<-ctx.Done()
	logger.Info("worker stopping", slog.Duration("timeout", cfg.Worker.ShutdownTimeout))
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Worker.ShutdownTimeout)
	defer cancel()
	if err := queue.Stop(stopCtx); err != nil {
		return err
	}
	logger.Info("worker stopped")
	return nil
}
