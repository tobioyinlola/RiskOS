# Threat model

**Purpose:** the register of threats, their controls, how each control is proven, and what is
implemented today. The design rationale is [ARCHITECTURE.md §15–16](ARCHITECTURE.md#15-threat-model).
Every control is layered: none relies on a single mechanism.

Reviewed with every new capability (PR security checklist) and in full during build step 8
(security hardening). "Build step" refers to the build order in ARCHITECTURE.md §17.

## Assets

Customer event and entity data; risk decisions and evaluations; rules and rule versions; API keys,
the API-key pepper and webhook secrets; per-tenant data keys; the audit log and its anchors;
dashboard and internal admin accounts; backups.

## Trust boundaries

1. Internet → edge (WAF, gateway, TLS).
2. Customer systems → API realm (API keys).
3. Customer users → dashboard realm (sessions, MFA).
4. RiskOS staff → internal admin realm (SSO, hardware keys, private access path).
5. Application → data zone (PostgreSQL with RLS, Redis).
6. Workers → customer webhook endpoints (egress proxy, SSRF-safe dialer).
7. Production account → backup and log-archive accounts.

## Register

Status: **Designed** (specified, not built) · **Partial** (some controls in place) · **In place**.

| # | Threat | Controls | Proof (test) | Status | Where / when |
|---|---|---|---|---|---|
| 1 | Compromised API key | Scopes, optional per-key IP allowlist, per-key rate limits, scanner-friendly key prefix, anomaly alerts, fast revocation | Revoked key rejected within the staleness bound; new-ASN use raises an alert | Partial: scopes, rotation, revocation within 5 s (tested), scanner-friendly prefixes, per-key rate limits (tested); anomaly alerts pending | `auth`, `httpapi` rate limiting; alerts in step 8 |
| 2 | Cross-tenant access (IDOR) | Tenant from credential only, tenant-scoped queries, row-level security, uniform 404 | Cross-tenant matrix on every endpoint in CI | In place | `test/integration/tenancy_test.go` ([TENANCY.md](TENANCY.md)) |
| 3 | Replay or duplicate submission | Unique event key, timestamp skew limits, stored-response replay | Replay and concurrent-duplicate tests | In place: unique key per tenant and environment, stored-response replay, payload-hash conflict, 24 h / 5 min window (tested with 20 concurrent duplicates) | `events` ([EVENTS.md](EVENTS.md)) |
| 4 | Credential stuffing, dashboard takeover | Mandatory MFA (passkeys preferred), breached-password check, login rate limits, session and device tracking | Brute-force test throttled; impossible-travel alert | Designed | Build steps 2 and 7 |
| 5 | Webhook spoofing or replay | HMAC signature, timestamp tolerance, event ID, secret rotation | Bad signature and stale timestamp rejected | Designed | Build step 6 ([WEBHOOKS.md](WEBHOOKS.md)) |
| 6 | SQL injection | sqlc parameterized queries only, no string-built SQL, least-privilege DB role without `BYPASSRLS` | Injection payload suite; static scan | In place: sqlc-only queries, least-privilege `riskos_app` role, injection-shaped paths tested | `backend/test/security`, `backend/test/integration` |
| 7 | SSRF via webhook URLs | HTTPS only, resolved-IP dialer blocking private ranges, egress proxy, redirects disabled | Private-IP, redirect and DNS-rebinding tests | Designed | Build step 6 |
| 8 | Privilege escalation, insider abuse | Least privilege, four-eyes approvals, just-in-time access, sealed break-glass, full audit | Self-approval denied; break-glass fires an alert | Partial: deny-by-default RBAC, no scope escalation, internal roles without tenant access, full audit; four-eyes and break-glass pending | `platform/authz`; step 8 |
| 9 | DDoS, resource exhaustion | WAF, per-tenant limits, body and depth limits, bounded rule complexity, RE2-only matching | Load test with oversized payloads and pathological rules | Partial: 64 KB body limit, header limit, server and request timeouts, strict contract validation, per-key and per-tenant rate limits | `platform/httpx`, `platform/httpserver`, `httpapi`; gateway and rule bounds later |
| 10 | Malicious or mistaken rule change | Versioning, four-eyes promotion, shadow mode, one-step rollback, audit | Unapproved promotion blocked; rollback restores prior decisions | Designed | Build step 4 |
| 11 | Score probing and evasion | Per-key limits, sweep detection, no rule internals in responses | Sweep simulation raises a Security Center event | Designed | Build steps 4 and 8 |
| 12 | Attacker-induced degradation | REVIEW floor for high-value events when degraded; degradation is a security event | Kill Redis in test; high-value transactions still reach REVIEW | Designed | Build step 4 ([RISK_ENGINE.md](RISK_ENGINE.md)) |
| 13 | Baseline poisoning | Capped baseline update rate, flagged events excluded, drift alerts | Poisoning simulation stays within the cap | Designed | Build step 4 |
| 14 | Secret leakage | Secrets manager, log redaction, pre-commit and CI scanning, no secrets in images or logs | Canary secret never appears in logs | Partial: redaction, gitleaks in hooks and CI | `platform/logging`, `backend/test/security`; secrets manager at deployment |
| 15 | Supply-chain compromise | Pinned modules, signed images, provenance, SBOM, two-person review, vulnerability gates | Unsigned image refused at deploy | Partial: pinned modules/images/actions, govulncheck, Trivy, SBOM | CI; signing and CodeQL in build step 8 |
| 16 | Audit tampering | Hash chain, external anchor, audit module is the only writer | Verification job detects a modified row | Partial: append-only table, hash chain, `riskosctl audit verify` (tamper test in CI); external anchoring pending | `audit`; anchoring in step 8 |
| 17 | Bulk exfiltration via export or analytics | Export scopes, step-up authentication, rate limits, volume alerts, masked defaults | Large export requires re-auth and raises an alert | Designed | Build step 7 |
| 18 | Backup theft or ransomware | Encrypted, immutable backups in a separate account, restore drills | Restore test from an isolated account | Designed | Build step 8 ([DISASTER_RECOVERY.md](DISASTER_RECOVERY.md)) |
