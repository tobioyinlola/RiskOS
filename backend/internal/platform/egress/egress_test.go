package egress_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/egress"
)

func TestBlocked(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "fd00::1", "100.64.0.1", "::ffff:192.168.1.1"} {
		assert.True(t, egress.Blocked(netip.MustParseAddr(s)), s)
	}
	for _, s := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		assert.False(t, egress.Blocked(netip.MustParseAddr(s)), s)
	}
}

func TestClientRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	resp, err := egress.NewClient(time.Second, false).Get(srv.URL) //nolint:noctx // test
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	assert.ErrorIs(t, err, egress.ErrBlocked)
}

func TestClientCapsResponseBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", egress.MaxResponseBytes+1))
	}))
	defer srv.Close()
	resp, err := egress.NewClient(time.Second, true).Get(srv.URL) //nolint:noctx // test
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	assert.Error(t, err, "a body past the cap fails instead of filling memory")
}

func TestClientFollowsNoRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer srv.Close()
	resp, err := egress.NewClient(time.Second, true).Get(srv.URL) //nolint:noctx // test
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}
