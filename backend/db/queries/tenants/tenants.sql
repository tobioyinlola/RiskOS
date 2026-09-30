-- name: InsertTenant :exec
INSERT INTO tenants (id, name, region) VALUES (@id, @name, @region);

-- name: InsertEnvironment :exec
INSERT INTO environments (id, tenant_id, kind) VALUES (@id, @tenant_id, @kind);

-- name: GetEnvironmentByKind :one
SELECT e.id, t.status AS tenant_status
FROM environments e JOIN tenants t ON t.id = e.tenant_id
WHERE e.tenant_id = @tenant_id AND e.kind = @kind;
