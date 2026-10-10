//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

// authStatus sends an authenticated request through a module's middleware alone and returns the
// status: 200 when the key verified, 401 when it did not.
func authStatus(t *testing.T, cfg auth.Config, key string) int {
	t.Helper()
	runner := db.NewTxRunner(shared.pool)
	cfg.Logger = shared.logger
	m := auth.New(shared.pool, runner, audit.New(runner, clock.System{}).Recorder(), clock.System{}, cfg)
	h := m.Middleware(shared.logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// TestAPIKeyPepperRotation walks ADR-0008: keys hashed under the old pepper keep working while
// it is configured as the previous one, and are moved to the new pepper by being used.
func TestAPIKeyPepperRotation(t *testing.T) {
	s := newStack(t, 0)
	a := s.createTenant(t, "Pepper rotation")
	const rotated = "rotated-pepper-0123456789abcdef-xyz"

	old := auth.Config{Pepper: []byte(testPepper)}
	both := auth.Config{Pepper: []byte(rotated), PepperPrevious: []byte(testPepper)}
	newOnly := auth.Config{Pepper: []byte(rotated)}

	assert.Equal(t, http.StatusOK, authStatus(t, old, a.testKey), "before the rotation")
	assert.Equal(t, http.StatusUnauthorized, authStatus(t, newOnly, a.testKey),
		"changing the pepper without a previous one locks the key out: why a rotation needs the previous pepper")

	assert.Equal(t, http.StatusOK, authStatus(t, both, a.testKey), "during the rotation, on the previous pepper")
	assert.Equal(t, http.StatusOK, authStatus(t, newOnly, a.testKey),
		"after being used once the key is on the new pepper alone")
	assert.Equal(t, http.StatusUnauthorized, authStatus(t, old, a.testKey),
		"and no longer verifies under the old pepper")

	assert.Equal(t, http.StatusOK, authStatus(t, both, a.testKey), "still fine with both configured")
	assert.Equal(t, http.StatusUnauthorized, authStatus(t, both, a.testKey[:len(a.testKey)-1]+"x"),
		"a wrong secret is refused during the rotation")
}
