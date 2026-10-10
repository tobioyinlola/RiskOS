# ADR-0010: The staff console: tenant directory, tenant status with four eyes, staff roster

- **Status:** accepted
- **Date:** 2026-10-10
- **Deciders:** product owner (delegated the design to engineering), engineering

## Context

The staff console ([ADMIN.md](../ADMIN.md)) lets RiskOS staff sign in with passkeys, review
self-serve onboarding, approve each other's support-access requests, read cross-tenant security
counts and read the staff audit log. What it cannot do is run the platform day to day:

- **No tenant directory.** The API lists tenants (`GET /api/admin/v1/tenants`) but no page shows
  them, and there is no tenant detail: staff cannot see a tenant's status, origin, whether
  production is open, how big it is, or what staff have done to it.
- **No way to suspend or reactivate a tenant.** `tenants.status` has the values and the
  authentication path already refuses a suspended tenant, but nothing can change the value except
  SQL. ARCHITECTURE.md §16 requires four-eyes approval for tenant suspension.
- **No staff roster.** Accounts exist only in the operator CLI; a security administrator cannot
  see who has access, when they last signed in, or disable an account without shell access.
- **No work queue.** The landing page is the security view. Pending onboarding reviews, support
  requests and tenant actions each need a separate visit to find out whether anything waits.

## Decision

The console gains four things, in one design so they read as one product.

1. **Overview (`/admin`).** A work queue first, then the existing security view: pending
   onboarding reviews, support requests waiting for *someone else*, tenant status requests waiting
   for someone else, and tenants by status. Each count links to the page that clears it. Counts
   only, from `SECURITY DEFINER` aggregates, like the security view.
2. **Tenants (`/admin/tenants`, `/admin/tenants/{id}`).** A directory with name search and status
   filter, and a detail page: status, region, origin, production enabled, created date, onboarding
   status, counts (active users, active API keys per environment), the security counters for that
   tenant, the staff audit entries about it, and its status requests. Identity and counts only:
   tenant *data* is reached only through support access, unchanged.
3. **Tenant status changes with four eyes.** Any active staff member requests *suspend* or
   *reactivate* for a tenant with a reason of 10 to 500 characters. A different active staff
   member approves or rejects; the requester may withdraw. One request may be pending per tenant.
   The database enforces that nobody decides their own request and that approval and the status
   change happen together (a `SECURITY DEFINER` function performs both, so no code path can do one
   without the other). Both audit logs record the request, the decision and the effect; the tenant
   sees `tenant.suspended` / `tenant.reactivated` in its own audit log without the internal
   reason. A suspended tenant's API keys and console sessions are refused, which the
   authentication path already does; a test pins it.
4. **Staff roster (`/admin/staff`, security administrators only).** Lists accounts with role,
   status, passkey count, last sign-in and whether an enrollment link is outstanding. A security
   administrator may disable or enable another account, never their own, and never the last
   active security administrator. **Creating accounts and issuing enrollment links stay in the
   operator CLI**: ADMIN.md's rule that there is no API to create staff is a deliberate control,
   because a compromised staff session that could mint credentials could take over any account.

Roles: `operations_admin` runs onboarding, tenants and support; `security_admin` additionally
sees the roster and the security view. Either may request a tenant status change; the approver
is any other active staff member.

## Alternatives considered

- **Immediate suspension by one security administrator (break-glass).** *Amended:* first
  rejected, because §16 asks for four eyes and an emergency path that bypasses them needs its own
  review trail. It is now built with that trail: security administrators only, suspension only,
  applied at once but flagged as an emergency in both audit logs, and unresolved on every other
  staff member's queue until a different staff member reviews it. The review is the second pair
  of eyes after the fact. Reactivation never has an emergency path.
- **Staff creation in the UI.** *Amended:* first rejected, because a hijacked session that could
  mint credentials could take over any account. It is now built so that no single session can: it
  is a four-eyes request between two security administrators, the enrollment link is emailed to
  the new person and shown to neither, the token is derived by the worker rather than stored in
  the job, and it is refused outright where email is not configured (the CLI remains). Two
  colluding or simultaneously hijacked security administrators could still add an account; that
  residual risk is why re-issuing a link for an existing account stays in the CLI, and why every
  step is audited. Staff SSO (built; ADMIN.md) and a private access path reduce it further.
- **A "disabled" tenant status in the UI.** The column allows it, but it has no defined meaning
  distinct from suspended (offboarding is undecided, ADR-0009), so the console offers only
  `active` and `suspended`.
- **Tenant limits (rate limits, quotas) editable by staff.** *Amended:* first deferred as a
  data-model decision; the request-rate limits are now built as a `tenant_limits` override table
  (migration `00038`) that the API's limiter consults through a short cache, set by staff with a
  reason and audited on both sides. It needs no second approver: lowering a limit protects the
  platform and raising one is bounded by the table's checks and visible to the tenant. Quotas
  other than request rate (storage, volume) remain undesigned.

## Consequences

- Operations no longer need SQL for suspension, and every suspension has a reason, two people and
  an audit trail on both sides.
- A suspension request waits for a second staff member: a team of one cannot suspend. That is the
  control working, and the runbook for incidents should say who the second approver is.
- The roster is the only place staff accounts can be disabled without a shell, and it is
  restricted to one role and audited.
- **Staff SSO.** *Amended:* first listed as a follow-up. Built as the platform's single
  deployment-wide OpenID Connect provider (environment configuration, not data, so no staff
  member can repoint it), and never as a replacement for the passkey: the provider identifies the
  person, the staff member's passkey with user verification opens the session, so a compromised
  provider account alone reaches nothing and a stolen passkey alone reaches nothing where SSO is
  required. The roster stays the authority for role and status; the provider's subject is bound
  at the first sign-in, which needs `email_verified`, and is cleared only by the operator CLI.
  Not built: roles from provider claims (they would let the provider mint a security
  administrator), and a second provider.
- Follow-ups unchanged from ADMIN.md: private access path, hardware-key attestation,
  hash chain on the staff audit log; and now break-glass suspension and per-tenant limits.

## Implementation

All four pieces are built and described in [ADMIN.md](../ADMIN.md): the Overview work queue and
tenant directory (migration `00035`), tenant status requests with four-eyes approval
(`00036`, `TestAdminTenantStatusFourEyes`) and the staff roster (`TestAdminStaffRoster`). One
observation from building the roster: "never the last active security administrator" can only
be reached by a race, since an actor who is an active security administrator makes the count at
least two; the lock on active security administrators is what keeps that true under concurrency.
