package health_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func ok(context.Context) error { return nil }

func failing(context.Context) error { return errors.New("connection refused") }

func blocking(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name      string
		checks    []health.Check
		drain     bool
		wantReady bool
		want      map[string]bool
	}{
		{
			name:      "all healthy",
			checks:    []health.Check{{Name: "postgres", Timeout: time.Second, Probe: ok}, {Name: "redis", Timeout: time.Second, Probe: ok}},
			wantReady: true,
			want:      map[string]bool{"postgres": true, "redis": true},
		},
		{
			name:      "one failing",
			checks:    []health.Check{{Name: "postgres", Timeout: time.Second, Probe: ok}, {Name: "redis", Timeout: time.Second, Probe: failing}},
			wantReady: false,
			want:      map[string]bool{"postgres": true, "redis": false},
		},
		{
			name:      "check exceeds timeout",
			checks:    []health.Check{{Name: "postgres", Timeout: 20 * time.Millisecond, Probe: blocking}},
			wantReady: false,
			want:      map[string]bool{"postgres": false},
		},
		{
			name:      "draining overrides healthy checks",
			checks:    []health.Check{{Name: "postgres", Timeout: time.Second, Probe: ok}},
			drain:     true,
			wantReady: false,
			want:      map[string]bool{"postgres": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := health.NewService(discard(), tt.checks...)
			if tt.drain {
				svc.StartDraining()
			}
			start := time.Now()
			ready, results := svc.Readiness(context.Background())
			assert.Equal(t, tt.wantReady, ready)
			assert.Equal(t, tt.want, results)
			assert.Less(t, time.Since(start), 500*time.Millisecond, "checks must be bounded by their timeouts")
		})
	}
}
