# Observability

**Purpose:** logs, metrics and traces, and what we alert on.

## In place

- **Logs:** JSON via `log/slog` to stdout; one access-log line per request (method, route
  pattern, path, status, bytes, duration, `request_id`). Sensitive data is redacted
  (see [SECURITY.md](SECURITY.md)).
- **Traces:** an OpenTelemetry server span per request named `METHOD /route/pattern`, with
  `http.route` and `riskos.request_id`; W3C trace context is propagated.
- **Metrics:** OpenTelemetry HTTP server metrics (duration, request and response size) with the
  route attribute. Raw paths are never used as attributes, keeping cardinality bounded.
- **Export:** OTLP/HTTP to an OpenTelemetry Collector (`infra/otel/collector.yaml`, which prints a
  summary locally). Toggle with `RISKOS_OTEL_ENABLED`.
- **Health:** `/health/live` and `/health/ready`.

## To be designed (TODO(phase-8))

- Backends for traces, metrics and logs; dashboards; SLOs and alerts for spec §14: risk
  evaluation latency, ingestion success and failure, queue depth and worker latency, webhook
  success, authentication failures, rate-limit violations, error rates.
- Database, Redis and job instrumentation (pgx, go-redis, River).
- Log retention and access controls.
