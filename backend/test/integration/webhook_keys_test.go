//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

func webhooksKeyUsage(t *testing.T) (map[int]int, error) {
	t.Helper()
	return webhooks.KeyUsage(t.Context(), shared.pool)
}

func verifyKeyring(t *testing.T, cfg webhooks.Config) error {
	t.Helper()
	return webhooks.VerifyKeyring(t.Context(), shared.pool, cfg)
}

// TestWebhookSigningKeyRotation walks ADR-0008 for the webhook signing key. A customer holds the
// secret they were shown once, so replacing the server-side key must not change what their
// endpoint is signed with; only secrets issued after the rotation use the new key.
func TestWebhookSigningKeyRotation(t *testing.T) {
	// The tenant's endpoint is created under key 1, the key every deployment has used.
	before := newStack(t, time.Second)
	a := before.createTenant(t, "Webhook key rotation")
	rcv := newReceiver(t)
	wh := before.createWebhook(t, a.testKey, rcv.srv.URL, "alert.created")
	path := "/api/v1/webhooks/" + wh.Webhook.Id

	// The deployment is then rotated: key 2 is current and key 1 is kept for what it issued.
	cfg := testWebhookConfig()
	cfg.SigningKey, cfg.SigningKeyID = []byte("rotated-webhook-key-0123456789abcdef-2"), 2
	cfg.PreviousSigningKey, cfg.PreviousSigningKeyID = []byte(testWebhookKey), 1
	after := newStackWith(t, time.Second, generousLimits(), shared.rdb, withWebhooks(cfg))
	after.startWorker(t)

	t.Run("an existing secret still verifies", func(t *testing.T) {
		r := after.call(t, http.MethodPost, path+"/test", a.testKey, nil)
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		rec := rcv.next(t)
		assert.True(t, verify(t, rec, wh.Secret), "the customer's secret, issued under key 1, is unchanged")
	})

	var rotated string
	t.Run("a rotated secret uses the new key, and the old one is signed during the overlap", func(t *testing.T) {
		r := after.call(t, http.MethodPost, path+"/rotate-secret", a.testKey, map[string]any{"overlap_hours": 1})
		require.Equal(t, http.StatusOK, r.status, r.String())
		rotated = decode[apigen.WebhookWithSecret](t, r).Secret
		assert.NotEqual(t, wh.Secret, rotated)

		after.call(t, http.MethodPost, path+"/test", a.testKey, nil)
		rec := rcv.next(t)
		assert.Len(t, strings.Split(rec.header.Get("RiskOS-Signature"), ","), 2)
		assert.True(t, verify(t, rec, rotated), "new secret, derived with key 2")
		assert.True(t, verify(t, rec, wh.Secret), "previous secret, still derived with key 1")
	})

	t.Run("a deployment without key 1 refuses to start while an endpoint uses it", func(t *testing.T) {
		usage, err := webhooksKeyUsage(t)
		require.NoError(t, err)
		assert.Positive(t, usage[1], "the overlap still depends on key 1")
		assert.Positive(t, usage[2])

		without := testWebhookConfig()
		without.SigningKey, without.SigningKeyID = cfg.SigningKey, 2
		err = verifyKeyring(t, without)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "[1]")
		assert.NoError(t, verifyKeyring(t, cfg), "with the previous key configured it starts")
	})

	t.Run("once the overlap is over the old key is no longer needed by this endpoint", func(t *testing.T) {
		usage, err := webhooksKeyUsage(t)
		require.NoError(t, err)
		keyOneBefore := usage[1]

		r := after.call(t, http.MethodPost, path+"/rotate-secret", a.testKey, map[string]any{"overlap_hours": 0})
		require.Equal(t, http.StatusOK, r.status, r.String())
		third := decode[apigen.WebhookWithSecret](t, r).Secret
		after.call(t, http.MethodPost, path+"/test", a.testKey, nil)
		rec := rcv.next(t)
		assert.True(t, verify(t, rec, third))
		assert.False(t, verify(t, rec, wh.Secret), "no overlap: the key-1 secret stops")

		usage, err = webhooksKeyUsage(t)
		require.NoError(t, err)
		assert.Equal(t, keyOneBefore-1, usage[1], "this endpoint no longer depends on key 1")
	})
}
