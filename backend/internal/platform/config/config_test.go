package config_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
)

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func minimalEnv() map[string]string {
	return map[string]string{
		"RISKOS_POSTGRES_URL": "postgres://riskos:secret-pw@localhost:5432/riskos?sslmode=disable",
		"RISKOS_REDIS_ADDR":   "localhost:6379",
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(lookupFrom(minimalEnv()))
	require.NoError(t, err)

	assert.Equal(t, config.EnvDevelopment, cfg.Env)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.Equal(t, ":8080", cfg.HTTP.Addr)
	assert.Equal(t, 5*time.Second, cfg.HTTP.ReadHeaderTimeout)
	assert.Equal(t, int64(64<<10), cfg.HTTP.MaxBodyBytes)
	assert.Equal(t, int32(20), cfg.Postgres.MaxConns)
	assert.False(t, cfg.Telemetry.Enabled)
}

func TestLoadOverrides(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_ENV"] = "test"
	env["RISKOS_LOG_LEVEL"] = "debug"
	env["RISKOS_HTTP_MAX_BODY_BYTES"] = "2048"
	env["RISKOS_POSTGRES_MAX_CONNS"] = "50"
	env["RISKOS_OTEL_ENABLED"] = "true"

	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)

	assert.Equal(t, config.EnvTest, cfg.Env)
	assert.Equal(t, slog.LevelDebug, cfg.LogLevel)
	assert.Equal(t, int64(2048), cfg.HTTP.MaxBodyBytes)
	assert.Equal(t, int32(50), cfg.Postgres.MaxConns)
	assert.True(t, cfg.Telemetry.Enabled)
}

func TestLoadRejectsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"missing postgres url", func(e map[string]string) { delete(e, "RISKOS_POSTGRES_URL") }, "RISKOS_POSTGRES_URL: required"},
		{"missing redis addr", func(e map[string]string) { delete(e, "RISKOS_REDIS_ADDR") }, "RISKOS_REDIS_ADDR: required"},
		{"unknown env", func(e map[string]string) { e["RISKOS_ENV"] = "staging" }, "RISKOS_ENV"},
		{"bad duration", func(e map[string]string) { e["RISKOS_HTTP_READ_TIMEOUT"] = "10" }, "RISKOS_HTTP_READ_TIMEOUT"},
		{"zero duration", func(e map[string]string) { e["RISKOS_HTTP_IDLE_TIMEOUT"] = "0s" }, "RISKOS_HTTP_IDLE_TIMEOUT"},
		{"bad integer", func(e map[string]string) { e["RISKOS_REDIS_POOL_SIZE"] = "many" }, "RISKOS_REDIS_POOL_SIZE"},
		{"int32 overflow", func(e map[string]string) { e["RISKOS_POSTGRES_MAX_CONNS"] = "99999999999" }, "RISKOS_POSTGRES_MAX_CONNS"},
		{"min above max conns", func(e map[string]string) { e["RISKOS_POSTGRES_MIN_CONNS"] = "30" }, "RISKOS_POSTGRES_MIN_CONNS"},
		{"request timeout above write timeout", func(e map[string]string) { e["RISKOS_HTTP_REQUEST_TIMEOUT"] = "1m" }, "RISKOS_HTTP_REQUEST_TIMEOUT"},
		{"non-postgres url", func(e map[string]string) { e["RISKOS_POSTGRES_URL"] = "mysql://x" }, "RISKOS_POSTGRES_URL"},
		{"bad log level", func(e map[string]string) { e["RISKOS_LOG_LEVEL"] = "loud" }, "RISKOS_LOG_LEVEL"},
		{"sample ratio out of range", func(e map[string]string) { e["RISKOS_OTEL_SAMPLE_RATIO"] = "1.5" }, "RISKOS_OTEL_SAMPLE_RATIO"},
		{"production requires redis password", func(e map[string]string) { e["RISKOS_ENV"] = "production" }, "RISKOS_REDIS_PASSWORD"},
		{"production requires verified postgres TLS", func(e map[string]string) { e["RISKOS_ENV"] = "production" }, "sslmode=verify-full"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minimalEnv()
			tt.mutate(env)
			_, err := config.Load(lookupFrom(env))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadReportsAllErrorsAtOnce(t *testing.T) {
	_, err := config.Load(lookupFrom(map[string]string{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RISKOS_POSTGRES_URL")
	assert.Contains(t, err.Error(), "RISKOS_REDIS_ADDR")
}

func TestLoadNeverEchoesSecrets(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_POSTGRES_URL"] = "not a url with secret-pw"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-pw")
}
