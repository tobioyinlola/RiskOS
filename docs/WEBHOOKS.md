# Webhooks

**Purpose:** outbound notifications to customer systems: signing, delivery, retries and logs.
Design: [ARCHITECTURE.md §6 and §9](ARCHITECTURE.md#9-security-architecture). Status:
**designed; implementation in build step 6.**

## Triggers

Alert created, decision made, case changed. Jobs are enqueued in the same transaction as the
change that causes them ([ADR-0003](adr/ADR-0003-river-job-queue.md)), so a webhook can never
describe something that did not commit.

## Signing

Each delivery carries a timestamp, the event ID and an HMAC-SHA256 signature over timestamp,
event ID and body. Receivers must reject timestamps outside a 5-minute tolerance and repeated event
IDs. Webhook secrets are shown once, rotate with an overlap window, and derive from a master secret
held in the secrets manager. Verification examples ship with the developer docs.

## Delivery

- Exponential backoff with jitter, then dead-letter after the retry limit; every attempt is
  logged and visible to the customer.
- Webhook delivery runs in its own worker pool and per-tenant queues, so a slow endpoint cannot
  delay alerts or other tenants.

## SSRF defense

HTTPS only. DNS is resolved, then the resolved IP is dialed through a custom dialer that refuses
private, loopback, link-local and metadata ranges (so DNS rebinding cannot bypass it). Redirects
are disabled. Traffic leaves through an egress proxy with an allow policy.
