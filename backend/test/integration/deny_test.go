//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// TestDenyList covers hard blocks: a listed identifier blocks whatever the score, expired and
// removed entries stop applying, and every change is audited.
func TestDenyList(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Deny list")
	b := s.createTenant(t, "Deny list other")
	add := func(key string, body map[string]any) response {
		return s.call(t, http.MethodPost, "/api/v1/deny-list", key, body)
	}

	// A low-risk event from a fresh device is allowed.
	d := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, actorTxn("before", "u_1", "acc_1", "dev_bad", 1000)))
	require.Equal(t, apigen.Decision("allow"), d.Decision)

	r := add(a.testKey, map[string]any{"kind": "device", "value": "dev_bad", "reason": "Emulator farm"})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	entry := decode[apigen.DenyEntry](t, r)

	t.Run("a listed identifier blocks whatever the score", func(t *testing.T) {
		d := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, actorTxn("after", "u_1", "acc_1", "dev_bad", 1000)))
		assert.Equal(t, apigen.Decision("block"), d.Decision)
		assert.Contains(t, d.Signals, apigen.Signal{Code: "deny_list", Label: "On your deny list: device dev_bad", Weight: 0})
		assert.Less(t, d.RiskScore, 50, "the score is not inflated; the block comes from the list")

		other := decode[apigen.RiskDecision](t, s.ingest(t, b.testKey, actorTxn("x", "u_1", "acc_1", "dev_bad", 1000)))
		assert.NotEqual(t, apigen.Decision("block"), other.Decision, "another tenant's list does not apply")
	})

	t.Run("countries, IP addresses and beneficiaries", func(t *testing.T) {
		require.Equal(t, http.StatusCreated, add(a.testKey, map[string]any{"kind": "ip", "value": "::ffff:198.51.100.9"}).status)
		e := actorTxn("ip", "u_2", "acc_2", "dev_ok", 1000)
		e["context"] = map[string]any{"ip": "198.51.100.9", "country": "NG"}
		assert.Equal(t, apigen.Decision("block"), decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, e)).Decision)

		require.Equal(t, http.StatusCreated, add(a.testKey, map[string]any{"kind": "beneficiary", "value": "ben_mule"}).status)
		e = actorTxn("ben", "u_3", "acc_3", "dev_ok", 1000)
		data, ok := e["data"].(map[string]any)
		require.True(t, ok)
		data["beneficiary_id"] = "ben_mule"
		assert.Equal(t, apigen.Decision("block"), decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, e)).Decision)
	})

	t.Run("validation", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, add(a.testKey, map[string]any{"kind": "device", "value": "dev_bad"}).status)
		assert.Equal(t, http.StatusBadRequest, add(a.testKey, map[string]any{"kind": "ip", "value": "999.1.1.1"}).status)
		assert.Equal(t, http.StatusBadRequest, add(a.testKey, map[string]any{"kind": "country", "value": "Nigeria"}).status)
		past := time.Now().Add(-time.Hour).Format(time.RFC3339)
		assert.Equal(t, http.StatusBadRequest, add(a.testKey, map[string]any{"kind": "user", "value": "u_9", "expires_at": past}).status)
		r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, ruleBody("deny_list", 10))
		assert.Equal(t, http.StatusBadRequest, r.status, "the deny-list signal code is reserved")
	})

	t.Run("expired entries stop applying", func(t *testing.T) {
		soon := time.Now().Add(time.Hour).Format(time.RFC3339)
		r := add(a.testKey, map[string]any{"kind": "user", "value": "u_temp", "expires_at": soon})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		assert.Equal(t, apigen.Decision("block"),
			decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, actorTxn("t1", "u_temp", "acc_t", "dev_t", 1000))).Decision)
		_, err := shared.owner.Exec(t.Context(),
			`UPDATE deny_list_entries SET expires_at = now() - interval '1 minute' WHERE tenant_id = $1 AND value = 'u_temp'`, a.id)
		require.NoError(t, err)
		// Any change to the list refreshes the cached copy at once.
		require.Equal(t, http.StatusCreated, add(a.testKey, map[string]any{"kind": "user", "value": "u_unrelated"}).status)
		assert.NotEqual(t, apigen.Decision("block"),
			decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, actorTxn("t2", "u_temp", "acc_t", "dev_t", 1000))).Decision)
	})

	t.Run("removal and listing", func(t *testing.T) {
		list := decode[apigen.DenyEntryList](t, s.call(t, http.MethodGet, "/api/v1/deny-list", a.testKey, nil))
		assert.Equal(t, 5, list.Total)
		assert.Equal(t, 10000, list.Limit)
		devices := decode[apigen.DenyEntryList](t, s.call(t, http.MethodGet, "/api/v1/deny-list?kind=device", a.testKey, nil))
		require.Len(t, devices.Data, 1)
		assert.Equal(t, "Emulator farm", devices.Data[0].Reason)

		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodDelete, "/api/v1/deny-list/"+entry.Id, b.testKey, nil).status,
			"another tenant cannot remove it")
		assert.Equal(t, http.StatusNoContent, s.call(t, http.MethodDelete, "/api/v1/deny-list/"+entry.Id, a.testKey, nil).status)
		d := decode[apigen.RiskDecision](t, s.ingest(t, a.testKey, actorTxn("removed", "u_1", "acc_1", "dev_bad", 1000)))
		assert.NotEqual(t, apigen.Decision("block"), d.Decision)

		actions := auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil))
		assert.Contains(t, actions, "deny_list.added")
		assert.Contains(t, actions, "deny_list.removed")
	})
}
