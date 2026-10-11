package handler

import (
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// AllowAddresses limits the admin realm to clients inside allowed. A request under Prefix from
// anywhere else, or whose address is unknown, gets the same 404 as a path that does not exist, so
// the realm is not revealed to the rest of the internet. An empty list allows every address: the
// realm's own sign-in and the private access path in front of it remain the controls.
//
// The address is the one httpx.ClientIP worked out, which believes X-Forwarded-For only from the
// configured trusted proxies.
func AllowAddresses(allowed []netip.Prefix, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, Prefix) {
				next.ServeHTTP(w, r)
				return
			}
			if ip, ok := requestctx.ClientIP(r.Context()); ok {
				ip = ip.Unmap()
				for _, p := range allowed {
					if p.Contains(ip) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			logger.WarnContext(r.Context(), "admin realm request from an address outside the allowlist")
			httpx.NotFoundHandler(logger)(w, r)
		})
	}
}
