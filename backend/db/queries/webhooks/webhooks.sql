-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: InsertEndpoint :exec
INSERT INTO webhook_endpoints (
  tenant_id, environment_id, id, url, description, event_types, status, secret_version,
  secret_key_id, created_at, created_by, updated_at
) VALUES (
  @tenant_id, @environment_id, @id, @url, @description, @event_types, 'active', 1,
  @secret_key_id, @now, @created_by, @now
);

-- name: CountEndpoints :one
SELECT count(*) FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND environment_id = @environment_id;

-- name: GetEndpoint :one
SELECT * FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: GetEndpointForUpdate :one
SELECT * FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: ListEndpoints :many
SELECT * FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
ORDER BY id DESC;

-- name: ListSubscribedEndpoints :many
-- FOR SHARE: an endpoint disabled concurrently is either seen before or after the change.
SELECT id FROM webhook_endpoints
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND status = 'active' AND @event_type::text = ANY (event_types)
ORDER BY id
FOR SHARE;

-- name: UpdateEndpoint :exec
UPDATE webhook_endpoints SET
  url = @url, description = @description, event_types = @event_types, status = @status,
  disabled_reason = sqlc.narg('disabled_reason'), failing_since = sqlc.narg('failing_since'),
  updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: RotateEndpointSecret :exec
UPDATE webhook_endpoints SET
  previous_secret_version = secret_version,
  previous_secret_key_id = secret_key_id,
  previous_secret_valid_until = @previous_valid_until,
  secret_version = secret_version + 1,
  secret_key_id = @secret_key_id,
  updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: SetEndpointHealth :exec
UPDATE webhook_endpoints SET
  failing_since = sqlc.narg('failing_since'), status = @status,
  disabled_reason = sqlc.narg('disabled_reason'), updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: InsertDelivery :exec
INSERT INTO webhook_deliveries (
  tenant_id, environment_id, id, endpoint_id, event_id, event_type, payload, status,
  next_attempt_at, created_at
) VALUES (
  @tenant_id, @environment_id, @id, @endpoint_id, @event_id, @event_type, @payload, 'pending',
  @now, @now
);

-- name: GetDelivery :one
SELECT * FROM webhook_deliveries
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: GetDeliveryForUpdate :one
SELECT * FROM webhook_deliveries
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: ListDeliveries :many
SELECT * FROM webhook_deliveries
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND endpoint_id = @endpoint_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- name: RecordDeliveryAttempt :exec
UPDATE webhook_deliveries SET
  status = @status, attempts = attempts + 1, last_attempt_at = @attempted_at,
  last_status_code = sqlc.narg('last_status_code'), last_error = sqlc.narg('last_error'),
  next_attempt_at = sqlc.narg('next_attempt_at'), completed_at = sqlc.narg('completed_at')
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: FailDelivery :exec
UPDATE webhook_deliveries SET
  status = 'failed', last_error = @last_error, next_attempt_at = NULL, completed_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: RestartDelivery :exec
UPDATE webhook_deliveries SET
  status = 'pending', attempts = 0, next_attempt_at = @now, completed_at = NULL
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: InsertAttempt :exec
INSERT INTO webhook_attempts (tenant_id, environment_id, id, delivery_id, attempt, started_at, duration_ms, status_code, error)
VALUES (@tenant_id, @environment_id, @id, @delivery_id, @attempt, @started_at, @duration_ms, sqlc.narg('status_code'), sqlc.narg('error'));

-- name: ListAttempts :many
SELECT attempt, started_at, duration_ms, status_code, error FROM webhook_attempts
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND delivery_id = @delivery_id
ORDER BY id;
