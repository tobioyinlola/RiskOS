# Webhooks

**Purpose:** outbound notifications to customer systems: events, signing, delivery, retries,
logs and SSRF defense. Design: [ARCHITECTURE.md §6 and §9](ARCHITECTURE.md#9-security-architecture).
Status: **built (build step 6).** Deviations are listed at the end.

## Events

| Type | Sent when | `data` |
|---|---|---|
| `alert.created` | An alert is raised (risk, security, or a webhook endpoint disabled for failing) | `alert` |
| `alert.updated` | An alert's status changes (triage, escalation into a case, closure with a case) | `alert`, `previous_status` |
| `case.created` | A case is opened | `case`, `alert_ids` |
| `case.resolved` | A case is resolved | `case` |
| `webhook.test` | On request (`POST /api/v1/webhooks/{id}/test`), to that endpoint only | `message` |

`alert` and `case` have exactly the shape the API returns (`GET /api/v1/alerts/{id}`,
`GET /api/v1/cases/{id}`). Every body is an envelope:

```json
{
  "id": "wev_06gf7h0...",
  "type": "alert.created",
  "created_at": "2026-09-30T14:02:11.123Z",
  "environment": "production",
  "data": { "alert": { "id": "alr_...", "kind": "risk", "severity": "critical", "...": "..." } }
}
```

Risk decisions are not sent as webhooks: `POST /api/v1/events` already returns them
synchronously.

**Transactional:** an event is recorded, with one delivery per subscribed active endpoint, in the
same database transaction as the change that caused it ([ADR-0003](adr/ADR-0003-river-job-queue.md)).
An event is sent only if the change committed, and a committed change is never missing its event.

## Endpoints

- `POST /api/v1/webhooks` registers an endpoint (`url`, `event_types`, optional `description`) and
  returns its **signing secret once**. At most 20 endpoints per environment. Test and production
  environments have separate endpoints, secrets and deliveries.
- `PATCH` changes URL, description, subscriptions, or status (`active`/`disabled`).
- `POST .../rotate-secret` issues a new secret (shown once). For `overlap_hours` (default 24,
  maximum 168) deliveries carry signatures from both the new and the previous secret.
- Scopes: `webhooks:read`, `webhooks:write`. Roles: `customer_admin` and `developer`.
- Every change is audited (`webhook.created`, `webhook.updated`, `webhook.secret_rotated`,
  `webhook.redelivered`, `webhook.disabled`). Audit entries record the URL's host only, since
  paths and query strings can carry tokens.

## Signing and verification

Each request carries:

| Header | Value |
|---|---|
| `RiskOS-Webhook-Id` | The event ID (`wev_...`): the same for every endpoint and every retry |
| `RiskOS-Webhook-Type` | The event type |
| `RiskOS-Webhook-Delivery` | This delivery's ID (`whd_...`), for support |
| `RiskOS-Webhook-Timestamp` | Unix seconds when this attempt was signed |
| `RiskOS-Signature` | One or more `v1=<hex>` values, comma-separated (two during a rotation overlap) |

`v1` is hex HMAC-SHA256, keyed by the endpoint's secret, over
`"<RiskOS-Webhook-Id>.<RiskOS-Webhook-Timestamp>.<raw body>"`. Receivers must:

1. recompute it over the **raw** body bytes and accept if it equals any `v1` value (constant-time
   comparison);
2. reject timestamps more than 5 minutes from their clock;
3. process each `RiskOS-Webhook-Id` once (retries and redeliveries reuse it);
4. respond with any 2xx quickly and do slow work afterwards.

```python
import hashlib, hmac, time

def verify(secret: str, headers, body: bytes, tolerance=300) -> bool:
    ts = headers["RiskOS-Webhook-Timestamp"]
    if abs(time.time() - int(ts)) > tolerance:
        return False
    msg = f'{headers["RiskOS-Webhook-Id"]}.{ts}.'.encode() + body
    want = "v1=" + hmac.new(secret.encode(), msg, hashlib.sha256).hexdigest()
    return any(hmac.compare_digest(want, s.strip()) for s in headers["RiskOS-Signature"].split(","))
```

```js
const crypto = require("crypto");
function verify(secret, headers, rawBody, toleranceSec = 300) {
  const ts = headers["riskos-webhook-timestamp"];
  if (Math.abs(Date.now() / 1000 - Number(ts)) > toleranceSec) return false;
  const want = "v1=" + crypto.createHmac("sha256", secret)
    .update(`${headers["riskos-webhook-id"]}.${ts}.`).update(rawBody).digest("hex");
  return headers["riskos-signature"].split(",").some(
    (s) => s.trim().length === want.length && crypto.timingSafeEqual(Buffer.from(s.trim()), Buffer.from(want)));
}
```

The integration tests verify every delivery with this exact algorithm, computed independently
of the server code.

**Secrets are never stored.** Each is derived with HMAC-SHA256 from a server-side key
(`RISKOS_WEBHOOK_SIGNING_KEY`, at least 32 bytes), the endpoint ID and a version number that
rotation increments. Rotating the server key changes every endpoint's secret at once and must be
coordinated with customers.

## Delivery and retries

- Deliveries run in the worker's own `webhooks` queue (`RISKOS_WEBHOOK_CONCURRENCY`, default 10),
  so a slow customer endpoint cannot delay alert creation or other jobs. The HTTP request runs
  outside any database transaction.
- Each attempt times out after `RISKOS_WEBHOOK_TIMEOUT` (default 10 s). Any 2xx is success;
  everything else (including redirects, which are never followed) is a failure.
- **Retries:** 9 attempts over about an hour: after 10 s, 30 s, 1 m, 2 m, 5 m, 10 m, 20 m and
  20 m, each ±20% jitter. Then the delivery is `failed` (dead-lettered).
- `POST .../deliveries/{id}/redeliver` starts a new cycle for a failed delivery (same event ID).
- **Delivery log:** every attempt is recorded (time, duration, status code, a short error such as
  `timeout` or `http status 500`) in an insert-only table and visible through
  `GET /api/v1/webhooks/{id}/deliveries[/{delivery_id}]`, with the exact payload sent. Response
  bodies are never stored.
- **Auto-disable:** when every attempt to an endpoint has failed for **3 days**, it is disabled,
  its pending deliveries fail, `webhook.disabled` is audited and a `security` alert is raised
  (subject: the endpoint). `PATCH ... {"status": "active"}` re-enables it and clears the failure
  state.

## SSRF defense

- Only `https` URLs; no credentials in URLs; no literal private, loopback, link-local, metadata
  (169.254.169.254), carrier-grade NAT, documentation, reserved or IPv6-translation addresses, and
  no `localhost`.
- The address check is repeated on the IP actually dialed, after DNS resolution (the dialer's
  control hook), so a hostname that resolves or re-resolves to an internal address is refused.
  Tested with a public name that resolves to 127.0.0.1.
- Redirects are never followed; no proxy is taken from the environment; TLS 1.2 minimum.
- **Local development:** `RISKOS_WEBHOOK_ALLOW_PRIVATE_TEST=true` (set in docker-compose) lets
  endpoints of the **test** environment use `http://` and private addresses, so developers can
  receive webhooks locally. Production endpoints are never affected, and the setting is refused
  when `RISKOS_ENV=production`.

## Deviations (ARCHITECTURE.md Appendix B)

- **Egress proxy:** outbound traffic is not yet forced through an egress proxy with an allow
  policy; the resolved-IP check is the control until deployment infrastructure exists.
  TODO(phase-8).
- **Per-tenant queues:** deliveries share one `webhooks` queue with a global concurrency limit;
  per-tenant fairness (so one tenant's slow endpoints cannot occupy every delivery worker) is not
  built. TODO(phase-8).
- **Signing key:** an environment variable locally; the secrets manager comes with deployment.
  TODO(phase-8).
- **Retention:** deliveries and attempts (including payloads) are kept until the retention sweep
  exists. TODO(phase-7).
