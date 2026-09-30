// Package security holds black-box tests of the HTTP surface against hostile input: a real
// http.Server with production limits, no internals. They run in the normal test suite (no Docker)
// and grow with every phase (spec §16: auth, authorization, tenant isolation, replay, …).
package security

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpserver"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	ch chan []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.ch <- bytes.Clone(p)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	var sb strings.Builder
	for {
		select {
		case p := <-b.ch:
			sb.Write(p)
		default:
			return sb.String()
		}
	}
}

type server struct {
	url  string
	logs *syncBuffer
}

// startServer runs the real API handler behind the real http.Server with default limits.
func startServer(t *testing.T) server {
	t.Helper()
	cfg, err := config.Load(func(k string) (string, bool) {
		v, ok := map[string]string{
			"RISKOS_POSTGRES_URL":          "postgres://unused@localhost/unused",
			"RISKOS_REDIS_ADDR":            "localhost:0",
			"RISKOS_HTTP_MAX_BODY_BYTES":   "1024",
			"RISKOS_HTTP_MAX_HEADER_BYTES": "4096",
		}[k]
		return v, ok
	})
	require.NoError(t, err)

	logs := &syncBuffer{ch: make(chan []byte, 1024)}
	logger := logging.New(logs, slog.LevelDebug, "security-test")
	tel, err := telemetry.Setup(t.Context(), cfg.Telemetry, "security-test", cfg.Env)
	require.NoError(t, err)
	authModule, auditModule := testModules()
	eventsMod, riskMod, alertsMod, casesMod := appModules()
	h, err := httpapi.New(httpapi.Deps{
		Logger: logger, Clock: clock.System{}, HTTP: cfg.HTTP, Telemetry: tel,
		Health: health.NewService(logger), Auth: authModule, Audit: auditModule,
		Events: eventsMod, Risk: riskMod, Alerts: alertsMod, Cases: casesMod, Limiter: allowAll{},
	})
	require.NoError(t, err)

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- httpserver.Run(ctx, httpserver.New(cfg.HTTP, h, logger), ln, httpserver.Lifecycle{ShutdownTimeout: 5 * time.Second}, logger)
	}()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	return server{url: "http://" + ln.Addr().String(), logs: logs}
}

// response is a fully read response; the body is already closed.
type response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func (s server) do(t *testing.T, method, path string, body io.Reader, headers map[string]string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, s.url+path, body)
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return response{StatusCode: resp.StatusCode, Header: resp.Header, Body: b}
}

func requireErrorContract(t *testing.T, resp response, status int, code string) {
	body := resp.Body
	t.Helper()
	assert.Equal(t, status, resp.StatusCode, string(body))
	var e apigen.ErrorResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&e), string(body))
	assert.Equal(t, code, e.Error.Code)
	assert.Equal(t, resp.Header.Get("X-Request-ID"), e.Error.RequestId)
}

func TestHostileRequestsGetTheErrorContract(t *testing.T) {
	s := startServer(t)
	tests := []struct {
		name   string
		method string
		path   string
		body   io.Reader
		status int
		code   string
	}{
		{"path traversal", http.MethodGet, "/health/../../etc/passwd", nil, http.StatusNotFound, "NOT_FOUND"},
		{"encoded traversal", http.MethodGet, "/api/v1/%2e%2e/%2e%2e/etc/passwd", nil, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"sql injection in path", http.MethodGet, "/api/v1/api-keys/1%27%20OR%20%271%27=%271/revoke", nil, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"whoami without a key", http.MethodGet, "/api/v1/whoami", nil, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"script in path", http.MethodGet, "/%3Cscript%3Ealert(1)%3C%2Fscript%3E", nil, http.StatusNotFound, "NOT_FOUND"},
		{"unsupported method", http.MethodDelete, "/health/ready", nil, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"oversized body", http.MethodPost, "/api/v1/events", strings.NewReader(strings.Repeat("a", 4096)), http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.do(t, tt.method, tt.path, tt.body, nil)
			if tt.status == http.StatusNotFound && resp.StatusCode == http.StatusMovedPermanently {
				// net/http cleans ".." paths with a redirect; the target must still be contained.
				assert.NotContains(t, resp.Header.Get("Location"), "..")
				return
			}
			requireErrorContract(t, resp, tt.status, tt.code)
			assert.NotContains(t, string(resp.Body), "<script>", "input must not be reflected")
			assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
		})
	}
}

func TestOversizedHeadersAreRejected(t *testing.T) {
	s := startServer(t)
	resp := s.do(t, http.MethodGet, "/health/live", nil, map[string]string{"X-Padding": strings.Repeat("a", 64<<10)})
	assert.Equal(t, http.StatusRequestHeaderFieldsTooLarge, resp.StatusCode)
}

func TestResponsesNeverLeakInternals(t *testing.T) {
	s := startServer(t)
	resp := s.do(t, http.MethodGet, "/nope", nil, nil)
	requireErrorContract(t, resp, http.StatusNotFound, "NOT_FOUND")
	for _, leak := range []string{"goroutine", ".go:", "chi", "runtime", "panic"} {
		assert.NotContains(t, string(resp.Body), leak)
	}
	assert.Empty(t, resp.Header.Get("Server"), "server software must not be advertised")
}

// TestCredentialsNeverReachLogs is acceptance criterion 4 end to end: credentials sent in
// headers or the query string do not appear in any log line.
func TestCredentialsNeverReachLogs(t *testing.T) {
	s := startServer(t)
	s.do(t, http.MethodGet, "/health/live?api_key=query-secret-value&token=tok_secret", nil, map[string]string{
		"Authorization": "Bearer xyz-header-secret",
		"X-API-Key":     "header-api-key-value",
		"Cookie":        "session=cookie-secret",
	})
	s.do(t, http.MethodGet, "/does-not-exist", nil, map[string]string{"Authorization": "Bearer xyz-header-secret"})
	// A well-formed but unknown API key on an authenticated route: the failure is logged as a
	// security signal, but the key itself never is.
	forged := "rsk_test_aaaaaaaaaaaa_" + strings.Repeat("b", 32)
	s.do(t, http.MethodGet, "/api/v1/whoami", nil, map[string]string{"Authorization": "Bearer " + forged})

	logs := s.logs.String() // drains the buffer: read once
	require.Contains(t, logs, `"msg":"http request"`, "requests must be logged")
	require.Contains(t, logs, "authentication failed")
	assert.NotContains(t, logs, forged)
	assert.NotContains(t, logs, strings.Repeat("b", 32))
	for _, secret := range []string{"query-secret-value", "tok_secret", "xyz-header-secret", "header-api-key-value", "cookie-secret"} {
		assert.NotContains(t, logs, secret)
	}
}

func TestRequestIDCannotInjectHeadersOrLogLines(t *testing.T) {
	s := startServer(t)
	resp := s.do(t, http.MethodGet, "/health/live", nil, map[string]string{"X-Request-ID": `evil","level":"ERROR","msg":"forged`})
	assert.Regexp(t, `^req_[0-9a-z]{26}$`, resp.Header.Get("X-Request-ID"))
	assert.NotContains(t, s.logs.String(), "forged")
}

// noKeys is a key store that knows no keys: every /api/v1 request is unauthenticated. Tests of
// authenticated behavior run against PostgreSQL in test/integration.
type noKeys struct{}

func (noKeys) QueryRow(context.Context, string, ...any) pgx.Row { return noRow{} }

type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }

// noDB fails any transaction; these tests never reach one.
type noDB struct{}

func (noDB) Run(context.Context, func(context.Context) error) error {
	return errors.New("no database in this test")
}

func testModules() (*auth.Module, *audit.Module) {
	auditModule := audit.New(noDB{}, clock.System{})
	return auth.New(noKeys{}, noDB{}, auditModule.Recorder(), clock.System{}, auth.Config{
		Pepper: []byte(strings.Repeat("p", 32)),
	}), auditModule
}

// allowAll is a rate limiter that never limits.
type allowAll struct{}

func (allowAll) Allow(context.Context, ...ratelimit.Rule) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true}, nil
}

// appModules returns the events and risk modules over a database that is never reached and a
// Redis that does not exist; these tests never evaluate an event.
// noJobs discards enqueued jobs.
type noJobs struct{}

func (noJobs) Enqueue(context.Context, jobs.Args) error { return nil }

func appModules() (*events.Module, *risk.Module, *alerts.Module, *cases.Module) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	riskModule := risk.New(rdb, noDB{}, audit.New(noDB{}, clock.System{}).Recorder(), clock.System{},
		slog.New(slog.DiscardHandler), risk.Config{FeatureHashKey: []byte(strings.Repeat("f", 32)), FeatureTimeout: 10 * time.Millisecond})
	alertsModule := alerts.New(noDB{}, audit.New(noDB{}, clock.System{}).Recorder(), clock.System{}, slog.New(slog.DiscardHandler))
	casesModule := cases.New(noDB{}, alertsModule.Linker(), audit.New(noDB{}, clock.System{}).Recorder(), clock.System{})
	return events.New(noDB{}, riskModule.Evaluator(), riskModule.RuleTester(), noJobs{}, clock.System{}), riskModule, alertsModule, casesModule
}
