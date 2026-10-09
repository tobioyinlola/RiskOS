# Incident response

**Purpose:** how operational and security incidents are detected, triaged, communicated,
resolved and reviewed.

Status: a first design. Targets and roles are proposals to confirm with the people who will
carry the pager, and it must be exercised once (a tabletop at minimum) before production
(spec §19, Phase 8 "Incident exercise"). Nothing here is a claim about regulatory or
contractual obligations; those are taken from counsel, not from this document.

## Severity levels

| Level | Meaning | Examples | Acknowledge | Update cadence |
|---|---|---|---|---|
| SEV-1 | Customers cannot ingest or get decisions, data may be exposed or corrupted | Ingestion down; cross-tenant data visible; a signing key or database credential leaked | 15 minutes, any hour | every 30 minutes |
| SEV-2 | Degraded for many customers or a protection is not working | Latency above target; queue backlog; Redis down (limits and history signals off) | 30 minutes, any hour | every hour |
| SEV-3 | Limited impact or one customer, workaround exists | One tenant's webhook endpoint failing; a failing job kind | next business day | at resolution |
| SEV-4 | No customer impact | Alert tuning, a runbook gap found | planned work | none |

When unsure between two levels, take the higher one and lower it with evidence.

## Roles

One person may hold several roles in a small team; the roles are separate so each task has an
owner.

- **Incident lead:** decides, assigns, and is the only person who changes the severity or
  declares resolution. Does not debug.
- **Operations:** investigates and mitigates.
- **Communications:** writes status and customer messages from facts the lead confirms.
- **Scribe:** keeps the timeline (UTC, what was seen, what was done, by whom).

On-call: a primary and a secondary, rotating weekly; the secondary takes over after 15 minutes
without acknowledgement. Escalate to the engineering owner at SEV-1 or when a SEV-2 passes
two hours. A security incident also brings in whoever owns security review, whatever its
severity.

## During an incident

1. **Declare** in the incident channel with the severity and a one-line symptom; name the lead.
2. **Stabilize before diagnosing:** roll back the last release, disable the feature, or shed
   load, whichever is fastest and reversible. Release order and rollback are in
   [DEPLOYMENT.md](DEPLOYMENT.md).
3. **Preserve evidence** for anything security-related before changing state: the audit log
   ([SECURITY.md](SECURITY.md)), access logs by `request_id`, and the affected rows. Do not
   delete data to contain an incident.
4. **Communicate** on the cadence above, even when there is nothing new.
5. **Resolve** only when the indicator that fired is back inside its objective
   ([OBSERVABILITY.md](OBSERVABILITY.md#service-level-objectives-proposed-to-confirm)).
6. **Review** within five working days.

## Runbooks

Each lists what to check first, in order. Metric names are in
[OBSERVABILITY.md](OBSERVABILITY.md#application-metrics).

### API degraded

Symptoms: ingestion error share, decision latency or 5xx share alert.

1. Did a release or a configuration change land in the last hour? If so, roll it back first.
2. Readiness: `/health/ready` per instance shows which dependency is failing.
3. Latency with healthy dependencies: compare `riskos.risk.evaluation.duration` with request
   duration. Evaluation slow means Redis or the database; request slow but evaluation fast
   means queueing at the instance (check CPU and connection pool saturation).
4. A single tenant dominating traffic shows in `riskos.ratelimit.rejected`; the limits exist to
   contain it. Raise or lower limits only with the lead's agreement.
5. `risk evaluation degraded` logs mean a signal source is failing and decisions are being made
   without it; this is also a security signal (it may be induced). Treat the cause as an
   incident of its own.

### Database unavailable

Readiness fails and ingestion returns errors; nothing is stored, and nothing is decided
without being stored (decisions are transactional).

1. Confirm from a second vantage point that it is the database and not the network path.
2. Do not restart the API in a loop: pool exhaustion and reconnect storms prolong recovery.
3. If the primary is lost, follow [DISASTER_RECOVERY.md](DISASTER_RECOVERY.md#restore-procedure).
4. After recovery, check the queue backlog and the webhook failure share; both lag the outage.

### Redis unavailable

Expected behaviour is in [DISASTER_RECOVERY.md](DISASTER_RECOVERY.md#redis-down): rate limits
fail open and history signals go silent. This is SEV-2 because protections are off, even though
requests succeed.

1. Watch `riskos.ratelimit.errors`; while it is above zero, abuse controls are not in force.
   Consider temporary limits at the gateway.
2. After Redis returns with empty state, see
   [Losing Redis state](DISASTER_RECOVERY.md#losing-redis-state): every user looks new until
   history rebuilds. Tell the customer success owner; decisions are weaker, not wrong.

### Queue backlog

1. Which queue and which job kind? `riskos.jobs.queue.depth` by queue, `riskos.jobs.duration`
   by kind and outcome.
2. Failing kind: read its error from the worker logs; if the cause is a dependency, fix that
   and let the retries run. Do not delete jobs.
3. Slow kind: add worker replicas; webhook delivery has its own queue so a slow receiver cannot
   starve alert creation.
4. Alerts are raised by jobs, so a default-queue backlog delays alerts and cases: tell analysts.

### Suspected key compromise

An API key, console session, webhook signing secret or staff credential may be in the wrong
hands.

1. Revoke the key (or rotate the webhook secret) first; revocation is effective on every
   instance within 5 seconds ([SECURITY.md](SECURITY.md)). Rotation overlap windows for
   webhooks keep the previous secret valid, so end the overlap if the previous secret is the
   one exposed.
2. Pull the audit log and access logs for the credential for the exposure window, and what it
   read or changed.
3. Notify the tenant's admins, with the window and the actions.
4. A spike in `riskos.auth.failures` or `auth_failure` rejections points to guessing; the
   per-address limit is already containing it, so look for a distributed source.

### Suspected cross-tenant exposure

Always SEV-1 until disproved.

1. Stop the bleeding: disable the affected endpoint or route at the gateway, or roll back.
2. Preserve evidence (step 3 above) and do not "fix forward" before the lead agrees.
3. Establish scope from logs and the audit log: which tenants, which data, which window.
4. Row-level security and the tenant scope are the controls expected to hold
   ([TENANCY.md](TENANCY.md)); the review asks which one failed and why the other did not
   catch it.
5. Disclosure decisions belong to the lead with counsel.

### Webhook abuse or failure

1. Failure: one endpoint failing is the tenant's receiver; the platform disables it after
   repeated failures and raises a security alert. Contact the tenant.
2. Abuse (endpoints pointed at internal or third-party targets): disable the endpoint, and
   review `CreateWebhook` and `TestWebhook` activity for the tenant in the audit log. The
   resolved-address check at delivery time is the SSRF control
   ([WEBHOOKS.md](WEBHOOKS.md)); a bypass is a SEV-1 security incident.
3. Sustained high volume to one receiver: delivery is per-endpoint retried with backoff, so
   look at what is publishing events (an alert storm) before touching the queue.

## Communication

- **Status:** a status page or equivalent, with one owner. Facts only: what customers see,
  since when, what is being done, next update time. No speculation about cause.
- **Customers directly affected:** the account contact, in plain language, with the window and
  what was and was not affected.
- **Regulators and contractual notices:** only through the lead and counsel. This document makes
  no statement about which notices are required or their deadlines, and RiskOS makes no
  compliance claims until they are verified (spec §23).
- **Internal:** a single channel per incident; the scribe's timeline is the record.

## Post-incident review

Blameless, within five working days of resolution, for every SEV-1 and SEV-2 and for any
incident that surprised us. Template:

```
Title, severity, date, duration (detect → mitigate → resolve), lead
Impact: who, what, how many, what data
Timeline (UTC): from the scribe, including what was tried that did not help
Detection: how it was noticed; how long it took; which alert should have fired
Cause: the chain of causes, including the controls that did not catch it
What went well / what went badly / where we got lucky
Actions: owner, due date, how we will know it is done (each action is a tracked item)
Customer follow-up: what was said, what is still owed
```

Actions that change a design are recorded as an ADR or a change to the relevant document, and
a runbook that was wrong or missing is fixed in the same review.

## Not built yet

- The paging tool, the on-call schedule and the status page: they follow the platform and
  organisation decisions. TODO(phase-8)
- An incident exercise on production-like infrastructure (spec §19, Phase 8).
