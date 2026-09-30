-- name: Ping :one
-- Readiness probe: proves a pooled connection can execute a statement.
SELECT 1::int4 AS ok;
