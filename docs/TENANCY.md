# Tenancy

**Purpose:** how tenant isolation is guaranteed across the API, services, storage, cache, jobs
and logs.

## Invariant (spec §12)

The tenant is resolved **only** from the authenticated identity (API key or session), never from
a client-supplied tenant ID, and every data access is scoped to it.

## To be designed (TODO(phase-0))

- Tenant and environment model (test/production boundary, spec §5).
- How tenant context is carried: a typed context value set only by authentication middleware.
- Query scoping pattern for stores: every sqlc query takes the tenant ID as a parameter.
- PostgreSQL Row-Level Security as defense in depth: policies, session variables, pool behavior.
- Redis key namespacing and tenant scoping of job arguments.
- Cross-tenant and IDOR test suite (spec §16) in `backend/test/security`.
