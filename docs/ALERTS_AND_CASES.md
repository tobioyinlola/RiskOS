# Alerts and cases

**Purpose:** how risk decisions become alerts, how alerts are grouped and triaged, and how cases
investigate and resolve them. Design: [ARCHITECTURE.md §6 and §7](ARCHITECTURE.md). Status:
**built (build step 5).** Alert and case events reach customer systems as webhooks
([WEBHOOKS.md](WEBHOOKS.md), build step 6); the console is build step 7.

## Flow

```
POST /api/v1/events ─▶ decision transaction: event + evaluation + alert job (River, same commit)
                                                   │
worker ─▶ alerts_raise job ─▶ tenant scope re-checked ─▶ group or create alert ─▶ audit
                                                   │
analyst (API) ─▶ triage alert ─▶ open a case with alerts ─▶ notes ─▶ resolve case ─▶ alerts closed
```

- **Never on the decision path.** The event request only inserts a job row, in the same
  transaction as the decision (a transactional outbox): if the decision is stored, the job exists;
  if it rolls back, there is no job. The worker raises the alert moments later. A slow or stopped
  worker delays alerts; it never delays or changes decisions.
- **Exactly one alert entry per evaluation.** Jobs are delivered at least once; an evaluation can
  join at most one alert of each kind (a primary key), so a retried job changes nothing.
- **Dry runs, rule tests and idempotent replays never raise alerts.**
- The job carries identifiers and the decision snapshot only (evaluation, event, user reference,
  score, level, decision, signals), never payloads or secrets. The worker re-establishes the
  tenant scope from the job's tenant and environment and re-checks that the tenant is active;
  jobs of suspended or unknown tenants are dropped and logged. The job's principal holds a single
  system permission (`system:alerts_raise`) that no customer credential can have.

## Which decisions raise alerts

| Alert kind | Raised when | Subject (grouping key) | Severity |
|---|---|---|---|
| `risk` | The decision reaches the environment's threshold: REVIEW by default; a tenant can lower it to STEP_UP (`alert_min_decision` in `PUT /api/v1/risk/settings`) | The event's `actor.user_id` | The evaluation's risk band; the highest band when grouped |
| `security` | Any signal was degraded (the feature store was slow or down), whatever the decision | The environment | high |

Severities and case priorities use the five risk bands (minimal, low, elevated, high, critical).

A fail-closed REVIEW caused by degradation raises both. BLOCK is never produced (RISK_ENGINE.md).

## Grouping

An evaluation joins the most recent `open` or `investigating` alert of the same kind and subject
whose last evaluation is at most **1 hour** older; otherwise it opens a new alert. Grouping keeps a
burst of risky events for one user, or an outage, from flooding the queue. Raising is serialized
per subject (a transaction-scoped advisory lock), so concurrent evaluations cannot open two alerts
for one user. An alert shows its evaluation count, highest score, first and last seen times, and
its newest 100 evaluations with their signals.

## Alert lifecycle

| From | Allowed changes (`PATCH /api/v1/alerts/{id}`) |
|---|---|
| `open`, `investigating` | `open`, `investigating`, `false_positive`, `resolved`; assignee; note |
| `escalated` (in a case) | none: resolve the case |
| `false_positive`, `resolved` | none (final) |

Escalation happens only by creating a case with the alert. A change that alters nothing is a no-op.
Notes on alerts go into the alert's timeline.

## Cases

- `POST /api/v1/cases` opens a case, optionally escalating up to 50 alerts into it (all must be
  `open` or `investigating`; any failure creates nothing). Without an explicit priority, the case
  takes the highest severity of its alerts (`elevated` without alerts).
- `PATCH` changes title, description, priority, assignee and status (`open`, `investigating`).
  Resolving requires a resolution: `confirmed_fraud`, `false_positive`, `legitimate` or
  `inconclusive`, with an optional note. Resolving closes the case's alerts: `false_positive` for a
  false-positive resolution, `resolved` otherwise. A resolved case is final.
- Notes (`POST /api/v1/cases/{id}/notes`) are append-only and allowed on resolved cases too.
- Case detail returns its alerts, notes and timeline. Reading a case (`cases:read`) shows its
  alerts without `alerts:read`: they are part of the investigation.

## Assignment

Dashboard user accounts arrive with the console (build step 7). Until then, `assignee` is a
free-text reference to the analyst (an email or staff ID, at most 254 characters, empty to
unassign). It is not validated against any user list and grants nothing. TODO(phase-7): replace it
with a reference to a dashboard user.

## Permissions

| Scope | Allows |
|---|---|
| `alerts:read` | List and read alerts |
| `alerts:write` | Triage alerts; escalate alerts into a case (with `cases:write`) |
| `cases:read` | List and read cases, with their alerts |
| `cases:write` | Create, update and resolve cases; add notes |

Roles: `customer_admin`, `risk_manager` and `analyst` hold all four; `developer` holds none.

## Records and audit

- Alert status, assignee and counters change in place; every change is also written to an
  insert-only activity table (the timeline). Grouped evaluations, case notes and case activity are
  insert-only (database triggers). Every table has row-level security.
- Audit log: `alert.created` (by `system:alerts`), `alert.updated`, `case.created`,
  `case.updated`, `case.resolved`, `case.note_added`. Free text (notes, descriptions, resolution
  notes) stays in the alert or case; the audit log records that it changed, not its content.

## Tests

`backend/test/integration/alerts_test.go` runs the spec's end-to-end flow (tenant → key → event →
decision → alert → case → note → resolution → audit chain verified), grouping, the threshold
setting, dry runs and replays, security alerts with Redis unreachable, the triage and case state
machines, scopes, cross-tenant isolation, insert-only history, RLS and job idempotency.
