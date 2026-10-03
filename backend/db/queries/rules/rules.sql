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

-- Rule configuration (per environment). Every query names the tenant and environment; row-level
-- security limits them to the scope's.

-- name: CurrentRuleVersions :many
SELECT DISTINCT ON (rule_id) rule_id, version, mode, settings, reason, created_at, created_by, change_id, restored_from
FROM pack_rule_versions
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND pack_id = @pack_id
ORDER BY rule_id, version DESC;

-- name: RuleVersions :many
SELECT version, mode, settings, reason, created_at, created_by, change_id, restored_from
FROM pack_rule_versions
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND pack_id = @pack_id AND rule_id = @rule_id
ORDER BY version DESC;

-- name: InsertRuleVersion :exec
INSERT INTO pack_rule_versions (tenant_id, environment_id, pack_id, rule_id, version, mode, settings, reason, created_at, created_by, change_id, restored_from)
VALUES (@tenant_id, @environment_id, @pack_id, @rule_id, @version, @mode, @settings, @reason, @created_at, @created_by, @change_id, @restored_from);

-- name: InsertRuleChange :exec
INSERT INTO pack_rule_changes (tenant_id, environment_id, id, pack_id, rule_id, base_version, mode, settings, reason, restored_from, status, proposed_by, proposed_at)
VALUES (@tenant_id, @environment_id, @id, @pack_id, @rule_id, @base_version, @mode, @settings, @reason, @restored_from, 'pending', @proposed_by, @proposed_at);

-- name: GetRuleChange :one
SELECT id, pack_id, rule_id, base_version, mode, settings, reason, restored_from, status, proposed_by, proposed_at,
       decided_by, decided_at, decision_note, applied_version
FROM pack_rule_changes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: ListRuleChanges :many
SELECT id, pack_id, rule_id, base_version, mode, settings, reason, restored_from, status, proposed_by, proposed_at,
       decided_by, decided_at, decision_note, applied_version
FROM pack_rule_changes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(rule_id)::text IS NULL OR rule_id = sqlc.narg(rule_id)::text)
ORDER BY proposed_at DESC
LIMIT 200;

-- name: DecideRuleChange :exec
UPDATE pack_rule_changes
SET status = @status, decided_by = @decided_by, decided_at = @decided_at, decision_note = @decision_note,
    applied_version = @applied_version
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- Exchange rates (per tenant).

-- name: FXRates :many
SELECT currency, usd_per_unit::float8 AS usd_per_unit, updated_at, updated_by
FROM compliance_fx_rates WHERE tenant_id = @tenant_id ORDER BY currency;

-- name: UpsertFXRate :exec
INSERT INTO compliance_fx_rates (tenant_id, currency, usd_per_unit, updated_at, updated_by)
VALUES (@tenant_id, @currency, @usd_per_unit::float8, @updated_at, @updated_by)
ON CONFLICT (tenant_id, currency) DO UPDATE SET
  usd_per_unit = EXCLUDED.usd_per_unit, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by;

-- name: DeleteFXRate :execrows
DELETE FROM compliance_fx_rates WHERE tenant_id = @tenant_id AND currency = @currency;
