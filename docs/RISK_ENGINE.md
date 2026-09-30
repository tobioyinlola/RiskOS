# Risk engine

**Purpose:** how signals are produced and combined into an explainable score, mapped to a risk
level and a decision, and how evaluations stay reproducible. Design:
[ARCHITECTURE.md §10 and §12](ARCHITECTURE.md#10-risk-engine). Status: **designed;
implementation in build step 4** (a stub evaluation arrives with event ingest in build step 3).

## Pipeline

```
event ─▶ signal generators (concurrent, each with a timeout)
           rules · velocity · behavioral · device · geo
        ─▶ aggregator (sum of weights, capped at 100)
        ─▶ level (score bands)
        ─▶ decision policy (tenant-configurable)
        ─▶ immutable evaluation record
```

Each generator returns zero or more signals `{code, label, weight, evidence_ref}`. A generator
that times out or whose dependency is down contributes no signals and is listed in
`degraded_signals`.

## Score

```math
\text{score} = \min\left(100,\ \sum_{s \in \text{triggered}} w_s\right)
```

- Weights are integer points. Every point in the score appears in the response as a listed
  signal, so any response can be recomputed from its own signals.
- The spec's example (score 87 from three signals summing to 59) is an illustration error.
- Tenants may tune weights only within bounds set by the engine version.

## Levels (product defaults, tenant-adjustable)

| Score | Level |
|---|---|
| 0–29 | LOW |
| 30–59 | MEDIUM |
| 60–79 | HIGH |
| 80–100 | CRITICAL |

These are not industry standards and are never presented as such.

## Decision policy

The score sets the level; a separate policy sets the decision.

| Level | Default | Overrides |
|---|---|---|
| LOW | ALLOW | none |
| MEDIUM | ALLOW | STEP-UP when a strengthenable signal fired (new device, geo anomaly) |
| HIGH | STEP-UP | REVIEW when the event type cannot be stepped up |
| CRITICAL | REVIEW | BLOCK only if the tenant opted in to auto-block, or a hard-block rule matched |

**Auto-block guardrails:** opt-in per tenant, only after at least 14 days (assumption) in shadow
mode showing would-have-blocked counts; hard-block rules are limited to the tenant's own
deny-lists; a tenant kill switch disables blocking immediately. Every BLOCK names its signals.
Decisions are advisory; the customer's own flow enforces them.

## Degradation

Each tenant chooses **fail-open** (ALLOW with a degraded flag) or **fail-closed** (REVIEW). The
default is fail-closed for transactions and fail-open for logins. Regardless of policy, while any
source is degraded, events above the tenant's high-value threshold get at least **REVIEW**, and
the degradation is raised as a Security Center event. A failure may reduce signal coverage; it may
never silently lower risk.

## Rules

- JSON documents: typed fields, operators, AND/OR groups, priority, severity, weight. No `eval`,
  no scripting, no customer code; pattern matching is RE2 only (linear time).
- Compiled into an in-memory evaluation tree cached per tenant, environment and version, with
  maximum depth, node count and evaluation time.
- Versioned, testable against synthetic and historical events, one-step rollback, four-eyes
  approval for production promotion, and shadow mode for challenger rule sets.

## Velocity

One Redis Lua script per entity per event increments 1-minute buckets and returns sliding
1-minute, 1-hour and 24-hour counts in the same round trip. Keys are prefixed by tenant and
environment, identifiers are keyed-hashed per tenant, and keys expire with their longest window.

## Behavioral baselines

Precomputed asynchronously per user (typical amounts, hours, countries, devices) and read on the
hot path. Cold start: no anomaly signals, a small "new entity" signal instead. Updates are
rate-capped and exclude events already flagged high risk (anti-poisoning).

## Money

Integer minor units plus an ISO 4217 currency. Thresholds are per currency; no FX conversion on
the hot path.

## Reproducibility

Every evaluation stores the engine version, rule version IDs, the feature values used, all signals
and the decision. Re-running it against its recorded inputs must produce the same result.
Evaluations are insert-only.

## Latency budget

Score and policy: 1 ms. Parallel generator group: 8 ms. See ARCHITECTURE.md §5 for the full
30 ms plan and [PERFORMANCE_BASELINE.md](PERFORMANCE_BASELINE.md) for measured numbers.
