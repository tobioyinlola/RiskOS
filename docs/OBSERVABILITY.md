# Observability

**Purpose:** logs, metrics and traces, and what we alert on.

## In place

- **Logs:** JSON via `log/slog` to stdout; one access-log line per request (method, route
  pattern, path, status, bytes, duration, `request_id`). Sensitive data is redacted
  (see [SECURITY.md](SECURITY.md)).
- **Traces:** an OpenTelemetry server span per request named `METHOD /route/pattern`, with
  `http.route` and `riskos.request_id`; W3C trace context is propagated.
- **Metrics:** OpenTelemetry HTTP server metrics (duration, request and response size) with the
  route attribute, and the application metrics below. Raw paths are never used as attributes,
  keeping cardinality bounded.
- **Export:** OTLP/HTTP to an OpenTelemetry Collector (`infra/otel/collector.yaml`, which prints a
  summary locally). Toggle with `RISKOS_OTEL_ENABLED`.
- **Health:** `/health/live` and `/health/ready`.

### Application metrics

Defined in `backend/internal/platform/appmetrics`. Every attribute takes values from a small
fixed set: never a tenant, key, user, address or route parameter. Names are as exported over
OTLP; a Prometheus-style backend renders dots as underscores and adds unit and `_total`
suffixes.

| Metric | Type | Attributes | Recorded by | Answers |
|---|---|---|---|---|
| `riskos.events.ingested` | counter | `result`: `accepted`, `replayed`, `rejected`, `error` | api | Ingestion success and failure. `rejected` is the caller's doing (validation, stale event, card data, idempotency conflict, permission); only `error` counts against us |
| `riskos.risk.evaluation.duration` | histogram, s | `decision` | api | Risk evaluation latency, for ingest and dry runs |
| `riskos.ratelimit.rejected` | counter | `limit`: `key`, `tenant`, `operation`, `auth_failure` | api | Rate-limit violations. A rejection by the combined per-key and per-tenant check is attributed to `key` |
| `riskos.ratelimit.errors` | counter | none | api | Checks that failed open because Redis was unavailable |
| `riskos.auth.failures` | counter | none | api | API requests answered 401 |
| `riskos.webhooks.deliveries` | counter | `result`: `succeeded`, `retrying`, `failed` | worker | Webhook success. `failed` means the schedule is exhausted or the endpoint was disabled |
| `riskos.webhooks.delivery.duration` | histogram, s | `result` | worker | Receiver latency, as seen by RiskOS |
| `riskos.jobs.duration` | histogram, s | `kind`, `outcome`: `succeeded`, `failed`, `snoozed` | worker | Worker latency and failure rate by job kind. A snooze is a webhook retry, not a failure |
| `riskos.jobs.queue.depth` | gauge | `queue`, `state`: `available`, `scheduled`, `retryable`, `running` | worker | Queue depth |

Notes on reading them:

- Every worker replica reports the same queue depth, since it is one table. Aggregate with `max`,
  not `sum`.
- `scheduled` and `retryable` jobs are waiting for a future time by design (webhook retries use
  them); alert on `available`.
- Jobs and evaluations that fail are not timed in `riskos.risk.evaluation.duration`: a rising
  `error` share of `riskos.events.ingested` is the signal for those.

## Service level objectives (proposed, to confirm)

Targets are proposals for the first production period. They are not commitments to customers
until reviewed against measured behaviour on production-like infrastructure
([PERFORMANCE_BASELINE.md](PERFORMANCE_BASELINE.md) was measured on one host).

| Objective | Indicator | Target |
|---|---|---|
| Decision latency (spec §20) | p95 of `riskos.risk.evaluation.duration` | under 200 ms, over any 5 minutes |
| Ingestion availability | `1 − error / (accepted + replayed + error)` of `riskos.events.ingested` | 99.9% over 30 days |
| API availability | share of non-5xx responses of `http.server.request.duration` | 99.9% over 30 days |
| Webhook delivery | share of attempts that end `failed` among those that end (`succeeded` or `failed`) | under 1% per day, excluding endpoints the tenant has disabled |
| Job freshness | `available` jobs in the default queue | under 1,000 for 10 minutes |

## Alerts

Severities are those of [INCIDENT_RESPONSE.md](INCIDENT_RESPONSE.md#severity-levels). Alert on
symptoms customers feel first; the rest are for diagnosis.

| Alert | Condition | Severity | Runbook |
|---|---|---|---|
| Ingestion errors | `error` share above 1% for 5 minutes | SEV-2 (SEV-1 above 10%) | [API degraded](INCIDENT_RESPONSE.md#api-degraded) |
| Decision latency | p95 above 200 ms for 10 minutes | SEV-2 | [API degraded](INCIDENT_RESPONSE.md#api-degraded) |
| API errors | 5xx share above 1% for 5 minutes | SEV-2 | [API degraded](INCIDENT_RESPONSE.md#api-degraded) |
| Readiness failing | `/health/ready` failing on any instance for 2 minutes | SEV-2 | [Database unavailable](INCIDENT_RESPONSE.md#database-unavailable), [Redis unavailable](INCIDENT_RESPONSE.md#redis-unavailable) |
| Limiter failing open | `riskos.ratelimit.errors` above 0 for 5 minutes | SEV-3 | [Redis unavailable](INCIDENT_RESPONSE.md#redis-unavailable) |
| Queue backlog | `available` depth above 1,000 for 10 minutes, or growing for 30 | SEV-2 | [Queue backlog](INCIDENT_RESPONSE.md#queue-backlog) |
| Job failures | `failed` outcomes above 5% of a kind's jobs for 15 minutes | SEV-3 | [Queue backlog](INCIDENT_RESPONSE.md#queue-backlog) |
| Webhook failures | `failed` share above 5% for 30 minutes | SEV-3 | [Webhook abuse or failure](INCIDENT_RESPONSE.md#webhook-abuse-or-failure) |
| Authentication failures | `riskos.auth.failures` above 10 times its 7-day median for 10 minutes | SEV-3; SEV-2 with `auth_failure` rejections | [Suspected key compromise](INCIDENT_RESPONSE.md#suspected-key-compromise) |
| Rate-limit pressure | `riskos.ratelimit.rejected` for one `limit` above 10 times its 7-day median for 15 minutes | SEV-3 | [API degraded](INCIDENT_RESPONSE.md#api-degraded) |
| Evaluation degraded | log `risk evaluation degraded` above 0 for 5 minutes | SEV-3 (a security event, ARCHITECTURE.md §16.8) | [API degraded](INCIDENT_RESPONSE.md#api-degraded) |

A per-tenant view (one tenant's rejections or errors) comes from logs, not metrics, so tenant
identifiers stay out of metric attributes.

## Backend requirements

The code speaks OTLP only, so the backend is chosen with the deployment platform
([DEPLOYMENT.md](DEPLOYMENT.md#platform-decision)). Whatever is chosen must provide:

- **Metrics:** at least 13 months of retention for the SLO indicators (the 30-day windows and
  year-on-year capacity comparisons), delta or cumulative temporality as the backend prefers
  (the SDK default is cumulative), and alert rules expressed as code in this repository.
- **Traces:** retention of 7 to 14 days; head sampling is `RISKOS_OTEL_SAMPLE_RATIO` (parent
  based), so keep 1.0 until volume says otherwise and prefer tail sampling in the Collector for
  errors and slow requests.
- **Logs:** from stdout, with `request_id` and the trace ID joinable. Application logs carry no
  secrets or card data by design; retention should still follow the audit and privacy policy
  once written, and no longer than needed. Read access is restricted to on-call and security
  roles and is itself logged. The audit log in PostgreSQL is a separate system of record and is
  not replaced by log retention.
- **Dashboards as code:** one dashboard per objective above, plus dependency health (database
  connections and latency, Redis latency, queue depth by queue).
- **Collector:** a Collector between services and the backend, so a backend outage never blocks
  a request: the SDK batches and drops, and decisions never wait on export.

## Not built yet

- Database, Redis and job-queue client instrumentation (pgx, go-redis, River spans and pool
  metrics). Until it exists, dependency health is read from the platform's own metrics and
  from `/health/ready`.
- Oldest-job age by queue, which is a better backlog signal than depth.
- A per-address limit and WAF metrics, which belong at the gateway.
- The backend, dashboards and alert rules themselves: they follow the platform decision.
  TODO(phase-8)
