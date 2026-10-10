# Internal admin realm

**Purpose:** how RiskOS staff sign in, how support access to a tenant works, and what staff can
see across tenants. Design: [ARCHITECTURE.md §16](ARCHITECTURE.md). Status: **built (after build
step 8).** Contract: [`backend/api/admin.yaml`](../backend/api/admin.yaml), separate from the
customer API.

## Staff accounts

- **Separate from tenant users.** Staff are their own accounts (`staff` table), with the internal
  roles `operations_admin` or `security_admin`. Neither role grants any access to tenant data.
- **Created only by the operator CLI.** There is no API to create staff:

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
- **Short sessions.** 15 minutes idle, 4 hours at most, in the `riskos_staff` cookie (HttpOnly,
  SameSite=Strict, Secure outside local development). Unsafe requests need the session's CSRF
  token and an allowed `Origin`. Passkey ceremonies are throttled per client address.
- **All audited.** Every staff action (creation, enrollment, sign-in and failures, sign-out,
  support access and each page read with it) goes to the staff audit log, which the application
  role can only append to. The console shows it under Staff audit log.

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

## Not built yet

- A private access path (zero-trust proxy or VPN) and an IP allowlist in front of `/admin`, and
  hardware-key attestation for staff passkeys. TODO(phase-9)
- SSO for staff. TODO(phase-9)
- A hash chain on the staff audit log like the tenant audit log's. TODO(phase-9)
- Tenant management (suspension and limits) by staff, with four-eyes approval. TODO(phase-9)
