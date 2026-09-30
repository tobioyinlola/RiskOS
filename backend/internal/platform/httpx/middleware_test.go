package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return logging.New(&buf, slog.LevelDebug, "test"), &buf
}

func decodeError(t *testing.T, body io.Reader) apigen.ErrorDetail {
	t.Helper()
	var resp apigen.ErrorResponse
	require.NoError(t, json.NewDecoder(body).Decode(&resp))
	return resp.Error
}

func TestRecoverReturnsContractWithoutStack(t *testing.T) {
	logger, logs := bufferLogger()
	h := httpx.RequestID(httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("database password is hunter2")
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "hunter2")
	assert.NotContains(t, body, "goroutine")
	e := decodeError(t, strings.NewReader(body))
	assert.Equal(t, httpx.CodeInternal, e.Code)
	assert.Equal(t, rec.Header().Get(httpx.HeaderRequestID), e.RequestId)
	assert.Contains(t, logs.String(), "panic recovered")
	assert.Contains(t, logs.String(), "stack")
}

func TestRecoverRepanicsAbortHandler(t *testing.T) {
	logger, _ := bufferLogger()
	h := httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	})
}

func TestBodyLimitOnRead(t *testing.T) {
	logger, _ := bufferLogger()
	h := httpx.BodyLimit(8, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			httpx.WriteError(w, r, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// Chunked body: no Content-Length, so the limit is enforced while reading.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 64)))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, httpx.CodePayloadTooLarge, decodeError(t, rec.Body).Code)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("small")))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestTimeoutSetsDeadline(t *testing.T) {
	var deadline time.Time
	var ok bool
	h := httpx.Timeout(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(time.Second), deadline, 100*time.Millisecond)
}

func TestWriteErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"typed error keeps its status", httpx.InvalidRequest("Field x is required.", nil), 400, httpx.CodeInvalidRequest, "Field x is required."},
		{"wrapped typed error", errWrap(httpx.InvalidRequest("Bad.", nil)), 400, httpx.CodeInvalidRequest, "Bad."},
		{"deadline", context.DeadlineExceeded, 503, httpx.CodeTimeout, "The request timed out."},
		{"max bytes", &http.MaxBytesError{Limit: 1}, 413, httpx.CodePayloadTooLarge, "The request body is too large."},
		{"unknown error hides details", errors.New("pq: relation users does not exist"), 500, httpx.CodeInternal, "An internal error occurred."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := bufferLogger()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req = req.WithContext(requestctx.WithRequestID(req.Context(), "req_x"))
			rec := httptest.NewRecorder()
			httpx.WriteError(rec, req, logger, tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.NotContains(t, rec.Body.String(), "relation users")
			e := decodeError(t, rec.Body)
			assert.Equal(t, tt.wantCode, e.Code)
			assert.Equal(t, tt.wantMsg, e.Message)
			assert.Equal(t, "req_x", e.RequestId)
		})
	}
}

func errWrap(err error) error { return errors.Join(errors.New("context"), err) }

func TestAccessLogRecordsStatus(t *testing.T) {
	logger, logs := bufferLogger()
	h := httpx.AccessLog(logger, fixedClock{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("abc"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))

	var line map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &line))
	assert.InDelta(t, http.StatusTeapot, line["status"], 0)
	assert.InDelta(t, 3, line["bytes"], 0)
	assert.Equal(t, "/x", line["path"])
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0) }
