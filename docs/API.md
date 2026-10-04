# API

**Purpose:** how the public HTTP API is specified, versioned and evolved.

The contract is [`backend/api/openapi.yaml`](../backend/api/openapi.yaml). It is the single source
of truth: Go server stubs (`backend/internal/apigen`) and the TypeScript client
(`frontend/lib/api/schema.gen.ts`) are generated from it with `make generate`, and CI fails when
generated code is stale. An operation that is not in the contract cannot be routed.

The public developer documentation (`/docs` on the site, linked from the landing page's footer)
holds the integration guides (`frontend/app/(marketing)/docs`) and an API reference rendered from
`frontend/lib/api/openapi.gen.json`, which `make generate` writes from the same contract. Guides
state only what the code and these documents establish; change them with the behaviour.

## Current operations

| Method | Path | Purpose |
|---|---|---|
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe (PostgreSQL, Redis, draining) |
| GET | `/api/v1/whoami` | The tenant, environment and scopes of the calling key |
| GET | `/api/v1/api-keys` | List keys of the caller's environment (`api_keys:read`) |
| POST | `/api/v1/api-keys` | Create a key; secret returned once (`api_keys:write`) |
| POST | `/api/v1/api-keys/{key_id}/rotate` | Replace a key, old one valid for an overlap window (`api_keys:write`) |
| POST | `/api/v1/api-keys/{key_id}/revoke` | Revoke a key (`api_keys:write`) |
| GET | `/api/v1/audit-logs` | Tenant-level and own-environment audit entries (`audit_logs:read`) |
| POST | `/api/v1/events` | Ingest an event, get the risk decision; idempotent on `event_id` (`events:write`) |
| GET | `/api/v1/events` | List events; filter by `type`, `decision`, `risk_level`, `user`, `account`, `device`; `after` returns only newer events (`events:read`) |
| GET | `/api/v1/events/{event_id}` | Event with raw and normalized payloads and its evaluation (`events:read`) |
| GET | `/api/v1/entities/{kind}/{ref}` | A user's, account's or device's profile from its most recent 5,000 events (`events:read`) |
| GET | `/api/v1/analytics` | Events per hour or day by band and decision, highest-risk users, most frequent signals; `window` 24h, 7d or 30d (`events:read`) |
| POST | `/api/v1/risk/evaluate` | Dry-run evaluation; stores no event (`risk:evaluate`) |
| GET | `/api/v1/risk/policy` | The environment's bands, thresholds and default decisions (any score-reading scope) |
| GET | `/api/v1/risk/catalog` | Built-in signals with effective weights, and the fields and operators rules can use (`rules:read`) |
| GET | `/api/v1/risk/settings` | Signal weights, high-value thresholds, fail policies (`risk:configure`) |
| PUT | `/api/v1/risk/settings` | Replace them; audited (`risk:configure`) |
| GET | `/api/v1/rules` | List rules with their current versions (`rules:read`) |
| POST | `/api/v1/rules` | Create a rule; audited (`rules:write`). In production, rule changes answer 202 with a proposal awaiting approval |
| GET | `/api/v1/rules/{rule_id}` | A rule's current version (`rules:read`) |
| PATCH | `/api/v1/rules/{rule_id}` | Change a rule, creating a new version; audited (`rules:write`) |
| GET | `/api/v1/rules/{rule_id}/versions` | Version history (`rules:read`) |
| POST | `/api/v1/rules/{rule_id}/rollback` | Restore an earlier version as a new version; audited (`rules:write`) |
| GET | `/api/v1/rule-changes` | Proposed production rule changes, by `status` (default pending) (`rules:read`) |
| GET | `/api/v1/rule-changes/{change_id}` | A proposed change (`rules:read`) |
| POST | `/api/v1/rule-changes/{change_id}/approve` | Apply a change; a person other than the proposer, never a key; audited (`rules:approve`) |
| POST | `/api/v1/rule-changes/{change_id}/reject` | Reject, or withdraw your own; audited (`rules:approve`, or the proposer) |
| GET | `/api/v1/deny-list` | Deny-list entries, by `kind`, with the total and the limit (`rules:read` or `risk:configure`) |
| POST | `/api/v1/deny-list` | List an identifier; events carrying it are blocked; audited (`risk:configure`) |
| DELETE | `/api/v1/deny-list/{entry_id}` | Remove an entry; audited (`risk:configure`) |
| GET | `/api/v1/alerts` | List alerts; filter by `status`, `kind` (`alerts:read`) |
| GET | `/api/v1/alerts/{alert_id}` | Alert with its evaluations and timeline (`alerts:read`) |
| PATCH | `/api/v1/alerts/{alert_id}` | Triage: status, assignee, note; audited (`alerts:write`) |
| GET | `/api/v1/cases` | List cases; filter by `status` (`cases:read`) |
| POST | `/api/v1/cases` | Open a case, optionally escalating alerts; audited (`cases:write`, plus `alerts:write` with alerts) |
| GET | `/api/v1/cases/{case_id}` | Case with its alerts, notes and timeline (`cases:read`) |
| PATCH | `/api/v1/cases/{case_id}` | Update or resolve a case; audited (`cases:write`) |
| POST | `/api/v1/cases/{case_id}/notes` | Add an append-only note; audited (`cases:write`) |
| POST | `/api/v1/auth/login` | Console sign-in: email and password (cookie session; see [CONSOLE_AUTH.md](CONSOLE_AUTH.md)) |
| GET | `/api/v1/auth/session` | The console session: user, tenant, permissions, factors, CSRF token |
| POST | `/api/v1/auth/logout` | End the console session |
| POST | `/api/v1/auth/mfa/totp/verify`, `.../recovery/verify` | Complete sign-in or re-authenticate with a code |
| POST | `/api/v1/auth/mfa/totp/enroll`, `.../totp/confirm` | Enroll an authenticator app |
| POST | `/api/v1/auth/mfa/passkeys/register/options`, `.../register` | Add a passkey |
| POST | `/api/v1/auth/mfa/passkeys/login/options`, `.../login` | Sign in or re-authenticate with a passkey |
| DELETE | `/api/v1/auth/mfa/passkeys/{passkey_id}` | Remove a passkey (re-authentication) |
| POST | `/api/v1/auth/mfa/recovery-codes` | Replace recovery codes (re-authentication) |
| POST | `/api/v1/auth/invites/accept` | Accept an invite: set name and password |
| POST | `/api/v1/auth/sso/start` | Start single sign-on for an email; returns the provider's authorization URL |
| POST | `/api/v1/auth/sso/callback` | Finish single sign-on with the provider's code and state; the session still needs its second factor |
| GET | `/api/v1/team/members` | Console users (`team:read`) |
| PATCH | `/api/v1/team/members/{user_id}` | Change a user's role or status (`team:write`, re-authentication) |
| GET, POST | `/api/v1/team/invites` | List or create invites (`team:read` / `team:write`, re-authentication). With email configured, the link is also emailed (`email_queued`, `email_sent_at`) |
| POST | `/api/v1/team/invites/{invite_id}/revoke` | Revoke a pending invite (`team:write`) |
| GET, PUT, DELETE | `/api/v1/sso` | The tenant's OpenID Connect provider: read (`team:read`), set up or change, remove (`team:write`, re-authentication; audited) |
| GET | `/api/v1/webhooks` | List webhook endpoints (`webhooks:read`) |
| POST | `/api/v1/webhooks` | Register an endpoint; secret returned once; audited (`webhooks:write`) |
| GET | `/api/v1/webhooks/{webhook_id}` | An endpoint (never its secret) (`webhooks:read`) |
| PATCH | `/api/v1/webhooks/{webhook_id}` | Change, disable or re-enable an endpoint; audited (`webhooks:write`) |
| POST | `/api/v1/webhooks/{webhook_id}/rotate-secret` | New secret, shown once, with an overlap; audited (`webhooks:write`) |
| POST | `/api/v1/webhooks/{webhook_id}/test` | Queue a `webhook.test` event (`webhooks:write`) |
| GET | `/api/v1/webhooks/{webhook_id}/deliveries` | Delivery log; filter by `status` (`webhooks:read`) |
| GET | `/api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}` | A delivery with its payload and attempts (`webhooks:read`) |
| POST | `/api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}/redeliver` | Retry a failed delivery; audited (`webhooks:write`) |
| POST | `/api/v1/rules/{rule_id}/test` | Test the rule, or a draft condition, against an event; stores nothing (`rules:read`) |

### Several jurisdictions (ADR-0007)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/compliance/jurisdictions` | The countries with a rule pack: currency, time zone, regulators, and whether the workspace activated each (`compliance:read`) |
| GET | `/api/v1/compliance/activations` | The workspace's activations per country and booking entity, the fallback first (`compliance:read`) |
| GET | `/api/v1/compliance/active-rules` | Every pack rule running in the environment, with the events, facts and refs to send and the settings still unset; `?jurisdiction=` (`compliance:read`) |

Contract changes, all additive except one: events take `compliance.booking_entity`,
`compliance.data_subject_country`, refs `national_id` and `agent`, and new kinds; decisions carry
`jurisdiction` (country, pack and how it was resolved); pack rule IDs may be country IDs
(`GH-REG-04`) on every `/compliance/rules/{rule_id}` path, and a single rule takes
`?jurisdiction=`; rules carry `provenance`, `lineage`, `off_reason`, `inherited_from`,
`template`, `changes` and `basis`; packs carry `activated`. **Breaking:** a clock's `hours` may be
null (a legal deadline no readable source states, `kind` not_found) and is 0 for "immediately";
clients that read it as a number must handle both.

## Conventions (in force)

- Every error uses the error contract `{"error":{"code","message","request_id"}}` (spec §7).
  Codes are stable UPPER_SNAKE_CASE; messages are safe to show and never contain internals.
- Every response carries `X-Request-ID`.
- Product endpoints live under `/api/v1` and require `Authorization: Bearer rsk_…`. The key
  determines tenant and environment; no request field can change them.
- Status codes: 400 `INVALID_REQUEST`, 401 `UNAUTHENTICATED` (never says why), 403 `FORBIDDEN`
  or `TENANT_SUSPENDED`, 404 `NOT_FOUND` (also for other tenants' resources), 409 `CONFLICT` or
  `IDEMPOTENCY_CONFLICT`, 413 `PAYLOAD_TOO_LARGE`, 422 `INVALID_EVENT`, `STALE_EVENT` or
  `SENSITIVE_DATA`, 429 `RATE_LIMITED` (with `Retry-After`), 503 `SERVICE_UNAVAILABLE` (with
  `Retry-After`; nothing was stored) or `TIMEOUT`.
- Every request is validated against this contract before it reaches a handler; unknown fields
  are rejected.
- IDs are type-prefixed (`ten_`, `env_`, `key_`, `aud_`, `evt_`, `evl_`, `rul_`, `alr_`, `cas_`, `cnt_`, `act_`, `whk_`, `whd_`, `wev_`, `rch_`, `jac_`); an ID of the wrong
  type is not found.
- Lists: `?limit=` (1-100, default 50) and `?cursor=` (the previous page's `next_cursor`);
  newest first.

## Retries

A request that fails with 429, 503, a timeout or a network error can be retried safely:

- Retry an event with the **same `event_id`**. If the first attempt was stored after all, the
  retry returns the stored decision with `Idempotent-Replay: true`; the event is never stored
  twice.
- Wait at least `Retry-After` seconds when it is present, back off exponentially otherwise, and
  add random jitter so clients do not retry in lockstep after an outage.
- While the decision is unavailable, apply your own fallback for the transaction (for example,
  hold it for review). The API fails fast instead of queueing: a database outage answers 503
  within about a second, and a Redis outage still returns decisions, degraded and failing
  closed ([RISK_ENGINE.md](RISK_ENGINE.md)).

The failure drill (`make failure-drill`, [DISASTER_RECOVERY.md](DISASTER_RECOVERY.md)) checks
that such a client loses no event and stores none twice across Redis, PostgreSQL, worker and API
restarts.

## To be designed (TODO(phase-0))

- Remaining operations of spec §7 (entities, analytics).
- Filtering and sorting conventions for lists (event type, decision, time range).
- Versioning and deprecation policy.

## Support access by RiskOS staff

RiskOS staff never hold standing access to tenant data. With support access that a second staff
member approved ([ADMIN.md](ADMIN.md)), the requesting staff member may send `GET` requests to
this API with the `RiskOS-Support-Grant` header and their staff session, holding read
permissions only, for at most four hours. Each such request appears in the tenant's audit log
with actor type `staff`. Every other method, and the session endpoints, answer 403. The internal
admin API itself is described separately in `backend/api/admin.yaml`.
