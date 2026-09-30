package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// HeaderRequestID carries the correlation ID in requests and responses.
const HeaderRequestID = "X-Request-ID"

// RequestID accepts a safe inbound X-Request-ID or generates one, stores it in the request
// context, echoes it in the response and records it on the active trace span.
//
// A caller-supplied ID is correlation data only; it is never used for authorization or
// idempotency (idempotency keys get their own contract in phase-2).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !ids.IsSafeExternalID(id) {
			id = ids.NewRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("riskos.request_id", id))
		next.ServeHTTP(w, r.WithContext(requestctx.WithRequestID(r.Context(), id)))
	})
}

// Recover converts a panic into a 500 in the error contract. The stack trace is logged, never
// sent. http.ErrAbortHandler is re-panicked so net/http can abort the connection as intended.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { //nolint:contextcheck // the request context is taken from r inside WriteError
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity, as net/http does
					panic(rec)
				}
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.String("panic", fmt.Sprint(rec)), slog.String("stack", string(debug.Stack())))
				WriteError(w, r, logger, NewError(http.StatusInternalServerError, CodeInternal, "An internal error occurred.", errors.New("panic")))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets headers appropriate for a JSON API that is never rendered as a document.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// BodyLimit rejects bodies larger than maxBytes: immediately when Content-Length declares it,
// otherwise when reading passes the limit (surfacing *http.MaxBytesError, mapped to 413).
func BodyLimit(maxBytes int64, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				WriteError(w, r, logger, &http.MaxBytesError{Limit: maxBytes})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout bounds the request context. Handlers and everything they call must honor ctx; the
// deadline propagates to database, Redis and outbound calls.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AccessLog logs one line per request: method, route pattern, path, status, bytes and duration.
// It never logs query strings, headers or bodies, which may carry credentials or personal data.
func AccessLog(logger *slog.Logger, clk clock.Clock) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := clk.Now()
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			route := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				route = rc.RoutePattern()
			}
			level := slog.LevelInfo
			if rw.status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Int64("bytes", rw.bytes),
				slog.Duration("duration", clk.Now().Sub(start)),
			)
		})
	}
}

// statusRecorder captures status and size. Unwrap lets http.ResponseController reach the
// underlying writer's Flush/Hijack/deadline methods.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
