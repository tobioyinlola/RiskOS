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
SELECT weights, high_value_default, high_value_by_currency, fail_policies, alert_min_decision, thresholds,
       auto_block_mode, auto_block_shadow_since, updated_at, updated_by
FROM risk_settings
WHERE tenant_id = @tenant_id AND environment_id = @environment_id;

-- name: UpsertRiskSettings :exec
INSERT INTO risk_settings (tenant_id, environment_id, weights, high_value_default, high_value_by_currency, fail_policies, alert_min_decision, thresholds, auto_block_mode, auto_block_shadow_since, updated_at, updated_by)
VALUES (@tenant_id, @environment_id, @weights, @high_value_default, @high_value_by_currency, @fail_policies, @alert_min_decision, @thresholds, @auto_block_mode, @auto_block_shadow_since, @updated_at, @updated_by)
ON CONFLICT (tenant_id, environment_id) DO UPDATE SET
  weights = EXCLUDED.weights,
  high_value_default = EXCLUDED.high_value_default,
  high_value_by_currency = EXCLUDED.high_value_by_currency,
  fail_policies = EXCLUDED.fail_policies,
  alert_min_decision = EXCLUDED.alert_min_decision,
  thresholds = EXCLUDED.thresholds,
  auto_block_mode = EXCLUDED.auto_block_mode,
  auto_block_shadow_since = EXCLUDED.auto_block_shadow_since,
  updated_at = EXCLUDED.updated_at,
  updated_by = EXCLUDED.updated_by;

-- name: CountRules :one
SELECT count(*) FROM rules;

-- Rule changes awaiting two-person approval (production) -------------------------------------

-- name: InsertRuleChange :exec
INSERT INTO rule_changes (
  tenant_id, environment_id, id, rule_id, code, kind, base_version, name, description, severity,
  weight, mode, enabled, condition, restored_from, status, proposed_by, proposed_at
) VALUES (
  @tenant_id, @environment_id, @id, @rule_id, @code, @kind, @base_version, @name, @description, @severity,
  @weight, @mode, @enabled, @condition, @restored_from, 'pending', @proposed_by, @proposed_at
);

-- name: GetRuleChange :one
SELECT * FROM rule_changes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: LockRuleChange :one
SELECT * FROM rule_changes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: ListRuleChanges :many
SELECT * FROM rule_changes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY id DESC
LIMIT @lim;

-- name: ListPendingRuleChanges :many
SELECT rule_id, id FROM rule_changes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND status = 'pending';

-- name: DecideRuleChange :execrows
UPDATE rule_changes
SET status = @status, decided_by = @decided_by, decided_at = @decided_at, reason = @reason,
    applied_version = @applied_version
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id AND status = 'pending';

-- name: RuleCodeExists :one
SELECT EXISTS (
  SELECT 1 FROM rules WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND code = @code
);

-- name: CountWouldBlock :one
-- Evaluations since @since that automatic blocking would have blocked (shadow evidence).
SELECT count(*) FROM evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND created_at >= @since
  AND NOT dry_run AND 'would_block' = ANY (policy_reasons);
