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
| Response headers | `nosniff`, `no-store`, restrictive CSP, `no-referrer`; the console adds `X-Frame-Options: DENY` | API; `frontend/next.config.ts` |
| Configuration | Fail-fast validation; secrets never echoed; production requires verified Postgres TLS and a Redis password | `platform/config` |
| Containers | Distroless, nonroot, read-only root filesystem, all capabilities dropped, `no-new-privileges` | Dockerfiles, `docker-compose.yml` |
| Local networking | PostgreSQL and Redis ports bound to 127.0.0.1 only; Redis requires a password | `docker-compose.yml` |
| Supply chain | Pinned toolchains, lockfiles, digest-pinned images, SHA-pinned actions, Dependabot, govulncheck, pnpm audit, Trivy, SBOMs | CI |
| Secrets in code | gitleaks in pre-commit and in CI over the full history | `lefthook.yml`, CI |

## Designed, not yet built

"Build step" refers to the build order in [ARCHITECTURE.md §17](ARCHITECTURE.md#17-decisions-and-build-order).

| Area | Design | Build step |
|---|---|---|
| Authentication realms | Three separate realms: API keys; dashboard sessions with MFA (passkeys preferred); internal admin with SSO, hardware keys and a private access path | 2 (API, dashboard), 8 (admin hardening) |
| API keys | Prefix plus high-entropy secret (`rsk_live_`/`rsk_test_`); only an HMAC with a server-side pepper is stored; constant-time compare; shown once; rotation overlap; revocation within 5 s (assumption); least-privilege scopes | 2 |
| Authorization | RBAC with the spec's roles; customer and internal roles are separate sets; deny by default | 2 |
| Tenant isolation | Credential-derived tenant, tenant-scoped queries, Postgres RLS, cross-tenant test matrix, uniform 404 | 2 |
| Audit log | Append-only, hash-chained, anchored to write-once storage in a separate account | 2 (chain), 8 (anchoring) |
| Privileged access | Four-eyes approvals, just-in-time expiring access, sealed break-glass with alerts, no standing production access | 2 (approvals), 8 |
| Replay and idempotency | Unique event key, skew limits, stored-response replay | 3 |
| Data minimization | Card numbers, CVV, PINs and passwords rejected; identifiers keyed-hashed before Redis; field classification drives masking, encryption, logging and retention | 3 |
| Rate limiting | Redis token buckets per tenant, key, IP and endpoint; concurrency caps; load shedding with 429 | 2–3 |
| Decision integrity | REVIEW floor for high-value events when degraded; capped, poison-resistant baselines; shadow mode for rules | 4 |
| Webhooks | HMAC-SHA256 signing, timestamp tolerance, SSRF-safe dialer and egress proxy ([WEBHOOKS.md](WEBHOOKS.md)) | 6 |
| Dashboard hardening | HttpOnly/Secure/SameSite=Strict cookies, CSRF tokens, nonce-based CSP, HSTS, session timeouts, SSO, step-up for exports, key creation and rule promotion | 7 |
| Cryptography | Envelope encryption with per-tenant data keys (crypto-shredding), secrets manager/KMS, TLS 1.2+ everywhere | 8 |
| Supply chain | CodeQL or Semgrep, signed images, build provenance, deploys refuse unsigned images | 8 |
| Assurance | Independent penetration test before the first production customer; `security.txt`; remediation targets (critical 7 days, high 30, assumption) | 8 |
