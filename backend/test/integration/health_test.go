//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
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
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/redisx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
)

func newAPI(t *testing.T) http.Handler {
	t.Helper()
	tel, err := telemetry.Setup(t.Context(), config.Telemetry{}, "integration", config.EnvTest)
	require.NoError(t, err)
	runner := db.NewTxRunner(shared.pool)
	auditModule := audit.New(runner, clock.System{})
	authModule := auth.New(shared.pool, runner, auditModule.Recorder(), clock.System{}, auth.Config{Pepper: []byte(testPepper)})
	riskModule := newRisk(shared.rdb, runner, auditModule, clock.System{})
	queue, err := jobs.NewInserter(shared.pool, shared.logger)
	require.NoError(t, err)
	alertsModule := alerts.New(runner, auditModule.Recorder(), clock.System{}, shared.logger)
	h, err := httpapi.New(httpapi.Deps{
		Logger:     shared.logger,
		Clock:      clock.System{},
		HTTP:       config.HTTP{RequestTimeout: 5 * time.Second, MaxBodyBytes: 1 << 20},
		Telemetry:  tel,
		Auth:       authModule,
		Audit:      auditModule,
		Events:     events.New(runner, riskModule.Evaluator(), riskModule.RuleTester(), queue, clock.System{}),
		Risk:       riskModule,
		Alerts:     alertsModule,
		Cases:      cases.New(runner, alertsModule.Linker(), auditModule.Recorder(), clock.System{}),
		Limiter:    ratelimit.New(shared.rdb),
		RateLimits: generousLimits(),
		Health: health.NewService(shared.logger,
			health.Check{Name: "postgres", Timeout: time.Second, Probe: func(ctx context.Context) error { return db.Ping(ctx, shared.pool) }},
			health.Check{Name: "redis", Timeout: time.Second, Probe: func(ctx context.Context) error { return redisx.Ping(ctx, shared.rdb) }},
		),
	})
	require.NoError(t, err)
	return h
}

func readiness(t *testing.T, h http.Handler) (int, apigen.ReadinessResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/ready", nil))
	var body apigen.ReadinessResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return rec.Code, body
}

// stopFor stops a dependency container for the duration of fn, then restarts it and reconnects
// the shared clients (the restarted container gets a new host port).
func stopFor(t *testing.T, c testcontainers.Container, fn func()) {
	t.Helper()
	timeout := 5 * time.Second
	require.NoError(t, c.Stop(t.Context(), &timeout))
	defer func() {
		ctx := context.WithoutCancel(t.Context())
		require.NoError(t, c.Start(ctx))
		require.NoError(t, shared.connect(ctx))
	}()
	fn()
}

// TestReadinessTracksDependencies is acceptance criterion 2: /health/ready is 200 with both
// dependencies up and 503 when either is stopped, while /health/live stays 200.
func TestReadinessTracksDependencies(t *testing.T) {
	h := newAPI(t)

	status, body := readiness(t, h)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, apigen.ReadinessResponseStatusOk, body.Status)

	t.Run("redis stopped", func(t *testing.T) {
		stopFor(t, shared.redis, func() {
			status, body := readiness(t, h)
			assert.Equal(t, http.StatusServiceUnavailable, status)
			assert.Equal(t, apigen.ReadinessResponseChecksUnavailable, body.Checks["redis"])
			assert.Equal(t, apigen.ReadinessResponseChecksOk, body.Checks["postgres"])

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/live", nil))
			assert.Equal(t, http.StatusOK, rec.Code, "liveness must not depend on redis")
		})
	})

	t.Run("postgres stopped", func(t *testing.T) {
		stopFor(t, shared.postgres, func() {
			status, body := readiness(t, h)
			assert.Equal(t, http.StatusServiceUnavailable, status)
			assert.Equal(t, apigen.ReadinessResponseChecksUnavailable, body.Checks["postgres"])
		})
	})
}
