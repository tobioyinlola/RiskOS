package config_test

import (
	"log/slog"
	"strings"
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
	assert.Empty(t, cfg.HTTP.AdminAllowedCIDRs, "the staff realm is open to every address unless limited")
}

func TestLoadAdminAllowlist(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_ADMIN_ALLOWED_CIDRS"] = "203.0.113.0/24, 2001:db8::/32"
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	require.Len(t, cfg.HTTP.AdminAllowedCIDRs, 2)
	assert.Equal(t, "203.0.113.0/24", cfg.HTTP.AdminAllowedCIDRs[0].String())
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
		{"bad migration url", func(e map[string]string) { e["RISKOS_POSTGRES_MIGRATION_URL"] = "not-a-url" }, "RISKOS_POSTGRES_MIGRATION_URL"},
		{"admin allowlist that is every address", func(e map[string]string) { e["RISKOS_ADMIN_ALLOWED_CIDRS"] = "0.0.0.0/0" }, "RISKOS_ADMIN_ALLOWED_CIDRS"},
		{"admin allowlist that is not a network", func(e map[string]string) { e["RISKOS_ADMIN_ALLOWED_CIDRS"] = "office" }, "RISKOS_ADMIN_ALLOWED_CIDRS"},
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

func TestAuthValidate(t *testing.T) {
	tests := []struct {
		name    string
		auth    config.Auth
		wantErr string
	}{
		{"valid", config.Auth{APIKeyPepper: strings.Repeat("p", 32), APIKeyCacheTTL: 5 * time.Second}, ""},
		{"cache disabled", config.Auth{APIKeyPepper: strings.Repeat("p", 32)}, ""},
		{"missing pepper", config.Auth{APIKeyCacheTTL: time.Second}, "RISKOS_API_KEY_PEPPER"},
		{"short pepper", config.Auth{APIKeyPepper: "short"}, "RISKOS_API_KEY_PEPPER"},
		{"cache ttl above revocation bound", config.Auth{APIKeyPepper: strings.Repeat("p", 32), APIKeyCacheTTL: 6 * time.Second}, "RISKOS_API_KEY_CACHE_TTL"},
		{"rotation", config.Auth{APIKeyPepper: strings.Repeat("n", 32), APIKeyPepperPrevious: strings.Repeat("o", 32)}, ""},
		{"previous too weak", config.Auth{APIKeyPepper: strings.Repeat("n", 32), APIKeyPepperPrevious: "tiny"}, "RISKOS_API_KEY_PEPPER_PREVIOUS"},
		{"previous equals current", config.Auth{APIKeyPepper: strings.Repeat("n", 32), APIKeyPepperPrevious: strings.Repeat("n", 32)}, "must differ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NotContains(t, err.Error(), "short", "the pepper must never be echoed")
		})
	}
}

func TestRiskValidate(t *testing.T) {
	key := strings.Repeat("k", 32)
	tests := []struct {
		name    string
		risk    config.Risk
		wantErr string
	}{
		{"valid", config.Risk{FeatureHashKey: key, FeatureTimeout: 50 * time.Millisecond}, ""},
		{"missing key", config.Risk{FeatureTimeout: 50 * time.Millisecond}, "RISKOS_FEATURE_HASH_KEY"},
		{"short key", config.Risk{FeatureHashKey: "short", FeatureTimeout: 50 * time.Millisecond}, "RISKOS_FEATURE_HASH_KEY"},
		{"no timeout", config.Risk{FeatureHashKey: key}, "RISKOS_FEATURE_TIMEOUT"},
		{"timeout too long", config.Risk{FeatureHashKey: key, FeatureTimeout: 2 * time.Second}, "RISKOS_FEATURE_TIMEOUT"},
		{"negative warm-up", config.Risk{FeatureHashKey: key, FeatureTimeout: 50 * time.Millisecond, HistoryWarmup: -time.Hour}, "RISKOS_HISTORY_WARMUP"},
		{"negative baseline cap", config.Risk{FeatureHashKey: key, FeatureTimeout: 50 * time.Millisecond, BaselineMaxPerHour: -1}, "RISKOS_BASELINE_MAX_UPDATES_PER_HOUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.risk.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NotContains(t, err.Error(), "short", "the key must never be echoed")
		})
	}
}

func TestWebhooksValidate(t *testing.T) {
	key := strings.Repeat("k", 32)
	assert.NoError(t, config.Webhooks{SigningKey: key, Timeout: 10 * time.Second, Concurrency: 1}.Validate())
	err := config.Webhooks{SigningKey: "short", Timeout: time.Minute}.Validate()
	require.Error(t, err)
	for _, want := range []string{"RISKOS_WEBHOOK_SIGNING_KEY", "RISKOS_WEBHOOK_TIMEOUT", "RISKOS_WEBHOOK_CONCURRENCY"} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), "short", "the key must never be echoed")
}

func TestWebhooksKeyRotationValidate(t *testing.T) {
	cur, prev := strings.Repeat("c", 32), strings.Repeat("p", 32)
	base := config.Webhooks{SigningKey: cur, Timeout: 10 * time.Second, Concurrency: 1}
	with := func(f func(*config.Webhooks)) config.Webhooks { w := base; f(&w); return w }

	assert.NoError(t, with(func(w *config.Webhooks) { w.SigningKeyID = 2 }).Validate(), "a current key id alone")
	assert.NoError(t, with(func(w *config.Webhooks) {
		w.SigningKeyID, w.PreviousSigningKey, w.PreviousSigningKeyID = 2, prev, 1
	}).Validate(), "a rotation")
	assert.NoError(t, with(func(w *config.Webhooks) {
		w.PreviousSigningKey, w.PreviousSigningKeyID = prev, 2
	}).Validate(), "the current id defaults to 1")

	for name, w := range map[string]config.Webhooks{
		"previous without an id": with(func(w *config.Webhooks) { w.PreviousSigningKey = prev }),
		"previous id equals current": with(func(w *config.Webhooks) {
			w.SigningKeyID, w.PreviousSigningKey, w.PreviousSigningKeyID = 2, prev, 2
		}),
		"previous key equals current": with(func(w *config.Webhooks) {
			w.SigningKeyID, w.PreviousSigningKey, w.PreviousSigningKeyID = 2, cur, 1
		}),
		"previous key too short": with(func(w *config.Webhooks) {
			w.SigningKeyID, w.PreviousSigningKey, w.PreviousSigningKeyID = 2, "tiny", 1
		}),
	} {
		t.Run(name, func(t *testing.T) {
			err := w.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "RISKOS_WEBHOOK_SIGNING_KEY_PREVIOUS")
			assert.NotContains(t, err.Error(), prev, "keys are never echoed")
		})
	}
}

func TestPrivateWebhooksRefusedInProduction(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_ENV"] = "production"
	env["RISKOS_WEBHOOK_ALLOW_PRIVATE_TEST"] = "true"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RISKOS_WEBHOOK_ALLOW_PRIVATE_TEST")
}

func TestConsoleConfig(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_CONSOLE_URL"] = "https://console.example.com/"
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	assert.Equal(t, "https://console.example.com", cfg.Console.URL)
	assert.Equal(t, []string{"https://console.example.com"}, cfg.Console.Origins)
	assert.Equal(t, "console.example.com", cfg.Console.RPID)
	assert.True(t, cfg.Console.CookieSecure, "secure by default")

	err = config.Console{Secret: "short", URL: "http://localhost:3000"}.Validate(config.EnvProduction)
	require.Error(t, err)
	for _, want := range []string{"RISKOS_CONSOLE_SECRET", "production requires https", "RISKOS_SESSION_COOKIE_SECURE"} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NoError(t, config.Console{Secret: strings.Repeat("s", 32), URL: "http://localhost:3000"}.Validate(config.EnvDevelopment))
}

func TestLoadRiskDefaults(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_FEATURE_HASH_KEY"] = strings.Repeat("x", 40)
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	assert.Equal(t, 50*time.Millisecond, cfg.Risk.FeatureTimeout)
	assert.Equal(t, 7*24*time.Hour, cfg.Risk.HistoryWarmup)
	assert.Equal(t, 60, cfg.Risk.BaselineMaxPerHour)
	assert.False(t, cfg.SMTP.Enabled())
	assert.Equal(t, 587, cfg.SMTP.Port)
	assert.Equal(t, "starttls", cfg.SMTP.TLS)
	require.NoError(t, cfg.Risk.Validate())
}

func TestLoadAuthDefaults(t *testing.T) {
	env := minimalEnv()
	env["RISKOS_API_KEY_PEPPER"] = strings.Repeat("x", 40)
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, cfg.Auth.APIKeyCacheTTL)
	require.NoError(t, cfg.Auth.Validate())
}

func TestSMTPValidate(t *testing.T) {
	ok := config.SMTP{Host: "smtp.example.com", Port: 587, From: "RiskOS <no-reply@example.com>", TLS: "starttls", Timeout: 10 * time.Second}
	for _, tc := range []struct {
		name string
		mod  func(*config.SMTP)
		env  config.Environment
		want string
	}{
		{"valid", func(*config.SMTP) {}, config.EnvProduction, ""},
		{"off", func(s *config.SMTP) { *s = config.SMTP{} }, config.EnvProduction, ""},
		{"no sender", func(s *config.SMTP) { s.From = "" }, config.EnvDevelopment, "RISKOS_SMTP_FROM"},
		{"bad port", func(s *config.SMTP) { s.Port = 0 }, config.EnvDevelopment, "RISKOS_SMTP_PORT"},
		{"plain text locally", func(s *config.SMTP) { s.TLS = "none" }, config.EnvDevelopment, ""},
		{"plain text in production", func(s *config.SMTP) { s.TLS = "none" }, config.EnvProduction, "RISKOS_SMTP_TLS"},
		{"unknown tls", func(s *config.SMTP) { s.TLS = "ssl" }, config.EnvDevelopment, "RISKOS_SMTP_TLS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := ok
			tc.mod(&c)
			err := c.Validate(tc.env)
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
