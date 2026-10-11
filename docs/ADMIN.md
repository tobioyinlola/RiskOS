# Internal admin realm

**Purpose:** how RiskOS staff sign in, how support access to a tenant works, and what staff can
see across tenants. Design: [ARCHITECTURE.md §16](ARCHITECTURE.md). Status: **built (after build
step 8).** Contract: [`backend/api/admin.yaml`](../backend/api/admin.yaml), separate from the
customer API.

## Staff accounts

- **Separate from tenant users.** Staff are their own accounts (`staff` table), with the internal
  roles `operations_admin` or `security_admin`. Neither role grants any access to tenant data.
- **Created by two people, or by the operator CLI.** No single console session can create an
  account (see [Staff roster](#staff-roster)); the operator CLI, run with shell access, can:

  ```bash
  make staff EMAIL=ops@riskos.example NAME="Ops Person" OPERATOR=you@example.com [ROLE=security_admin]
  # riskosctl staff create | enroll | disable | enable | list
  ```

  `create` prints a single-use link (24 hours) to `/admin/enroll`, where the person registers
  a passkey. `enroll` issues a new link (for example after losing a passkey); `disable` ends all
  of the person's sessions at once.
- **Passkeys only.** No passwords, no authenticator codes. Sign-in uses discoverable credentials
  with user verification, so the sign-in page asks for no email. A passkey whose signature
  counter goes backwards (a possible clone) is refused.
- **Single sign-on (optional).** A deployment can name one identity provider for staff
  (`RISKOS_STAFF_SSO_ISSUER`, `_CLIENT_ID`, `_CLIENT_SECRET`; register
  `<console URL>/admin/sso/callback` as the redirect URI). The sign-in page then offers
  "Continue with single sign-on": authorization code flow with PKCE (S256), state and nonce, the
  ID token's signature, issuer, audience, expiry and nonce checked, provider requests through the
  platform egress client. The provider only *identifies* the person; the roster stays the
  authority for who is staff, their role and their status, and **the passkey is still required**
  to open a session. At the first SSO sign-in the provider must vouch for the account's email
  (`email_verified`), and its subject is bound to the staff member; later sign-ins match by
  subject, so an email reassigned at the provider reaches nothing. The person must already have a
  passkey (enrolled from the invitation link). Every failure is the same answer to the caller
  (`SSO_FAILED`) and is audited with its reason (`staff.sso_failed`: `subject_mismatch`,
  `email_unverified`, `no_passkey`, `staff_disabled`, `token_rejected`, `unknown_staff`).
  `RISKOS_STAFF_SSO_REQUIRED=true` makes the provider the first step of every sign-in: direct
  passkey sign-in answers `403 SSO_REQUIRED`, and enrolling a passkey from a link registers it
  without opening a session. If a person's account at the provider is replaced, `riskosctl staff
  unbind-sso --operator NAME --email EMAIL` forgets the bound subject and ends their sessions; the
  next sign-in binds afresh. Disabling a person at the provider does not by itself end an open
  console session (at most 4 hours, 15 idle); disable them in the roster too.
- **Short sessions.** 15 minutes idle, 4 hours at most, in the `riskos_staff` cookie (HttpOnly,
  SameSite=Strict, Secure outside local development). Unsafe requests need the session's CSRF
  token and an allowed `Origin`. Passkey ceremonies are throttled per client address.
- **All audited.** Every staff action (creation, enrollment, sign-in and failures, sign-out,
  support access and each page read with it) goes to the staff audit log. The console shows it
  under Staff audit log.
- **Tamper-evident.** Each entry written since migration `00040` carries a sequence number and a
  SHA-256 over its fields and the previous entry's hash (`riskos.staffaudit.v1`, length-prefixed,
  over the exact canonical metadata text), the table rejects update, delete and truncate for
  everyone including the owner, and a single locked head row serializes appends. A modified,
  removed, reordered or truncated entry is found by `riskosctl staff-audit verify` (it reads as the
  schema owner, like `audit verify`). Entries from before the chain are outside it. The chain
  detects tampering; it does not prevent someone with the owner's rights from disabling the
  trigger, so the head should also be anchored outside the database once the platform has
  write-once storage (`TODO(phase-8)`, with the tenant chain's).

## Support access

Staff read a tenant's data only through support access:

1. A staff member requests access to one tenant, with a reason (the customer sees it) and a
   duration of 15 minutes to 4 hours.
2. Another staff member approves or rejects it. Nobody can decide on their own request; the
   database enforces this too. Access runs from the approval for the requested duration.
3. While approved, the requester (only) reads the tenant through the customer API with the
   `RiskOS-Support-Grant` header and their staff session. Only `GET` requests are accepted, with
   read permissions only (events, alerts, cases, rules, audit log, webhooks, API key metadata,
   team); risk settings and every change are out of reach, and so are session endpoints. The
   staff console's read-only view shows recent events, alerts and the tenant's audit log.
4. Any staff member can end access early or withdraw a pending request.

The tenant's own audit log records the request, the decision, the revocation and every request
made with the access (`support_access.*`, actor type `staff`), so the customer can see who looked
at what and why.

## Overview and tenant directory

Design: [ADR-0010](adr/ADR-0010-staff-console.md). The console opens on **Overview**: what waits
for the signed-in staff member, each count linking to the page that clears it (onboarding
reviews waiting, support requests made by *someone else*, tenants by status, support access in
use), with the security view below.

**Tenants** lists every workspace with identity, status, origin, whether production is open,
active users, active API keys and onboarding status, searchable by name or ID and filterable by
status. A tenant's page adds the security counters for it and the staff audit entries that name
it. Like the security view, the figures come from `SECURITY DEFINER` functions
(`admin_tenant_directory()`, `admin_overview()`) that return identity and counts only; what a
tenant stores is reached only through support access.

## Cross-tenant security view

The security view, on the Overview page, lists, per tenant, counts only, never tenant data:

- open security alerts (degradation, disabled webhook endpoints);
- evaluations with degraded signals in the last 24 hours;
- failed console sign-ins (password, second factor, SSO) in the last 24 hours;
- disabled webhook endpoints.

The counts come from one `SECURITY DEFINER` function that returns aggregates only.

## Onboarding reviews

Self-serve workspaces ([ADR-0005](adr/ADR-0005-self-serve-signup.md)) answer a compliance
onboarding (business, regulation, use case, data and privacy) before their production
environment opens. Operations admins (`operations_admin`) review it in the staff console's
Onboarding reviews page:

- The queue (`admin_onboarding_queue()`, a `SECURITY DEFINER` function) shows identity, status and
  dates only, submitted first.
- Opening one reads its answers through a staff tenant scope; the read is recorded in the staff
  audit log and the tenant's (`onboarding.viewed`, `onboarding.viewed_by_staff`).
- **Approve** opens production (`tenants.production_enabled = true`); **Request changes** needs a
  note of 10 to 1000 characters, which the customer sees, and production stays locked. Both are
  recorded in both audit logs in one transaction.

## Suspending and reactivating a tenant

Four eyes ([ADR-0010](adr/ADR-0010-staff-console.md)). From a tenant's page, or the Status
requests page, any staff member requests **suspend** (only for an active tenant) or
**reactivate** (only for a suspended one) with a reason of 10 to 500 characters. A *different*
staff member approves or rejects; the requester may withdraw. One request may be pending per
tenant.

- **The database enforces it.** `tenant_status_requests` refuses a decision by the requester, and
  `admin_decide_tenant_status()` (`SECURITY DEFINER`) applies the request and the tenant's status
  change as one step, so no code path changes a status without an approved request. If the
  tenant changed since the request, approval is refused and the request can be rejected.
- **Effect.** A suspended tenant's API keys and console sessions are refused (a key lookup is
  cached for up to 5 seconds); reactivation restores them. Support-access grants are not usable
  while suspended.
- **Both logs.** The staff audit log records the request (with the reason), the decision and any
  note (`tenant_status.*`). The tenant's own audit log records the effect, `tenant.suspended` or
  `tenant.reactivated`, naming the staff member but not the reason.
- **Break-glass.** In an incident one security administrator may **suspend now** from the tenant's
  page: it applies at once, with a reason and an explicit confirmation. It is never silent and
  never final on one person's word: it is flagged as an emergency in both audit logs
  (`tenant_status.emergency_suspended`; the tenant's log shows `tenant.suspended` marked
  `emergency`), it appears on every other staff member's queue until a *different* staff member
  marks it reviewed (`tenant_status.emergency_reviewed`), and the database refuses a review by
  the person who did it. A waiting request for the tenant must be decided or withdrawn first.
  Reviewing does not undo it; **reactivation has no emergency path** and stays an ordinary
  two-person request. The role and tenant state are checked inside
  `admin_emergency_suspend()`, not only by the caller.

## Request limits per tenant

On a tenant's page, staff see the platform's per-key and per-tenant request limits and may
replace them for that tenant (a rate and a burst for each), with a reason of 10 to 500
characters; **Remove override** restores the platform's.

- **Scope.** An override changes only how much that tenant may send; it never changes who is
  limited. Auth-failure and per-operation limits, and every other tenant, are untouched.
- **Where it applies.** The API's limiter looks the override up, cached for up to 30 seconds per
  instance, so a change is in force within about that time. A failed lookup falls back to the
  platform's limits instead of failing requests.
- **Draining.** The limiter keeps each key's state in Redis and a change of limit does not reset
  it. After removing a slow override, a tenant that had exhausted it waits for that debt to drain
  (up to burst divided by the old rate) before the platform's higher limit is felt. Choose bursts
  that keep that short.
- **Both logs.** The staff audit log keeps the reason (`tenant_limits.set`,
  `tenant_limits.cleared`); the tenant's own log records `tenant.limits_changed` with the new
  values and not the reason.

## Staff roster

Security administrators (`security_admin`) see **Staff** in the console: every account with its
role, status, passkey count, last sign-in and whether an enrollment link is outstanding. Never a
credential. They may **disable** or **enable** another account:

- Never their own account, and never the last active security administrator (a lock on the
  active security administrators stops two of them disabling each other at once).
- Disabling ends the person's sessions at once, and they cannot sign in again until enabled.
- Each change is in the staff audit log (`staff.disabled`, `staff.enabled`).

### Adding a person

A hijacked staff session that could mint credentials could take over any account, so the console
never lets one session do it. Adding a person is a **four-eyes request between two security
administrators**, and the credential never passes through either of their sessions:

1. A security administrator requests an account (email, name, role, a reason of 10 to 500
   characters). One request may be pending per address; an existing account is refused.
2. A *different* security administrator approves or rejects; the requester may withdraw. An
   operations administrator can neither request nor decide.
3. Approval creates the account and queues, in the same transaction, an email to the new
   person's address with a single-use enrollment link (24 hours). The queued job holds only the
   account's ID and issue time: the worker derives the token again from the console secret, so it
   is stored nowhere readable and nobody is shown it. A used, expired, disabled or replaced link
   is not re-sent.
4. Every step is in the staff audit log (`staff_account.requested`, `.approved`, `.rejected`,
   `.withdrawn`).

**Email must be configured** (`RISKOS_SMTP_*`, with the worker running): without it the console
refuses the request and accounts are created with the operator CLI (`make staff`, above).
**Re-issuing a link for someone who lost their passkey stays in the CLI**, since it replaces a
live credential.

## Address allowlist

`RISKOS_ADMIN_ALLOWED_CIDRS` (comma-separated networks, e.g. `203.0.113.0/24,2001:db8::/32`)
limits the staff realm, every path under `/api/admin/`, to clients inside those networks. Any
other address, and a request whose address is unknown, gets the 404 of a path that does not
exist, so the realm is not revealed to the rest of the internet; each refusal is logged as a
warning. Empty (the default) allows every address.

- **It is a second control, not the first.** Sign-in (SSO and passkey) is unchanged and still
  required from inside the networks. It narrows who can attempt it.
- **The address must be right.** It is the client address `RISKOS_TRUSTED_PROXIES` yields:
  `X-Forwarded-For` is believed only from those proxies. A list that is wrong behind a proxy
  either locks staff out or lets the proxy's address through, so set both together and test
  from outside the networks.
- **`0.0.0.0/0` and `::/0` are refused at start-up**, since they would look like a control and be
  none.
- **Lock-out.** There is no way in from outside the networks, including the sign-in
  ceremonies. Change the variable and restart to recover; the operator CLI (`riskosctl staff`)
  does not go through this check.
- It filters the API only. The console's `/admin` pages are a static shell that loads nothing
  without the API; put them behind the same path if hiding them matters.

## Not built yet

- Quotas other than request rate (storage, event volume) per tenant. TODO(phase-9)
- A private access path (zero-trust proxy or VPN) in front of `/admin`: it is deployment
  infrastructure. TODO(phase-8)
- Hardware-key attestation for staff passkeys (needs a decision on which authenticators to
  trust). TODO(phase-9)
- Group or role claims from the provider (roles stay in the roster), and a second provider.
- Anchoring the staff and tenant audit chains' heads in write-once storage. TODO(phase-8)
