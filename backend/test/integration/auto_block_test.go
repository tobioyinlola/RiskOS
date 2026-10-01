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

// TestAutoBlock covers automatic blocking: off by default, shadow evidence first, on only after
// the shadow period, and the kill switch.
func TestAutoBlock(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Auto block")

	// An active rule that makes large transfers Critical.
	r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, map[string]any{
		"code": "huge_transfer", "name": "Huge transfer", "severity": "critical", "weight": 100, "mode": "active",
		"condition": map[string]any{"field": "transaction.amount", "op": "gte", "value": 50_000_000},
	})
	require.Equal(t, http.StatusCreated, r.status, r.String())
	n := 0
	huge := func() apigen.RiskDecision {
		n++
		e := txn(fmt.Sprintf("huge-%d", n))
		data, ok := e["data"].(map[string]any)
		require.True(t, ok)
		data["amount"] = 90_000_000
		r := s.ingest(t, a.testKey, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}
	settings := func(mode string) response {
		cur := decode[apigen.RiskSettings](t, s.call(t, http.MethodGet, "/api/v1/risk/settings", a.testKey, nil))
		cur.AutoBlock = &apigen.AutoBlock{Mode: apigen.AutoBlockMode(mode)}
		return s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, cur)
	}
	policy := func() apigen.AutoBlockStatus {
		return decode[apigen.RiskPolicy](t, s.call(t, http.MethodGet, "/api/v1/risk/policy", a.testKey, nil)).AutoBlock
	}

	d := huge()
	assert.Equal(t, apigen.RiskLevel("critical"), d.RiskLevel)
	assert.Equal(t, apigen.Decision("review"), d.Decision, "off by default: Critical gets REVIEW")
	assert.Equal(t, apigen.AutoBlockMode("off"), policy().Mode)

	r = settings("on")
	assert.Equal(t, http.StatusBadRequest, r.status, "never straight to on: %s", r)

	r = settings("shadow")
	require.Equal(t, http.StatusOK, r.status, r.String())
	got := decode[apigen.RiskSettings](t, r).AutoBlock
	require.NotNil(t, got)
	require.NotNil(t, got.ShadowSince)
	assert.Equal(t, apigen.Decision("review"), huge().Decision, "shadow changes no decision")
	p := policy()
	assert.Equal(t, 1, p.WouldBlockLast14Days, "the evidence counts what blocking would have blocked")
	require.NotNil(t, p.EligibleAt)
	assert.WithinDuration(t, got.ShadowSince.Add(14*24*time.Hour), *p.EligibleAt, time.Second)
	assert.Equal(t, http.StatusBadRequest, settings("on").status, "not before 14 days of shadow")
	echo := decode[apigen.RiskSettings](t, s.call(t, http.MethodGet, "/api/v1/risk/settings", a.testKey, nil))
	require.NotNil(t, echo.AutoBlock.ShadowSince)
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, echo).status,
		"settings read back can be saved as they are")

	// Fourteen days later (the shadow period started earlier).
	_, err := shared.owner.Exec(t.Context(),
		`UPDATE risk_settings SET auto_block_shadow_since = now() - interval '15 days' WHERE tenant_id = $1`, a.id)
	require.NoError(t, err)
	r = settings("on")
	require.Equal(t, http.StatusOK, r.status, r.String())
	d = huge()
	assert.Equal(t, apigen.Decision("block"), d.Decision)
	assert.NotEmpty(t, d.Signals, "a block is explained by named signals")
	bands := decode[apigen.RiskPolicy](t, s.call(t, http.MethodGet, "/api/v1/risk/policy", a.testKey, nil)).Bands
	assert.Equal(t, apigen.Decision("block"), bands[len(bands)-1].DefaultDecision)

	// The kill switch takes effect at once.
	require.Equal(t, http.StatusOK, settings("off").status)
	assert.Equal(t, apigen.Decision("review"), huge().Decision)
	assert.Nil(t, policy().ShadowSince, "switching off discards the evidence")
	assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)), "risk.auto_block_changed")
}
