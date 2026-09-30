-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: InsertRule :exec
INSERT INTO rules (tenant_id, environment_id, id, code, current_version, created_by)
VALUES (@tenant_id, @environment_id, @id, @code, 1, @created_by);

-- name: InsertRuleVersion :exec
INSERT INTO rule_versions (
  tenant_id, environment_id, rule_id, version, name, description, severity, weight, mode,
  enabled, condition, created_by, restored_from
) VALUES (
  @tenant_id, @environment_id, @rule_id, @version, @name, @description, @severity, @weight, @mode,
  @enabled, @condition, @created_by, @restored_from
);

-- name: SetRuleCurrentVersion :execrows
-- Optimistic concurrency: moves current_version only if it is still the version the caller read.
UPDATE rules SET current_version = @new_version
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
  AND current_version = @expected_version;

-- name: GetCurrentRule :one
SELECT r.id, r.code, r.created_at, v.version, v.name, v.description, v.severity, v.weight, v.mode,
       v.enabled, v.condition, v.created_at AS version_created_at, v.created_by AS version_created_by, v.restored_from
FROM rules r
JOIN rule_versions v ON v.rule_id = r.id AND v.version = r.current_version
WHERE r.tenant_id = @tenant_id AND r.environment_id = @environment_id AND r.id = @id;

-- name: ListCurrentRules :many
SELECT r.id, r.code, r.created_at, v.version, v.name, v.description, v.severity, v.weight, v.mode,
       v.enabled, v.condition, v.created_at AS version_created_at, v.created_by AS version_created_by, v.restored_from
FROM rules r
JOIN rule_versions v ON v.rule_id = r.id AND v.version = r.current_version
WHERE r.tenant_id = @tenant_id AND r.environment_id = @environment_id
ORDER BY r.code;

-- name: ListRuleVersions :many
SELECT version, name, description, severity, weight, mode, enabled, condition, created_at, created_by, restored_from
FROM rule_versions
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND rule_id = @rule_id
ORDER BY version DESC;

-- name: GetRuleVersion :one
SELECT version, name, description, severity, weight, mode, enabled, condition, created_at, created_by, restored_from
FROM rule_versions
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND rule_id = @rule_id AND version = @version;

-- name: GetRiskSettings :one
SELECT weights, high_value_default, high_value_by_currency, fail_policies, updated_at, updated_by
FROM risk_settings
WHERE tenant_id = @tenant_id AND environment_id = @environment_id;

-- name: UpsertRiskSettings :exec
INSERT INTO risk_settings (tenant_id, environment_id, weights, high_value_default, high_value_by_currency, fail_policies, updated_at, updated_by)
VALUES (@tenant_id, @environment_id, @weights, @high_value_default, @high_value_by_currency, @fail_policies, @updated_at, @updated_by)
ON CONFLICT (tenant_id, environment_id) DO UPDATE SET
  weights = EXCLUDED.weights,
  high_value_default = EXCLUDED.high_value_default,
  high_value_by_currency = EXCLUDED.high_value_by_currency,
  fail_policies = EXCLUDED.fail_policies,
  updated_at = EXCLUDED.updated_at,
  updated_by = EXCLUDED.updated_by;

-- name: CountRules :one
SELECT count(*) FROM rules;
