//go:build integration

package integration

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/events"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

// TestAnalyticsRollups proves the analytics page answers the same from the hourly rollups as from
// the raw events: before the worker has rolled anything up, after it has, with a rollup that is
// behind, and with an event that committed late into an hour already rolled up. The highest-risk
// users are also checked against the aggregate they stand in for, with users tied at the cutoff.
func TestAnalyticsRollups(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb)
	tenant := s.createTenant(t, "Rollup Co")
	admin := s.onboard(t, tenant, "admin@rollup.example", authz.RoleCustomerAdmin)
	var envID string
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT id::text FROM environments WHERE tenant_id = $1 AND kind = 'test'`, tenant.id).Scan(&envID))

	// Events over the last ten days (and some in the current and the previous hour), 40 users,
	// every score, every decision, and two of six signals each.
	seed := func(where string, n int) {
		t.Helper()
		_, err := shared.owner.Exec(t.Context(), `
WITH s AS (
  SELECT g AS i, gen_random_uuid() AS id, gen_random_uuid() AS eval_id,
         `+where+` AS ts, (random() * 100)::int AS score, floor(random() * 10)::int AS d
  FROM generate_series(1, $3::int) g
), e AS (
  INSERT INTO events (tenant_id, environment_id, id, received_at, event_key, type, occurred_at, user_ref,
                      raw_payload, normalized, payload_hash, evaluation_id, risk_score, risk_level, decision, request_id)
  SELECT $1::uuid, $2::uuid, id, ts, 'k-' || id, 'transaction', ts, 'usr_' || (i % 40),
         '{}', '{}'::jsonb, decode(repeat('00', 32), 'hex'), eval_id, score,
         CASE WHEN score >= 90 THEN 'critical' WHEN score >= 70 THEN 'high' WHEN score >= 40 THEN 'elevated'
              WHEN score >= 20 THEN 'low' ELSE 'minimal' END,
         (ARRAY['allow','fee','flag','step_up','review','restrict','hold','block_session','decline','block'])[d + 1],
         'req-' || id
  FROM s
)
INSERT INTO evaluations (tenant_id, environment_id, id, created_at, event_id, event_key, dry_run, engine_version,
                         rule_versions, features, risk_score, risk_level, decision, signals, degraded_signals,
                         request_id, duration_us)
SELECT $1::uuid, $2::uuid, eval_id, ts, id, 'k-' || id, false, 'test', ARRAY[]::text[], '{}'::jsonb, score,
       'low', 'allow',
       jsonb_build_array(
         jsonb_build_object('code', 'sig_' || (i % 6), 'label', 'Signal ' || (i % 6)),
         jsonb_build_object('code', 'sig_' || ((i + 2) % 6), 'label', 'Signal ' || ((i + 2) % 6))),
       ARRAY[]::text[], 'req-' || id, 1500
FROM s`, tenant.id, envID, n)
		require.NoError(t, err)
	}
	seed(`now() - (random() * interval '10 days')`, 3000)
	seed(`now() - (random() * interval '50 minutes')`, 60)
	seed(`date_trunc('hour', now()) - (random() * interval '55 minutes')`, 60)
	// Fifteen users tie at the top score, more than the ten the page shows.
	_, err := shared.owner.Exec(t.Context(), `
INSERT INTO events (tenant_id, environment_id, id, received_at, event_key, type, occurred_at, user_ref,
                    raw_payload, normalized, payload_hash, evaluation_id, risk_score, risk_level, decision, request_id)
SELECT $1::uuid, $2::uuid, gen_random_uuid(), now() - (g * interval '7 minutes'), 'tie-' || g, 'transaction',
       now(), 'tied_' || g, '{}', '{}'::jsonb, decode(repeat('00', 32), 'hex'), gen_random_uuid(), 100,
       'critical', 'block', 'req-tie'
FROM generate_series(1, 15) g`, tenant.id, envID)
	require.NoError(t, err)

	page := func(window string) map[string]any {
		t.Helper()
		r := admin.b.do(t, http.MethodGet, "/api/v1/analytics?window="+window, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		var m map[string]any
		require.NoError(t, json.Unmarshal(r.raw, &m))
		delete(m, "since") // moves with the clock; the buckets and counts are what is compared
		return m
	}
	all := func() map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, w := range []string{"24h", "7d", "30d"} {
			out[w] = page(w)
		}
		return out
	}
	compare := func(t *testing.T, want, got map[string]map[string]any, label string) {
		t.Helper()
		for w := range want {
			assert.Equal(t, want[w], got[w], "%s: the %s window", label, w)
		}
	}
	roll := func() {
		t.Helper()
		job := events.New(db.NewTxRunner(shared.pool), nil, nil, nil, clock.System{}, nil).
			RollupJob(shared.pool, s.tenants, risk.NewSignalRollup(db.NewTxRunner(shared.pool), clock.System{}),
				slog.New(slog.DiscardHandler))
		require.NoError(t, job(t.Context(), events.RollupArgs{}))
	}
	hasRollup := func() bool {
		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM rollup_state WHERE tenant_id = $1`, tenant.id).Scan(&n))
		return n >= 2 // an analytics and a signals row for each environment the job visited
	}

	raw := all()
	require.False(t, hasRollup(), "nothing is rolled up yet: the page reads events")
	require.NotEmpty(t, raw["7d"]["series"])

	t.Run("the highest-risk users are the aggregate's", func(t *testing.T) {
		rows, err := shared.owner.Query(t.Context(), `
SELECT user_ref, max(risk_score)::int, count(*)::int,
       (count(*) FILTER (WHERE decision IN ('review', 'block')))::int
FROM events WHERE tenant_id = $1 AND environment_id = $2 AND received_at >= $3
GROUP BY user_ref ORDER BY max(risk_score) DESC, max(received_at) DESC LIMIT 10`,
			tenant.id, envID, time.Now().UTC().Add(-30*24*time.Hour).Truncate(24*time.Hour))
		require.NoError(t, err)
		defer rows.Close()
		type user struct {
			User                      string
			MaxScore, Events, Flagged int
		}
		var want []user
		for rows.Next() {
			var u user
			require.NoError(t, rows.Scan(&u.User, &u.MaxScore, &u.Events, &u.Flagged))
			want = append(want, u)
		}
		require.NoError(t, rows.Err())
		require.Len(t, want, 10)

		var got []user
		for _, raw := range raw["30d"]["top_users"].([]any) {
			m := raw.(map[string]any)
			got = append(got, user{
				User: m["user_id"].(string), MaxScore: int(m["max_score"].(float64)),
				Events: int(m["events"].(float64)), Flagged: int(m["flagged"].(float64)),
			})
		}
		assert.Equal(t, want, got)
	})

	t.Run("rolled up, the page answers the same", func(t *testing.T) {
		roll()
		require.True(t, hasRollup())
		compare(t, raw, all(), "after the first run")

		roll() // idempotent
		compare(t, raw, all(), "after a second run")
		var hours, signals int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM analytics_hourly WHERE tenant_id = $1`, tenant.id).Scan(&hours))
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM signals_hourly WHERE tenant_id = $1`, tenant.id).Scan(&signals))
		assert.Positive(t, hours)
		assert.Positive(t, signals)
	})

	t.Run("a rollup that is behind is slower, not wrong", func(t *testing.T) {
		_, err := shared.owner.Exec(t.Context(),
			`UPDATE rollup_state SET rolled_through = rolled_through - interval '30 hours' WHERE tenant_id = $1`, tenant.id)
		require.NoError(t, err)
		compare(t, raw, all(), "30 hours behind")
		roll()
		compare(t, raw, all(), "caught up again")
	})

	t.Run("an event that commits late into a finished hour is picked up", func(t *testing.T) {
		// Two hours ago: inside the hours a run recomputes.
		seed(`date_trunc('hour', now()) - interval '2 hours' + (random() * interval '50 minutes')`, 25)
		stale := all() // the rollup still has the old hour, but the page reads the hours it finished
		roll()
		fresh := all()

		// The truth: the same page with the rollup switched off.
		_, err := shared.owner.Exec(t.Context(), `DELETE FROM rollup_state WHERE tenant_id = $1`, tenant.id)
		require.NoError(t, err)
		truth := all()
		compare(t, truth, fresh, "after the next run")
		assert.NotEqual(t, truth["24h"]["series"], stale["24h"]["series"], "before the run the rolled-up hour was stale")
	})
}
