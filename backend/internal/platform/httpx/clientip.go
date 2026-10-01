package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// ClientIP records the client's address in the request context (requestctx.ClientIP), for
// per-address limits and session records. The address is the direct peer, unless the peer is
// one of the trusted proxies: then it is the right-most address in X-Forwarded-For that is not
// itself a trusted proxy. Entries left of it were written by the client and are ignored, so a
// client cannot choose its own address. With no trusted proxies, X-Forwarded-For is ignored.
func ClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip, ok := clientIP(r, isTrusted); ok {
				r = r.WithContext(requestctx.WithClientIP(r.Context(), ip))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request, trusted func(netip.Addr) bool) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	ip := peer.Unmap()
	if !trusted(ip) {
		return ip, true
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break // a malformed entry: stop at the last address a trusted proxy vouched for
		}
		ip = hop.Unmap()
		if !trusted(ip) {
			break
		}
	}
	return ip, true
}
