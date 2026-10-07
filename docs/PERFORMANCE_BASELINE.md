# Performance baseline

**Purpose:** the starting reference for RiskOS latency and throughput, measured on the empty
platform (`GET /health/live`: full middleware stack, routing and JSON, with no dependencies
touched). Every later endpoint is compared against this floor. The product target in spec §20
(core risk evaluation p95 < 200 ms) applies to risk evaluation, not to this endpoint.

Recorded 2026-09-30 at commit `0e8bc68` plus the load-script fixes committed alongside this
document.

## Environment

These numbers come from a developer laptop, **not** production hardware. Treat them as a relative
reference for regressions, not as capacity figures.

| | |
|---|---|
| CPU | Intel Core i7-8665U @ 1.90 GHz, 4 cores / 8 threads (laptop, thermally limited) |
| Memory | 7 GiB available to WSL2 |
| OS | Windows 11 → WSL2 Ubuntu 26.04 → Docker Desktop 29.8.0 |
| Stack | `make up` images (distroless, production build); API with default config |
| Load generator | k6 2.3.0 in Docker on the **same machine**, competing for the same CPUs |
| Network path | k6 → `127.0.0.1:8080` → Docker Desktop port publishing → api container (keep-alive) |

## 1. In-process micro-benchmarks (`make bench`)

`go test -bench -benchmem -count=5`, median of 5 runs:

| Benchmark | Time/op | Allocs/op | Bytes/op | What it covers |
|---|---|---|---|---|
| `BenchmarkLiveness` | ~17 µs | 75 | 6.0 KiB | Full handler: otelhttp (no-op), request ID, access log (production logger with redaction), recovery, route telemetry, security headers, timeout, body limit, chi routing, strict handler, JSON |
| `BenchmarkLogLine` | ~3.2 µs | 5 | 160 B | One access-log line through the redacting JSON logger |
| `BenchmarkNewRequestID` | ~170 ns | 1 | 32 B | 128-bit crypto-random request ID |

The HTTP stack costs about 0.01% of the 200 ms risk-evaluation budget.

## 2. Load test (`make load`, k6, 30 s constant arrival rate)

Default configuration (telemetry on with 100% sampling, info-level access logs):

| Run | Offered | Achieved | Failed | Dropped* | Median | p95 | p99 | Max | API CPU |
|---|---|---|---|---|---|---|---|---|---|
| r1 | 2,000/s | 2,000/s | 0 | 0 | 0.64 ms | 2.01 ms | 8.21 ms | 61 ms | 134% |
| r2 | 2,000/s | 2,000/s | 0 | 0 | 0.63 ms | 1.54 ms | 5.68 ms | 90 ms | 133% |
| r3 | 2,000/s | 2,000/s | 0 | 0 | 0.64 ms | 1.69 ms | 7.66 ms | 75 ms | 140% |
| saturation | 5,000/s | **4,827/s** | 0 | 5,194 | 1.45 ms | 48.1 ms | 115 ms | 337 ms | 197% |

\* Dropped iterations are requests k6 could not start on schedule because all virtual users were
busy waiting. They show the system is saturated; they are not server errors.

**Baseline at 2,000 req/s:** 0 errors, median ≈ 0.64 ms, p95 ≈ 1.5–2.0 ms, p99 ≈ 6–8 ms.
API memory stays at ≈ 22 MiB. **Saturation on this machine: ≈ 4,800 req/s**, where p99 exceeds
100 ms (the script's guardrail threshold fails at that rate, as expected).

### Single-variable comparisons (same image, config changed via environment)

| Variant | Offered | Median | p95 | p99 | API CPU |
|---|---|---|---|---|---|
| Default (from r1–r3) | 2,000/s | 0.64 ms | ≈1.7 ms | ≈7 ms | ≈135% |
| Telemetry off (`RISKOS_OTEL_ENABLED=false`) | 2,000/s | 0.64 ms | 2.71 ms | 10.9 ms | 140% |
| Telemetry off | 5,000/s | 0.91 ms (4,853/s achieved) | 19.1 ms | 81.2 ms | 199% |
| Access logs off (`RISKOS_LOG_LEVEL=warn`) | 2,000/s | 0.54 ms | 1.11 ms | 6.0 ms | 121% |

## 3. Decision path (`make load-events`, build step 3)

`POST /api/v1/events` with a new transaction event per request: authentication (cached key
lookup), rate limiting (Redis), contract validation, card-number scan, normalization, then one
PostgreSQL transaction (idempotency claim, evaluation insert, event insert, response update). The
engine is still the stub (mean 7 µs), so this measures the fixed cost of the path that every real
evaluation will add to. Same laptop and stack as above; rate limits raised for the run.

| Offered | Achieved | Failed | Median | p90 | p95 | p99 | Max | CPU (api / postgres / redis) |
|---|---|---|---|---|---|---|---|---|
| 100 events/s | 99/s | 0 | 10.8 ms | - | **13.5 ms** | 40.0 ms | - | 54% / 39% / 14% |
| 500 events/s (design peak) | 499/s | 0 | 11.5 ms | 34.3 ms | **87.3 ms** | 187.4 ms | 316 ms | - |

Every accepted request was stored: 17,987 events and 17,987 non-dry-run evaluations.

Against the targets (ARCHITECTURE.md §1, §5): the spec ceiling (p95 < 200 ms) holds at the design
peak. The internal target (p95 < 60 ms) holds at 100 events/s but not at 500 events/s on this
laptop, where k6, the API, PostgreSQL and Redis share four cores; the median barely moves (10.8 →
11.5 ms), so the tail is contention, not per-request cost. The persist step (planned at 12 ms) is
the dominant cost. Re-measure on separated, production-like hosts before treating the tail as a
product number, and before build step 4 adds real signal generators.

## 4. Decision path with the real engine (build step 4, `risk-engine-1.0.0`)

Same path as §3 plus the feature-store round trip (one Redis pipeline: seen sets, velocity script,
HyperLogLogs), rule evaluation, the policy, and the amount-baseline write for low-risk debits. The
load uses one user, account and device per k6 VU (100 pre-allocated, up to 1,000) paying one
beneficiary, so velocity and fan-in signals fire during the run. Same laptop; `RISKOS_FEATURE_TIMEOUT` at its 50 ms default.

| Run | Offered | Achieved | Failed | Dropped | Median | p90 | p95 | p99 | Max |
|---|---|---|---|---|---|---|---|---|---|
| 1 | 100 events/s | 100/s | 0 | 0 | 8.1 ms | 10.7 ms | **12.5 ms** | 21.0 ms | 86 ms |
| 2 | 100 events/s | 100/s | 0 | 0 | 8.1 ms | 10.5 ms | **13.2 ms** | 106 ms | 239 ms |
| 1 | 500 events/s | 500/s | 0 | 0 | 15.2 ms | 58.2 ms | **76.6 ms** | 106 ms | 157 ms |
| 2 | 500 events/s | 500/s | 0 | 12 | 15.8 ms | 39.0 ms | **58.0 ms** | 112 ms | 246 ms |
| 3 | 500 events/s | 463/s | 0 | 708 | 38.2 ms | 719 ms | **856 ms** | 1,033 ms | 1,114 ms |

CPU at 500/s: API ≈190%, PostgreSQL ≈160%, Redis ≈60% of a 4-core / 8-thread laptop that also
runs k6. Five evaluations across the 500/s runs exceeded the 50 ms feature timeout at the tail and
were degraded (fail-closed to REVIEW, logged), which is the designed behavior under overload.

Against the targets: at 100 events/s the p95 is unchanged from the stub engine (13.5 → 12.5–13.2
ms). At the 500 events/s design peak, two of three runs are within the spec ceiling (p95 < 200 ms)
and better than the stub-engine run in §3; the third saturated the machine (dropped iterations,
queueing) and did not. The design peak is therefore **not demonstrated on this laptop**; it must
be re-measured on separated hosts (TODO(phase-8)).

What the measurements changed:

1. **`EVAL` → `EVALSHA`.** Redis commandstats showed each pipelined script call sending its whole
   body at ≈100 µs per call. Scripts are now loaded once and called by SHA: ≈20 µs per call.
   go-redis v9.22's `Script.Load` on a pipeline overwrites the script's SHA with an empty string, so
   the store loads script sources with `SCRIPT LOAD` directly; the first measurement after the
   change was invalid (every script call failed with `NOSCRIPT` and fell back to degraded) until
   this was found.
2. **Evaluations of ingested events no longer duplicate the event** in their `features` column (it
   is in `events`); dry runs keep it. Less JSON to encode and less WAL per event.
3. **A larger database pool made it worse** (40 connections: p95 3.2 s), so the tail is contention
   in a saturated machine, not pool starvation.

A CPU profile of the API at 500/s (pprof, taken with a temporary local build) put ≈29% of API CPU
in system calls (network I/O through Docker Desktop's VM), ≈30% under PostgreSQL round trips
and ≈10% in the feature store.

## 5. Build step 5 check (alerts and cases)

The decision path gains one job-row insert, only for decisions that raise an alert (REVIEW by
default, or degraded); all other work happens in the worker. An A/B run of the step 4 and step 5
API images, alternating on the same stack, found no difference attributable to the change: in the
first round step 5 was no slower (100 events/s p95 21.9 ms vs 54.1 ms for step 4; 300/s p95 150 ms
vs 419 ms), and in the second round **both** builds degraded to multi-second tails as the laptop
throttled (host load average ≈9 on 8 threads; the untouched health endpoint's p95 was 5.5 ms
against its 1.5-2.0 ms baseline in section 1). These numbers describe the machine's state, not
the code, so section 4 remains the reference until the separated-host measurement (TODO(phase-8)).

## 6. Decision path with the Nigeria rule pack (rule pack phase 6)

Same path as §4, for a workspace with the Nigeria pack (`ng@2026.10.1`, bank, defaults) and
events carrying the compliance block a pack reads (`PACK=1`: customer profile, device, telco and
screening facts). Every request also runs the pack's rules: about 18 more window scripts in the
second Redis round trip (time windows, activity profile), the interpreter, and an evaluation
row with the controls and the rules not evaluated. Measured against a workspace without a pack
(the same events, `PACK=0`), alternating, on the same laptop and stack (2026-10-03, WSL 2 under
Docker Desktop, 8 threads, 7.6 GiB). Before each set, the two benchmark workspaces' Redis keys
were cleared and warmed for 60 s at 100 events/s.

**Concentrated load** (one customer per k6 VU, as in §4: every customer is busy):

| Run | Offered | Achieved | Failed | Median | p90 | p95 | p99 | Redis script time per event |
|---|---|---|---|---|---|---|---|---|
| No pack 1 | 100/s | 100/s | 0 | 7.9 ms | 9.7 ms | **10.6 ms** | 17.7 ms | 0.39 ms |
| Pack 1 | 100/s | 100/s | 0 | 10.8 ms | 14.7 ms | **18.7 ms** | 43.9 ms | 1.63 ms |
| No pack 2 | 100/s | 100/s | 0 | 7.9 ms | 9.7 ms | **10.8 ms** | 21.3 ms | 0.40 ms |
| Pack 2 | 100/s | 100/s | 0 | 10.6 ms | 13.5 ms | **16.1 ms** | 31.7 ms | 1.72 ms |
| No pack 3 | 100/s | 100/s | 0 | 8.0 ms | 11.5 ms | **14.4 ms** | 25.0 ms | 0.45 ms |
| Pack 3 | 100/s | 100/s | 0 | 11.6 ms | 16.2 ms | **21.3 ms** | 40.9 ms | 2.18 ms |

**Spread load** (`ACCOUNTS=20000`: traffic from 20,000 customers, as production traffic at these
rates would be):

| Run | Offered | Achieved | Failed | Median | p90 | p95 | p99 | Redis script time per event |
|---|---|---|---|---|---|---|---|---|
| No pack 1 | 100/s | 100/s | 0 | 8.6 ms | 10.8 ms | **12.0 ms** | 20.1 ms | 0.45 ms |
| Pack 1 | 100/s | 100/s | 0 | 10.3 ms | 12.8 ms | **14.4 ms** | 22.1 ms | 0.97 ms |
| No pack 2 | 100/s | 100/s | 0 | 8.5 ms | 10.7 ms | **12.2 ms** | 23.9 ms | 0.46 ms |
| Pack 2 | 100/s | 100/s | 0 | 10.5 ms | 14.1 ms | **16.6 ms** | 27.8 ms | 1.04 ms |
| No pack 3 | 100/s | 100/s | 0 | 8.6 ms | 11.0 ms | **12.3 ms** | 23.4 ms | 0.46 ms |
| Pack 3 | 100/s | 100/s | 0 | 10.8 ms | 14.2 ms | **16.4 ms** | 35.5 ms | 1.10 ms |
| No pack 1 | 250/s | 250/s | 0 | 9.8 ms | 15.5 ms | **19.9 ms** | 43.2 ms | 0.60 ms |
| Pack 1 | 250/s | 250/s | 0 | 16.9 ms | 92.6 ms | **167.6 ms** | 301.0 ms | 1.62 ms |
| No pack 2 | 250/s | 250/s | 0 | 8.8 ms | 13.3 ms | **16.7 ms** | 31.4 ms | 0.54 ms |
| Pack 2 | 250/s | 250/s | 0 | 13.9 ms | 24.6 ms | **36.4 ms** | 106.9 ms | 1.36 ms |

Against the targets: at 100 events/s the pack adds 2–7 ms to the p95 (14–21 ms with the pack,
within the internal 60 ms target). At 250 events/s both runs are within the spec ceiling (p95 <
200 ms); one is within the internal target and one is not, where k6, the API, PostgreSQL and Redis
share the laptop. 500 events/s was not attempted with the pack: this laptop under WSL no longer
sustains it even without one (connection-pool timeouts in §4's setup; saturated in this
session's no-pack runs too). The design peak with a pack must be measured on separated hosts
(TODO(phase-8)). No per-event PostgreSQL query was added: windows and history live in Redis, and
the pack's configuration is cached with the engine's.

What the measurements changed (each found by these runs, each fixed and re-measured):

1. **Window reads scanned the whole window.** Every query parsed every entry of its window (up
   to 20,000) on every event. With one busy entity, Redis spent 217 µs per window call and the
   pack saturated at 250 events/s (p95 ≈ 1 s) while the same load without a pack held p95 26–36
   ms; on a shared Redis it is also a cross-tenant availability risk. Windows now keep indexes
   (unknown entries, each identifier's latest time, per-bucket counts and sums): counts and
   distinct counts are one `ZCOUNT`, sums add buckets and scan only the edges (RISK_ENGINE.md).
   Results are exactly the backtest's, tested on random event streams
   (`internal/risk/internal/store/windows_integration_test.go`).
2. **A read could size a window's buckets.** An event outside a window's filter reads it with no
   retention; the first version sized buckets from that (1 s), so a 30-day query spanned millions
   of buckets (175 ms per call, Redis circuit open). Bucket size now follows writes only; a test
   covers it.
3. **Concurrent arrivals made every distinct count a scan.** Events reach Redis slightly out of
   `occurred_at` order; an event at most a minute older than the window's newest now uses the
   index (counting identifiers first seen in that minute too: more risk, never less).
4. **Empty windows and writes did needless work.** Most windows an event reads are empty for its
   entity; they now answer after one `ZCARD`. Trimming, old-bucket removal and index expiry run
   once per bucket instead of on every write.

Together: the pack's Redis time per event went from 4–6 ms to 1.0–1.7 ms, and its p95 at 100
events/s from 36–115 ms to 14–21 ms.

## 7. Three rule packs (ADR-0007 phase 6)

**Interpreter** (`go test -bench Outbound ./internal/rules/internal/interp/`, 2026-10-07, same
laptop): every rule an outbound transfer triggers in each pack, with typical facts and every
institution setting given a value, so whole conditions run. Windows are answered in memory; this
is the interpreter's share of the decision path only.

| Pack | Rules per event | Mean | p95 | Allocations |
|---|---|---|---|---|
| Nigeria (`ng`) | 34 | 85 µs | 177 µs | 322 (18.6 KB) |
| Ghana (`gh`) | 31 | 115 µs | 191 µs | 373 (18.6 KB) |
| Rwanda (`rw`) | 26 | 134 µs | 189 µs | 268 (15.3 KB) |

All three stay below 0.2 ms at p95: the packs add Redis round trips, not interpreter time.

**`days_over` on a busy wallet** (GH-TM-07, three tiers over 7 days; `windows_integration_test.go`):
an agent wallet with 20,000 debits in its window cost 75 to 91 ms of Redis script time per event
when every entry was read and parsed for each tier. Buckets now divide an hour or a day, so days
are totalled from whole buckets plus the two partial edges, once per window and day offset: 1.4
to 1.7 ms per event, the write included, and exact against a full scan on random streams.

**Decision path under load** (`ROUNDS=1 WARM_S=30 scripts/bench-packs.sh` against `make up`,
2026-10-07, same laptop; concentrated load as in §6, 100 events/s for 30 s after a 30 s warm-up,
one round per case). Each pack workspace has exchange rates, so US dollar thresholds run.

| Case | Achieved | Failed | Median | p90 | p95 | p99 | Redis script time per event |
|---|---|---|---|---|---|---|---|
| No pack | 100/s | 0 | 7.8 ms | 9.5 ms | **10.4 ms** | 20.5 ms | 0.42 ms |
| Nigeria (bank) | 100/s | 0 | 9.4 ms | 11.5 ms | **12.5 ms** | 17.5 ms | 0.94 ms |
| Ghana (mobile money issuer) | 100/s | 0 | 13.5 ms | 20.6 ms | **29.4 ms** | 362.9 ms | 1.54 ms |
| Rwanda (e-money issuer) | 100/s | 0 | 11.5 ms | 13.7 ms | **15.0 ms** | 23.8 ms | 1.05 ms |

Every pack stays far inside the budget at p95. Ghana costs the most (GH-TM-07's `days_over`
tiers and the agent rules), and its p99 in this single short round is an outlier the other
percentiles do not share: rerun with `ROUNDS=2 WARM_S=60` before treating it as a tail.

## Findings

1. **Latency is far inside budget.** At 2,000 req/s the p99 of the full network path is below
   10 ms, versus a 200 ms p95 target for risk evaluation.
2. **Telemetry cost is not measurable at this load.** Disabling OpenTelemetry did not reduce CPU or
   latency; differences are within run-to-run noise.
3. **Access logging costs about 14% of API CPU** at 2,000 req/s (135% → 121%), with a visibly lower
   p95. It stays on (it is the audit trail for requests); production log volume and sampling are
   decided in the observability design (TODO(phase-8)).
4. **Open question: unexplained CPU per request.** The API uses ≈0.6 ms of CPU per request under
   load, against ≈17 µs in-process. Candidates are kernel networking and Docker Desktop port
   publishing charged to the container, Go runtime scheduling across 8 threads, and log I/O through
   Docker's log driver. This will be profiled with pprof on Linux hosts before capacity planning
   (TODO(phase-8)). It does not threaten the latency target.
5. **Tail latency is noisy on this machine.** Max latencies of 60–150 ms appear at 2,000 req/s
   with no errors, consistent with laptop CPU contention (k6 shares the CPUs) and virtualization
   pauses. The VM clock also produced one negative minimum duration, which is a timer artifact.
   Compare medians and p95 across runs, not maxima.

## Reproducing

```bash
make up
make bench                        # micro-benchmarks → backend/bench.txt
make load                         # 2,000 req/s for 30 s (defaults)
RATE=5000 DURATION=60s make load  # custom rate and duration
# results: backend/test/load/results/health-summary.json
make load-events API_KEY=rsk_test_... RATE=500   # decision path; raise RISKOS_RATE_LIMIT_* first
make load-events API_KEY=rsk_test_... RATE=100 PACK=1 ACCOUNTS=20000   # with a rule pack (§6)
```

Re-baseline whenever the middleware chain, logging, server configuration or the decision path
changes, and record the new numbers here with the commit.
