package handler

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

func TestAllowAddresses(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	logger := slog.New(slog.DiscardHandler)
	allowed := []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("2001:db8::/32")}

	tests := []struct {
		name    string
		allowed []netip.Prefix
		path    string
		ip      string // empty: the address is unknown
		want    int
	}{
		{"inside", allowed, "/api/admin/v1/auth/methods", "203.0.113.9", http.StatusNoContent},
		{"inside, IPv6", allowed, "/api/admin/v1/tenants", "2001:db8::7", http.StatusNoContent},
		{"inside, IPv4 as IPv6", allowed, "/api/admin/v1/tenants", "::ffff:203.0.113.9", http.StatusNoContent},
		{"outside", allowed, "/api/admin/v1/tenants", "198.51.100.1", http.StatusNotFound},
		{"outside, sign-in ceremony", allowed, "/api/admin/v1/auth/login", "198.51.100.1", http.StatusNotFound},
		{"unknown address", allowed, "/api/admin/v1/tenants", "", http.StatusNotFound},
		{"not the realm", allowed, "/api/v1/events", "198.51.100.1", http.StatusNoContent},
		{"no list allows everyone", nil, "/api/admin/v1/tenants", "198.51.100.1", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.ip != "" {
				ctx = requestctx.WithClientIP(ctx, netip.MustParseAddr(tt.ip))
			}
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, tt.path, http.NoBody)
			rec := httptest.NewRecorder()
			AllowAddresses(tt.allowed, logger)(ok).ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
