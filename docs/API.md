# API

**Purpose:** how the public HTTP API is specified, versioned and evolved.

The contract is [`backend/api/openapi.yaml`](../backend/api/openapi.yaml). It is the single source
of truth: Go server stubs (`backend/internal/apigen`) and the TypeScript client
(`frontend/lib/api/schema.gen.ts`) are generated from it with `make generate`, and CI fails when
generated code is stale. An operation that is not in the contract cannot be routed.

## Current operations

| Method | Path | Purpose |
|---|---|---|
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe (PostgreSQL, Redis, draining) |
| GET | `/api/v1/whoami` | The tenant, environment and scopes of the calling key |
| GET | `/api/v1/api-keys` | List keys of the caller's environment (`api_keys:read`) |
| POST | `/api/v1/api-keys` | Create a key; secret returned once (`api_keys:write`) |
| POST | `/api/v1/api-keys/{key_id}/rotate` | Replace a key, old one valid for an overlap window (`api_keys:write`) |
| POST | `/api/v1/api-keys/{key_id}/revoke` | Revoke a key (`api_keys:write`) |
| GET | `/api/v1/audit-logs` | Tenant-level and own-environment audit entries (`audit_logs:read`) |

## Conventions (in force)

- Every error uses the error contract `{"error":{"code","message","request_id"}}` (spec §7).
  Codes are stable UPPER_SNAKE_CASE; messages are safe to show and never contain internals.
- Every response carries `X-Request-ID`.
- Product endpoints live under `/api/v1` and require `Authorization: Bearer rsk_…`. The key
  determines tenant and environment; no request field can change them.
- Status codes: 400 `INVALID_REQUEST`, 401 `UNAUTHENTICATED` (never says why), 403 `FORBIDDEN`
  or `TENANT_SUSPENDED`, 404 `NOT_FOUND` (also for other tenants' resources), 409 `CONFLICT`.
- IDs are type-prefixed (`ten_`, `env_`, `key_`, `aud_`); an ID of the wrong type is not found.
- Lists: `?limit=` (1-100, default 50) and `?cursor=` (the previous page's `next_cursor`);
  newest first.

## To be designed (TODO(phase-0))

- Remaining operations of spec §7 (events, risk evaluation, entities, alerts, cases, rules,
  webhooks, analytics).
- Console session authentication (build step 7).
- Filtering and sorting conventions; idempotency-key header; rate-limit headers (build step 3).
- Versioning and deprecation policy.
