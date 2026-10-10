//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// signInWithTOTP signs the user in with password and the authenticator's current code, and
// returns whether the session can read the API. The user's replay marker (the last accepted
// step) is cleared first, so several sign-ins can follow within one 30-second step.
func signInWithTOTP(t *testing.T, s stack, email, secret string) bool {
	t.Helper()
	_, err := shared.owner.Exec(t.Context(), `UPDATE users SET totp_last_step = NULL WHERE email = $1`, email)
	require.NoError(t, err)
	b := s.browser()
	r := b.do(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": testPassword})
	if r.status != http.StatusOK {
		return false
	}
	r = b.do(t, http.MethodPost, "/api/v1/auth/mfa/totp/verify", map[string]any{"code": totp(t, secret, time.Now())})
	if r.status != http.StatusOK {
		return false
	}
	b.refresh(t)
	return b.do(t, http.MethodGet, "/api/v1/alerts", nil).status == http.StatusOK
}

// TestConsoleSecretRotation walks ADR-0008 for the console secret: a person enrolled under the old
// secret keeps signing in while it is the previous one, the reseal command moves their stored
// authenticator secret to the new one, and the old secret is then no longer needed.
func TestConsoleSecretRotation(t *testing.T) {
	const oldSecret = "console-secret-old-0123456789abcdef-aa"
	const newSecret = "console-secret-new-0123456789abcdef-bb"
	console := func(cc auth.ConsoleConfig) stackOption { return withConsole(cc, false) }

	before := newStackWith(t, time.Second, generousLimits(), shared.rdb, console(auth.ConsoleConfig{Secret: []byte(oldSecret)}))
	tenant := before.createTenant(t, "Console rotation")
	m := before.onboard(t, tenant, "rotate@bank.example", authz.RoleAnalyst)
	require.True(t, signInWithTOTP(t, before, "rotate@bank.example", m.secret), "before the rotation")

	during := newStackWith(t, time.Second, generousLimits(), shared.rdb,
		console(auth.ConsoleConfig{Secret: []byte(newSecret), SecretPrevious: []byte(oldSecret)}))
	onlyNew := newStackWith(t, time.Second, generousLimits(), shared.rdb, console(auth.ConsoleConfig{Secret: []byte(newSecret)}))

	t.Run("without the previous secret the authenticator cannot be opened", func(t *testing.T) {
		assert.False(t, signInWithTOTP(t, onlyNew, "rotate@bank.example", m.secret),
			"replacing the secret outright locks the user out: why a rotation needs the previous one")
	})
	t.Run("with it, sign-in keeps working", func(t *testing.T) {
		assert.True(t, signInWithTOTP(t, during, "rotate@bank.example", m.secret))
	})
	t.Run("reseal moves the stored secret to the new one", func(t *testing.T) {
		res, err := auth.ResealConsoleSecrets(t.Context(), shared.owner, []byte(newSecret), []byte(oldSecret))
		require.NoError(t, err)
		assert.GreaterOrEqual(t, res.Resealed, 1, "this user's authenticator secret")

		again, err := auth.ResealConsoleSecrets(t.Context(), shared.owner, []byte(newSecret), []byte(oldSecret))
		require.NoError(t, err)
		assert.Zero(t, again.Resealed, "idempotent: nothing is left on the previous secret")
		assert.GreaterOrEqual(t, again.Current, 1)

		assert.True(t, signInWithTOTP(t, onlyNew, "rotate@bank.example", m.secret),
			"the previous secret is no longer needed")
	})
	t.Run("reseal needs a previous secret", func(t *testing.T) {
		_, err := auth.ResealConsoleSecrets(t.Context(), shared.owner, []byte(newSecret), nil)
		require.Error(t, err)
	})
}
