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
```

Re-baseline whenever the middleware chain, logging, server configuration or the decision path
changes, and record the new numbers here with the commit.
