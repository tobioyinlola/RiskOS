package egress_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/egress"
)

func TestBlocksPrivateDestinationsAfterResolution(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	t.Cleanup(srv.Close)
	c := egress.New(2 * time.Second)

	// "localhost" passes URL validation only in dev mode; here it proves the check runs on the
	// resolved address, which is what defeats DNS names pointing at internal hosts.
	for _, u := range []string{srv.URL, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)} {
		res := c.Send(t.Context(), egress.Request{URL: u, Body: []byte("{}")})
		assert.False(t, res.OK(), u)
		assert.Equal(t, "destination address not allowed", res.Error, u)
	}
	assert.False(t, hit, "no request reached the internal server")

	res := c.Send(t.Context(), egress.Request{URL: srv.URL, Body: []byte("{}"), AllowPrivate: true})
	assert.True(t, res.OK(), "allowed only in the flag-gated test mode")
}

func TestHeadersRedirectsAndFailures(t *testing.T) {
	var got http.Header
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("secret internal stack trace"))
	})
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := egress.New(300 * time.Millisecond)
	send := func(path string) egress.Result {
		return c.Send(t.Context(), egress.Request{
			URL: srv.URL + path, Body: []byte(`{"a":1}`), EventID: "wev_1", EventType: "alert.created",
			DeliveryID: "whd_1", Timestamp: 1790000000, Signatures: []string{"v1=aa", "v1=bb"}, AllowPrivate: true,
		})
	}

	res := send("/ok")
	require.True(t, res.OK())
	assert.Equal(t, `{"a":1}`, body)
	assert.Equal(t, "wev_1", got.Get("RiskOS-Webhook-Id"))
	assert.Equal(t, "1790000000", got.Get("RiskOS-Webhook-Timestamp"))
	assert.Equal(t, "v1=aa,v1=bb", got.Get("RiskOS-Signature"))
	assert.Equal(t, "application/json", got.Get("Content-Type"))

	res = send("/redirect")
	assert.Equal(t, egress.Result{StatusCode: http.StatusFound, Error: "redirect not followed"}, res)
	res = send("/error")
	assert.Equal(t, egress.Result{StatusCode: http.StatusInternalServerError, Error: "http status 500"}, res,
		"the response body is never kept")
	res = send("/slow")
	assert.Equal(t, "timeout", res.Error)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.False(t, c.Send(ctx, egress.Request{URL: srv.URL + "/ok", AllowPrivate: true}).OK())
}
