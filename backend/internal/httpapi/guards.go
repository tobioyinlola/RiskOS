package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// DefaultOperationLimits are per-tenant limits on operations that act outside RiskOS or change
// who can act, so a compromised or buggy credential cannot use them at volume. Keys are the
// strict server's operation names. Ingest and reads are covered by the per-key and per-tenant
// limits alone.
func DefaultOperationLimits() map[string]ratelimit.Limit {
	return map[string]ratelimit.Limit{
		"TestWebhook":      {PerSecond: 0.5, Burst: 5},  // outbound HTTP to a customer URL
		"RedeliverWebhook": {PerSecond: 2, Burst: 20},   // outbound HTTP
		"CreateWebhook":    {PerSecond: 0.2, Burst: 10}, // new outbound destinations
		"CreateAPIKey":     {PerSecond: 0.2, Burst: 10},
		"RotateAPIKey":     {PerSecond: 0.2, Burst: 10},
		"CreateInvite":     {PerSecond: 0.2, Burst: 10},
		"PutSSO":           {PerSecond: 0.2, Burst: 10}, // outbound HTTP to an identity provider
		"EvaluateRisk":     {PerSecond: 20, Burst: 50},  // dry runs; ingest has its own budget
	}
}

func tooMany(w http.ResponseWriter, r *http.Request, logger *slog.Logger, d ratelimit.Decision) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
	httpx.WriteError(w, r, logger, httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited,
		"Too many requests. Retry after the time given in Retry-After.", nil))
}

// statusRecorder captures the response status for middleware that acts after the handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// authFailureGuard limits failed authentication per client address (it wraps authentication).
// Each 401 consumes from the address's budget; once it is spent, requests from that address
// get 429 before any credential lookup, which bounds key guessing and the database load of
// invalid keys. Successful requests consume nothing, so normal traffic never trips it. Like
// every limit it fails open when Redis is unavailable.
func authFailureGuard(limiter Allower, limit ratelimit.Limit, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if limit.PerSecond <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, ok := requestctx.ClientIP(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			rule := ratelimit.Rule{Key: "rl:authfail:" + ip.String(), Limit: limit}
			if d, err := limiter.Peek(r.Context(), rule); err == nil && !d.Allowed {
				logger.WarnContext(r.Context(), "authentication failures limit exceeded")
				tooMany(w, r, logger, d)
				return
			}
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == http.StatusUnauthorized {
				// Counted after the response; WithoutCancel so a client hanging up still counts.
				if _, err := limiter.Allow(context.WithoutCancel(r.Context()), rule); err != nil {
					logger.WarnContext(r.Context(), "rate limiter unavailable; failure not counted", slog.Any("error", err))
				}
			}
		})
	}
}

// operationLimit applies limits per tenant and operation (see DefaultOperationLimits).
func operationLimit(limiter Allower, limits map[string]ratelimit.Limit, logger *slog.Logger) apigen.StrictMiddlewareFunc {
	return func(f apigen.StrictHandlerFunc, operationID string) apigen.StrictHandlerFunc {
		limit, ok := limits[operationID]
		if !ok {
			return f
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
			scope, err := tenancy.FromContext(ctx)
			if err != nil {
				return f(ctx, w, r, req)
			}
			d, err := limiter.Allow(ctx, ratelimit.Rule{Key: "rl:op:" + scope.TenantID.String() + ":" + operationID, Limit: limit})
			if err != nil {
				logger.WarnContext(ctx, "rate limiter unavailable; request allowed", slog.Any("error", err))
				return f(ctx, w, r, req)
			}
			if !d.Allowed {
				return nil, httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited,
					"Too many requests for this operation. Retry after the time given in Retry-After.", nil).
					WithHeader("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
			}
			return f(ctx, w, r, req)
		}
	}
}
