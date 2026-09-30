# Architecture

Status: **foundation in place; domain design pending (TODO(phase-0))**.
Product source of truth: [`docs/spec/`](spec/RiskOS_Complete_Implementation_Specification.md).
Decisions: [`docs/adr/`](adr/).

## 1. Shape of the system

RiskOS is a **modular monolith** ([ADR-0001](adr/ADR-0001-modular-monolith.md)) with a Go backend
([ADR-0002](adr/ADR-0002-go-backend.md)) and a Next.js console, joined by one OpenAPI contract.

```
           ┌──────────────────────────┐        ┌──────────────────────────┐
 browser ─▶│ frontend (Next.js)       │        │ customer platform        │
           │ typed client generated   │        │ (API keys, phase-2)      │
           │ from api/openapi.yaml    │        └────────────┬─────────────┘
           └────────────┬─────────────┘                     │
                        └───────────────┬───────────────────┘
                                        ▼
                         ┌──────────────────────────────┐
                         │ api (cmd/api)                │  one binary, many modules
                         │ middleware → router → modules│
                         └───────┬───────────────┬──────┘
                                 │               │
                        ┌────────▼───┐     ┌─────▼──────┐
                        │ PostgreSQL │     │   Redis    │
                        │  (+ jobs)  │     │ (velocity) │
                        └────────▲───┘     └────────────┘
                                 │
                         ┌───────┴──────────────────────┐
                         │ worker (cmd/worker)          │  async work off the request path
                         └──────────────────────────────┘
      api + worker ──OTLP──▶ OpenTelemetry Collector ──▶ (backends chosen in phase-8)
```

Binaries: `cmd/api` (HTTP), `cmd/worker` (background jobs) and `cmd/migrate` (schema). All three
share `internal/` and are built from one module, so a release is one consistent version.

## 2. Backend layout

```
backend/
├── api/openapi.yaml          contract: the source of truth for every HTTP operation
├── cmd/{api,worker,migrate}  composition roots: build dependencies, inject, run
├── db/migrations             goose SQL migrations (embedded in binaries)
├── db/queries/<module>       SQL for sqlc
├── internal/
│   ├── apigen/               generated from openapi.yaml (never edited)
│   ├── httpapi/              composes middleware + generated router + module handlers
│   ├── dbmigrate/            migration runner shared by cmd/migrate and tests
│   ├── jobs/                 background-job boundary (River, provisional)
│   ├── platform/             technical foundation, no business logic
│   │   ├── config  logging  requestctx  ids  clock
│   │   ├── httpx (errors, middleware)  httpserver  health  telemetry
│   │   └── db (pgxpool, sqlcgen)  redisx
│   └── <module>/             auth tenants events transactions users accounts devices
│                             risk rules alerts cases webhooks admin audit observability
└── test/{integration,security,e2e,load}
```

## 3. Module boundaries (enforced)

Each domain module has the same shape:

```
internal/<module>/
├── <module>.go (+ doc.go)    PUBLIC: service interface, DTOs, constructor. Nothing else.
└── internal/
    ├── handler/              HTTP: decode/validate, call the service, map to generated types
    ├── service/              business logic
    └── store/                the only code that touches this module's tables
```

Rules:

1. **Handlers call services; services call stores; only stores touch the database.**
2. **Modules talk to each other only through public (root) packages**, never through another
   module's service, store or models.
3. **Platform never depends on modules.** It is the foundation layer.
4. **Services are transport-agnostic.** No `net/http`, no generated API types in services/stores.
5. **The job queue implementation stays inside `internal/jobs`.**

Enforcement:

| Rule | Enforced by |
|---|---|
| No access to another module's `internal/` | Go compiler (`internal/` visibility) |
| Only stores use pgx, Redis, `platform/db`, `platform/redisx` | depguard `data-access-only-in-stores` |
| Platform does not import modules | depguard `platform-is-foundation` |
| Services/stores do not import HTTP or `apigen` | depguard `services-are-transport-agnostic` |
| River only in `internal/jobs` | depguard `queue-implementation-confined` |
| No OpenTelemetry/slog globals, env reads only in `cmd/` | forbidigo |
| No `init()`, no mutable package-level state | gochecknoinits, gochecknoglobals |

## 4. Request lifecycle

Middleware, outermost first (see `internal/httpapi/httpapi.go`):

1. **OpenTelemetry** (`otelhttp`): server span and request metrics, with injected providers.
2. **Request ID**: accept a safe inbound `X-Request-ID` (`[A-Za-z0-9._-]{1,64}`) or generate
   `req_<26 base32>`; echoed in the response, stored in context, logged and added to the span.
3. **Access log**: method, route pattern, path, status, bytes, duration. Never query strings,
   headers or bodies.
4. **Panic recovery**: 500 in the error contract; stack trace logged, never returned.
5. **Route telemetry**: span name and `http.route` from the matched pattern (bounded cardinality).
6. **Security headers**: `nosniff`, `no-store`, restrictive CSP, `no-referrer`.
7. **Timeout**: request context deadline (`RISKOS_HTTP_REQUEST_TIMEOUT`), which propagates to
   every downstream call.
8. **Body limit**: `http.MaxBytesReader` (`RISKOS_HTTP_MAX_BODY_BYTES`), 413 on excess.
9. **Router**: routes generated from the OpenAPI contract; unknown routes and methods get
   contract-shaped 404/405.

Server limits (`internal/platform/httpserver`): explicit `ReadHeaderTimeout`, `ReadTimeout`,
`WriteTimeout`, `IdleTimeout` and `MaxHeaderBytes`.

**Error contract** (spec §7) for every error, generated from the OpenAPI schema:

```json
{ "error": { "code": "NOT_FOUND", "message": "The requested resource does not exist.", "request_id": "req_…" } }
```

Unknown errors map to `500 INTERNAL_ERROR` with a generic message. Causes are logged server-side.

## 5. Health and shutdown

- `GET /health/live`: process can serve HTTP; never checks dependencies.
- `GET /health/ready`: PostgreSQL and Redis each answer within their check timeout, and the
  instance is not draining. The body reports per-dependency `ok`/`unavailable`, with no error details.
- On SIGTERM: readiness flips to 503, the server keeps serving for
  `RISKOS_HTTP_SHUTDOWN_DRAIN_DELAY`, then stops accepting connections and waits for in-flight
  requests up to `RISKOS_HTTP_SHUTDOWN_TIMEOUT`; pools and exporters are closed last.

## 6. Configuration

Environment variables only (`RISKOS_*`), loaded once in `cmd/*` by
`internal/platform/config`, validated at startup (all errors reported together, secrets never
echoed) and passed down explicitly. Production additionally requires verified Postgres TLS and a
Redis password.

## 7. Data and async work

- PostgreSQL 16 via `pgxpool`; queries in SQL compiled by `sqlc`; migrations with goose, applied
  by `cmd/migrate` under an advisory lock. Every connection uses UTC.
- Redis 7 via `go-redis/v9`, reserved for velocity counters (Lua for atomicity, phase-3).
- Background jobs through `internal/jobs` (River on PostgreSQL, provisional). Jobs are
  at-least-once and must be idempotent. The worker runs a no-op self-check job at startup.
  **Nothing slow or external runs on the synchronous risk path** (spec §20).

## 8. Frontend

Next.js App Router, strict TypeScript, Tailwind v4, shadcn/ui (Radix), TanStack Query. The API
client (`frontend/lib/api/client.ts`) is typed from `lib/api/schema.gen.ts`, generated from the
same OpenAPI file; CI fails on drift. Only `NEXT_PUBLIC_*` variables reach the browser (inlined at
build time). The browser→API topology and dashboard authentication are designed in phase-0.

## 9. Pending design (TODO(phase-0))

- Domain model and schema per module (spec §5), tenant and environment model ([TENANCY.md](TENANCY.md)).
- API contracts for spec §7 operations in `api/openapi.yaml`.
- Authentication: API-key format, hashing and scopes; dashboard sessions and MFA.
- Event schema and ingestion pipeline ([EVENTS.md](EVENTS.md)); risk engine contract ([RISK_ENGINE.md](RISK_ENGINE.md)).
- Final job-queue decision (ADR); transactional outbox for webhooks ([WEBHOOKS.md](WEBHOOKS.md)).
- Security model and threat model ([SECURITY.md](SECURITY.md), [THREAT_MODEL.md](THREAT_MODEL.md)).
