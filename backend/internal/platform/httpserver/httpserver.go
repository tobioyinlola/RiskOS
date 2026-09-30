// Package httpserver runs an http.Server with explicit limits and graceful shutdown.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
)

// New returns a server with every timeout and the header limit set explicitly. The zero values
// of http.Server mean "no timeout", which lets slow clients hold connections indefinitely.
func New(cfg config.HTTP, h http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// Lifecycle configures shutdown behavior.
type Lifecycle struct {
	// OnDrain is called once when shutdown begins, before the listener closes (e.g. to fail
	// readiness so load balancers stop sending new requests).
	OnDrain func()
	// DrainDelay is how long to keep serving after OnDrain before closing the listener.
	DrainDelay time.Duration
	// ShutdownTimeout bounds how long in-flight requests may take to complete.
	ShutdownTimeout time.Duration
}

// Run serves on ln until ctx is canceled, then shuts down gracefully: signal draining, wait
// DrainDelay, stop accepting connections and wait for in-flight requests (bounded by
// ShutdownTimeout). It returns nil on a clean shutdown.
func Run(ctx context.Context, srv *http.Server, ln net.Listener, lc Lifecycle, logger *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	logger.InfoContext(ctx, "http server listening", slog.String("addr", ln.Addr().String()))

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown started", slog.Duration("drain_delay", lc.DrainDelay), slog.Duration("timeout", lc.ShutdownTimeout))
	if lc.OnDrain != nil {
		lc.OnDrain()
	}
	if lc.DrainDelay > 0 {
		// Deliberately not cancelable: shutdown has begun, and the delay is what lets load
		// balancers observe failing readiness before the listener closes.
		<-time.After(lc.DrainDelay)
	}

	// ctx is already canceled, so shutdown gets its own deadline.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lc.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", errors.Join(err, srv.Close()))
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}
