//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// Events can carry what the rule pack needs (EVENTS.md#compliance); what does not fit the
// catalog is refused before anything is stored.
func TestComplianceEventContract(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Compliance events")

	cash := txn("cash-1")
	cash["compliance"] = map[string]any{
		"kind":  "cash_withdrawal",
		"facts": map[string]any{"customer.type": "individual", "account.age": 86400, "device.rooted": false},
		"refs":  map[string]any{"card": "crd_1"},
	}
	r := s.ingest(t, a.testKey, cash)
	require.Equal(t, http.StatusOK, r.status, r.String())
	id := *decode[apigen.RiskDecision](t, r).EventId
	stored := decode[apigen.EventDetail](t, s.call(t, http.MethodGet, "/api/v1/events/"+id, a.testKey, nil))
	assert.Equal(t, map[string]any{
		"kind":  "cash_withdrawal",
		"facts": map[string]any{"customer.type": "individual", "account.age": 86400.0, "device.rooted": false},
		"refs":  map[string]any{"card": "crd_1"},
	}, stored.NormalizedPayload["compliance"])

	export := map[string]any{
		"event_id": "exp-1", "type": "activity", "occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		"actor":      map[string]any{"user_id": "u_1"},
		"compliance": map[string]any{"kind": "data_export"},
	}
	r = s.ingest(t, a.testKey, export)
	require.Equal(t, http.StatusOK, r.status, "activity events are accepted: %s", r.String())

	n := 0
	for name, tc := range map[string]struct {
		compliance map[string]any
		status     int
		code       string
	}{
		"a kind that does not fit": {map[string]any{"kind": "login"}, http.StatusUnprocessableEntity, "INVALID_EVENT"},
		"a fact RiskOS computes":   {map[string]any{"facts": map[string]any{"device.is_new": true}}, http.StatusUnprocessableEntity, "INVALID_EVENT"},
		"an enum value not listed": {map[string]any{"facts": map[string]any{"customer.type": "trust"}}, http.StatusUnprocessableEntity, "INVALID_EVENT"},
		"a raw BVN":                {map[string]any{"refs": map[string]any{"bvn": "22212345678"}}, http.StatusUnprocessableEntity, "SENSITIVE_DATA"},
		"an unknown reference":     {map[string]any{"refs": map[string]any{"nin": "x"}}, http.StatusBadRequest, "INVALID_REQUEST"},
		"an unknown kind":          {map[string]any{"kind": "teleport"}, http.StatusBadRequest, "INVALID_REQUEST"},
	} {
		n++
		e := txn(fmt.Sprintf("bad-%d", n))
		e["compliance"] = tc.compliance
		r := s.ingest(t, a.testKey, e)
		assert.Equal(t, tc.status, r.status, "%s: %s", name, r.String())
		assert.Equal(t, tc.code, r.errorCode(t), name)
	}
	delete(export, "compliance")
	export["event_id"] = "exp-2"
	r = s.ingest(t, a.testKey, export)
	assert.Equal(t, http.StatusBadRequest, r.status, "an activity names its kind: %s", r.String())
}
