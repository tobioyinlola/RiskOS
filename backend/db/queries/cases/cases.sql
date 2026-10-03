-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: InsertCase :exec
INSERT INTO cases (
  tenant_id, environment_id, id, title, description, status, priority, assignee,
  created_at, created_by, updated_at
) VALUES (
  @tenant_id, @environment_id, @id, @title, @description, 'open', @priority, sqlc.narg('assignee'),
  @now, @created_by, @now
);

-- name: GetCase :one
SELECT * FROM cases
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: GetCaseForUpdate :one
SELECT * FROM cases
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: ListCases :many
SELECT * FROM cases
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('resolution')::text IS NULL OR resolution = sqlc.narg('resolution')::text)
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- name: UpdateCase :exec
UPDATE cases SET
  title = @title, description = @description, status = @status, priority = @priority,
  assignee = sqlc.narg('assignee'), resolution = sqlc.narg('resolution'),
  resolution_note = sqlc.narg('resolution_note'), resolved_at = sqlc.narg('resolved_at'),
  resolved_by = sqlc.narg('resolved_by'), updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: InsertCaseNote :exec
INSERT INTO case_notes (tenant_id, environment_id, id, case_id, body, author, created_at)
VALUES (@tenant_id, @environment_id, @id, @case_id, @body, @author, @created_at);

-- name: ListCaseNotes :many
SELECT id, body, author, created_at FROM case_notes
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND case_id = @case_id
ORDER BY id;

-- name: InsertCaseActivity :exec
INSERT INTO case_activity (tenant_id, environment_id, id, case_id, action, actor, detail, created_at)
VALUES (@tenant_id, @environment_id, @id, @case_id, @action, @actor, @detail, @created_at);

-- name: ListCaseActivity :many
SELECT id, action, actor, detail, created_at FROM case_activity
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND case_id = @case_id
ORDER BY id;
