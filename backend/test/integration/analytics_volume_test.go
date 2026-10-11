//go:build integration

package integration

import (
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/events"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

// TestAnalyticsAtVolume measures the console's analytics page (events per bucket, the highest-risk
// users, the most frequent signals) against a large event table. It is a measurement, not a
// regression test, so it runs only when RISKOS_ANALYTICS_ROWS names how many events to seed:
//
//	RISKOS_ANALYTICS_ROWS=2000000 go test -tags integration ./test/integration -run TestAnalyticsAtVolume -v
//
// Events and evaluations are spread evenly over the last 30 days for one tenant, with 50,000 users
// and two signals per evaluation. The method and results are in docs/PERFORMANCE_BASELINE.md.
func TestAnalyticsAtVolume(t *testing.T) {
	raw := os.Getenv("RISKOS_ANALYTICS_ROWS")
	if raw == "" {
		t.Skip("set RISKOS_ANALYTICS_ROWS to measure analytics against that many events")
	}
	rows, err := strconv.Atoi(raw)
	require.NoError(t, err)
	require.Positive(t, rows)

	s := newStackWith(t, time.Second, generousLimits(), shared.rdb)
	tenant := s.createTenant(t, "Analytics Volume Co")
	admin := s.onboard(t, tenant, "admin@volume.example", authz.RoleCustomerAdmin)
	var envID string
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT id::text FROM environments WHERE tenant_id = $1 AND kind = 'test'`, tenant.id).Scan(&envID))

	// The window reaches into last month: make sure its partitions exist.
	_, err = shared.owner.Exec(t.Context(), `
DO $$
DECLARE
  m date := (date_trunc('month', now() - interval '30 days'))::date;
  parent text;
BEGIN
  FOREACH parent IN ARRAY ARRAY['events', 'evaluations'] LOOP
    IF to_regclass(format('public.%s_p%s', parent, to_char(m, 'YYYYMM'))) IS NULL THEN
      EXECUTE format('CREATE TABLE public.%I PARTITION OF public.%I FOR VALUES FROM (%L) TO (%L)',
                     format('%s_p%s', parent, to_char(m, 'YYYYMM')), parent, m, (m + interval '1 month')::date);
    END IF;
  END LOOP;
END $$`)
	require.NoError(t, err)

	const batch = 250_000
	started := time.Now()
	for done := 0; done < rows; done += batch {
		n := min(batch, rows-done)
		_, err = shared.owner.Exec(t.Context(), `
WITH s AS (
  SELECT g AS i, gen_random_uuid() AS id, gen_random_uuid() AS eval_id,
         now() - (($4::bigint - ($3::bigint + g)) * interval '30 days' / $4::bigint) AS ts,
         (random() * 100)::int AS score
  FROM generate_series(1, $5::int) g
), e AS (
  INSERT INTO events (tenant_id, environment_id, id, received_at, event_key, type, occurred_at, user_ref,
                      raw_payload, normalized, payload_hash, evaluation_id, risk_score, risk_level, decision, request_id)
  SELECT $1::uuid, $2::uuid, id, ts, 'k-' || id, 'transaction', ts, 'usr_' || (($3::bigint + i) % 50000),
         '{}', '{}'::jsonb, decode(repeat('00', 32), 'hex'), eval_id, score,
         CASE WHEN score >= 80 THEN 'high' WHEN score >= 40 THEN 'elevated' ELSE 'low' END,
         CASE WHEN score >= 90 THEN 'block' WHEN score >= 70 THEN 'review' ELSE 'allow' END, 'req-' || id
  FROM s
)
INSERT INTO evaluations (tenant_id, environment_id, id, created_at, event_id, event_key, dry_run, engine_version,
                         rule_versions, features, risk_score, risk_level, decision, signals, degraded_signals,
                         request_id, duration_us)
SELECT $1::uuid, $2::uuid, eval_id, ts, id, 'k-' || id, false, 'measure', ARRAY[]::text[], '{}'::jsonb, score,
       CASE WHEN score >= 80 THEN 'high' WHEN score >= 40 THEN 'elevated' ELSE 'low' END,
       CASE WHEN score >= 90 THEN 'block' WHEN score >= 70 THEN 'review' ELSE 'allow' END,
       jsonb_build_array(
         jsonb_build_object('code', 'sig_' || (i % 12), 'label', 'Signal ' || (i % 12)),
         jsonb_build_object('code', 'sig_' || ((i + 5) % 12), 'label', 'Signal ' || ((i + 5) % 12))),
       ARRAY[]::text[], 'req-' || id, 1500
FROM s`, tenant.id, envID, done, rows, n)
		require.NoError(t, err)
	}
	for _, table := range []string{"events", "evaluations"} {
		_, err = shared.owner.Exec(t.Context(), "ANALYZE "+table)
		require.NoError(t, err)
	}
	t.Logf("seeded %d events and evaluations in %s", rows, time.Since(started).Round(time.Second))

	measure := func(label string) {
		for _, window := range []string{"24h", "7d", "30d"} {
			var took []time.Duration
			for range 5 {
				start := time.Now()
				r := admin.b.do(t, http.MethodGet, "/api/v1/analytics?window="+window, nil)
				took = append(took, time.Since(start))
				if r.status != http.StatusOK {
					// A page that does not answer in time is a result, not a test failure.
					t.Logf("%s, %s: status %d after %s: %.200s", label, window, r.status, time.Since(start).Round(time.Millisecond), r.String())
				}
			}
			sort.Slice(took, func(a, b int) bool { return took[a] < took[b] })
			t.Logf("%s, %s over %d events (30 days): min %s, median %s, max %s", label, window, rows,
				took[0].Round(time.Millisecond), took[len(took)/2].Round(time.Millisecond), took[len(took)-1].Round(time.Millisecond))
		}
	}
	measure("before the rollup")

	rolled := time.Now()
	job := events.New(db.NewTxRunner(shared.pool), nil, nil, nil, clock.System{}, nil).
		RollupJob(shared.pool, s.tenants, risk.NewSignalRollup(db.NewTxRunner(shared.pool), clock.System{}),
			slog.New(slog.DiscardHandler))
	require.NoError(t, job(t.Context(), events.RollupArgs{}))
	t.Logf("the first rollup run (a month of back-fill) took %s", time.Since(rolled).Round(time.Millisecond))
	measure("after the rollup")
	rolled = time.Now()
	require.NoError(t, job(t.Context(), events.RollupArgs{}))
	t.Logf("a later run (the last few hours) took %s", time.Since(rolled).Round(time.Millisecond))

	// Where the time goes: the page's three statements over the 30-day window, as the database
	// plans them (the tenant is named explicitly, as the queries do).
	since := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(24 * time.Hour)
	for _, stmt := range []struct{ name, sql string }{
		{"timeline", `SELECT date_bin('1 day', received_at, $3::timestamptz), count(*), avg(risk_score),
  count(*) FILTER (WHERE decision = 'allow'), count(*) FILTER (WHERE decision = 'review'),
  count(*) FILTER (WHERE risk_level = 'high'), count(*) FILTER (WHERE risk_level = 'low')
FROM events WHERE tenant_id = $1 AND environment_id = $2 AND received_at >= $3 GROUP BY 1`},
		{"top users", `SELECT user_ref, max(risk_score), count(*), count(*) FILTER (WHERE decision IN ('review', 'block')), max(received_at)
FROM events WHERE tenant_id = $1 AND environment_id = $2 AND received_at >= $3
GROUP BY user_ref ORDER BY max(risk_score) DESC, max(received_at) DESC LIMIT 10`},
		{"top signals", `SELECT sig->>'code', max(sig->>'label'), count(*)
FROM evaluations, jsonb_array_elements(signals) AS sig
WHERE tenant_id = $1 AND environment_id = $2 AND created_at >= $3 AND NOT dry_run
GROUP BY sig->>'code' ORDER BY count(*) DESC LIMIT 15`},
	} {
		rs, err := shared.owner.Query(t.Context(), "EXPLAIN (ANALYZE, TIMING OFF, SUMMARY ON) "+stmt.sql, tenant.id, envID, since)
		require.NoError(t, err)
		for rs.Next() {
			var line string
			require.NoError(t, rs.Scan(&line))
			if strings.HasPrefix(line, "Execution Time") {
				t.Logf("%s over 30 days: %s", stmt.name, line)
			}
		}
		require.NoError(t, rs.Err())
	}
}
