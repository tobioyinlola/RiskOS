-- Jurisdiction rule packs (ADR-0006). Pack versions are global and read-only to the application;
-- only the migrate command (schema owner) inserts them.

-- name: PackChecksum :one
SELECT checksum FROM rule_packs WHERE id = @id AND version = @version;

-- name: InsertPack :exec
INSERT INTO rule_packs (id, version, jurisdiction, name, checksum, meta)
VALUES (@id, @version, @jurisdiction, @name, @checksum, @meta);

-- name: InsertPackRule :exec
INSERT INTO pack_rules (pack_id, pack_version, rule_id, kind, definition, wording)
VALUES (@pack_id, @pack_version, @rule_id, @kind, @definition, @wording);

-- name: GetPack :one
SELECT id, version, jurisdiction, name, checksum, meta, loaded_at
FROM rule_packs WHERE id = @id AND version = @version;

-- name: LatestPack :one
SELECT id, version, jurisdiction, name, checksum, meta, loaded_at
FROM rule_packs
WHERE (sqlc.narg(jurisdiction)::text IS NULL OR jurisdiction = sqlc.narg(jurisdiction)::text)
ORDER BY loaded_at DESC, version DESC
LIMIT 1;

-- name: ListPackRules :many
SELECT rule_id, definition, wording
FROM pack_rules WHERE pack_id = @pack_id AND pack_version = @pack_version
ORDER BY rule_id;

-- Compliance profiles. Row-level security limits them to the scope's tenant; every query also
-- names the tenant explicitly.

-- name: GetCompliance :one
SELECT tenant_id, pack_id, pack_version, licence_type, capabilities, updated_at, updated_by
FROM tenant_compliance WHERE tenant_id = @tenant_id;

-- name: UpsertCompliance :one
INSERT INTO tenant_compliance (tenant_id, pack_id, pack_version, licence_type, capabilities, updated_at, updated_by)
VALUES (@tenant_id, @pack_id, @pack_version, @licence_type, @capabilities, @updated_at, @updated_by)
ON CONFLICT (tenant_id) DO UPDATE SET
  pack_id = EXCLUDED.pack_id, pack_version = EXCLUDED.pack_version,
  licence_type = EXCLUDED.licence_type, capabilities = EXCLUDED.capabilities,
  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by
RETURNING tenant_id, pack_id, pack_version, licence_type, capabilities, updated_at, updated_by;
