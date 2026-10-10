# ADR-0001: Modular monolith

- **Status:** accepted
- **Date:** 2026-09-30

## Context

The specification (§4, "Architecture principle") asks for a modular monolith with strict module
boundaries, extracting high-volume components only when scale or operations justify it. The team
is small, the domain is still being designed, and the core decision path must be fast and easy to
reason about. Distributed systems add network hops to the synchronous risk path, partial-failure
modes and operational cost before any of their benefits are needed.

## Decision

- One Go module and repository for the backend, producing three binaries from shared code:
  `api`, `worker`, `migrate`.
- Domain capabilities are **modules** under `backend/internal/<module>/`. A module exposes only a
  public root package (service interface, DTOs, constructor); its handlers, services and stores
  live in `internal/<module>/internal/…`, which the Go compiler makes unreachable from other modules.
- Dependency rules the compiler cannot express are enforced by depguard in CI
  (see [ARCHITECTURE.md §4](../ARCHITECTURE.md#4-module-map-and-layering)).
- A module owns its tables. Other modules read its data only through its public interface.

## Alternatives considered

- **Microservices from day one:** network latency on the risk path, distributed transactions
  between events/risk/alerts, and much higher operational load, with no current scaling need.
- **Unstructured monolith (layered by technical role):** fast to start, but boundaries erode,
  and later extraction becomes a rewrite.

## Consequences

- Cross-module calls are in-process function calls: fast, and transactional where needed.
- Extraction stays possible: a module's public interface is the future service API, and its
  tables are already private to it.
- Discipline is automated (compiler + lint), not left to review.
- All modules share one deploy cadence and one database until a module is extracted.
