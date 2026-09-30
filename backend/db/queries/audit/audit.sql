-- name: EnsureChainHead :exec
INSERT INTO audit_chain_heads (tenant_id, seq, hash)
VALUES (@tenant_id, 0, '\x0000000000000000000000000000000000000000000000000000000000000000'::bytea)
ON CONFLICT (tenant_id) DO NOTHING;

-- name: LockChainHead :one
SELECT seq, hash FROM audit_chain_heads WHERE tenant_id = @tenant_id FOR UPDATE;

-- name: AdvanceChainHead :exec
UPDATE audit_chain_heads SET seq = @seq, hash = @hash WHERE tenant_id = @tenant_id;

-- name: InsertAuditEntry :exec
INSERT INTO audit_log (
  tenant_id, seq, id, environment_id, occurred_at, actor_type, actor_id, action,
  resource_type, resource_id, outcome, request_id, metadata, prev_hash, hash
) VALUES (
  @tenant_id, @seq, @id, @environment_id, @occurred_at, @actor_type, @actor_id, @action,
  @resource_type, @resource_id, @outcome, @request_id, @metadata, @prev_hash, @hash
);

-- name: ListAuditEntries :many
SELECT tenant_id, seq, id, environment_id, occurred_at, actor_type, actor_id, action,
       resource_type, resource_id, outcome, request_id, metadata, prev_hash, hash
FROM audit_log
WHERE tenant_id = @tenant_id
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- name: ListChainSegment :many
-- Verification reads the whole chain in seq order (operator connection; see audit.Verify).
SELECT tenant_id, seq, id, environment_id, occurred_at, actor_type, actor_id, action,
       resource_type, resource_id, outcome, request_id, metadata, prev_hash, hash
FROM audit_log
WHERE tenant_id = @tenant_id AND seq > @after_seq
ORDER BY seq
LIMIT @row_limit;

-- name: GetChainHead :one
SELECT seq, hash FROM audit_chain_heads WHERE tenant_id = @tenant_id;
