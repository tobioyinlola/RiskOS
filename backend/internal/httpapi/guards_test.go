package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// scriptedLimiter answers Peek with peek and counts what Allow consumes.
type scriptedLimiter struct {
	peek    ratelimit.Decision
	allowed int
}

func (s *scriptedLimiter) Allow(context.Context, ...ratelimit.Rule) (ratelimit.Decision, error) {
	s.allowed++
	return ratelimit.Decision{Allowed: true}, nil
}

func (s *scriptedLimiter) Peek(context.Context, ratelimit.Rule) (ratelimit.Decision, error) {
	return s.peek, nil
}

func counterTotal(t *testing.T, r *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == name {
				for _, dp := range sum.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func TestAuthFailureGuardCountsFailuresAndRejections(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m, err := appmetrics.New(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	lim := &scriptedLimiter{peek: ratelimit.Decision{Allowed: true}}
	status := http.StatusUnauthorized
	h := authFailureGuard(lim, ratelimit.Limit{PerSecond: 1, Burst: 1}, slog.New(slog.DiscardHandler), m)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	call := func() int {
		ctx := requestctx.WithClientIP(context.Background(), netip.MustParseAddr("192.0.2.1"))
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/events", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := call(); got != http.StatusUnauthorized {
		t.Fatalf("status = %d", got)
	}
	status = http.StatusOK
	call() // a success counts as nothing
	if n := counterTotal(t, reader, "riskos.auth.failures"); n != 1 {
		t.Errorf("auth failures = %d, want 1", n)
	}
	if lim.allowed != 1 {
		t.Errorf("budget consumed %d times, want 1", lim.allowed)
	}

	lim.peek = ratelimit.Decision{Allowed: false, RetryAfter: 1}
	if got := call(); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", got)
	}
	if n := counterTotal(t, reader, "riskos.ratelimit.rejected"); n != 1 {
		t.Errorf("rejections = %d, want 1", n)
	}
}

func TestAuthFailureGuardCountsWithoutLimit(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	m, err := appmetrics.New(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	h := authFailureGuard(&scriptedLimiter{}, ratelimit.Limit{}, slog.New(slog.DiscardHandler), m)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if n := counterTotal(t, reader, "riskos.auth.failures"); n != 1 {
		t.Errorf("auth failures = %d, want 1", n)
	}
}
