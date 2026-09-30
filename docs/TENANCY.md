# Tenancy

**Purpose:** how tenant isolation is guaranteed across the API, services, storage, cache, jobs
and logs. Design: [ARCHITECTURE.md §8](ARCHITECTURE.md#8-tenancy-and-isolation). Status:
**built (build step 2)** for the API realm; tests in `backend/test/integration/tenancy_test.go`.

## Model

- **Tenant:** a customer organization, pinned at creation to one **regional cell** (its own
  Postgres, Redis, workers and keys). Its data never leaves that cell (§16.6).
- **Environment:** `test` or `production` under a tenant, with separate API keys, rules, data and
  webhooks. A test key can never read production rows.
- **Every table** that holds tenant data carries `tenant_id` and `environment_id`, and every index
  on it starts with `(tenant_id, environment_id, …)`.

## Invariant

The tenant and environment come **only** from the authenticated credential: the API key record
for machine calls, the session for dashboard users. A tenant ID in a URL, header or body is ignored
for authorization and rejected if it disagrees with the credential.

## How tenant context travels

1. Authentication middleware resolves the credential and places an immutable `TenantContext`
   (tenant, environment, actor, scopes) in the request `context.Context`. It is the only code
   that can create one.
2. Stores take the context and refuse to run without a `TenantContext`.
3. Job arguments and Redis keys carry the tenant and environment as a prefix. Identifiers used in
   Redis are keyed-hashed per tenant (§16.3).

## Three enforcement layers

| Layer | Mechanism | Proof |
|---|---|---|
| Application | Every sqlc query on a tenant table takes `tenant_id` and `environment_id` parameters, filled from the scope by the store; stores only run inside a scoped transaction | Code review (an automated SQL check is a TODO(phase-7) hardening item); RLS catches any miss |
| Database | Row-level security policies compare each row to `current_setting('app.tenant_id')` and `app.environment_id`, set with `SET LOCAL` inside each transaction. The application role has no `BYPASSRLS` and does not own the tables | Integration test: a query without the setting returns no rows |
| Test | A matrix calls every ID-taking operation with IDs of another tenant and of the other environment, and expects the same 404 as a nonexistent ID | `backend/test/integration/tenancy_test.go`, required in CI |

`SET LOCAL` is transaction-scoped, so it cannot leak through a pooled connection, including under
PgBouncer transaction pooling.

## Implementation

| Piece | Where |
|---|---|
| Scope type and context helpers | `backend/internal/platform/tenancy` |
| Scoped transaction (`SET LOCAL` semantics via `set_config(..., true)`) | `backend/internal/platform/db/tx.go` |
| RLS policies, `riskos_app` grants, `auth_lookup_api_key` | `backend/db/migrations/00002_tenancy_auth_audit.sql` |
| Application role login (local) | `infra/postgres/init/10-app-role.sh`; migrations use `RISKOS_POSTGRES_MIGRATION_URL` (owner) |
| Who may create a scope | forbidigo rule on `tenancy.WithScope`: the auth module (API keys and console sessions; a session's tenant comes from its stored session, its environment from the `RiskOS-Environment` header among that tenant's two), the tenants module (bootstrap, operator tooling and `JobScope` for background jobs, which re-checks that the tenant is active and grants only the permissions the job needs), `cmd/*`, tests |

Operator tooling (`riskosctl`) acts through the same application role and RLS as customers,
with its operator's name as the audit actor. Only audit-chain verification reads as the owner,
because it must see every environment's entries.

Existing local databases created before build step 2 lack the `riskos_app` login: run
`make clean && make up` once.

## Internal support access

RiskOS staff have no standing access. Support access is a time-boxed, four-eyes-approved grant
(`support_access_grant`), visible to the tenant, and every read under it is audited.

## Noisy neighbors

Rate limits and concurrency caps apply per tenant, per key and per endpoint; webhook delivery uses
per-tenant queues.
