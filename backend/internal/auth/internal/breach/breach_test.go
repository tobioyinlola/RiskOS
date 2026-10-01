package breach

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SHA-1("password") = 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8.
const prefix, suffix = "5BAA6", "1E4C9B93F3F0682250B6CF8331B7EE68FD8"

func TestBreached(t *testing.T) {
	var gotPath, gotPadding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotPadding = r.URL.Path, r.Header.Get("Add-Padding")
		fmt.Fprintln(w, "0018A45C4D1DEF81644B54AB7F969B88D65:1")
		fmt.Fprintln(w, suffix+":9659365")
		fmt.Fprintln(w, "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:0")
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second)

	found, err := c.Breached(t.Context(), "password")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "/range/"+prefix, gotPath, "only the first five hash characters leave RiskOS")
	assert.Equal(t, "true", gotPadding)

	found, err = c.Breached(t.Context(), "a passphrase nobody has used before 7f3a")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPaddingIsNotAMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, suffix+":0")
	}))
	defer srv.Close()
	found, err := New(srv.URL, time.Second).Breached(t.Context(), "password")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestFailures(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	_, err := New(down.URL, time.Second).Breached(t.Context(), "password")
	assert.Error(t, err)

	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()
	start := time.Now()
	_, err = New(slow.URL, 100*time.Millisecond).Breached(t.Context(), "password")
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
}
