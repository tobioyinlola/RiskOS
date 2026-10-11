//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookTenantFairness shows that a tenant with slow endpoints cannot occupy every delivery
// slot: its deliveries run one at a time (the per-tenant bound) while another tenant's are
// delivered at once, and nothing is lost, only put off.
func TestWebhookTenantFairness(t *testing.T) {
	cfg := testWebhookConfig()
	cfg.TenantConcurrency = 1
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withWebhooks(cfg))
	s.startWorker(t)

	slowTenant := s.createTenant(t, "Fair slow")
	quickTenant := s.createTenant(t, "Fair quick")
	slow, quick := newReceiver(t), newReceiver(t)
	slow.delay.Store(int64(700 * time.Millisecond))
	slowHook := s.createWebhook(t, slowTenant.testKey, slow.srv.URL+"/slow", "case.created")
	quickHook := s.createWebhook(t, quickTenant.testKey, quick.srv.URL+"/quick", "case.created")

	// Four deliveries to the slow tenant's endpoint, then one to the quick tenant's.
	for range 4 {
		r := s.call(t, http.MethodPost, "/api/v1/webhooks/"+slowHook.Webhook.Id+"/test", slowTenant.testKey, nil)
		require.Equal(t, http.StatusAccepted, r.status, r.String())
	}
	start := time.Now()
	r := s.call(t, http.MethodPost, "/api/v1/webhooks/"+quickHook.Webhook.Id+"/test", quickTenant.testKey, nil)
	require.Equal(t, http.StatusAccepted, r.status, r.String())

	quick.next(t)
	assert.Less(t, time.Since(start), 2*time.Second, "the other tenant is not queued behind the slow one")

	for range 4 {
		slow.next(t)
	}
	assert.Equal(t, int32(1), slow.peak.Load(), "the slow tenant's deliveries ran one at a time")
	assert.Equal(t, int32(1), quick.peak.Load())
}
