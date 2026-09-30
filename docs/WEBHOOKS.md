# Webhooks

**Purpose:** outbound notifications to customer systems: signing, delivery, retries and logs.

## Requirements (spec §12, §20)

HMAC (or equivalent) signing with timestamp and event ID; replay protection; retries with
exponential backoff; delivery logs; secrets never shown after creation; SSRF protection for
customer-supplied URLs. Webhooks never block the synchronous risk response.

## To be designed (TODO(phase-0), implemented in phase-5)

- Payload envelope and signature scheme, with verification examples.
- Transactional outbox or transactional job enqueue (see `backend/internal/jobs`).
- Retry schedule, dead-lettering and customer-visible delivery status.
- URL validation and egress controls against SSRF.
