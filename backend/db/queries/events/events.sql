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
  count(*) FILTER (WHERE decision = 'allow')::bigint AS allowed,
  count(*) FILTER (WHERE decision = 'step_up')::bigint AS stepped_up,
  count(*) FILTER (WHERE decision = 'review')::bigint AS reviewed,
  count(*) FILTER (WHERE decision = 'block')::bigint AS blocked,
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
  count(*) FILTER (WHERE decision = 'allow')::bigint AS allowed,
  count(*) FILTER (WHERE decision = 'step_up')::bigint AS stepped_up,
  count(*) FILTER (WHERE decision = 'review')::bigint AS reviewed,
  count(*) FILTER (WHERE decision = 'block')::bigint AS blocked,
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
  count(*) FILTER (WHERE decision = 'allow')::bigint AS allowed,
  count(*) FILTER (WHERE decision = 'step_up')::bigint AS stepped_up,
  count(*) FILTER (WHERE decision = 'review')::bigint AS reviewed,
  count(*) FILTER (WHERE decision = 'block')::bigint AS blocked,
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
-- Analytics: events per time bucket since @since (non-dry-run evaluations are events).
SELECT date_bin(@step::interval, received_at, @origin::timestamptz)::timestamptz AS bucket,
       count(*)::bigint AS events,
       coalesce(round(avg(risk_score)), 0)::int AS mean_score,
       count(*) FILTER (WHERE decision = 'allow')::bigint AS allowed,
       count(*) FILTER (WHERE decision = 'step_up')::bigint AS stepped_up,
       count(*) FILTER (WHERE decision = 'review')::bigint AS reviewed,
       count(*) FILTER (WHERE decision = 'block')::bigint AS blocked,
       count(*) FILTER (WHERE risk_level = 'minimal')::bigint AS minimal,
       count(*) FILTER (WHERE risk_level = 'low')::bigint AS low,
       count(*) FILTER (WHERE risk_level = 'elevated')::bigint AS elevated,
       count(*) FILTER (WHERE risk_level = 'high')::bigint AS high,
       count(*) FILTER (WHERE risk_level = 'critical')::bigint AS critical
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND received_at >= @since
GROUP BY bucket
ORDER BY bucket;

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
