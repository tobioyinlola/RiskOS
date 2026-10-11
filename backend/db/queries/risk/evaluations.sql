-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: InsertEvaluation :exec
INSERT INTO evaluations (
  tenant_id, environment_id, id, created_at, event_id, event_key, dry_run,
  engine_version, rule_versions, features, risk_score, risk_level, decision,
  signals, degraded_signals, request_id, duration_us, policy_reasons, shadow_signals,
  warming_signals, controls, not_evaluated, restricted_signals
) VALUES (
  @tenant_id, @environment_id, @id, @created_at, @event_id, @event_key, @dry_run,
  @engine_version, @rule_versions, @features, @risk_score, @risk_level, @decision,
  @signals, @degraded_signals, @request_id, @duration_us, @policy_reasons, @shadow_signals,
  @warming_signals, @controls, @not_evaluated, @restricted_signals
);

-- name: GetEvaluation :one
-- created_at is derived from the id, so this reads exactly one partition.
SELECT id, created_at, event_id, engine_version, risk_score, risk_level, decision, signals, degraded_signals, request_id,
       controls, not_evaluated,
       -- The jurisdiction that judged the event is kept with the features (ADR-0007).
       COALESCE(features->'jurisdiction', 'null'::jsonb)::jsonb AS jurisdiction
FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at = @created_at AND id = @id;

-- name: TopSignals :many
-- Analytics: how often each signal fired in events since @since. Hours [@covered_from,
-- @rolled_through) come from the hourly rollup (signals_hourly); the rest is read from
-- evaluations. With no rollup, pass both as @since and everything is read raw.
WITH parts AS (
  SELECT s.code, s.label, s.hits FROM signals_hourly s
  WHERE s.tenant_id = @tenant_id AND s.environment_id = @environment_id
    AND s.bucket >= GREATEST(@since::timestamptz, @covered_from::timestamptz) AND s.bucket < @rolled_through::timestamptz
  UNION ALL
  SELECT (sig->>'code')::text, coalesce(max(sig->>'label'), '')::text, count(*)::bigint
  FROM evaluations ev, jsonb_array_elements(ev.signals) AS sig
  WHERE ev.tenant_id = @tenant_id AND ev.environment_id = @environment_id AND NOT ev.dry_run
    AND ((ev.created_at >= @since AND ev.created_at < @covered_from)
      OR ev.created_at >= GREATEST(@since::timestamptz, @rolled_through::timestamptz))
  GROUP BY sig->>'code'
)
SELECT code::text AS code, coalesce(max(label), '')::text AS label, sum(hits)::bigint AS hits
FROM parts
GROUP BY code
ORDER BY sum(hits) DESC
LIMIT 15;

-- name: GetSignalsRollupState :one
SELECT covered_from, rolled_through FROM rollup_state
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND name = 'signals';

-- name: PutSignalsRollupState :exec
INSERT INTO rollup_state (tenant_id, environment_id, name, covered_from, rolled_through, updated_at)
VALUES (@tenant_id, @environment_id, 'signals', @covered_from, @rolled_through, @now)
ON CONFLICT (tenant_id, environment_id, name) DO UPDATE SET
  rolled_through = EXCLUDED.rolled_through, updated_at = EXCLUDED.updated_at;

-- name: RollupSignalHours :execrows
-- Recomputes the hourly signal counts of [@range_start, @range_end) (whole hours) from evaluations. A rollup
-- row is a function of its hour's immutable evaluations, so this is idempotent.
INSERT INTO signals_hourly (tenant_id, environment_id, bucket, code, label, hits)
SELECT @tenant_id, @environment_id, date_bin('1 hour', created_at, '2000-01-01T00:00:00Z'::timestamptz),
       sig->>'code', coalesce(max(sig->>'label'), ''), count(*)
FROM evaluations, jsonb_array_elements(signals) AS sig
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND NOT dry_run
  AND created_at >= @range_start::timestamptz AND created_at < @range_end::timestamptz AND sig->>'code' IS NOT NULL
GROUP BY 3, 4
ON CONFLICT (tenant_id, environment_id, bucket, code) DO UPDATE SET label = EXCLUDED.label, hits = EXCLUDED.hits;

-- name: PackReplays :many
-- Backtests: evaluations of ingested events with a rule pack replay record, oldest first. One
-- batch read per backtest, never per event.
SELECT id, event_id, created_at, features
FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since
  AND NOT dry_run AND features #> '{pack,replay}' IS NOT NULL
ORDER BY created_at
LIMIT @max_rows;

-- name: RestrictedSignals :many
-- Compliance-only signals (GOV-09) of ingested events since @since, newest first.
SELECT id, event_id, created_at, restricted_signals
FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since
  AND NOT dry_run AND restricted_signals <> '[]'::jsonb
ORDER BY created_at DESC
LIMIT 200;

-- name: RuleMatchesByDay :many
-- Matches of the given signal codes (active and monitoring) per UTC day since a time.
SELECT s.code::text AS code,
       date_trunc('day', e.created_at AT TIME ZONE 'UTC')::date AS day,
       count(*)::bigint AS matches,
       max(e.created_at)::timestamptz AS last_at
FROM evaluations e
CROSS JOIN LATERAL (
  SELECT x->>'code' AS code FROM jsonb_array_elements(e.signals) x
  UNION ALL
  SELECT x->>'code' AS code FROM jsonb_array_elements(e.shadow_signals) x
) s
WHERE e.tenant_id = @tenant_id AND e.environment_id = @environment_id
  AND e.created_at >= @since AND NOT e.dry_run
  AND s.code = ANY(@codes::text[])
GROUP BY s.code, date_trunc('day', e.created_at AT TIME ZONE 'UTC')::date;

-- name: CountEvaluationsSince :one
SELECT count(*)::bigint FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since AND NOT dry_run;

-- name: RecentFacts :many
-- The newest evaluations' recorded facts, for trying a rule condition on them.
SELECT id, event_id, created_at, (features->'facts')::jsonb AS facts, decision, risk_score
FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since AND NOT dry_run
ORDER BY created_at DESC
LIMIT @max_rows;
