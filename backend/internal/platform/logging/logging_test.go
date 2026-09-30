package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m), "log line: %s", buf.String())
	return m
}

func TestRedactsSensitiveKeys(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"authorization", "Bearer xyz"},
		{"Authorization", "Bearer xyz"},
		{"api_key", "rk_live_123"},
		{"X-API-Key", "rk_live_123"},
		{"apiKey", "rk_live_123"},
		{"client_secret", "s3cr3t"},
		{"password", "hunter2"},
		{"refresh_token", "rt_abc123"},
		{"Cookie", "session=abc"},
		{"webhook_signature", "sha256=abc"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			var buf bytes.Buffer
			logging.New(&buf, slog.LevelInfo, "test").Info("msg", slog.String(tt.key, tt.value))
			assert.NotContains(t, buf.String(), tt.value)
			assert.Equal(t, logging.Redacted, decode(t, &buf)[tt.key])
		})
	}
}

func TestRedactsInsideGroups(t *testing.T) {
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelInfo, "test").Info("request",
		slog.Group("headers", slog.String("Authorization", "Bearer xyz"), slog.String("Accept", "application/json")))

	out := buf.String()
	assert.NotContains(t, out, "xyz")
	headers, ok := decode(t, &buf)["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, logging.Redacted, headers["Authorization"])
	assert.Equal(t, "application/json", headers["Accept"])
}

func TestRedactsCredentialsInFreeText(t *testing.T) {
	tests := []struct {
		name, in, leaked string
	}{
		{"bearer in value", "upstream rejected Authorization: Bearer xyz.abc-123 (401)", "xyz.abc-123"},
		{"lowercase scheme", "got bearer tok123", "tok123"},
		{"basic auth", "header was Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"two tokens", "Bearer aaa then Bearer bbb", "bbb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logging.New(&buf, slog.LevelInfo, "test").Info("failed", slog.String("detail", tt.in))
			assert.NotContains(t, buf.String(), tt.leaked)
			assert.Contains(t, buf.String(), logging.Redacted)
		})
	}
}

func TestRedactsMessage(t *testing.T) {
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelInfo, "test").Info("sending Authorization: Bearer xyz")
	assert.NotContains(t, buf.String(), "xyz")
}

func TestRedactsHTTPHeaderValues(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer xyz")
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelInfo, "test").Info("headers", slog.Any("headers", h))
	assert.NotContains(t, buf.String(), "xyz")
}

func TestLeavesOrdinaryValuesAlone(t *testing.T) {
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelInfo, "test").Info("ok", slog.String("path", "/health/live"), slog.Int("status", 200))
	m := decode(t, &buf)
	assert.Equal(t, "/health/live", m["path"])
	assert.InDelta(t, 200, m["status"], 0)
	assert.Equal(t, "test", m["service"])
}

func TestAddsRequestIDFromContext(t *testing.T) {
	var buf bytes.Buffer
	ctx := requestctx.WithRequestID(context.Background(), "req_abc")
	logging.New(&buf, slog.LevelInfo, "test").InfoContext(ctx, "hello")
	assert.Equal(t, "req_abc", decode(t, &buf)["request_id"])
}

func TestRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logging.New(&buf, slog.LevelWarn, "test").Info("hidden")
	assert.Empty(t, buf.String())
}

func BenchmarkLogLine(b *testing.B) {
	var buf bytes.Buffer
	l := logging.New(&buf, slog.LevelInfo, "bench")
	ctx := requestctx.WithRequestID(context.Background(), "req_abc")
	for b.Loop() {
		buf.Reset()
		l.InfoContext(ctx, "http request", slog.String("method", "GET"), slog.String("route", "/health/live"), slog.Int("status", 200))
	}
}
