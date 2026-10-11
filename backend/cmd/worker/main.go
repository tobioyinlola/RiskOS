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

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/mail"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
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
	if err := cfg.Webhooks.Validate(); err != nil {
		return err
	}
	if err := cfg.SMTP.Validate(cfg.Env); err != nil {
		return err
	}
	if cfg.SMTP.Enabled() {
		// Invite links are derived from the console secret and point at the console.
		if err := cfg.Console.Validate(cfg.Env); err != nil {
			return err
		}
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.ServiceName+"-worker")

	tel, err := telemetry.Setup(ctx, cfg.Telemetry, cfg.ServiceName+"-worker", cfg.Env)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tel.Shutdown(context.WithoutCancel(ctx))) }()
	appMetrics, err := appmetrics.New(tel.MeterProvider)
	if err != nil {
		return err
	}

	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()

	clk := clock.System{}
	runner := db.NewTxRunner(pool, db.WithBeginTimeout(cfg.Postgres.AcquireTimeout))
	auditModule := audit.New(runner, clk)
	tenantsModule := tenants.New(runner, auditModule.Recorder())
	// Jobs publish follow-up jobs (webhook deliveries) inside their own transactions.
	inserter, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return err
	}
	// alerts and webhooks depend on each other at run time only: alerts publish webhook events,
	// and a webhook endpoint disabled for failing raises a security alert.
	var alertsModule *alerts.Module
	webhookCfg := webhooks.Config{
		SigningKey: []byte(cfg.Webhooks.SigningKey), SigningKeyID: cfg.Webhooks.SigningKeyID,
		PreviousSigningKey: []byte(cfg.Webhooks.PreviousSigningKey), PreviousSigningKeyID: cfg.Webhooks.PreviousSigningKeyID,
		Timeout: cfg.Webhooks.Timeout, AllowPrivateTest: cfg.Webhooks.AllowPrivateTest,
		TenantConcurrency: cfg.Webhooks.EffectiveTenantConcurrency(),
		Metrics:           appMetrics,
	}
	// Refuse to start without a signing key some endpoint still uses (ADR-0008).
	if err := webhooks.VerifyKeyring(ctx, pool, webhookCfg); err != nil {
		return err
	}
	logging.WarnRotations(ctx, logger, cfg.RotationsInProgress())
	if unused, err := webhooks.PreviousKeyUnused(ctx, pool, webhookCfg); err == nil && unused {
		logger.WarnContext(ctx, "no webhook endpoint uses the previous signing key any more; remove RISKOS_WEBHOOK_SIGNING_KEY_PREVIOUS")
	}
	webhooksModule := webhooks.New(runner, auditModule.Recorder(), clk, inserter, webhookCfg, webhooks.NotifierFunc(func(ctx context.Context, id uuid.UUID) error {
		return alertsModule.WebhookEndpointDisabled(ctx, id)
	}), logger)
	alertsModule = alerts.New(runner, auditModule.Recorder(), clk, webhooksModule.Publisher(), logger)

	registry := jobs.NewRegistry()
	// The staff realm: its enrollment emails (below, with email) and the hourly usage check.
	adminModule, err := admin.New(pool, tenantsModule, runner, auditModule.Recorder(), clk, admin.Config{
		Secret: []byte(cfg.Console.Secret), SecretPrevious: []byte(cfg.Console.SecretPrevious), Logger: logger,
	})
	if err != nil {
		return err
	}
	jobs.RegisterPeriodic(registry, time.Hour, admin.UsageCheckArgs{}, adminModule.UsageJob())
	if cfg.SMTP.Enabled() {
		sender, err := mail.NewSMTP(mail.Config{
			Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, Username: cfg.SMTP.Username, Password: cfg.SMTP.Password,
			From: cfg.SMTP.From, TLS: cfg.SMTP.TLS, Timeout: cfg.SMTP.Timeout,
		})
		if err != nil {
			return err
		}
		cc := auth.DefaultConsole()
		cc.Secret, cc.RPID, cc.Origins = []byte(cfg.Console.Secret), cfg.Console.RPID, cfg.Console.Origins
		cc.SecretPrevious = []byte(cfg.Console.SecretPrevious)
		authModule := auth.New(pool, runner, auditModule.Recorder(), clk, auth.Config{
			Console: cc, Cookie: auth.CookieConfig{ConsoleURL: cfg.Console.URL}, Logger: logger,
		})
		jobs.Register(registry, authModule.InviteEmailJob(sender))
		jobs.Register(registry, authModule.SignupEmailJob(sender))
		// Staff accounts created from the console are emailed their enrollment link here.
		jobs.Register(registry, adminModule.EnrollmentEmailJob(sender, cfg.Console.URL))
		logger.InfoContext(ctx, "invite and sign-up emails enabled", slog.String("smtp_host", cfg.SMTP.Host))
	}
	jobs.Register(registry, jobs.NoopHandler(logger))
	jobs.Register(registry, alertsModule.RaiseJob(tenantsModule))
	rulesModule := rules.New(runner, auditModule.Recorder())
	jobs.Register(registry, rulesModule.TasksJob(tenantsModule, clk, logger))
	jobs.RegisterPeriodic(registry, 24*time.Hour, rules.DailyArgs{}, rulesModule.DailyJob(pool, tenantsModule, clk, logger))
	jobs.Register(registry, webhooksModule.DeliverJob(tenantsModule))
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
		Queues:      map[string]int{jobs.QueueWebhooks: cfg.Webhooks.Concurrency},
		Metrics:     appMetrics,
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
