# Security

**Purpose:** the security controls of RiskOS, their status, and how to report a vulnerability.
The design is [ARCHITECTURE.md §9 and §16](ARCHITECTURE.md#9-security-architecture); the threat
register is [THREAT_MODEL.md](THREAT_MODEL.md); tenant isolation is [TENANCY.md](TENANCY.md).
Nothing here claims regulatory compliance or certification.

## Reporting a vulnerability

Do not open a public issue. Use GitHub private vulnerability reporting on this repository.
A dedicated disclosure contact and response targets are defined before any production launch
(TODO(phase-7)).

## Controls in place (foundation)

| Area | Control | Where |
|---|---|---|
| Error handling | Unknown errors return a generic 500; causes and stack traces are logged, never returned | `platform/httpx` |
| Logging | Sensitive keys and inline `Bearer`/`Basic` credentials are redacted; query strings, headers and bodies are never logged | `platform/logging`, `platform/httpx` |
| Input limits | Body size limit, default 64 KB (413); header size limit (431); request timeout | `platform/httpx`, `platform/httpserver` |
| Slow clients | Explicit read-header, read, write and idle timeouts | `platform/httpserver` |
| Correlation IDs | Inbound request IDs restricted to `[A-Za-z0-9._-]{1,64}`, preventing log and header injection | `platform/ids` |
| Response headers | API: `nosniff`, `no-store`, restrictive CSP, `no-referrer`. Console: a nonce-based CSP per page (`strict-dynamic`, `frame-ancestors 'none'`), HSTS, `nosniff`, `X-Frame-Options: DENY` | API; `frontend/proxy.ts`, `frontend/next.config.ts` |
| Console sessions | HttpOnly, Secure, SameSite=Strict cookie through a same-origin proxy; CSRF token and Origin check on every change; 12 h absolute and 30 min idle timeouts; a recent second factor for key, webhook, rule, settings and team changes ([CONSOLE_AUTH.md](CONSOLE_AUTH.md)) | `internal/auth`, `frontend/app/api/v1` |
| Configuration | Fail-fast validation; secrets never echoed; production requires verified Postgres TLS and a Redis password | `platform/config` |
| Containers | Distroless, nonroot, read-only root filesystem, all capabilities dropped, `no-new-privileges` | Dockerfiles, `docker-compose.yml` |
| Local networking | PostgreSQL and Redis ports bound to 127.0.0.1 only; Redis requires a password | `docker-compose.yml` |
| Supply chain | Pinned toolchains, lockfiles, digest-pinned images, SHA-pinned actions, Dependabot, govulncheck, pnpm audit, Trivy, SBOMs | CI |
| Secrets in code | gitleaks in pre-commit and in CI over the full history | `lefthook.yml`, CI |
| API keys | `rsk_test_`/`rsk_live_` keys; only an HMAC with a server-side pepper is stored; shown once; scoped; rotation with overlap; revocation effective within 5 s; uniform 401 | `internal/auth` |
| Console sign-in | Argon2id passwords, mandatory passkey or TOTP second factor, recovery codes, throttling, HttpOnly SameSite=Strict session cookies with hashed tokens, CSRF tokens and Origin checks, re-authentication for high-impact actions | `internal/auth` ([CONSOLE_AUTH.md](CONSOLE_AUTH.md)) |
| Authorization | Deny-by-default permission catalog and roles; a key cannot grant or rotate scopes it lacks | `platform/authz`, `internal/auth` |
| Tenant isolation | Credential-derived scope, tenant-scoped queries, Postgres RLS under the non-owner `riskos_app` role, uniform 404, cross-tenant test matrix | [TENANCY.md](TENANCY.md) |
| Audit log | Append-only (grants and trigger), per-tenant hash chain written in the action's transaction, verification detects tampering | `internal/audit`, `riskosctl audit verify` |
| Strict input validation | Every request validated against the OpenAPI contract before any handler runs; unknown fields rejected | `internal/httpapi/contract.go` |
| Rate limiting | Per API key and per tenant (GCRA in Redis, atomic across both limits); fails open if Redis is down | `platform/ratelimit` |
| Data minimization | Card-number-like values are rejected anywhere in an event and never echoed; identifiers are opaque references; profile changes carry field names, not values | `internal/events` |
| Replay and idempotency | Unique event key per tenant and environment, stored-response replay, payload-hash conflicts, time window | `internal/events` |
| Pseudonymized features | Identifiers are HMAC-keyed per tenant and environment (`RISKOS_FEATURE_HASH_KEY`) before they reach Redis | `internal/risk` |
| Decision integrity | Degraded sources are listed, never guessed; fail-closed default for transactions; REVIEW floor for high-value events while degraded; high-risk events never update baselines; BLOCK never produced | `internal/risk` ([RISK_ENGINE.md](RISK_ENGINE.md)) |
| Alerts and cases | Raised off the decision path through a transactional outbox; jobs re-check the tenant and hold one system-only permission; RLS, insert-only notes and timelines, audit without free text | `internal/alerts`, `internal/cases` ([ALERTS_AND_CASES.md](ALERTS_AND_CASES.md)) |
| Webhooks | Signed (HMAC-SHA256, id + timestamp + body), secrets derived and never stored, rotation with overlap, SSRF-safe dialer on the resolved IP, no redirects, dedicated delivery queue, auto-disable of failing endpoints with a security alert | `internal/webhooks` ([WEBHOOKS.md](WEBHOOKS.md)) |
| Rules | Typed JSON conditions only (no code, no regex), depth, node, list and count bounds, immutable versions, rollback, shadow mode, every change audited | `internal/risk` |

## Designed, not yet built

"Build step" refers to the build order in [ARCHITECTURE.md §17](ARCHITECTURE.md#17-decisions-and-build-order).

| Area | Design | Build step |
|---|---|---|
| Authentication realms (remaining) | Breached-password check and SSO for the console; internal admin with SSO, hardware keys and a private access path | 8 |
| Audit anchoring | Chain head anchored to write-once storage in a separate account | 8 |
| Privileged access | Four-eyes approvals, just-in-time expiring access, sealed break-glass with alerts, no standing production access | 2 (approvals), 8 |
| Data minimization (remaining) | Field classification driving masking, encryption, logging and retention | 8 |
| Rate limiting (remaining) | Per IP and per endpoint at the gateway; concurrency caps; load shedding | 8 |
| Decision integrity (remaining) | Four-eyes approval of production rule changes; an operator-wide view of degradation across tenants; baseline drift alerts; auto-block with shadow evidence and a kill switch | 8 |
| Webhooks (remaining) | Egress proxy with an allow policy; per-tenant delivery queues; signing key in the secrets manager | 8 |
| Dashboard hardening (remaining) | SSO; step-up for exports once exports exist | 8 |
| Cryptography | Envelope encryption with per-tenant data keys (crypto-shredding), secrets manager/KMS, TLS 1.2+ everywhere | 8 |
| Supply chain | CodeQL or Semgrep, signed images, build provenance, deploys refuse unsigned images | 8 |
| Assurance | Independent penetration test before the first production customer; `security.txt`; remediation targets (critical 7 days, high 30, assumption) | 8 |
