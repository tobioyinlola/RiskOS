# Events

**Purpose:** the event schema and the ingestion pipeline: authentication, validation,
normalization, idempotency, persistence and hand-off to risk evaluation.

## To be designed (TODO(phase-0), implemented in phase-2)

- Event envelope and per-type payload schemas, defined in `backend/api/openapi.yaml`.
- Validation rules and error codes.
- Normalization into the domain model (users, accounts, devices, transactions).
- Idempotency and replay policy: event IDs, idempotency keys, timestamp windows (spec §12).
- Storage model, partitioning and retention.
- What runs synchronously versus asynchronously (spec §20).
