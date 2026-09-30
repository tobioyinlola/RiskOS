**RISKOS**

**Complete Product, System Architecture, Frontend, Backend, Security & Technical Implementation Specification**

Master specification for Claude Code / Codex / Lovable implementation.

> Note (repository): the backend language recommendation in §4 ("Python 3.12+, FastAPI …") has been
> superseded by a deliberate decision to use Go. See `docs/adr/ADR-0002-go-backend.md`. The rest of
> this specification stands as the product source of truth.

# 1. Product Overview

RiskOS is an API-first financial risk intelligence infrastructure layer for banks, fintechs, payment companies, wallets, lending platforms and other financial technology businesses.

RiskOS integrates with existing customer systems, receives financial and behavioral events, normalizes and enriches them, evaluates risk signals, produces an explainable risk score, returns a decision, and provides alerts and investigation tooling.

- Positioning: real-time financial risk intelligence infrastructure.

- Core loop: INGEST → UNDERSTAND → ENRICH → ANALYZE → SCORE → DECIDE → ALERT → INVESTIGATE.

- Decisions: ALLOW / REVIEW / STEP-UP / BLOCK.

- Initial score range: 0–100.

- Risk levels: LOW / MEDIUM / HIGH / CRITICAL.

- The system must be explainable and auditable.

- AI should initially assist investigations, not autonomously make high-impact financial decisions.

# 2. Product Goals and MVP

- Customer creates an organization and test/production environment.

- Customer creates a scoped API key.

- Customer securely sends an event.

- RiskOS authenticates, validates, normalizes and stores the event.

- RiskOS enriches the event with available user, account, device, IP, beneficiary and historical context.

- RiskOS evaluates rules, velocity and behavioral/context signals.

- RiskOS returns score, level, signals, engine version and decision.

- High-risk activity creates an alert.

- Analyst investigates the alert and can create a case.

- Analyst can assign, note, escalate and resolve a case.

- All material administrative and security actions are audited.

# 3. Users and Roles

| User                    | Primary needs                        | Main surfaces                                                    |
|-------------------------|--------------------------------------|------------------------------------------------------------------|
| Risk/Fraud Analyst      | Monitor, investigate, resolve        | Dashboard, Risk Monitor, Alerts, Cases, Users, Accounts, Devices |
| Risk Manager            | Configure strategy, review trends    | Analytics, Rules, Risk Configuration                             |
| Developer               | Integrate, debug, manage credentials | Developer Center, API Keys, Events, Webhooks, Docs               |
| Customer Admin          | Manage team and configuration        | Team, Settings, Environments, Audit Logs                         |
| RiskOS Operations Admin | Manage tenants and operations        | Admin Console, System Health                                     |
| Security Admin          | Manage privileged security controls  | Security Center, Audit Logs, Sessions, API Keys                  |

# 4. System Architecture

```
INTERNET
|
+-------v-------+
| WAF / API GW |
| DDoS / Limits |
+-------+-------+
|
+-------v-------+
| Auth Layer |
| API Keys/MFA |
+-------+-------+
|
+-------v-------+
| Tenant/RBAC |
| Authorization |
+-------+-------+
|
+-------------------+-------------------+
| |
v v
+---------------+ +---------------+
| Event Ingest | | Admin/Control |
| Validate | | Tenants |
| Normalize | | Users/Roles |
| Idempotency | | Security |
+-------+-------+ | Audit |
| +---------------+
v
+---------------+
| Enrichment |
| User/Account |
| Device/IP/Geo |
| History |
+-------+-------+
|
v
+---------------+
| Risk Engine |
| Rules |
| Velocity |
| Behaviour |
| Device/Geo |
+-------+-------+
|
v
+---------------+
| Score/Decision|
| Allow |
| Review |
| Step-up |
| Block |
+-------+-------+
|
+---+---+
| |
v v
Alerts Webhooks
|
v
Cases / Investigation

Shared infrastructure:
PostgreSQL | Redis | Queue/Workers | Secrets Manager
Observability | Backups | CI/CD | Security Monitoring
```

## Architecture principle

Start as a modular monolith with strict module boundaries. Extract high-volume components later only when scale or operational requirements justify it.

## Recommended stack

- Backend: Python 3.12+, FastAPI, Pydantic v2, SQLAlchemy 2.x, Alembic.

- Database: PostgreSQL.

- Cache/velocity: Redis.

- Async jobs: Celery, RQ, Arq or equivalent.

- Frontend: Next.js/React + TypeScript + Tailwind + shadcn/ui + TanStack Query + Recharts.

- Infrastructure: Docker, private networking, WAF/API gateway.

- Observability: OpenTelemetry, structured logs and metrics.

# 5. Core Domain Model

| Entity      | Purpose                     | Important relationships                                   |
|-------------|-----------------------------|-----------------------------------------------------------|
| Tenant      | Customer organization       | Owns environments, users, events, rules, alerts and cases |
| Environment | Test/production boundary    | Owns API keys and configuration                           |
| User        | End customer identity       | Accounts, devices, sessions, transactions                 |
| Account     | Financial account/wallet    | User, transactions, beneficiaries, devices                |
| Device      | Device identity             | Users, accounts, sessions, IPs                            |
| Session     | Authentication activity     | User, device, IP, events                                  |
| Transaction | Financial activity          | User, account, beneficiary, risk                          |
| Event       | Normalized activity         | Tenant, environment, entities, risk                       |
| RiskScore   | Evaluation result           | Event/transaction, signals, decision                      |
| RiskSignal  | Reason contributing to risk | Risk score, evidence                                      |
| Rule        | Risk logic                  | Tenant/environment, versions                              |
| Alert       | Human attention item        | Risk, event, entities, case                               |
| Case        | Investigation container     | Alerts, entities, notes, resolution                       |
| APIKey      | Integration credential      | Tenant/environment, scopes                                |
| Webhook     | Outbound notification       | Tenant/environment, endpoint, deliveries                  |
| AuditLog    | Governance trail            | Actor, resource, action, result                           |

# 6. Event and Risk Lifecycle

```
Customer platform
|
v
POST /api/v1/events
|
Authentication
|
Tenant + Environment Resolution
|
Schema Validation
|
Idempotency / Replay Check
|
Normalization
|
Context Enrichment
|
Signal Generation
| | |
Rules Velocity Behaviour
| | |
+-------+-------+
|
Risk Engine
|
Score + Level
|
Decision
/ | \
Allow Review Step-up/Block
|
Alert if needed
|
Case
|
Resolve
|
Audit
```

# 7. Backend API

```
POST /api/v1/events
POST /api/v1/risk/evaluate

GET /api/v1/events
GET /api/v1/events/{event_id}

GET /api/v1/transactions
GET /api/v1/transactions/{transaction_id}

GET /api/v1/users
GET /api/v1/users/{user_id}

GET /api/v1/accounts
GET /api/v1/accounts/{account_id}

GET /api/v1/devices
GET /api/v1/devices/{device_id}

GET /api/v1/alerts
GET /api/v1/alerts/{alert_id}
PATCH /api/v1/alerts/{alert_id}

GET /api/v1/cases
POST /api/v1/cases
GET /api/v1/cases/{case_id}
PATCH /api/v1/cases/{case_id}
POST /api/v1/cases/{case_id}/notes

GET /api/v1/rules
POST /api/v1/rules
PATCH /api/v1/rules/{rule_id}
POST /api/v1/rules/{rule_id}/test

GET /api/v1/api-keys
POST /api/v1/api-keys
POST /api/v1/api-keys/{key_id}/rotate
POST /api/v1/api-keys/{key_id}/revoke

GET /api/v1/webhooks
POST /api/v1/webhooks
PATCH /api/v1/webhooks/{webhook_id}

GET /api/v1/audit-logs
GET /api/v1/analytics/*
```

## Risk response contract

```
{
"request_id": "req_82J91",
"event_id": "evt_82191",
"risk_score": 87,
"risk_level": "critical",
"decision": "review",
"engine_version": "risk-engine-v1.4",
"signals": [
{"code":"new_device","label":"New device","weight":20},
{"code":"new_beneficiary","label":"New beneficiary","weight":15},
{"code":"amount_anomaly","label":"Unusual amount","weight":24}
]
}
```

## Error contract

```
{
"error": {
"code": "INVALID_EVENT",
"message": "The event payload is invalid.",
"request_id": "req_82J91"
}
}
```

# 8. Risk Engine

- Evaluate active rules.

- Calculate velocity signals using atomic Redis operations.

- Generate behavioral anomaly signals.

- Generate device, network and geo signals where data exists.

- Combine signals into an explainable score.

- Map score to risk level.

- Map risk context to decision.

- Persist evaluation and engine version.

- Generate alerts when configured.

| Score  | Level    | Meaning                                         |
|--------|----------|-------------------------------------------------|
| 0–29   | Low      | Appears consistent with configured expectations |
| 30–59  | Medium   | Some risk signals require awareness             |
| 60–79  | High     | Material risk signals                           |
| 80–100 | Critical | Strong combination of signals                   |

**These bands are initial product defaults, not universal fraud standards.**

```
Example:
New device +20
New beneficiary +15
Unusual amount +24
Velocity anomaly +15
Geo anomaly +10
-------------------------
Score 84
Level CRITICAL
Decision REVIEW
```

# 9. Rules Engine

- Structured conditions only; never arbitrary eval/exec or customer-supplied executable code.

- Typed fields and operators.

- AND/OR condition groups.

- Priority and severity.

- Enable/disable.

- Versioning.

- Testing against synthetic/historical events.

- Audit trail and rollback.

- Optional approval workflow for production changes.

```
IF transaction.amount > 1000000
AND beneficiary.is_new = true
THEN
signal = high_value_new_beneficiary
weight = 25
severity = high
```

# 10. Frontend Product

## Navigation

- Overview

- Risk Monitor

- Transactions

- Events

- Users

- Accounts

- Devices

- Alerts

- Cases

- Rules

- Analytics

- Developer

- Team

- Audit Logs

- Settings

## Dashboard

- Total events, risk evaluations, transactions monitored, high-risk activity.

- Open alerts, open cases, blocked activity, average risk score.

- Activity volume chart, risk distribution, decision distribution, alert trend, top signals.

- Recent risk activity table.

## Risk Monitor

- Live event stream.

- Events/min, risk evaluations/min, high-risk and critical events, processing latency.

- Filters for risk level, decision, event type, user, account, device, location and time.

- Click event → detail drawer.

## Detail screens

- Event: raw payload, normalized payload, processing timeline, risk and related activity.

- Transaction: amount, currency, sender, recipient, beneficiary, risk and history.

- User: accounts, devices, transactions, logins, alerts, cases and risk history.

- Account: transactions, beneficiaries, devices, login history and risk history.

- Device: associated users/accounts/IPs/countries/sessions and unusual relationships.

## Alerts and Cases

- Alert queue with severity, user, account, transaction, score, analyst and status.

- Alert detail with signals, evidence, related entities and timeline.

- Actions: assign, investigate, create case, escalate, false positive, resolve.

- Case workspace: summary, alerts, transactions, users, devices, events, evidence, notes, assignment and resolution.

## Developer Center

- API keys, event explorer, webhooks and API docs.

- JavaScript, Python and cURL examples.

- Show raw API secret only once.

- Event delivery and request debugging.

# 11. Internal Admin Console

- Admin Overview.

- Organizations/Tenants.

- Internal Users & Roles.

- API Keys.

- Environments.

- Integrations.

- Security Center.

- Audit Logs.

- System Health.

- Risk Configuration.

- Rules & Versions.

- Webhooks.

- Usage & Limits.

- Data Retention.

- Incident Management.

- Settings.

## Tenant management

- Create, suspend and disable tenants.

- View tenant environments, API volume, events, risk evaluations, alerts and cases.

- Configure limits and retention.

- Controlled support access must be time-limited and fully audited.

# 12. Security Architecture

## Identity and access

- MFA mandatory for privileged internal users.

- Prefer passkeys/security keys where practical.

- Session and device tracking.

- Session revocation.

- Re-authentication for high-impact actions.

- RBAC and least privilege.

- Separate customer roles from RiskOS internal admin roles.

## Tenant isolation

- Resolve tenant from authenticated identity/key, never from an untrusted client tenant ID.

- Every query must enforce tenant ownership.

- Test IDOR and cross-tenant access.

- Consider PostgreSQL Row-Level Security as defense in depth.

## API security

- TLS everywhere.

- Hash API secrets at rest.

- Scoped keys.

- Rotation and revocation.

- Per-tenant/per-key/IP/endpoint rate limits.

- Request size limits.

- Strict schema validation.

- Idempotency and request IDs.

## Replay protection

- Unique event IDs/idempotency keys.

- Processed-ID tracking.

- Timestamp validation.

- Reject or ignore stale/replayed requests according to endpoint policy.

## Webhook security

- HMAC or equivalent signing.

- Timestamp and event ID.

- Replay protection.

- Retries with exponential backoff.

- Delivery logs.

- Never expose webhook secrets after creation.

## Data security

- Encryption in transit and at rest.

- Production secrets manager.

- No secrets in source control.

- Mask sensitive data in UI/logs.

- Data classification and retention.

- Encrypted backups and restricted exports.

## Infrastructure security

- WAF/API gateway.

- DDoS protection.

- Private database and Redis networking.

- Restricted firewall/security groups.

- Separate test and production.

- Controlled production access.

## Application attack protection

- SQL injection protection.

- XSS protection.

- CSRF protection where applicable.

- SSRF protection for customer-supplied URLs.

- Path traversal protection.

- Request smuggling defenses.

- Payload/file size restrictions.

# 13. Security Center

```
SECURITY OVERVIEW

Active incidents 3
Failed admin logins 18
Suspicious API keys 2
Rate-limit violations 41
Blocked requests 382
Critical events 4
```

- Failed privileged login.

- Suspicious API key activity.

- Unusual request volume.

- Repeated invalid requests.

- Cross-tenant authorization failure.

- Unexpected privilege change.

- New internal admin account.

- Production configuration change.

- Webhook authentication failure.

- Repeated rate-limit violations.

# 14. Observability and System Health

- API request rate and p50/p95/p99 latency.

- Risk evaluation latency.

- Event ingestion success/failure.

- Queue depth and worker latency.

- Database and Redis health.

- Webhook success/failure.

- Authentication failures.

- Rate-limit violations.

- Error rates.

- OpenTelemetry traces with correlation/request IDs.

# 15. Reliability and Disaster Recovery

- Encrypted automated backups.

- Point-in-time recovery.

- Documented RTO/RPO.

- Regular restore testing.

- Queue retries and dead-letter handling.

- Graceful degradation when optional providers fail.

- Health/readiness/liveness endpoints.

- Incident response runbook.

# 16. Testing and Security Validation

| Test                      | Expected result             |
|---------------------------|-----------------------------|
| Invalid API key           | Rejected                    |
| Revoked API key           | Rejected                    |
| Expired credential        | Rejected                    |
| Cross-tenant resource ID  | Denied                      |
| Insufficient role         | Denied                      |
| Malformed payload         | Validation error            |
| Oversized payload         | Rejected                    |
| Replay event              | Rejected/ignored per policy |
| SQL injection             | No database manipulation    |
| Invalid webhook signature | Rejected                    |
| Brute-force login         | Rate limited/blocked        |
| Secrets in logs           | Must not appear             |

## Test layers

- Unit tests for rules, scoring and utilities.

- Integration tests for API/database/Redis.

- Security tests for authentication, authorization, tenant isolation and replay.

- End-to-end test: tenant → key → event → risk → alert → case → resolution.

- Frontend component and end-to-end tests.

- Load/performance testing before production scale.

# 17. Technical Documentation

- README.md — setup and local development.

- ARCHITECTURE.md — modules, boundaries and diagrams.

- API.md/OpenAPI — public contracts.

- SECURITY.md — security controls and reporting.

- THREAT_MODEL.md — assets, threats and controls.

- TENANCY.md — isolation strategy.

- RISK_ENGINE.md — scoring, signals and decisions.

- EVENTS.md — event schema and ingestion.

- WEBHOOKS.md — signing, retries and delivery.

- DEPLOYMENT.md — environments and deployment.

- OBSERVABILITY.md — logs, metrics and tracing.

- INCIDENT_RESPONSE.md — operational/security response.

- DISASTER_RECOVERY.md — backup and restore.

- CONTRIBUTING.md — development workflow.

# 18. Threat Model

| Threat                | Impact                         | Controls                                         |
|-----------------------|--------------------------------|--------------------------------------------------|
| Compromised API key   | Unauthorized activity          | Scopes, rotation, limits, monitoring, revocation |
| Cross-tenant access   | Customer data exposure         | Tenant context, server-side authz, RLS, tests    |
| Replay attack         | Duplicate/malicious processing | Idempotency, IDs, timestamps                     |
| Credential stuffing   | Account takeover               | MFA, rate limits, anomaly detection              |
| Webhook spoofing      | Fake events                    | Signatures, timestamps, replay protection        |
| SQL injection         | Data compromise                | Parameterized queries, validation, testing       |
| SSRF                  | Internal network access        | Validation, allowlists, egress controls          |
| Privilege escalation  | Admin compromise               | RBAC, least privilege, re-authentication, audit  |
| DDoS                  | Availability loss              | WAF, rate limits, gateway controls               |
| Malicious rule change | Bad decisions                  | Versioning, approval, audit, rollback            |
| Secret leakage        | Credential compromise          | Secrets manager, scanning, masking               |

# 19. Implementation Phases

## Phase 0 — System Design

- Domain model

- API contracts

- Tenant model

- Event schema

- Risk engine contract

- Security model

- Architecture diagrams

## Phase 1 — Foundation

- Repository

- Docker

- PostgreSQL

- Redis

- FastAPI

- Config

- Logging

- Health endpoints

- Auth

- Tenant/RBAC

## Phase 2 — Event Infrastructure

- API keys

- Event ingestion

- Validation

- Normalization

- Persistence

- Idempotency

- Tracing

## Phase 3 — Risk Engine

- Rules

- Velocity

- Behavioral signals

- Scoring

- Risk levels

- Decisions

- Explainability

## Phase 4 — Alerts and Cases

- Alert generation

- Queue

- Assignment

- Cases

- Notes

- Resolution

- Audit

## Phase 5 — Webhooks and Developer Platform

- Signing

- Retries

- Delivery logs

- Docs

- Event explorer

- Key management

## Phase 6 — Frontend

- Design system

- Shell

- Dashboard

- Risk Monitor

- Events

- Transactions

- Users/Accounts/Devices

- Alerts/Cases

- Rules

- Analytics

- Developer/Admin

## Phase 7 — Security Hardening

- Threat model review

- Tenant-isolation testing

- Security scanning

- Rate limits

- WAF

- Secrets management

- Audit validation

- Penetration testing

## Phase 8 — Production Readiness

- Load testing

- Failure testing

- Backup restore

- Incident exercise

- Observability

- Deployment controls

- Documentation review

# 20. Performance Targets

| Area                 | Initial target                               |
|----------------------|----------------------------------------------|
| Core risk evaluation | p95 \< 200 ms where practical                |
| Event ingestion      | Horizontally scalable                        |
| Database             | Tenant-scoped indexed queries                |
| Velocity             | Fast atomic Redis counters                   |
| Webhooks             | Asynchronous; never block core risk response |
| Frontend             | Fast initial render; lazy-load heavy modules |

Keep the synchronous decision path small. Move notifications, analytics, optional enrichment and AI workloads to asynchronous processing.

# 21. Future AI Layer

- Case summarization.

- Investigation assistant.

- Permission-aware natural-language search.

- Related-event discovery.

- Natural-language risk explanations.

- Investigation report generation.

- Analyst suggestions.

- Anomaly explanation.

AI must not silently override deterministic controls or autonomously execute high-impact financial decisions in the initial architecture.

# 22. MVP Acceptance Criteria

1.  Tenant can authenticate.

2.  Tenant can create test/production environments.

3.  Tenant can create/revoke scoped API keys.

4.  Authenticated client can send an event.

5.  Event is validated, normalized and stored.

6.  Replay/duplicate processing is controlled.

7.  Rules/signals are evaluated.

8.  Risk score, level, signals and decision are returned.

9.  High-risk activity can create an alert.

10. Analyst can inspect evidence.

11. Analyst can create, assign, note and resolve a case.

12. Material actions are audited.

13. Cross-tenant access is denied.

14. Revoked credentials stop working.

15. Webhook signatures are verifiable.

16. Health and operational metrics exist.

17. Frontend works on desktop, tablet and mobile.

18. Core demo journey works without a real customer integration.

# 23. Master Claude Code Instruction

```
You are the lead staff engineer and product architect for RiskOS.

Treat this document as the product and engineering source of truth.

Before coding:
1. Inspect the repository and preserve useful existing code.
2. Identify the current stack.
3. Create/update ARCHITECTURE.md.
4. Define modules, domain models and boundaries.
5. Define API contracts and schemas.
6. Define security boundaries and threat model.
7. Create an implementation plan with dependencies.

Implement incrementally.

For every phase:
1. Explain the goal.
2. Implement a production-quality slice.
3. Add tests.
4. Run tests and type/lint checks.
5. Review security implications.
6. Update documentation.
7. Summarize changed files and remaining work.

Do not:
- build a static fake prototype;
- expose secrets;
- bypass authorization;
- trust client-supplied tenant IDs;
- use arbitrary eval/exec for rules;
- put long-running work on the synchronous risk path;
- invent compliance claims;
- create undocumented APIs.

First prove this end-to-end flow:

Tenant
→ API Key
→ Event
→ Validation
→ Normalization
→ Risk Evaluation
→ Score
→ Decision
→ Alert
→ Case
→ Resolution
→ Audit Log

Make this flow secure, observable, explainable and fully testable before expanding the platform.
```

# 24. Reference Repository Structure

```
riskos/
├── backend/
│ ├── app/
│ │ ├── api/
│ │ ├── auth/
│ │ ├── tenants/
│ │ ├── events/
│ │ ├── transactions/
│ │ ├── users/
│ │ ├── accounts/
│ │ ├── devices/
│ │ ├── risk/
│ │ ├── rules/
│ │ ├── alerts/
│ │ ├── cases/
│ │ ├── webhooks/
│ │ ├── admin/
│ │ ├── audit/
│ │ └── observability/
│ ├── migrations/
│ └── tests/
├── frontend/
│ ├── app/
│ ├── components/
│ ├── features/
│ ├── lib/
│ ├── hooks/
│ ├── types/
│ ├── mocks/
│ └── tests/
├── docs/
│ ├── ARCHITECTURE.md
│ ├── SECURITY.md
│ ├── THREAT_MODEL.md
│ ├── TENANCY.md
│ ├── RISK_ENGINE.md
│ ├── EVENTS.md
│ ├── WEBHOOKS.md
│ ├── DEPLOYMENT.md
│ ├── OBSERVABILITY.md
│ ├── INCIDENT_RESPONSE.md
│ └── DISASTER_RECOVERY.md
├── docker-compose.yml
└── README.md
```

# 25. Final Product Principle

RiskOS should be built as infrastructure first and dashboard second. The dashboard is the human interface to the risk engine. The API, event model, risk engine, tenant isolation, security controls, observability and auditability are the foundation.

The strongest proof of the product is: an external financial platform can securely send an event, receive a fast and explainable risk decision, and allow a human analyst to understand and investigate that activity.
