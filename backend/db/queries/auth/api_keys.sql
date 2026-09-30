-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1) in addition to
-- row-level security (layer 2).

-- name: InsertAPIKey :exec
INSERT INTO api_keys (id, tenant_id, environment_id, name, lookup, secret_hash, scopes, created_by, expires_at)
VALUES (@id, @tenant_id, @environment_id, @name, @lookup, @secret_hash, @scopes, @created_by, @expires_at);

-- name: GetAPIKey :one
SELECT id, name, lookup, scopes, created_at, expires_at, revoked_at, replaced_by
FROM api_keys
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: ListAPIKeys :many
SELECT id, name, lookup, scopes, created_at, expires_at, revoked_at, replaced_by
FROM api_keys
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET revoked_at = COALESCE(revoked_at, @revoked_at)
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
RETURNING id, name, lookup, scopes, created_at, expires_at, revoked_at, replaced_by;

-- name: MarkAPIKeyReplaced :one
-- Rotation: the old key keeps working until the overlap ends (never extended), then expires.
UPDATE api_keys
SET replaced_by = @replaced_by,
    expires_at = LEAST(COALESCE(expires_at, @expires_at), @expires_at)
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
  AND revoked_at IS NULL AND replaced_by IS NULL
RETURNING id, name, lookup, scopes, created_at, expires_at, revoked_at, replaced_by;
