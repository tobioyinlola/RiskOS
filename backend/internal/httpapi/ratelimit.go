package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// RateLimits configures request limits for authenticated API traffic.
type RateLimits struct {
	PerKey    ratelimit.Limit
	PerTenant ratelimit.Limit
}

// Allower checks rate limit rules. *ratelimit.Limiter implements it.
type Allower interface {
	Allow(ctx context.Context, rules ...ratelimit.Rule) (ratelimit.Decision, error)
}

// rateLimit applies per-key and per-tenant limits to authenticated requests (it must run after
// authentication). If Redis is unavailable the request is allowed and the failure logged: a
// limiter outage must not take the API down (ARCHITECTURE.md §12).
func rateLimit(limiter Allower, limits RateLimits, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, err := tenancy.FromContext(r.Context())
			if err != nil || scope.Principal.Type != tenancy.PrincipalAPIKey {
				next.ServeHTTP(w, r)
				return
			}
			d, err := limiter.Allow(r.Context(),
				ratelimit.Rule{Key: "rl:key:" + scope.Principal.ID, Limit: limits.PerKey},
				ratelimit.Rule{Key: "rl:tenant:" + scope.TenantID.String(), Limit: limits.PerTenant},
			)
			if err != nil {
				logger.WarnContext(r.Context(), "rate limiter unavailable; request allowed", slog.Any("error", err))
				next.ServeHTTP(w, r)
				return
			}
			if !d.Allowed {
				logger.InfoContext(r.Context(), "rate limit exceeded", slog.String("api_key_id", scope.Principal.ID))
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
				httpx.WriteError(w, r, logger, httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited,
					"Too many requests. Retry after the time given in Retry-After.", nil))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
