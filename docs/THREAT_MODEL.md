# Threat model

**Purpose:** the assets, trust boundaries, threats and controls of RiskOS. Reviewed with every new
capability (PR security checklist) and in full in phase-7.

## Assets

TODO(phase-0): customer event and entity data, risk decisions, rules, API keys and webhook
secrets, audit logs, analyst accounts, internal admin access.

## Trust boundaries

TODO(phase-0): internet to API gateway; customer integrations (API keys); console users; RiskOS
internal admins; api to PostgreSQL and Redis; worker to customer webhook endpoints (egress).

## Threats and controls

The initial register is spec §18. Each row gains the implemented control and a test reference as
its phase lands.

| Threat | Impact | Planned controls (spec §18) | Implemented | Tests |
|---|---|---|---|---|
| Compromised API key | Unauthorized activity | Scopes, rotation, limits, monitoring, revocation | TODO(phase-2) | |
| Cross-tenant access | Customer data exposure | Tenant context, server-side authz, RLS, tests | TODO(phase-1) | |
| Replay attack | Duplicate or malicious processing | Idempotency, IDs, timestamps | TODO(phase-2) | |
| Credential stuffing | Account takeover | MFA, rate limits, anomaly detection | TODO(phase-1) | |
| Webhook spoofing | Fake events | Signatures, timestamps, replay protection | TODO(phase-5) | |
| SQL injection | Data compromise | Parameterized queries, validation, testing | Queries are sqlc-generated and parameterized | `backend/test/security` |
| SSRF | Internal network access | Validation, allowlists, egress controls | TODO(phase-5) | |
| Privilege escalation | Admin compromise | RBAC, least privilege, re-authentication, audit | TODO(phase-1) | |
| DDoS | Availability loss | WAF, rate limits, gateway controls | Server timeouts, body and header limits only | `backend/test/security` |
| Malicious rule change | Bad decisions | Versioning, approval, audit, rollback | TODO(phase-3) | |
| Secret leakage | Credential compromise | Secrets manager, scanning, masking | Log redaction, gitleaks | `platform/logging`, `backend/test/security` |
