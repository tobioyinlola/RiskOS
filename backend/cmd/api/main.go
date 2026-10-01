// Command api runs the RiskOS HTTP API. This file is the composition root: every dependency is
// constructed here and injected; no package holds mutable global state.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases"
	"github.com/tobioyinlola/RiskOS/backend/internal/events"
	"github.com/tobioyinlola/RiskOS/backend/internal/httpapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpserver"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/redisx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

func main() {
	run := run
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		run = healthcheck
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

// healthcheck probes the local liveness endpoint. Runtime images are distroless (no shell or
// curl), so container healthchecks invoke the binary itself: `api healthcheck`.
func healthcheck() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(cfg.HTTP.Addr)
	if err != nil {
		return fmt.Errorf("parse RISKOS_HTTP_ADDR: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", port)+"/health/live", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("liveness probe: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("liveness probe: status %d", resp.StatusCode)
	}
	return nil
}

func run() (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	if err := cfg.Auth.Validate(); err != nil {
		return err
	}
	if err := cfg.Risk.Validate(); err != nil {
		return err
	}
	if err := cfg.Webhooks.Validate(); err != nil {
		return err
	}
	if err := cfg.Console.Validate(cfg.Env); err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.ServiceName+"-api")

	tel, err := telemetry.Setup(ctx, cfg.Telemetry, cfg.ServiceName+"-api", cfg.Env)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tel.Shutdown(context.WithoutCancel(ctx))) }()

	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb, err := redisx.New(ctx, cfg.Redis, logger)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rdb.Close()) }()

	healthSvc := health.NewService(logger,
		health.Check{Name: "postgres", Timeout: cfg.Postgres.CheckTimeout, Probe: func(ctx context.Context) error { return db.Ping(ctx, pool) }},
		health.Check{Name: "redis", Timeout: cfg.Redis.CheckTimeout, Probe: func(ctx context.Context) error { return redisx.Ping(ctx, rdb) }},
	)

	clk := clock.System{}
	runner := db.NewTxRunner(pool, db.WithBeginTimeout(cfg.Postgres.AcquireTimeout))
	auditModule := audit.New(runner, clk)
	authModule := auth.New(pool, runner, auditModule.Recorder(), clk, auth.Config{
		Pepper:   []byte(cfg.Auth.APIKeyPepper),
		CacheTTL: cfg.Auth.APIKeyCacheTTL,
		Console:  consoleConfig(cfg.Console),
		Cookie: auth.CookieConfig{
			Secure: cfg.Console.CookieSecure, Origins: cfg.Console.Origins, ConsoleURL: cfg.Console.URL,
		},
		Limiter: ratelimit.New(rdb, cfg.RateLimit.Timeout),
		Logger:  logger,
	})

	riskModule := risk.New(rdb, runner, auditModule.Recorder(), clk, logger, risk.Config{
		FeatureHashKey: []byte(cfg.Risk.FeatureHashKey),
		FeatureTimeout: cfg.Risk.FeatureTimeout,
	})
	// The API only inserts jobs (in the decision transaction); the worker runs them.
	queue, err := jobs.NewInserter(pool, logger)
	if err != nil {
		return err
	}
	eventsModule := events.New(runner, riskModule.Evaluator(), riskModule.RuleTester(), queue, clk)
	// The API publishes webhook events and manages endpoints; it never delivers (no notifier).
	webhooksModule := webhooks.New(runner, auditModule.Recorder(), clk, queue, webhookConfig(cfg.Webhooks), nil, logger)
	alertsModule := alerts.New(runner, auditModule.Recorder(), clk, webhooksModule.Publisher(), logger)
	casesModule := cases.New(runner, alertsModule.Linker(), auditModule.Recorder(), clk, webhooksModule.Publisher())

	handler, err := httpapi.New(httpapi.Deps{
		Logger:    logger,
		Clock:     clk,
		HTTP:      cfg.HTTP,
		Telemetry: tel,
		Health:    healthSvc,
		Auth:      authModule,
		Audit:     auditModule,
		Events:    eventsModule,
		Risk:      riskModule,
		Alerts:    alertsModule,
		Cases:     casesModule,
		Webhooks:  webhooksModule,
		Limiter:   ratelimit.New(rdb, cfg.RateLimit.Timeout),
		RateLimits: httpapi.RateLimits{
			PerKey:    ratelimit.Limit{PerSecond: cfg.RateLimit.KeyPerSecond, Burst: cfg.RateLimit.KeyBurst},
			PerTenant: ratelimit.Limit{PerSecond: cfg.RateLimit.TenantPerSecond, Burst: cfg.RateLimit.TenantBurst},
		},
	})
	if err != nil {
		return err
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTP.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTP.Addr, err)
	}
	logger.InfoContext(ctx, "api starting", slog.String("env", string(cfg.Env)))
	return httpserver.Run(ctx, httpserver.New(cfg.HTTP, handler, logger), ln, httpserver.Lifecycle{
		OnDrain:         healthSvc.StartDraining,
		DrainDelay:      cfg.HTTP.ShutdownDrainDelay,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
	}, logger)
}

func webhookConfig(c config.Webhooks) webhooks.Config {
	return webhooks.Config{SigningKey: []byte(c.SigningKey), Timeout: c.Timeout, AllowPrivateTest: c.AllowPrivateTest}
}

func consoleConfig(c config.Console) auth.ConsoleConfig {
	cc := auth.DefaultConsole()
	cc.Secret, cc.RPID, cc.Origins = []byte(c.Secret), c.RPID, c.Origins
	return cc
}
