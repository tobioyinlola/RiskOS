-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: InsertEvaluation :exec
INSERT INTO evaluations (
  tenant_id, environment_id, id, created_at, event_id, event_key, dry_run,
  engine_version, rule_versions, features, risk_score, risk_level, decision,
  signals, degraded_signals, request_id, duration_us, policy_reasons, shadow_signals,
  warming_signals, controls, not_evaluated
) VALUES (
  @tenant_id, @environment_id, @id, @created_at, @event_id, @event_key, @dry_run,
  @engine_version, @rule_versions, @features, @risk_score, @risk_level, @decision,
  @signals, @degraded_signals, @request_id, @duration_us, @policy_reasons, @shadow_signals,
  @warming_signals, @controls, @not_evaluated
);

-- name: GetEvaluation :one
-- created_at is derived from the id, so this reads exactly one partition.
SELECT id, created_at, event_id, engine_version, risk_score, risk_level, decision, signals, degraded_signals, request_id,
       controls, not_evaluated
FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at = @created_at AND id = @id;

-- name: TopSignals :many
-- Analytics: how often each signal fired in events since @since.
SELECT (sig->>'code')::text AS code, coalesce(max(sig->>'label'), '')::text AS label, count(*)::bigint AS hits
FROM evaluations, jsonb_array_elements(signals) AS sig
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since
  AND NOT dry_run
GROUP BY sig->>'code'
ORDER BY count(*) DESC
LIMIT 15;

-- name: PackReplays :many
-- Backtests: evaluations of ingested events with a rule pack replay record, oldest first. One
-- batch read per backtest, never per event.
SELECT id, event_id, created_at, features
FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since
  AND NOT dry_run AND features #> '{pack,replay}' IS NOT NULL
ORDER BY created_at
LIMIT @max_rows;
