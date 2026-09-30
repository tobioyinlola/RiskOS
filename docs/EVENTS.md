# Events

**Purpose:** the event contract and the ingestion pipeline: authentication, validation,
normalization, idempotency, persistence and hand-off to risk evaluation. Design:
[ARCHITECTURE.md §11](ARCHITECTURE.md#11-events-and-idempotency). Status: **designed;
implementation in build step 3.**

## Endpoints

| Endpoint | Persists | Use |
|---|---|---|
| `POST /api/v1/events` | Event, evaluation and outbox rows, in one transaction | Primary path: ingest an event and return the risk decision in the same response |
| `POST /api/v1/risk/evaluate` | An evaluation flagged as a dry run; no event record | What-if checks and integration testing; never triggers alerts or webhooks |

## Envelope

| Field | Meaning |
|---|---|
| `event_id` | Customer-supplied, unique per tenant and environment; the default idempotency key |
| `type` | `login`, `transaction`, `beneficiary_added`, `device_change`, `session_change`, `profile_change` (initial set) |
| `occurred_at` | When it happened on the customer side (RFC 3339) |
| `actor` | References: user, account, device, session |
| `context` | IP, user agent, location |
| `data` | Typed object per event type |

Schemas are versioned in `backend/api/openapi.yaml`. Unknown fields are rejected in strict mode;
additive changes are backward compatible.

## Validation limits

| Limit | Default |
|---|---|
| Body size | 64 KB (assumption; in force via `RISKOS_HTTP_MAX_BODY_BYTES`) |
| Nesting depth | bounded (value set with the schema) |
| Timestamp skew | up to 5 minutes ahead and 24 hours behind server time (assumption) |
| Money | integer minor units plus ISO 4217 currency |
| Forbidden content | full card numbers, CVV, PINs, passwords, authentication secrets: rejected. Bank account numbers only as tokens or masked values |

## Idempotency

1. Key: `event_id`, or an `Idempotency-Key` header, scoped to `(tenant, environment, key)`.
2. Fast path: Redis `SET NX` with a short TTL marks the key in flight.
3. Source of truth: unique index on `(tenant_id, environment_id, event_key)`; insert with
   `ON CONFLICT DO NOTHING`, and the loser reads the winner's stored response.
4. Same key, same payload hash: replay the stored response with `Idempotent-Replay: true`.
5. Same key, different payload hash: `409 IDEMPOTENCY_CONFLICT`; nothing is evaluated.
6. Idempotency records are retained for the event retention window.

If Redis is down, the Postgres unique index alone guarantees correctness.

## Replay protection

Skew limits plus the unique key stop old requests being reprocessed. Stale events: `422
STALE_EVENT` on real-time endpoints; accept-without-alerting for batch backfills.

## Storage

`events` and `evaluations` are time-partitioned; retention drops whole partitions (90 days hot by
default, per-tenant configurable). Raw and normalized payloads are stored separately, and the raw
payload can be dropped earlier.

## Contracts

Risk response: `request_id`, `event_id`, `risk_score`, `risk_level`, `decision`,
`engine_version`, `signals`, plus `degraded_signals` when optional sources failed. Errors:
`{"error":{"code","message","request_id"}}`.
