// Package egress sends webhook requests to customer endpoints without letting them reach
// internal networks (SSRF, ARCHITECTURE.md §9).
//
// The address check runs in the dialer's Control hook, on the IP actually being connected to
// after DNS resolution, so a hostname that resolves (or re-resolves, DNS rebinding) to a private
// address is refused. Redirects are never followed, and no proxy from the environment is used.
package egress

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/domain"
)

// ErrBlocked is returned when the destination address is not allowed.
var ErrBlocked = errors.New("destination address is not allowed")

// maxResponseRead bounds how much of a response is read (and discarded) per attempt.
const maxResponseRead = 4 << 10

// Client sends signed webhook requests.
type Client struct {
	strict  *http.Client
	private *http.Client
}

// New returns a client whose requests time out after timeout.
func New(timeout time.Duration) *Client {
	return &Client{strict: newHTTPClient(timeout, false), private: newHTTPClient(timeout, true)}
}

func newHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlocked
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return ErrBlocked
			}
			if !allowPrivate && domain.Blocked(ip) {
				return ErrBlocked
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil, // never an environment proxy: it would bypass the address check
		DialContext:           dialer.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // a redirect is a failed delivery, never followed
		},
	}
}

// Request is one delivery attempt.
type Request struct {
	URL          string
	Body         []byte
	EventID      string
	EventType    string
	DeliveryID   string
	Timestamp    int64
	Signatures   []string
	AllowPrivate bool
}

// Result is the outcome of an attempt. StatusCode is 0 when no response was received.
type Result struct {
	StatusCode int
	// Error is a short, sanitized reason, empty on success.
	Error string
}

// OK reports whether the endpoint accepted the delivery (any 2xx).
func (r Result) OK() bool { return r.Error == "" && r.StatusCode >= 200 && r.StatusCode < 300 }

// Send posts the request. It never returns response bodies or raw transport errors, which could
// carry data from the customer's systems into RiskOS logs and APIs.
func (c *Client) Send(ctx context.Context, r Request) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return Result{Error: "invalid endpoint url"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RiskOS-Webhooks/1.0")
	req.Header.Set("RiskOS-Webhook-Id", r.EventID)
	req.Header.Set("RiskOS-Webhook-Type", r.EventType)
	req.Header.Set("RiskOS-Webhook-Delivery", r.DeliveryID)
	req.Header.Set("RiskOS-Webhook-Timestamp", fmt.Sprint(r.Timestamp))
	req.Header.Set("RiskOS-Signature", strings.Join(r.Signatures, ","))
	client := c.strict
	if r.AllowPrivate {
		client = c.private
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Error: classify(err)}
	}
	defer resp.Body.Close()                                                //nolint:errcheck // nothing to do on close failure
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseRead)) //nolint:errcheck // body is discarded
	res := Result{StatusCode: resp.StatusCode}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		res.Error = "redirect not followed"
	default:
		res.Error = fmt.Sprintf("http status %d", resp.StatusCode)
	}
	return res
}

func classify(err error) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, ErrBlocked):
		return "destination address not allowed"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.As(err, &dnsErr):
		return "dns lookup failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case strings.Contains(err.Error(), "tls:") || strings.Contains(err.Error(), "x509:"):
		return "tls error"
	default:
		return "connection failed"
	}
}
