//go:build integration

package integration

import (
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

func hasSignal(d apigen.RiskDecision, code string) bool {
	return slices.ContainsFunc(d.Signals, func(s apigen.Signal) bool { return s.Code == code })
}

// Money flows (PON-01) and travel speed (BEH-02) come from behaviour history; pack versions are
// adopted by saving the compliance profile.
func TestPackFlowsTravelAndVersions(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Flows")
	admin := s.onboard(t, a, "admin@flows.example", authz.RoleCustomerAdmin)
	r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
		map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
	require.Equal(t, http.StatusOK, r.status, r.String())
	for rule, settings := range map[string]map[string]any{"PON-01": {"senders": 5}, "BEH-02": {}} {
		r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/rules/"+rule+"/config",
			map[string]any{"mode": "live", "settings": settings, "reason": "Live for the test"})
		require.Equal(t, http.StatusOK, r.status, r.String())
	}

	t.Run("PON-01: many unrelated senders, payouts back to earlier ones", func(t *testing.T) {
		pay := func(id, direction, counterparty string, naira int64) apigen.RiskDecision {
			t.Helper()
			kind := "transfer_outbound"
			if direction == "credit" {
				kind = "transfer_inbound"
			}
			e := money(id, kind, "acc_pool", naira*100, "NGN", nil)
			data, _ := e["data"].(map[string]any) // built by txn
			data["direction"] = direction
			if direction == "credit" {
				delete(data, "beneficiary_id")
				c, _ := e["compliance"].(map[string]any) // built by money
				c["refs"] = map[string]any{"sender": counterparty}
			} else {
				data["beneficiary_id"] = counterparty
			}
			return decide(t, s, a.testKey, e)
		}
		for i := 1; i <= 5; i++ {
			pay(fmt.Sprintf("in-%d", i), "credit", fmt.Sprintf("snd_%d", i), 10_000)
		}
		pay("out-1", "debit", "snd_1", 300)   // back to someone who paid in
		pay("out-2", "debit", "ben_new", 700) // 30% of payouts went back
		d := pay("in-6", "credit", "snd_6", 10_000)
		assert.Empty(t, notEvaluated(d, "PON-01"))
		assert.True(t, hasSignal(d, "PON-01"), "six unrelated senders, 30%% back to earlier senders: %+v", d.Signals)
	})

	t.Run("BEH-02: two places too far apart for the time between them", func(t *testing.T) {
		login := func(id string, ago time.Duration, lat, lon float64) apigen.RiskDecision {
			t.Helper()
			return decide(t, s, a.testKey, map[string]any{
				"event_id": id, "type": "login", "occurred_at": time.Now().Add(-ago).Format(time.RFC3339Nano),
				"actor":   map[string]any{"user_id": "u_travel", "device_id": "dev_travel"},
				"context": map[string]any{"country": "NG", "geo": map[string]any{"lat": lat, "lon": lon}},
				"data":    map[string]any{"outcome": "success", "method": "password"},
			})
		}
		first := login("geo-1", 61*time.Minute, 6.52, 3.38) // Lagos
		assert.False(t, hasSignal(first, "BEH-02"))
		second := login("geo-2", time.Minute, 51.51, -0.13) // London, an hour later
		assert.True(t, hasSignal(second, "BEH-02"), "about 5,000 km/h: %+v", second.Signals)
		third := login("geo-3", 0, 51.5, -0.12) // still London
		assert.False(t, hasSignal(third, "BEH-02"))
	})

	t.Run("saving the profile adopts the latest pack version", func(t *testing.T) {
		older := fstest.MapFS{}
		require.NoError(t, fs.WalkDir(ng.Files, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			raw, err := fs.ReadFile(ng.Files, path)
			older[path] = &fstest.MapFile{Data: raw}
			return err
		}))
		meta := string(older["pack.json"].Data)
		require.Contains(t, meta, `"version": "2026.10.1"`)
		older["pack.json"].Data = []byte(strings.Replace(meta, `"version": "2026.10.1"`, `"version": "2026.9.0"`, 1))
		// Loaded after the current version: "latest" compares versions, not load times.
		require.NoError(t, rules.LoadPacks(t.Context(), shared.owner, []fs.FS{older}, shared.logger))
		_, err := shared.owner.Exec(t.Context(), "UPDATE tenant_compliance SET pack_version = '2026.9.0' WHERE tenant_id = $1", a.id)
		require.NoError(t, err)

		p := decode[apigen.CompliancePack](t, s.call(t, http.MethodGet, "/api/v1/compliance/pack", a.testKey, nil))
		assert.Equal(t, "2026.9.0", p.Version)
		assert.Equal(t, "2026.10.1", p.LatestVersion)
		r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
			map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{}})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, "2026.10.1", decode[apigen.ComplianceProfile](t, r).PackVersion)
		cfg := decode[apigen.PackRuleConfig](t, s.call(t, http.MethodGet, "/api/v1/compliance/rules/PON-01/config", a.testKey, nil))
		assert.Equal(t, apigen.PackRuleMode("live"), cfg.Mode, "configuration carries over")
	})
}
