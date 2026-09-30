# Security

**Purpose:** the security controls of RiskOS and how to report a vulnerability. The threat model
is in [THREAT_MODEL.md](THREAT_MODEL.md); tenant isolation in [TENANCY.md](TENANCY.md).

## Reporting a vulnerability

Do not open a public issue. Use GitHub private vulnerability reporting on this repository.
A dedicated disclosure contact and response targets are defined before any production launch
(TODO(phase-7)).

## Controls in place (foundation)

| Area | Control | Where |
|---|---|---|
| Error handling | Unknown errors return a generic 500; causes and stack traces are logged, never returned | `platform/httpx` |
| Logging | Sensitive keys and inline `Bearer`/`Basic` credentials are redacted; query strings, headers and bodies are never logged | `platform/logging`, `platform/httpx` |
| Input limits | Body size limit (413), header size limit (431), request timeout | `platform/httpx`, `platform/httpserver` |
| Slow clients | Explicit read-header, read, write and idle timeouts | `platform/httpserver` |
| Correlation IDs | Inbound request IDs restricted to `[A-Za-z0-9._-]{1,64}`, preventing log and header injection | `platform/ids` |
| Response headers | `nosniff`, `no-store`, restrictive CSP, `no-referrer`; the console adds `X-Frame-Options: DENY` | API; `frontend/next.config.ts` |
| Configuration | Fail-fast validation; secrets never echoed; production requires verified Postgres TLS and a Redis password | `platform/config` |
| Containers | Distroless, nonroot, read-only root filesystem, all capabilities dropped, `no-new-privileges` | Dockerfiles, `docker-compose.yml` |
| Local networking | PostgreSQL and Redis ports bound to 127.0.0.1 only; Redis requires a password | `docker-compose.yml` |
| Supply chain | Pinned toolchains, lockfiles, digest-pinned images, SHA-pinned actions, Dependabot, govulncheck, pnpm audit, Trivy, SBOMs | CI |
| Secrets in code | gitleaks in pre-commit and in CI over the full history | `lefthook.yml`, CI |

## To be designed (TODO(phase-0), hardened in phase-7)

- Identity and access: API keys (hashed at rest, scoped, rotation, revocation), console sessions,
  MFA for privileged users, re-authentication for high-impact actions, RBAC (spec §12).
- Rate limiting per tenant, key, IP and endpoint.
- Replay protection and idempotency (spec §12).
- Webhook signing ([WEBHOOKS.md](WEBHOOKS.md)).
- Data classification, encryption at rest, masking and retention.
- Production secrets management, WAF/API gateway, penetration testing.
