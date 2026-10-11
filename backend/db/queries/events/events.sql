-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: ClaimEventKey :execrows
-- Claims an idempotency key. 0 rows means the key exists; a concurrent claim blocks here until
-- the other transaction commits or rolls back.
INSERT INTO event_idempotency (tenant_id, environment_id, event_key, payload_hash, event_id)
VALUES (@tenant_id, @environment_id, @event_key, @payload_hash, @event_id)
ON CONFLICT (tenant_id, environment_id, event_key) DO NOTHING;

-- name: GetEventKey :one
SELECT payload_hash, event_id, response
FROM event_idempotency
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND event_key = @event_key;

-- name: SetEventKeyResponse :exec
UPDATE event_idempotency SET response = @response
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND event_key = @event_key;

-- name: InsertEvent :exec
INSERT INTO events (
  tenant_id, environment_id, id, received_at, event_key, type, occurred_at,
  user_ref, account_ref, device_ref, session_ref, ip, country,
  raw_payload, normalized, payload_hash, evaluation_id, risk_score, risk_level, decision, request_id
) VALUES (
  @tenant_id, @environment_id, @id, @received_at, @event_key, @type, @occurred_at,
  @user_ref, @account_ref, @device_ref, @session_ref, @ip, @country,
  @raw_payload, @normalized, @payload_hash, @evaluation_id, @risk_score, @risk_level, @decision, @request_id
);

-- name: GetEvent :one
-- received_at is derived from the id, so this reads exactly one partition.
SELECT id, received_at, event_key, type, occurred_at, raw_payload, normalized, evaluation_id
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND received_at = @received_at AND id = @id;

-- name: ListEvents :many
-- Newest first. Every filter is optional; the entity filters use their own indexes.
SELECT id, received_at, event_key, type, occurred_at, risk_score, risk_level, decision,
       user_ref, account_ref, device_ref, country,
       coalesce(CASE WHEN type = 'transaction' THEN (normalized->'data'->>'amount')::bigint END, 0)::bigint AS amount,
       coalesce(CASE WHEN type = 'transaction' THEN normalized->'data'->>'currency' END, '')::text AS currency,
       coalesce(CASE WHEN type = 'transaction' THEN normalized->'data'->>'direction' END, '')::text AS direction,
       coalesce(CASE WHEN type = 'transaction' THEN normalized->'data'->>'beneficiary_id' END, '')::text AS beneficiary,
       coalesce(CASE WHEN type = 'transaction' THEN normalized->'data'->>'channel' END, '')::text AS channel
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
  AND (sqlc.narg('after_id')::uuid IS NULL OR id > sqlc.narg('after_id')::uuid)
  AND (sqlc.narg('event_type')::text IS NULL OR type = sqlc.narg('event_type')::text)
  AND (sqlc.narg('decision')::text IS NULL OR decision = sqlc.narg('decision')::text)
  AND (sqlc.narg('risk_level')::text IS NULL OR risk_level = sqlc.narg('risk_level')::text)
  AND (sqlc.narg('user_ref')::text IS NULL OR user_ref = sqlc.narg('user_ref')::text)
  AND (sqlc.narg('account_ref')::text IS NULL OR account_ref = sqlc.narg('account_ref')::text)
  AND (sqlc.narg('device_ref')::text IS NULL OR device_ref = sqlc.narg('device_ref')::text)
ORDER BY id DESC
LIMIT @row_limit;

-- name: UserProfile :one
-- A user's profile from its most recent 5,000 events (docs/CONSOLE.md, Entities).
WITH ev AS (
  SELECT received_at, risk_score, risk_level, decision, user_ref, account_ref, device_ref, country, ip
  FROM events
  WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND user_ref = @ref
  ORDER BY id DESC
  LIMIT 5000
)
SELECT
  count(*)::bigint AS event_count,
  coalesce(min(received_at), 'epoch')::timestamptz AS first_seen,
  coalesce(max(received_at), 'epoch')::timestamptz AS last_seen,
  coalesce(max(risk_score), 0)::int AS max_score,
  coalesce(round(avg(risk_score)), 0)::int AS mean_score,
  jsonb_build_object(
    'allow', count(*) FILTER (WHERE decision = 'allow'),
    'fee', count(*) FILTER (WHERE decision = 'fee'),
    'flag', count(*) FILTER (WHERE decision = 'flag'),
    'step_up', count(*) FILTER (WHERE decision = 'step_up'),
    'review', count(*) FILTER (WHERE decision = 'review'),
    'restrict', count(*) FILTER (WHERE decision = 'restrict'),
    'hold', count(*) FILTER (WHERE decision = 'hold'),
    'block_session', count(*) FILTER (WHERE decision = 'block_session'),
    'decline', count(*) FILTER (WHERE decision = 'decline'),
    'block', count(*) FILTER (WHERE decision = 'block')
  ) AS decisions,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT user_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    GROUP BY user_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS users,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT account_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE account_ref IS NOT NULL GROUP BY account_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS accounts,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT device_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE device_ref IS NOT NULL GROUP BY device_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS devices,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT country AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE country IS NOT NULL GROUP BY country ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS countries,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT host(ip) AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE ip IS NOT NULL GROUP BY ip ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS ips
FROM ev;

-- name: AccountProfile :one
-- A account's profile from its most recent 5,000 events (docs/CONSOLE.md, Entities).
WITH ev AS (
  SELECT received_at, risk_score, risk_level, decision, user_ref, account_ref, device_ref, country, ip
  FROM events
  WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND account_ref = @ref
  ORDER BY id DESC
  LIMIT 5000
)
SELECT
  count(*)::bigint AS event_count,
  coalesce(min(received_at), 'epoch')::timestamptz AS first_seen,
  coalesce(max(received_at), 'epoch')::timestamptz AS last_seen,
  coalesce(max(risk_score), 0)::int AS max_score,
  coalesce(round(avg(risk_score)), 0)::int AS mean_score,
  jsonb_build_object(
    'allow', count(*) FILTER (WHERE decision = 'allow'),
    'fee', count(*) FILTER (WHERE decision = 'fee'),
    'flag', count(*) FILTER (WHERE decision = 'flag'),
    'step_up', count(*) FILTER (WHERE decision = 'step_up'),
    'review', count(*) FILTER (WHERE decision = 'review'),
    'restrict', count(*) FILTER (WHERE decision = 'restrict'),
    'hold', count(*) FILTER (WHERE decision = 'hold'),
    'block_session', count(*) FILTER (WHERE decision = 'block_session'),
    'decline', count(*) FILTER (WHERE decision = 'decline'),
    'block', count(*) FILTER (WHERE decision = 'block')
  ) AS decisions,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT user_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    GROUP BY user_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS users,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT account_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE account_ref IS NOT NULL GROUP BY account_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS accounts,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT device_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE device_ref IS NOT NULL GROUP BY device_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS devices,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT country AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE country IS NOT NULL GROUP BY country ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS countries,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT host(ip) AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE ip IS NOT NULL GROUP BY ip ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS ips
FROM ev;

-- name: DeviceProfile :one
-- A device's profile from its most recent 5,000 events (docs/CONSOLE.md, Entities).
WITH ev AS (
  SELECT received_at, risk_score, risk_level, decision, user_ref, account_ref, device_ref, country, ip
  FROM events
  WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND device_ref = @ref
  ORDER BY id DESC
  LIMIT 5000
)
SELECT
  count(*)::bigint AS event_count,
  coalesce(min(received_at), 'epoch')::timestamptz AS first_seen,
  coalesce(max(received_at), 'epoch')::timestamptz AS last_seen,
  coalesce(max(risk_score), 0)::int AS max_score,
  coalesce(round(avg(risk_score)), 0)::int AS mean_score,
  jsonb_build_object(
    'allow', count(*) FILTER (WHERE decision = 'allow'),
    'fee', count(*) FILTER (WHERE decision = 'fee'),
    'flag', count(*) FILTER (WHERE decision = 'flag'),
    'step_up', count(*) FILTER (WHERE decision = 'step_up'),
    'review', count(*) FILTER (WHERE decision = 'review'),
    'restrict', count(*) FILTER (WHERE decision = 'restrict'),
    'hold', count(*) FILTER (WHERE decision = 'hold'),
    'block_session', count(*) FILTER (WHERE decision = 'block_session'),
    'decline', count(*) FILTER (WHERE decision = 'decline'),
    'block', count(*) FILTER (WHERE decision = 'block')
  ) AS decisions,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT user_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    GROUP BY user_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS users,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT account_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE account_ref IS NOT NULL GROUP BY account_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS accounts,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT device_ref AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE device_ref IS NOT NULL GROUP BY device_ref ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS devices,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT country AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE country IS NOT NULL GROUP BY country ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS countries,
  coalesce((SELECT jsonb_agg(x) FROM (
    SELECT host(ip) AS ref, count(*) AS events, max(received_at) AS last_seen FROM ev
    WHERE ip IS NOT NULL GROUP BY ip ORDER BY max(received_at) DESC LIMIT 20) x), '[]')::jsonb AS ips
FROM ev;

-- name: EventTimeline :many
-- Analytics: events per time bucket since @since (non-dry-run evaluations are events). Hours
-- [@covered_from, @rolled_through) come from the hourly rollup; the rest is read from events. With
-- no rollup, pass both as @since: the rollup part is then empty and everything is read raw.
-- Buckets are re-binned from hours, which is exact because @step is a whole number of hours and
-- @origin is on the hour.
WITH hours AS (
  SELECT h.bucket, h.events, h.score_sum, h.allow, h.fee, h.flag, h.step_up, h.review, h.restrict, h.hold,
         h.block_session, h.decline, h.block, h.minimal, h.low, h.elevated, h.high, h.critical
  FROM analytics_hourly h
  WHERE h.tenant_id = @tenant_id AND h.environment_id = @environment_id
    AND h.bucket >= GREATEST(@since::timestamptz, @covered_from::timestamptz) AND h.bucket < @rolled_through::timestamptz
  UNION ALL
  SELECT date_bin('1 hour', e.received_at, '2000-01-01T00:00:00Z'::timestamptz), count(*), sum(e.risk_score),
         count(*) FILTER (WHERE e.decision = 'allow'), count(*) FILTER (WHERE e.decision = 'fee'),
         count(*) FILTER (WHERE e.decision = 'flag'), count(*) FILTER (WHERE e.decision = 'step_up'),
         count(*) FILTER (WHERE e.decision = 'review'), count(*) FILTER (WHERE e.decision = 'restrict'),
         count(*) FILTER (WHERE e.decision = 'hold'), count(*) FILTER (WHERE e.decision = 'block_session'),
         count(*) FILTER (WHERE e.decision = 'decline'), count(*) FILTER (WHERE e.decision = 'block'),
         count(*) FILTER (WHERE e.risk_level = 'minimal'), count(*) FILTER (WHERE e.risk_level = 'low'),
         count(*) FILTER (WHERE e.risk_level = 'elevated'), count(*) FILTER (WHERE e.risk_level = 'high'),
         count(*) FILTER (WHERE e.risk_level = 'critical')
  FROM events e
  WHERE e.tenant_id = @tenant_id AND e.environment_id = @environment_id
    AND ((e.received_at >= @since AND e.received_at < @covered_from)
      OR e.received_at >= GREATEST(@since::timestamptz, @rolled_through::timestamptz))
  GROUP BY 1
)
SELECT date_bin(@step::interval, bucket, @origin::timestamptz)::timestamptz AS bucket,
       sum(events)::bigint AS events, sum(score_sum)::bigint AS score_sum,
       sum(allow)::bigint AS allow, sum(fee)::bigint AS fee, sum(flag)::bigint AS flag,
       sum(step_up)::bigint AS step_up, sum(review)::bigint AS review, sum(restrict)::bigint AS restrict,
       sum(hold)::bigint AS hold, sum(block_session)::bigint AS block_session,
       sum(decline)::bigint AS decline, sum(block)::bigint AS block,
       sum(minimal)::bigint AS minimal, sum(low)::bigint AS low, sum(elevated)::bigint AS elevated,
       sum(high)::bigint AS high, sum(critical)::bigint AS critical
FROM hours
GROUP BY 1
ORDER BY 1;

-- name: GetRollupState :one
SELECT covered_from, rolled_through FROM rollup_state
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND name = @name;

-- name: PutRollupState :exec
INSERT INTO rollup_state (tenant_id, environment_id, name, covered_from, rolled_through, updated_at)
VALUES (@tenant_id, @environment_id, @name, @covered_from, @rolled_through, @now)
ON CONFLICT (tenant_id, environment_id, name) DO UPDATE SET
  rolled_through = EXCLUDED.rolled_through, updated_at = EXCLUDED.updated_at;

-- name: RollupEventHours :execrows
-- Recomputes the hourly aggregates of [@range_start, @range_end) (whole hours) from events. A rollup row is a
-- function of its hour's immutable events, so this is idempotent and picks up late commits.
INSERT INTO analytics_hourly (tenant_id, environment_id, bucket, events, score_sum, minimal, low, elevated,
                              high, critical, allow, fee, flag, step_up, review, restrict, hold,
                              block_session, decline, block)
SELECT @tenant_id, @environment_id, date_bin('1 hour', received_at, '2000-01-01T00:00:00Z'::timestamptz),
       count(*), sum(risk_score),
       count(*) FILTER (WHERE risk_level = 'minimal'), count(*) FILTER (WHERE risk_level = 'low'),
       count(*) FILTER (WHERE risk_level = 'elevated'), count(*) FILTER (WHERE risk_level = 'high'),
       count(*) FILTER (WHERE risk_level = 'critical'),
       count(*) FILTER (WHERE decision = 'allow'), count(*) FILTER (WHERE decision = 'fee'),
       count(*) FILTER (WHERE decision = 'flag'), count(*) FILTER (WHERE decision = 'step_up'),
       count(*) FILTER (WHERE decision = 'review'), count(*) FILTER (WHERE decision = 'restrict'),
       count(*) FILTER (WHERE decision = 'hold'), count(*) FILTER (WHERE decision = 'block_session'),
       count(*) FILTER (WHERE decision = 'decline'), count(*) FILTER (WHERE decision = 'block')
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND received_at >= @range_start::timestamptz AND received_at < @range_end::timestamptz
GROUP BY 3
ON CONFLICT (tenant_id, environment_id, bucket) DO UPDATE SET
  events = EXCLUDED.events, score_sum = EXCLUDED.score_sum, minimal = EXCLUDED.minimal, low = EXCLUDED.low,
  elevated = EXCLUDED.elevated, high = EXCLUDED.high, critical = EXCLUDED.critical, allow = EXCLUDED.allow,
  fee = EXCLUDED.fee, flag = EXCLUDED.flag, step_up = EXCLUDED.step_up, review = EXCLUDED.review,
  restrict = EXCLUDED.restrict, hold = EXCLUDED.hold, block_session = EXCLUDED.block_session,
  decline = EXCLUDED.decline, block = EXCLUDED.block;

-- name: TopScoreEvents :many
-- Analytics: the users of the window's highest-scoring events, best first. Read in score order
-- (events_by_score), so it stops after @row_limit rows instead of aggregating the window.
SELECT user_ref, risk_score::int AS risk_score
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND received_at >= @since
ORDER BY risk_score DESC, received_at DESC
LIMIT @row_limit;

-- name: UsersAtScore :many
-- Analytics: every user with an event of at least @min_score since @since.
SELECT DISTINCT user_ref
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND received_at >= @since
  AND risk_score >= @min_score
LIMIT @row_limit;

-- name: UserWindowStats :many
-- Analytics: what the window holds for each of the given users.
SELECT user_ref, max(risk_score)::int AS max_score, count(*)::bigint AS events,
       count(*) FILTER (WHERE decision IN ('review', 'block'))::bigint AS flagged,
       max(received_at)::timestamptz AS last_seen
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND received_at >= @since
  AND user_ref = ANY(@users::text[])
GROUP BY user_ref;

-- name: TopUsers :many
-- Analytics: the users with the highest scores since @since.
SELECT user_ref, max(risk_score)::int AS max_score, count(*)::bigint AS events,
       count(*) FILTER (WHERE decision IN ('review', 'block'))::bigint AS flagged,
       max(received_at)::timestamptz AS last_seen
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND received_at >= @since
GROUP BY user_ref
ORDER BY max(risk_score) DESC, max(received_at) DESC
LIMIT @row_limit;
