# ADR-0004: Stay on OpenAPI 3.0.3 until code generation supports 3.1

- **Status:** accepted
- **Date:** 2026-09-30

## Context

The Phase 0 design ([ARCHITECTURE.md §1](../ARCHITECTURE.md#1-purpose-goals-and-stack)) names
OpenAPI 3.1 as the contract format. The contract is only useful if it generates the Go server
(`oapi-codegen`) and the TypeScript client (`openapi-typescript`), and CI fails when either drifts.
`openapi-typescript` handles 3.1, but `oapi-codegen` (v2.8) supports 3.0 fully and 3.1 only
partially, so a 3.1 document risks generation failures or silently incomplete types on the Go
side.

## Decision

Keep `backend/api/openapi.yaml` at **OpenAPI 3.0.3**. Everything else about the contract-first
principle stands: every operation is defined there first, both sides are generated from it, and
drift fails CI.

Constructs that differ between 3.0 and 3.1 are written the 3.0 way (`nullable: true` rather than
`type: [x, "null"]`; `example` rather than `examples` on schemas), which keeps a later upgrade
mechanical.

## Alternatives considered

- **Adopt 3.1 now:** would force hand-written Go types or post-processing for unsupported
  constructs, which undermines "never hand-write API types".
- **Switch Go generator:** alternatives with full 3.1 support are less mature for `net/http` +
  `chi` strict servers.

## Consequences

- Revisit when `oapi-codegen` declares full 3.1 support. The upgrade is a version bump in the
  document plus regeneration, verified by `make generate-check`.
- The public contract's behavior is unaffected; 3.0 versus 3.1 is a documentation-format detail
  for API consumers.
