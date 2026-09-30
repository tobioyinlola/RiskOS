package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpserver"
)

func get(t *testing.T, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func testConfig() config.HTTP {
	return config.HTTP{
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       5 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

// TestInFlightRequestCompletesAfterShutdownSignal is acceptance criterion 7: a request that is
// being processed when SIGTERM arrives (modeled by canceling the run context) still completes
// successfully, the drain hook fires first, and new connections are refused afterwards.
func TestInFlightRequestCompletesAfterShutdownSignal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond) // simulated slow in-flight work
		_, _ = io.WriteString(w, "done")
	})

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := "http://" + ln.Addr().String()

	ctx, signal := context.WithCancel(context.Background())
	var drained atomic.Bool
	runErr := make(chan error, 1)
	go func() {
		runErr <- httpserver.Run(ctx, httpserver.New(testConfig(), handler, logger), ln, httpserver.Lifecycle{
			OnDrain:         func() { drained.Store(true) },
			ShutdownTimeout: 5 * time.Second,
		}, logger)
	}()

	type result struct {
		status int
		body   string
		err    error
	}
	inflight := make(chan result, 1)
	go func() {
		resp, err := get(t, addr)
		if err != nil {
			inflight <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		inflight <- result{status: resp.StatusCode, body: string(b), err: err}
	}()

	<-started
	signal() // SIGTERM while the request is in flight

	res := <-inflight
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusOK, res.status)
	assert.Equal(t, "done", res.body)

	require.NoError(t, <-runErr)
	assert.True(t, drained.Load(), "drain hook must run on shutdown")

	resp, err := get(t, addr)
	if err == nil {
		_ = resp.Body.Close()
	}
	assert.Error(t, err, "listener must be closed after shutdown")
}

func TestShutdownTimeoutBoundsSlowRequests(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, signal := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- httpserver.Run(ctx, httpserver.New(testConfig(), handler, logger), ln,
			httpserver.Lifecycle{ShutdownTimeout: 100 * time.Millisecond}, logger)
	}()
	go func() {
		resp, err := get(t, "http://"+ln.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	<-started
	signal()
	select {
	case err := <-runErr:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown was not bounded by ShutdownTimeout")
	}
}
