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
| POST | `/api/v1/events` | Ingest an event, get the risk decision; idempotent on `event_id` (`events:write`) |
| GET | `/api/v1/events` | List events (`events:read`) |
| GET | `/api/v1/events/{event_id}` | Event with raw and normalized payloads and its evaluation (`events:read`) |
| POST | `/api/v1/risk/evaluate` | Dry-run evaluation; stores no event (`risk:evaluate`) |
| GET | `/api/v1/risk/policy` | The environment's bands, thresholds and default decisions (any score-reading scope) |
| GET | `/api/v1/risk/catalog` | Built-in signals with effective weights, and the fields and operators rules can use (`rules:read`) |
| GET | `/api/v1/risk/settings` | Signal weights, high-value thresholds, fail policies (`risk:configure`) |
| PUT | `/api/v1/risk/settings` | Replace them; audited (`risk:configure`) |
| GET | `/api/v1/rules` | List rules with their current versions (`rules:read`) |
| POST | `/api/v1/rules` | Create a rule; audited (`rules:write`) |
| GET | `/api/v1/rules/{rule_id}` | A rule's current version (`rules:read`) |
| PATCH | `/api/v1/rules/{rule_id}` | Change a rule, creating a new version; audited (`rules:write`) |
| GET | `/api/v1/rules/{rule_id}/versions` | Version history (`rules:read`) |
| POST | `/api/v1/rules/{rule_id}/rollback` | Restore an earlier version as a new version; audited (`rules:write`) |
| GET | `/api/v1/alerts` | List alerts; filter by `status`, `kind` (`alerts:read`) |
| GET | `/api/v1/alerts/{alert_id}` | Alert with its evaluations and timeline (`alerts:read`) |
| PATCH | `/api/v1/alerts/{alert_id}` | Triage: status, assignee, note; audited (`alerts:write`) |
| GET | `/api/v1/cases` | List cases; filter by `status` (`cases:read`) |
| POST | `/api/v1/cases` | Open a case, optionally escalating alerts; audited (`cases:write`, plus `alerts:write` with alerts) |
| GET | `/api/v1/cases/{case_id}` | Case with its alerts, notes and timeline (`cases:read`) |
| PATCH | `/api/v1/cases/{case_id}` | Update or resolve a case; audited (`cases:write`) |
| POST | `/api/v1/cases/{case_id}/notes` | Add an append-only note; audited (`cases:write`) |
| GET | `/api/v1/webhooks` | List webhook endpoints (`webhooks:read`) |
| POST | `/api/v1/webhooks` | Register an endpoint; secret returned once; audited (`webhooks:write`) |
| GET | `/api/v1/webhooks/{webhook_id}` | An endpoint (never its secret) (`webhooks:read`) |
| PATCH | `/api/v1/webhooks/{webhook_id}` | Change, disable or re-enable an endpoint; audited (`webhooks:write`) |
| POST | `/api/v1/webhooks/{webhook_id}/rotate-secret` | New secret, shown once, with an overlap; audited (`webhooks:write`) |
| POST | `/api/v1/webhooks/{webhook_id}/test` | Queue a `webhook.test` event (`webhooks:write`) |
| GET | `/api/v1/webhooks/{webhook_id}/deliveries` | Delivery log; filter by `status` (`webhooks:read`) |
| GET | `/api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}` | A delivery with its payload and attempts (`webhooks:read`) |
| POST | `/api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}/redeliver` | Retry a failed delivery; audited (`webhooks:write`) |
| POST | `/api/v1/rules/{rule_id}/test` | Test the rule, or a draft condition, against an event; stores nothing (`rules:read`) |

## Conventions (in force)

- Every error uses the error contract `{"error":{"code","message","request_id"}}` (spec §7).
  Codes are stable UPPER_SNAKE_CASE; messages are safe to show and never contain internals.
- Every response carries `X-Request-ID`.
- Product endpoints live under `/api/v1` and require `Authorization: Bearer rsk_…`. The key
  determines tenant and environment; no request field can change them.
- Status codes: 400 `INVALID_REQUEST`, 401 `UNAUTHENTICATED` (never says why), 403 `FORBIDDEN`
  or `TENANT_SUSPENDED`, 404 `NOT_FOUND` (also for other tenants' resources), 409 `CONFLICT` or
  `IDEMPOTENCY_CONFLICT`, 413 `PAYLOAD_TOO_LARGE`, 422 `INVALID_EVENT`, `STALE_EVENT` or
  `SENSITIVE_DATA`, 429 `RATE_LIMITED` (with `Retry-After`).
- Every request is validated against this contract before it reaches a handler; unknown fields
  are rejected.
- IDs are type-prefixed (`ten_`, `env_`, `key_`, `aud_`, `evt_`, `evl_`, `rul_`, `alr_`, `cas_`, `cnt_`, `act_`, `whk_`, `whd_`, `wev_`); an ID of the wrong
  type is not found.
- Lists: `?limit=` (1-100, default 50) and `?cursor=` (the previous page's `next_cursor`);
  newest first.

## To be designed (TODO(phase-0))

- Remaining operations of spec §7 (entities, analytics).
- Console session authentication (build step 7).
- Filtering and sorting conventions for lists (event type, decision, time range).
- Versioning and deprecation policy.
