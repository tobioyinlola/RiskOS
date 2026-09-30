//go:build integration

package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// received is one webhook request as the customer's receiver saw it.
type received struct {
	header http.Header
	body   []byte
	event  struct {
		ID          string         `json:"id"`
		Type        string         `json:"type"`
		Environment string         `json:"environment"`
		Data        map[string]any `json:"data"`
	}
}

// receiver is a customer endpoint. status decides each response (200 by default).
type receiver struct {
	srv    *httptest.Server
	got    chan received
	status atomic.Int32
	mu     sync.Mutex
	all    []received
	// held are received deliveries not yet consumed by nextOfType (test goroutine only).
	held []received
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{got: make(chan received, 100)}
	r.status.Store(http.StatusOK)
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var rec received
		rec.header = req.Header.Clone()
		rec.body, _ = io.ReadAll(req.Body)
		_ = json.Unmarshal(rec.body, &rec.event)
		r.mu.Lock()
		r.all = append(r.all, rec)
		r.mu.Unlock()
		r.got <- rec
		w.WriteHeader(int(r.status.Load()))
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) next(t *testing.T) received {
	t.Helper()
	select {
	case rec := <-r.got:
		return rec
	case <-time.After(20 * time.Second):
		t.Fatal("no webhook received within 20s")
		return received{}
	}
}

// nextOfType waits for the next delivery of eventType. Deliveries run concurrently, so other
// events that arrive first are kept for later calls, never dropped.
func (r *receiver) nextOfType(t *testing.T, eventType string) received {
	t.Helper()
	for i, rec := range r.held {
		if rec.event.Type == eventType {
			r.held = append(r.held[:i], r.held[i+1:]...)
			return rec
		}
	}
	deadline := time.After(20 * time.Second)
	for {
		select {
		case rec := <-r.got:
			if rec.event.Type == eventType {
				return rec
			}
			r.held = append(r.held, rec)
		case <-deadline:
			t.Fatalf("no %s webhook within 20s", eventType)
			return received{}
		}
	}
}

// verify checks a delivery the way docs/WEBHOOKS.md tells customers to: recompute the HMAC over
// "<id>.<timestamp>.<body>" with the secret and compare it to any v1 signature.
func verify(t *testing.T, rec received, secret string) bool {
	t.Helper()
	ts := rec.header.Get("RiskOS-Webhook-Timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), time.Unix(sec, 0), 5*time.Minute, "timestamp within tolerance")
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(rec.header.Get("RiskOS-Webhook-Id") + "." + ts + "."))
	m.Write(rec.body)
	want := "v1=" + hex.EncodeToString(m.Sum(nil))
	for _, sig := range strings.Split(rec.header.Get("RiskOS-Signature"), ",") {
		if hmac.Equal([]byte(sig), []byte(want)) {
			return true
		}
	}
	return false
}

func (s stack) createWebhook(t *testing.T, key, url string, types ...string) apigen.WebhookWithSecret {
	t.Helper()
	r := s.call(t, http.MethodPost, "/api/v1/webhooks", key, map[string]any{"url": url, "event_types": types})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	return decode[apigen.WebhookWithSecret](t, r)
}

// obj returns m[key] as a JSON object, failing the test otherwise.
func obj(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	require.True(t, ok, "%s is an object", key)
	return v
}

func allEvents() []string {
	return []string{"alert.created", "alert.updated", "case.created", "case.resolved"}
}

// TestWebhooksEndToEnd: every alert and case event reaches the customer's endpoint, signed.
func TestWebhooksEndToEnd(t *testing.T) {
	s := newStack(t, time.Second)
	s.startWorker(t)
	a := s.createTenant(t, "Webhooks E2E")
	rcv := newReceiver(t)
	wh := s.createWebhook(t, a.testKey, rcv.srv.URL+"/riskos", allEvents()...)
	assert.Regexp(t, `^whsec_`, wh.Secret)
	s.mustRule(t, a.testKey, alwaysReview())

	d := s.decide(t, a.testKey, debit(actor{"u_wh", "acc_wh", "dev_wh"}, "ben", "NG", "NGN", 5000))
	created := rcv.nextOfType(t, "alert.created")
	assert.True(t, verify(t, created, wh.Secret), "signature verifies with the secret")
	assert.Equal(t, created.event.ID, created.header.Get("RiskOS-Webhook-Id"))
	assert.Equal(t, "alert.created", created.header.Get("RiskOS-Webhook-Type"))
	assert.Equal(t, "application/json", created.header.Get("Content-Type"))
	assert.Regexp(t, `^wev_`, created.event.ID)
	assert.Equal(t, "test", created.event.Environment)
	alert := obj(t, created.event.Data, "alert")
	assert.Equal(t, "u_wh", obj(t, alert, "subject")["ref"])
	assert.NotContains(t, string(created.body), wh.Secret)
	assert.NotContains(t, string(created.body), d.EvaluationId, "payloads carry the alert, not internals")

	alertID := alert["id"].(string) //nolint:errcheck // test fixture
	c := decode[apigen.Case](t, s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Webhook case", "alert_ids": []string{alertID}}))
	esc := rcv.nextOfType(t, "alert.updated")
	assert.Equal(t, "open", esc.event.Data["previous_status"])
	assert.Equal(t, "escalated", obj(t, esc.event.Data, "alert")["status"])
	cc := rcv.nextOfType(t, "case.created")
	assert.Equal(t, c.Id, obj(t, cc.event.Data, "case")["id"])
	assert.Equal(t, []any{alertID}, cc.event.Data["alert_ids"])

	r := s.call(t, http.MethodPatch, "/api/v1/cases/"+c.Id, a.testKey, map[string]any{"status": "resolved", "resolution": "legitimate"})
	require.Equal(t, http.StatusOK, r.status, r.String())
	resolved := rcv.nextOfType(t, "case.resolved")
	assert.Equal(t, "legitimate", obj(t, resolved.event.Data, "case")["resolution"])

	t.Run("delivery log", func(t *testing.T) {
		require.Eventually(t, func() bool {
			list := decode[apigen.WebhookDeliveryList](t, s.call(t, http.MethodGet, "/api/v1/webhooks/"+wh.Webhook.Id+"/deliveries?status=succeeded", a.testKey, nil))
			return len(list.Data) >= 5
		}, 10*time.Second, 50*time.Millisecond, "5 deliveries: created, escalated, case created, closed, resolved")
		list := decode[apigen.WebhookDeliveryList](t, s.call(t, http.MethodGet, "/api/v1/webhooks/"+wh.Webhook.Id+"/deliveries", a.testKey, nil))
		detail := decode[apigen.WebhookDeliveryDetail](t, s.call(t, http.MethodGet,
			"/api/v1/webhooks/"+wh.Webhook.Id+"/deliveries/"+list.Data[0].Id, a.testKey, nil))
		require.Len(t, detail.Attempts, 1)
		require.NotNil(t, detail.Attempts[0].StatusCode)
		assert.Equal(t, http.StatusOK, *detail.Attempts[0].StatusCode)
		assert.NotEmpty(t, detail.Payload["id"])
	})
	t.Run("the secret is never shown again", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/webhooks/"+wh.Webhook.Id, a.testKey, nil)
		assert.NotContains(t, r.String(), wh.Secret)
		list := s.call(t, http.MethodGet, "/api/v1/webhooks", a.testKey, nil)
		assert.NotContains(t, list.String(), wh.Secret)
	})
	t.Run("only subscribed events are sent", func(t *testing.T) {
		only := newReceiver(t)
		s.createWebhook(t, a.testKey, only.srv.URL, "case.created")
		s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Second"})
		assert.Equal(t, "case.created", only.next(t).event.Type)
		s.decide(t, a.testKey, debit(actor{"u_wh2", "acc", "dev"}, "ben", "NG", "NGN", 5000))
		rcv.nextOfType(t, "alert.created")
		select {
		case rec := <-only.got:
			t.Fatalf("unsubscribed event delivered: %s", rec.event.Type)
		case <-time.After(300 * time.Millisecond):
		}
	})
}

func TestWebhookRetriesRedeliveryAndRotation(t *testing.T) {
	s := newStack(t, time.Second)
	s.startWorker(t)
	a := s.createTenant(t, "Webhook retries")
	rcv := newReceiver(t)
	wh := s.createWebhook(t, a.testKey, rcv.srv.URL, "alert.created")
	path := "/api/v1/webhooks/" + wh.Webhook.Id

	rcv.status.Store(http.StatusInternalServerError)
	r := s.call(t, http.MethodPost, path+"/test", a.testKey, nil)
	require.Equal(t, http.StatusAccepted, r.status, r.String())
	delivery := decode[apigen.WebhookDelivery](t, r)
	first := rcv.next(t)
	assert.Equal(t, "webhook.test", first.event.Type)
	rcv.next(t)
	rcv.next(t)

	var detail apigen.WebhookDeliveryDetail
	require.Eventually(t, func() bool {
		detail = decode[apigen.WebhookDeliveryDetail](t, s.call(t, http.MethodGet, path+"/deliveries/"+delivery.Id, a.testKey, nil))
		return detail.Delivery.Status == "failed"
	}, 10*time.Second, 50*time.Millisecond, "dead-lettered after the schedule (3 attempts in this test config)")
	require.Len(t, detail.Attempts, 3)
	for _, at := range detail.Attempts {
		assert.Equal(t, "http status 500", *at.Error)
	}
	assert.Equal(t, delivery.EventId, first.header.Get("RiskOS-Webhook-Id"), "every retry keeps the event ID")
	wh1 := decode[apigen.Webhook](t, s.call(t, http.MethodGet, path, a.testKey, nil))
	assert.NotNil(t, wh1.FailingSince)

	t.Run("redeliver", func(t *testing.T) {
		rcv.status.Store(http.StatusNoContent)
		r := s.call(t, http.MethodPost, path+"/deliveries/"+delivery.Id+"/redeliver", a.testKey, nil)
		require.Equal(t, http.StatusAccepted, r.status, r.String())
		assert.Equal(t, apigen.WebhookDeliveryStatus("pending"), decode[apigen.WebhookDelivery](t, r).Status)
		again := rcv.next(t)
		assert.Equal(t, delivery.EventId, again.header.Get("RiskOS-Webhook-Id"))
		require.Eventually(t, func() bool {
			d := decode[apigen.WebhookDeliveryDetail](t, s.call(t, http.MethodGet, path+"/deliveries/"+delivery.Id, a.testKey, nil))
			return d.Delivery.Status == "succeeded" && len(d.Attempts) == 4
		}, 10*time.Second, 50*time.Millisecond)
		assert.Nil(t, decode[apigen.Webhook](t, s.call(t, http.MethodGet, path, a.testKey, nil)).FailingSince, "a success clears the failure state")
		assert.Equal(t, http.StatusConflict, s.call(t, http.MethodPost, path+"/deliveries/"+delivery.Id+"/redeliver", a.testKey, nil).status,
			"only failed deliveries")
	})
	t.Run("rotation signs with both secrets during the overlap", func(t *testing.T) {
		r := s.call(t, http.MethodPost, path+"/rotate-secret", a.testKey, map[string]any{"overlap_hours": 1})
		require.Equal(t, http.StatusOK, r.status, r.String())
		rotated := decode[apigen.WebhookWithSecret](t, r)
		assert.NotEqual(t, wh.Secret, rotated.Secret)
		require.NotNil(t, rotated.Webhook.PreviousSecretValidUntil)
		s.call(t, http.MethodPost, path+"/test", a.testKey, nil)
		rec := rcv.next(t)
		assert.Len(t, strings.Split(rec.header.Get("RiskOS-Signature"), ","), 2)
		assert.True(t, verify(t, rec, rotated.Secret), "new secret")
		assert.True(t, verify(t, rec, wh.Secret), "previous secret, during the overlap")

		r = s.call(t, http.MethodPost, path+"/rotate-secret", a.testKey, map[string]any{"overlap_hours": 0})
		require.Equal(t, http.StatusOK, r.status)
		third := decode[apigen.WebhookWithSecret](t, r).Secret
		s.call(t, http.MethodPost, path+"/test", a.testKey, nil)
		rec = rcv.next(t)
		assert.True(t, verify(t, rec, third))
		assert.False(t, verify(t, rec, rotated.Secret), "no overlap: the old secret stops at once")
	})
	logs := auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil))
	for _, want := range []string{"webhook.created", "webhook.redelivered", "webhook.secret_rotated"} {
		assert.Contains(t, logs, want)
	}
}

func TestWebhookAutoDisable(t *testing.T) {
	cfg := testWebhookConfig()
	cfg.DisableAfter = time.Millisecond
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withWebhooks(cfg))
	s.startWorker(t)
	a := s.createTenant(t, "Webhook disable")
	rcv := newReceiver(t)
	rcv.status.Store(http.StatusServiceUnavailable)
	wh := s.createWebhook(t, a.testKey, rcv.srv.URL, "alert.created")
	path := "/api/v1/webhooks/" + wh.Webhook.Id

	require.Equal(t, http.StatusAccepted, s.call(t, http.MethodPost, path+"/test", a.testKey, nil).status)
	var ep apigen.Webhook
	require.Eventually(t, func() bool {
		ep = decode[apigen.Webhook](t, s.call(t, http.MethodGet, path, a.testKey, nil))
		return ep.Status == "disabled"
	}, 10*time.Second, 50*time.Millisecond)
	require.NotNil(t, ep.DisabledReason)
	assert.Contains(t, *ep.DisabledReason, "every delivery failed since")

	sec := s.waitForAlerts(t, a.testKey, "kind=security", 1)
	assert.Equal(t, apigen.AlertSubjectType("webhook_endpoint"), sec[0].Subject.Type)
	assert.Equal(t, wh.Webhook.Id, sec[0].Subject.Ref)
	assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)), "webhook.disabled")
	assert.Equal(t, http.StatusConflict, s.call(t, http.MethodPost, path+"/test", a.testKey, nil).status)

	rcv.status.Store(http.StatusOK)
	r := s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"status": "active"})
	require.Equal(t, http.StatusOK, r.status, r.String())
	enabled := decode[apigen.Webhook](t, r)
	assert.Nil(t, enabled.FailingSince)
	assert.Nil(t, enabled.DisabledReason)
}

func TestWebhookURLPolicy(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Webhook URLs")
	prod := s.productionKey(t, a.id)
	create := func(key, url string) int {
		return s.call(t, http.MethodPost, "/api/v1/webhooks", key, map[string]any{"url": url, "event_types": []string{"alert.created"}}).status
	}

	for _, u := range []string{"http://hooks.example.com", "https://127.0.0.1/x", "https://10.0.0.8/", "https://169.254.169.254/latest",
		"https://localhost/x", "https://user:pw@hooks.example.com/", "https://[::1]/"} {
		assert.Equal(t, http.StatusBadRequest, create(prod, u), "production: %s", u)
	}
	assert.Equal(t, http.StatusCreated, create(prod, "https://hooks.example.com/riskos"))
	assert.Equal(t, http.StatusCreated, create(a.testKey, "http://127.0.0.1:9/local"), "test environment with the dev flag")
	assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodPost, "/api/v1/webhooks", a.testKey,
		map[string]any{"url": "https://hooks.example.com", "event_types": []string{"everything"}}).status)

	t.Run("production deliveries never reach private addresses", func(t *testing.T) {
		// A hostname passes URL validation but resolves to loopback; the delivery must refuse it.
		cfg := testWebhookConfig()
		cfg.AllowPrivateTest = false
		strict := newStackWith(t, time.Second, generousLimits(), shared.rdb, withWebhooks(cfg))
		strict.startWorker(t)
		b := strict.createTenant(t, "Webhook rebinding")
		rcv := newReceiver(t)
		port := rcv.srv.URL[strings.LastIndex(rcv.srv.URL, ":")+1:]
		wh := strict.createWebhook(t, b.testKey, "https://localtest.me:"+port+"/x", "alert.created")
		d := decode[apigen.WebhookDelivery](t, strict.call(t, http.MethodPost, "/api/v1/webhooks/"+wh.Webhook.Id+"/test", b.testKey, nil))
		require.Eventually(t, func() bool {
			detail := decode[apigen.WebhookDeliveryDetail](t, strict.call(t, http.MethodGet,
				"/api/v1/webhooks/"+wh.Webhook.Id+"/deliveries/"+d.Id, b.testKey, nil))
			return len(detail.Attempts) > 0 && detail.Attempts[0].Error != nil &&
				(*detail.Attempts[0].Error == "destination address not allowed" || *detail.Attempts[0].Error == "dns lookup failed")
		}, 10*time.Second, 50*time.Millisecond)
		select {
		case <-rcv.got:
			t.Fatal("a delivery reached a loopback address")
		case <-time.After(200 * time.Millisecond):
		}
	})
	t.Run("endpoint limit", func(t *testing.T) {
		c := s.createTenant(t, "Webhook limit")
		for range 20 {
			require.Equal(t, http.StatusCreated, create(c.testKey, "https://hooks.example.com/x"))
		}
		assert.Equal(t, http.StatusConflict, create(c.testKey, "https://hooks.example.com/x"))
	})
}

func TestWebhookScopesAndIsolation(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Webhooks A")
	b := s.createTenant(t, "Webhooks B")
	wh := s.createWebhook(t, a.testKey, "https://hooks.example.com/a", "alert.created")
	path := "/api/v1/webhooks/" + wh.Webhook.Id

	_, reader := s.createKey(t, a.testKey, "webhook reader", "webhooks:read")
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, path, reader, nil).status)
	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, "/api/v1/webhooks", reader,
		map[string]any{"url": "https://x.example.com", "event_types": []string{"alert.created"}}).status)
	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, path+"/rotate-secret", reader, map[string]any{}).status)

	for _, p := range []string{path, path + "/deliveries"} {
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, p, b.testKey, nil).status, p)
	}
	assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPatch, path, b.testKey, map[string]any{"status": "disabled"}).status)
	assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPost, path+"/rotate-secret", b.testKey, map[string]any{}).status)
	assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPost, path+"/test", b.testKey, nil).status)
	assert.Empty(t, decode[apigen.WebhookList](t, s.call(t, http.MethodGet, "/api/v1/webhooks", b.testKey, nil)).Data)

	t.Run("history is insert-only and scoped", func(t *testing.T) {
		_, err := shared.owner.Exec(t.Context(), "UPDATE webhook_attempts SET attempt = attempt")
		assert.Error(t, err)
		for _, table := range []string{"webhook_endpoints", "webhook_deliveries", "webhook_attempts"} {
			var n int
			require.NoError(t, shared.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
			assert.Zero(t, n, table)
		}
	})
}
