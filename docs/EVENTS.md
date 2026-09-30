# Events

**Purpose:** the event contract and the ingestion pipeline: authentication, validation,
normalization, idempotency, persistence and hand-off to risk evaluation. Design:
[ARCHITECTURE.md §11](ARCHITECTURE.md#11-events-and-idempotency). Status: **built (build step
3)**; tests in `backend/test/integration/events_test.go`.

## Endpoints

| Endpoint | Scope | Persists | Use |
|---|---|---|---|
| `POST /api/v1/events` | `events:write` | Event, evaluation and idempotency record, in one transaction | Ingest an event and return the risk decision in the same response |
| `POST /api/v1/risk/evaluate` | `risk:evaluate` | An evaluation flagged `dry_run`; no event | What-if checks and integration testing; never alerts; not idempotent |
| `GET /api/v1/events` | `events:read` | - | Newest-first list with each event's score, level and decision |
| `GET /api/v1/events/{event_id}` | `events:read` | - | Raw payload, normalized payload and the stored evaluation |

## Pipeline (`POST /api/v1/events`)

1. **Authenticate** the API key; tenant and environment come from it (TENANCY.md).
2. **Rate limit** per key and per tenant (Redis GCRA; `429 RATE_LIMITED` with `Retry-After`).
3. **Contract validation** against `api/openapi.yaml` (kin-openapi): types, formats, patterns,
   enums, ranges, required fields, and **unknown fields rejected everywhere**. `400 INVALID_REQUEST`
   names the offending field.
4. **Semantic checks:** `occurred_at` window, IP syntax, card-number detection.
5. **Normalize:** UTC timestamps at microsecond precision, canonical IP (IPv4-mapped IPv6
   unmapped), compact data; the canonical bytes are hashed for idempotency.
6. **One transaction:** claim the idempotency key, evaluate (risk module), store the event with a
   copy of its decision, store the response. Nothing is visible until it commits.

## Envelope

Every event has `event_id`, `type`, `occurred_at`, `actor` (`user_id` required; `account_id`,
`device_id`, `session_id` optional), optional `context` (`ip`, `user_agent`, `country`) and a
`data` object whose shape depends on `type`:

| type | data |
|---|---|
| `login` | `outcome` (success, failure), `method` (password, passkey, otp, sso, biometric) |
| `transaction` | `transaction_id`, `amount` (integer minor units), `currency` (ISO 4217), `direction` (debit, credit), optional `beneficiary_id`, `channel` |
| `beneficiary_added` | `beneficiary_id`, optional `country` |
| `device_change` | `new_device_id`, optional `previous_device_id` |
| `session_change` | `change` (created, refreshed, terminated) |
| `profile_change` | `fields` (email, phone, address, name, password), which fields changed, never their values |

Identifiers are opaque customer references (`[A-Za-z0-9._:-]{1,128}`); never names or emails.

## Validation limits and errors

| Check | Response |
|---|---|
| Body over 64 KB | `413 PAYLOAD_TOO_LARGE` |
| Contract violation, unknown field, malformed JSON | `400 INVALID_REQUEST` |
| `occurred_at` more than 5 minutes ahead, or invalid IP | `422 INVALID_EVENT` |
| `occurred_at` more than 24 hours old | `422 STALE_EVENT` |
| A string contains a card-number-like value (13-19 digits, Luhn-valid, network prefix 2-6) | `422 SENSITIVE_DATA` (the value is never echoed) |
| Same `event_id`, different payload | `409 IDEMPOTENCY_CONFLICT` |

Numeric customer identifiers that look like card numbers are rejected too: use prefixed or opaque
identifiers.

## Idempotency

- Key: `event_id`, scoped to (tenant, environment). The same key in another environment or tenant
  is a different event.
- The `event_idempotency` primary key is the source of truth: a concurrent duplicate blocks on it
  until the first transaction commits, then reads the stored response. Tested with 20
  simultaneous duplicates: one event, one evaluation, 20 identical answers.
- Same key and same normalized payload (key order and whitespace do not matter): the stored
  response is returned verbatim, including the original `request_id`, with
  `Idempotent-Replay: true`.
- Same key, different payload: `409 IDEMPOTENCY_CONFLICT`; nothing is evaluated.
- The Redis `SET NX` fast path in the design is not built: the database already guarantees
  correctness, and the fast path only saves a query on retries (TODO(phase-7) if measurements
  show a need).

## Storage

- `events` and `evaluations` are range-partitioned by month on the time embedded in their UUIDv7
  ids, so a lookup by id reads one partition. The worker's daily `ensure_time_partitions` job
  keeps partitions `db.PartitionMonthsAhead` (3) months ahead; there is no default partition.
- Both tables are insert-only (grants and trigger). Raw and normalized payloads are stored
  separately. Partitions are reachable only through the RLS-protected parent tables.
- Retention (dropping old partitions per tenant policy) is not built yet (TODO(phase-7)).
