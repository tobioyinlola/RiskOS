//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/httpapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
)

// txn returns a valid transaction event as a generic map, so tests can mutate any field.
func txn(key string) map[string]any {
	return map[string]any{
		"event_id":    key,
		"type":        "transaction",
		"occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		"actor":       map[string]any{"user_id": "u_1", "account_id": "acc_1", "device_id": "dev_1"},
		"context":     map[string]any{"ip": "::ffff:203.0.113.7", "country": "NG"},
		"data": map[string]any{
			"transaction_id": "t_" + key, "amount": 250000, "currency": "NGN",
			"direction": "debit", "beneficiary_id": "ben_9", "channel": "bank_transfer",
		},
	}
}

func (s stack) ingest(t *testing.T, key string, event any) response {
	t.Helper()
	return s.call(t, http.MethodPost, "/api/v1/events", key, event)
}

func TestIngestReturnsTheStubDecision(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Ingest")

	r := s.ingest(t, a.testKey, txn("e1"))
	require.Equal(t, http.StatusOK, r.status, r.String())
	d := decode[apigen.RiskDecision](t, r)
	require.NotNil(t, d.EventId)
	assert.True(t, strings.HasPrefix(*d.EventId, "evt_"))
	assert.True(t, strings.HasPrefix(d.EvaluationId, "evl_"))
	assert.Equal(t, 0, d.RiskScore)
	assert.Equal(t, apigen.RiskLevel("low"), d.RiskLevel)
	assert.Equal(t, apigen.Decision("allow"), d.Decision)
	assert.Equal(t, "risk-engine-stub-0.1", d.EngineVersion)
	assert.Empty(t, d.Signals)
	assert.NotNil(t, d.Signals, "signals is an empty array, never null")
	assert.Empty(t, d.DegradedSignals)
	assert.True(t, strings.HasPrefix(d.RequestId, "req_"))
}

func TestIdempotency(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Idempotency")
	event := txn("same-key")

	first := s.ingest(t, a.testKey, event)
	require.Equal(t, http.StatusOK, first.status, first.String())

	t.Run("same payload replays the stored response", func(t *testing.T) {
		// Re-encode with a different key order: still the same payload.
		reordered := map[string]any{}
		for k, v := range event {
			reordered[k] = v
		}
		rec := s.rawCall(t, http.MethodPost, "/api/v1/events", a.testKey, reordered)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "true", rec.Header().Get("Idempotent-Replay"))
		assert.JSONEq(t, first.String(), rec.Body.String(), "identical body, including the original request_id")
		assert.NotEqual(t, decode[apigen.RiskDecision](t, first).RequestId, rec.Header().Get("X-Request-ID"),
			"the replay has its own request ID in the header")
	})
	t.Run("same key, different payload is a conflict", func(t *testing.T) {
		changed := txn("same-key")
		changed["data"].(map[string]any)["amount"] = 250001 //nolint:errcheck // test fixture
		r := s.ingest(t, a.testKey, changed)
		assert.Equal(t, http.StatusConflict, r.status)
		assert.Equal(t, "IDEMPOTENCY_CONFLICT", r.errorCode(t))
	})
	t.Run("only one event was stored", func(t *testing.T) {
		list := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", a.testKey, nil))
		assert.Len(t, list.Data, 1)
	})
}

func TestConcurrentDuplicatesProduceOneEvent(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Concurrent events")
	event := txn("race")

	const n = 20
	eventIDs := make(chan string, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			r := s.ingest(t, a.testKey, event)
			if assert.Equal(t, http.StatusOK, r.status, r.String()) {
				eventIDs <- *decode[apigen.RiskDecision](t, r).EventId
			}
		})
	}
	wg.Wait()
	close(eventIDs)
	distinct := map[string]bool{}
	for id := range eventIDs {
		distinct[id] = true
	}
	assert.Len(t, distinct, 1, "every concurrent duplicate got the same event")
	list := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", a.testKey, nil))
	assert.Len(t, list.Data, 1, "the database stored exactly one event")
}

func TestContractValidation(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Contract")
	mutate := func(f func(e map[string]any)) map[string]any {
		e := txn(fmt.Sprintf("v_%d", time.Now().UnixNano()))
		f(e)
		return e
	}
	data := func(e map[string]any) map[string]any { return e["data"].(map[string]any) } //nolint:errcheck // test fixture
	cases := map[string]map[string]any{
		"unknown top-level field": mutate(func(e map[string]any) { e["card_number"] = "x" }),
		"unknown data field":      mutate(func(e map[string]any) { data(e)["cvv"] = "123" }),
		"unknown actor field":     mutate(func(e map[string]any) { e["actor"].(map[string]any)["email"] = "a@b.c" }), //nolint:errcheck // test fixture
		"missing required":        mutate(func(e map[string]any) { delete(data(e), "currency") }),
		"negative amount":         mutate(func(e map[string]any) { data(e)["amount"] = -1 }),
		"fractional amount":       mutate(func(e map[string]any) { data(e)["amount"] = 10.5 }),
		"lowercase currency":      mutate(func(e map[string]any) { data(e)["currency"] = "ngn" }),
		"unknown enum":            mutate(func(e map[string]any) { data(e)["direction"] = "sideways" }),
		"unknown event type":      mutate(func(e map[string]any) { e["type"] = "wire" }),
		"data of another type":    mutate(func(e map[string]any) { e["type"] = "login" }),
		"bad event_id":            mutate(func(e map[string]any) { e["event_id"] = "has spaces" }),
		"bad timestamp":           mutate(func(e map[string]any) { e["occurred_at"] = "yesterday" }),
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			r := s.ingest(t, a.testKey, event)
			assert.Equal(t, http.StatusBadRequest, r.status, r.String())
			e := decode[apigen.ErrorResponse](t, r).Error
			assert.Equal(t, "INVALID_REQUEST", e.Code)
			assert.True(t, strings.HasPrefix(e.Message, "Invalid "), e.Message)
		})
	}
	t.Run("malformed JSON", func(t *testing.T) {
		rec := s.rawBody(t, http.MethodPost, "/api/v1/events", a.testKey, `{"event_id":`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("nothing was stored", func(t *testing.T) {
		list := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", a.testKey, nil))
		assert.Empty(t, list.Data)
	})
}

func TestSemanticValidation(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Semantic")
	cases := []struct {
		name  string
		event map[string]any
		code  string
	}{
		{"stale", withField(txn("stale"), "occurred_at", time.Now().Add(-25*time.Hour).Format(time.RFC3339)), "STALE_EVENT"},
		{"future", withField(txn("future"), "occurred_at", time.Now().Add(10*time.Minute).Format(time.RFC3339)), "INVALID_EVENT"},
		{"bad ip", withField(txn("ip"), "context", map[string]any{"ip": "999.1.1.1"}), "INVALID_EVENT"},
		{"card number as an identifier", withField(txn("pan"), "actor", map[string]any{"user_id": "4111111111111111"}), "SENSITIVE_DATA"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := s.ingest(t, a.testKey, tt.event)
			assert.Equal(t, http.StatusUnprocessableEntity, r.status, r.String())
			assert.Equal(t, tt.code, r.errorCode(t))
			assert.NotContains(t, r.String(), "4111111111111111", "the card number is never echoed")
		})
	}
	list := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", a.testKey, nil))
	assert.Empty(t, list.Data, "rejected events are not stored")
}

func withField(e map[string]any, k string, v any) map[string]any {
	e[k] = v
	return e
}

func TestEventExplorer(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Explorer")
	sent := txn("explore")
	ingested := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, sent))

	r := s.call(t, http.MethodGet, "/api/v1/events/"+*ingested.EventId, a.testKey, nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	d := decode[apigen.EventDetail](t, r)
	assert.Equal(t, "explore", d.EventKey)
	assert.Equal(t, apigen.EventType("transaction"), d.Type)
	assert.Equal(t, apigen.Environment("test"), d.Environment)
	assert.Equal(t, ingested, d.Evaluation, "the stored evaluation is exactly what was returned")

	rawSent, _ := json.Marshal(sent)
	rawBack, _ := json.Marshal(d.RawPayload)
	assert.JSONEq(t, string(rawSent), string(rawBack), "raw payload is stored as received")
	ctx := d.NormalizedPayload["context"].(map[string]any) //nolint:errcheck // shape asserted by the contract
	assert.Equal(t, "203.0.113.7", ctx["ip"], "normalized: IPv4-mapped address unmapped")
	assert.True(t, strings.HasSuffix(d.NormalizedPayload["occurred_at"].(string), "Z"), "normalized: UTC") //nolint:errcheck // shape asserted by the contract

	for i := range 3 {
		s.ingest(t, a.testKey, txn(fmt.Sprintf("page-%d", i)))
	}
	page1 := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events?limit=2", a.testKey, nil))
	require.Len(t, page1.Data, 2)
	assert.Equal(t, "page-2", page1.Data[0].EventKey, "newest first")
	require.NotNil(t, page1.NextCursor)
	page2 := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events?limit=2&cursor="+*page1.NextCursor, a.testKey, nil))
	require.Len(t, page2.Data, 2)
	assert.Nil(t, page2.NextCursor)
	assert.Equal(t, "explore", page2.Data[1].EventKey)
}

func TestDryRunStoresNoEvent(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Dry run")

	first := decode[apigen.RiskDecision](t, s.call(t, http.MethodPost, "/api/v1/risk/evaluate", a.testKey, txn("what-if")))
	second := decode[apigen.RiskDecision](t, s.call(t, http.MethodPost, "/api/v1/risk/evaluate", a.testKey, txn("what-if")))
	assert.Nil(t, first.EventId)
	assert.NotEqual(t, first.EvaluationId, second.EvaluationId, "dry runs are not idempotent: each is evaluated")

	list := decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", a.testKey, nil))
	assert.Empty(t, list.Data)
	var dryRuns int
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT count(*) FROM evaluations WHERE tenant_id = $1 AND dry_run AND event_id IS NULL`, a.id).Scan(&dryRuns))
	assert.Equal(t, 2, dryRuns, "each dry run is recorded, flagged")

	// A real ingest with the same key afterwards is unaffected by the dry runs.
	assert.Equal(t, http.StatusOK, s.ingest(t, a.testKey, txn("what-if")).status)
}

func TestEventScopes(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Event scopes")
	_, reader := s.createKey(t, a.testKey, "reader", "events:read")
	_, writer := s.createKey(t, a.testKey, "writer", "events:write")

	assert.Equal(t, http.StatusForbidden, s.ingest(t, reader, txn("r1")).status)
	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, "/api/v1/risk/evaluate", writer, txn("r2")).status)
	assert.Equal(t, http.StatusOK, s.ingest(t, writer, txn("r3")).status)
	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodGet, "/api/v1/events", writer, nil).status)
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/events", reader, nil).status)
}

func TestEventIsolation(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Events A")
	b := s.createTenant(t, "Events B")
	aProd := s.productionKey(t, a.id)

	aEvent := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, txn("shared-key")))
	t.Run("other tenant cannot read it", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/events/"+*aEvent.EventId, b.testKey, nil)
		assert.Equal(t, http.StatusNotFound, r.status)
	})
	t.Run("other environment cannot read it", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/events/"+*aEvent.EventId, aProd, nil)
		assert.Equal(t, http.StatusNotFound, r.status)
	})
	t.Run("idempotency keys are per tenant and environment", func(t *testing.T) {
		bEvent := s.ingest(t, b.testKey, txn("shared-key"))
		require.Equal(t, http.StatusOK, bEvent.status)
		assert.NotEqual(t, aEvent.EventId, decode[apigen.RiskDecision](t, bEvent).EventId, "not a replay of A's event")
		prod := s.ingest(t, aProd, txn("shared-key"))
		require.Equal(t, http.StatusOK, prod.status)
		assert.NotEqual(t, aEvent.EventId, decode[apigen.RiskDecision](t, prod).EventId, "production is separate from test")
	})
	t.Run("lists are scoped", func(t *testing.T) {
		for _, e := range decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", b.testKey, nil)).Data {
			assert.NotEqual(t, *aEvent.EventId, e.Id)
		}
	})
	t.Run("rls hides events without a scope", func(t *testing.T) {
		var n int
		require.NoError(t, shared.pool.QueryRow(t.Context(), `SELECT count(*) FROM events`).Scan(&n))
		assert.Zero(t, n)
	})
}

func TestRateLimits(t *testing.T) {
	s := newStackWithLimits(t, time.Second, httpapi.RateLimits{
		PerKey:    ratelimit.Limit{PerSecond: 1, Burst: 3},
		PerTenant: ratelimit.Limit{PerSecond: 1, Burst: 5},
	})
	a := s.createTenant(t, "Rate limited")
	// Budget accounting. createKey is one request: key A 2/3 left, tenant 4/5 left.
	_, second := s.createKey(t, a.testKey, "second", "events:read")

	statuses := make([]int, 3)
	for i := range statuses {
		statuses[i] = s.call(t, http.MethodGet, "/api/v1/whoami", a.testKey, nil).status
	}
	assert.Equal(t, []int{200, 200, http.StatusTooManyRequests}, statuses, "key A exhausts its burst of 3")
	// Rejected requests spend nothing: tenant 2/5 left.

	rec := s.rawCall(t, http.MethodGet, "/api/v1/whoami", a.testKey, nil)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), `"code":"RATE_LIMITED"`)

	// Key B has its full burst of 3, but the tenant has only 2 left.
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/whoami", second, nil).status)
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/whoami", second, nil).status)
	assert.Equal(t, http.StatusTooManyRequests, s.call(t, http.MethodGet, "/api/v1/whoami", second, nil).status,
		"the tenant limit applies across keys")

	assert.Eventually(t, func() bool {
		return s.call(t, http.MethodGet, "/api/v1/whoami", a.testKey, nil).status == http.StatusOK
	}, 3*time.Second, 100*time.Millisecond, "budget refills at the sustained rate")
}

func TestTimePartitions(t *testing.T) {
	ctx := t.Context()
	created, err := db.EnsureTimePartitions(ctx, shared.pool, db.PartitionMonthsAhead)
	require.NoError(t, err, "the application role can maintain partitions through the function")
	again, err := db.EnsureTimePartitions(ctx, shared.pool, db.PartitionMonthsAhead)
	require.NoError(t, err)
	assert.Zero(t, again, "idempotent")
	assert.GreaterOrEqual(t, created, 0)

	var parts int
	require.NoError(t, shared.owner.QueryRow(ctx,
		`SELECT count(*) FROM pg_inherits WHERE inhparent IN ('events'::regclass, 'evaluations'::regclass)`).Scan(&parts))
	assert.Equal(t, 2*(db.PartitionMonthsAhead+1), parts, "current month plus months ahead, for both tables")

	month := time.Now().UTC().Format("200601")
	_, err = shared.pool.Exec(ctx, "SELECT * FROM events_p"+month)
	assert.ErrorContains(t, err, "permission denied", "partitions are reachable only through the RLS-protected parent")
}

func TestEventsAreInsertOnly(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Insert only")
	e := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, txn("immutable")))
	id, err := ids.Parse(ids.PrefixEvent, *e.EventId)
	require.NoError(t, err)

	_, err = shared.owner.Exec(t.Context(), `UPDATE events SET risk_score = 99 WHERE id = $1`, id)
	assert.ErrorContains(t, err, "insert-only")
	_, err = shared.owner.Exec(t.Context(), `DELETE FROM evaluations WHERE tenant_id = $1`, a.id)
	assert.ErrorContains(t, err, "insert-only")
}
