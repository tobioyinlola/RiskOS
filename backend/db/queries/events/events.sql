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
SELECT id, received_at, event_key, type, occurred_at, risk_score, risk_level, decision
FROM events
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
ORDER BY id DESC
LIMIT @row_limit;
