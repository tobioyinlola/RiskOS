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
`device_id`, `session_id` optional), optional `context` (`ip`, `user_agent`, `country`, and `geo`
with `lat` and `lon`, rounded to 2 decimals, about 1 km, before it is stored) and a
`data` object whose shape depends on `type`:

| type | data |
|---|---|
| `login` | `outcome` (success, failure), `method` (password, passkey, otp, sso, biometric) |
| `transaction` | `transaction_id`, `amount` (integer minor units), `currency` (ISO 4217), `direction` (debit, credit), optional `beneficiary_id`, `channel` |
| `beneficiary_added` | `beneficiary_id`, optional `country` |
| `device_change` | `new_device_id`, optional `previous_device_id` |
| `session_change` | `change` (created, refreshed, terminated) |
| `profile_change` | `fields` (email, phone, address, name, password), which fields changed, never their values |
| `activity` | none; `compliance.kind` (required) says what it is: an account opened, an API request, a data export, a staff action... |

Identifiers are opaque customer references (`[A-Za-z0-9._:-]{1,128}`); never names or emails.

## Compliance

Every event may carry a `compliance` block with what the workspace's rule pack needs
([ADR-0006](adr/ADR-0006-jurisdiction-rule-packs.md); the pack's catalog is in the console's
Rules library). Everything in it is optional: a rule whose data is missing is reported as not
evaluated, never treated as no risk.

- **`kind`** is what the event is in the pack's terms. Money kinds (`cash_withdrawal`,
  `card_authorization`, `crypto_purchase`, `loan_repayment`...) go with `transaction` events;
  `login`, `auth_failure` and `auth_success` with `login`; `device_bound` and
  `device_reregistered` with `device_change`; the rest with `activity`. Without it, the kind is
  derived: a debit is `transfer_outbound` (`card_authorization` on the card channel), a credit
  `transfer_inbound`, a successful login `login` and `auth_success`, a failed one `auth_failure`.
  Every event also counts as `activity_any`, and money movements as `transaction_any`; forex
  deals also count as `forex_any`, cash deposits, withdrawals and payments as `cash_any`, and SIM
  registrations and replacements as `sim_change`. The country packs add money kinds (`forex_buy`,
  `forex_sell`, `wallet_cash_out`) and activity kinds (incidents, breaches, vendors, data
  transfers, security reviews: the full list is `ComplianceEventKind` in the API reference).
- **`facts`** are values from the customer's systems and providers, by catalog name
  (`device.rooted`, `telco.sim_swap`, `customer.type`, `screening.sanctions_score`...): `true` or
  `false`, a number at least 0 (durations in seconds; scores up to 100; hours up to 24), one of
  the fact's listed values, or `{"amount": <whole minor units>, "currency": "NGN"}`. Facts RiskOS
  computes are refused: behaviour history, workflow facts, identifiers (sent as `refs`), and the
  event's own amount, local hour and changed profile fields. At most 64. `account.has_bvn`
  sent with an `actor.account_id` also updates the account register REG-16's daily check reads
  (whether the account has a BVN, never the BVN): send it on `account_opened` and whenever it
  changes ([WORKFLOW.md](compliance/WORKFLOW.md#tasks)).
- **`booking_entity`** names the licensed entity that holds the customer relationship, as
  activated in the workspace ([ADR-0007](adr/ADR-0007-multi-jurisdiction.md)). Its country's pack
  judges the event; without it the workspace's fallback activation does, and with no fallback
  the decision's `jurisdiction.resolved_by` is `unresolved` and no pack runs.
- **`data_subject_country`** is the country of the person the data concerns, for
  data-protection rules only (a Rwandan data subject on a Ghana-booked account).
- **`refs`** are opaque references for the entities rules keep time windows for: `card`, `bvn`,
  `phone`, `address`, `loan`, `inflow`, `merchant`, `api_key`, `sender`, `reporter`,
  `funding_source`, `national_id` (Ghana Card, NIDA), `agent`. Never the value itself: a `bvn` of 11 digits or a `phone` that is a phone
  number is refused with `422 SENSITIVE_DATA`, like card numbers anywhere.

A kind that does not fit the event type, an unknown or computed fact, or a value of the wrong
type is `422 INVALID_EVENT`, naming the problem. The block is part of the normalized event, so
it is stored, hashed for idempotency and shown with the event.

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
