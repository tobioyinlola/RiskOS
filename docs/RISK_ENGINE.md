# Risk engine

**Purpose:** how signals are produced and combined into an explainable score, mapped to a risk
level and a decision, and how engine versions are managed.

## Product defaults (spec §8)

| Score | Level |
|---|---|
| 0–29 | LOW |
| 30–59 | MEDIUM |
| 60–79 | HIGH |
| 80–100 | CRITICAL |

Decisions: ALLOW / REVIEW / STEP-UP / BLOCK. These bands are initial product defaults, not
universal fraud standards.

## To be designed (TODO(phase-0), implemented in phase-3)

- Signal model (code, label, weight, evidence) and the risk response contract (spec §7).
- Scoring function, clamping and explainability guarantees.
- Level and decision mapping, including per-tenant configuration.
- Velocity counters in Redis (atomic Lua scripts, windows, key design).
- Engine versioning and persistence of every evaluation.
- Latency budget for the synchronous path (spec §20: p95 < 200 ms).
- Rules engine: structured, typed conditions only; never eval/exec (spec §9).
