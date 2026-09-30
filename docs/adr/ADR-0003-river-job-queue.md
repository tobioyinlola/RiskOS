# ADR-0003: River as the job queue

- **Status:** accepted (confirms the provisional choice made in build step 1)
- **Date:** 2026-09-30

## Context

The decision path must hand work (alert creation, webhook delivery, rollups, baseline updates) to
background workers without ever losing it or letting it disagree with the decision the customer
received ([ARCHITECTURE.md §6](../ARCHITECTURE.md#6-asynchronous-paths)). That requires the
enqueue to commit atomically with the evaluation. The foundation shipped River behind the
`internal/jobs` interface as a provisional choice, pending this decision.

## Decision

Use **River** (PostgreSQL-backed) as the job queue ([ARCHITECTURE.md §17](../ARCHITECTURE.md#17-decisions-and-build-order), decision 6).

- Jobs are enqueued in the **same transaction** as the evaluation (transactional outbox). If the
  transaction rolls back, the jobs never existed.
- Jobs are delivered at least once; every handler is idempotent and every job carries an
  idempotency key.
- River stays confined to `backend/internal/jobs` (depguard `queue-implementation-confined`);
  modules use the `jobs` interfaces. The interface gains a transactional enqueue method in build
  step 3, when the first transaction needs it.
- Webhook delivery and alert creation run in separate queues, so slow customer endpoints cannot
  delay alerts.
- Queue depth and oldest-job age are first-class metrics with alerts.

## Alternatives considered

- **Redis-based queue (e.g. Asynq):** no atomic link to the decision commit. A crash between
  commit and enqueue loses the alert or webhook, or an enqueue before commit publishes a decision
  that never happened.
- **Separate outbox table plus a relay:** also correct, but it builds by hand what River already
  provides (retries, backoff, scheduling, dead-lettering, observability).
- **External broker (Kafka, SQS, NATS):** a new stateful system to operate, and it still needs an
  outbox for atomicity. Revisit only at the extraction triggers in ARCHITECTURE.md §13.

## Consequences

- Queue load lands on the primary database. At the design point (500 events/s, about one job per
  event) this fits the capacity plan in §2. Queue tables are part of database monitoring and
  vacuum tuning.
- River's schema migrations run through `cmd/migrate` after the application migrations.
- Replacing River later means reimplementing `internal/jobs` only; callers do not change.
