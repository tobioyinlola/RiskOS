// Package egress makes outbound HTTP requests to addresses customers choose (identity
// providers) without letting them reach internal networks (SSRF, ARCHITECTURE.md §9).
//
// The address check runs in the dialer's Control hook, on the IP actually being connected to
// after DNS resolution, so a hostname that resolves (or re-resolves) to a private address is
// refused. No proxy from the environment is used.
package egress

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"syscall"
	"time"
)

// ErrBlocked is returned when the destination address is not allowed.
var ErrBlocked = errors.New("destination address is not allowed")

// blockedPrefixes are special-purpose ranges beyond what netip classifies.
var blockedPrefixes = func() []netip.Prefix { //nolint:gochecknoglobals // immutable table
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
		"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "2002::/16",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// Blocked reports whether outbound requests may not connect to ip: loopback, private, link-local
// (including cloud metadata at 169.254.169.254), multicast, unspecified, carrier-grade NAT,
// documentation, benchmarking, reserved, and IPv6 translation ranges that could reach them.
func Blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	return slices.ContainsFunc(blockedPrefixes, func(p netip.Prefix) bool { return p.Contains(ip) })
}

// NewClient returns an HTTP client that refuses blocked addresses (unless allowPrivate, for
// local development and tests), follows no redirects and times out after timeout.
func NewClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlocked
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || (!allowPrivate && Blocked(ip)) {
				return ErrBlocked
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil, // an environment proxy would bypass the address check
			DialContext:           dialer.DialContext,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          20,
			IdleConnTimeout:       60 * time.Second,
			ForceAttemptHTTP2:     true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
