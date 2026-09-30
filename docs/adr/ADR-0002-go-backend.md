# ADR-0002: Go for the backend (deviation from spec §4)

- **Status:** accepted
- **Date:** 2026-09-30

## Context

Spec §4 recommends Python 3.12+/FastAPI/SQLAlchemy/Alembic with Celery/RQ/Arq. RiskOS returns a
synchronous risk decision on every event, with a target of core evaluation p95 well under 200 ms
(spec §20), and must run cheaply and predictably under load as a multi-tenant service.

## Decision

The backend is written in **Go** (version pinned in `backend/go.mod`). Everything else in the
specification stands. The stack:

| Concern | Choice | Reason |
|---|---|---|
| HTTP | `net/http` + `chi` | stdlib compatibility (middleware, OpenTelemetry, testing); **not** Fiber/fasthttp |
| Contract | OpenAPI 3.0.3 + `oapi-codegen` (strict server) | contract-first; undocumented endpoints impossible ([ADR-0004](ADR-0004-openapi-3-0.md)) |
| Database | PostgreSQL 16, `pgx/v5` + `pgxpool` | fastest mature driver; native types |
| Queries | `sqlc` | SQL stays SQL; type-safe code generated; no ORM |
| Migrations | `goose` | plain SQL files that sqlc reads directly; embeddable; advisory locking |
| Cache/velocity | Redis 7, `go-redis/v9` | Lua scripts for atomic counters |
| Jobs | River behind `internal/jobs` | Postgres-backed, transactional enqueue ([ADR-0003](ADR-0003-river-job-queue.md)) |
| Logging | `log/slog` JSON + redaction | stdlib |
| Telemetry | OpenTelemetry (OTLP), injected providers | vendor-neutral |
| Tests | `testing` + testify, testcontainers-go | real dependencies in integration tests |

This supersedes spec §4 "Recommended stack: Backend", spec §19 Phase 1 "FastAPI" and the
spec §24 reference layout (`backend/app/…`, `migrations/`, `tests/`).

## Alternatives considered

- **Python/FastAPI (spec default):** fast to write, but per-request CPU cost and GIL-bound
  concurrency make tail latency harder to keep predictable, and deployment needs an interpreter
  and dependency tree.
- **Go with Fiber/fasthttp:** marginally faster in microbenchmarks, but incompatible with the
  `net/http` ecosystem (OpenTelemetry instrumentation, middleware, `httptest`, HTTP/2).
- **Rust:** excellent latency, but slower iteration for a small team and a thinner ecosystem for
  this stack.

## Consequences

- Single static binaries on distroless images; low memory; fast startup; predictable latency.
- **Two languages** (Go backend, TypeScript frontend), bridged by one OpenAPI contract: Go server
  types and the TypeScript client are generated from `backend/api/openapi.yaml`, and CI fails if
  either drifts. API changes start in the contract.
- Data-science-style risk work, if it needs Python later, runs as an asynchronous side process and
  never on the synchronous decision path.
