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

## Cross-tenant security view

The staff console's Security page lists, per tenant, counts only, never tenant data:

- open security alerts (degradation, disabled webhook endpoints);
- evaluations with degraded signals in the last 24 hours;
- failed console sign-ins (password, second factor, SSO) in the last 24 hours;
- disabled webhook endpoints.

The counts come from one `SECURITY DEFINER` function that returns aggregates only.

## Not built yet

- A private access path (zero-trust proxy or VPN) and an IP allowlist in front of `/admin`, and
  hardware-key attestation for staff passkeys. TODO(phase-9)
- SSO for staff. TODO(phase-9)
- A hash chain on the staff audit log like the tenant audit log's. TODO(phase-9)
- Tenant management (suspension and limits) by staff, with four-eyes approval. TODO(phase-9)
