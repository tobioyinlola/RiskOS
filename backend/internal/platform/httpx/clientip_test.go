package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name, peer, xff, want string
		trusted               []netip.Prefix
	}{
		{name: "direct client", peer: "203.0.113.7:4000", want: "203.0.113.7", trusted: trusted},
		{name: "untrusted peer cannot claim an address", peer: "203.0.113.7:4000", xff: "198.51.100.1", want: "203.0.113.7", trusted: trusted},
		{name: "no trusted proxies ignores the header", peer: "10.0.0.5:4000", xff: "198.51.100.1", want: "10.0.0.5"},
		{name: "behind a trusted proxy", peer: "10.0.0.5:4000", xff: "198.51.100.1", want: "198.51.100.1", trusted: trusted},
		{name: "client-written entries are ignored", peer: "10.0.0.5:4000", xff: "1.2.3.4, 198.51.100.1", want: "198.51.100.1", trusted: trusted},
		{name: "through two trusted proxies", peer: "10.0.0.5:4000", xff: "198.51.100.1, 10.0.0.9", want: "198.51.100.1", trusted: trusted},
		{name: "malformed entry stops the walk", peer: "10.0.0.5:4000", xff: "198.51.100.1, junk, 10.0.0.9", want: "10.0.0.9", trusted: trusted},
		{name: "IPv4-mapped IPv6 peer", peer: "[::ffff:203.0.113.7]:4000", want: "203.0.113.7", trusted: trusted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got netip.Addr
			h := ClientIP(tc.trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got, _ = requestctx.ClientIP(r.Context())
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req.RemoteAddr = tc.peer
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.want, got.String())
		})
	}
}
