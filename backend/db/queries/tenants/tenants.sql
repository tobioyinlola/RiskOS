-- name: InsertTenant :exec
INSERT INTO tenants (id, name, region, origin, production_enabled)
VALUES (@id, @name, @region, @origin, @production_enabled);

-- name: InsertEnvironment :exec
INSERT INTO environments (id, tenant_id, kind) VALUES (@id, @tenant_id, @kind);

-- name: GetEnvironmentByKind :one
SELECT e.id, t.status AS tenant_status
FROM environments e JOIN tenants t ON t.id = e.tenant_id
WHERE e.tenant_id = @tenant_id AND e.kind = @kind;

-- name: GetEnvironment :one
SELECT e.kind, t.status AS tenant_status
FROM environments e JOIN tenants t ON t.id = e.tenant_id
WHERE e.tenant_id = @tenant_id AND e.id = @id;

-- Compliance onboarding (docs/CONSOLE.md#onboarding). Every query takes tenant_id explicitly.

-- name: InsertOnboarding :exec
INSERT INTO tenant_onboarding (tenant_id, status, updated_at, updated_by)
VALUES (@tenant_id, 'draft', @now, @updated_by);

-- name: GetOnboarding :one
SELECT o.*, t.name AS tenant_name, t.production_enabled
FROM tenant_onboarding o JOIN tenants t ON t.id = o.tenant_id
WHERE o.tenant_id = @tenant_id;

-- name: GetOnboardingForUpdate :one
SELECT * FROM tenant_onboarding WHERE tenant_id = @tenant_id FOR UPDATE;

-- name: SaveOnboardingAnswers :exec
UPDATE tenant_onboarding
SET answers = @answers, updated_at = @now, updated_by = @updated_by,
    status = CASE WHEN status = 'rejected' THEN 'draft' ELSE status END
WHERE tenant_id = @tenant_id;

-- name: SubmitOnboarding :exec
UPDATE tenant_onboarding
SET status = 'submitted', submitted_at = @now, submitted_by = @submitted_by, updated_at = @now, updated_by = @submitted_by
WHERE tenant_id = @tenant_id;

-- name: ReviewOnboarding :exec
UPDATE tenant_onboarding
SET status = @status, reviewed_at = @now, reviewed_by = @reviewed_by, review_note = @review_note
WHERE tenant_id = @tenant_id;

-- name: SetProductionEnabled :exec
UPDATE tenants SET production_enabled = @enabled WHERE id = @tenant_id;
