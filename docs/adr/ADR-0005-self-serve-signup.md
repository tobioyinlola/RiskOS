# ADR-0005: Self-serve sign-up with a production gate

- **Status:** accepted
- **Date:** 2026-10-01
- **Deciders:** product owner, engineering

## Context

Until now a tenant existed only after an operator ran `riskosctl tenant create` and invited its
first administrator. The marketing site's "Get started" needs people to create a workspace
themselves, with email and password or with Google, and RiskOS must learn enough about each
business (know your business) before it scores live traffic. Opening tenant creation to the
internet adds abuse (spam workspaces, resource use) and account-takeover risks that the
operator path never had.

## Decision

1. **Anyone can create a workspace** at `/signup` when `RISKOS_SIGNUP_ENABLED=true`: with email,
   business name and password, or with Google (OpenID Connect).
2. **Nothing is created before the email is proven**: by a 6-digit code (derived from the
   sign-up ID with a server key, never stored, one hour, five codes, five wrong tries), or by
   Google's `email_verified`. A sign-up with an email that already has an account looks the same
   to the requester; the email says so instead of carrying a code.
3. **One transaction** creates the tenant (both environments), its onboarding draft, the first
   administrator and a session. The tenants module creates the tenant (`SelfServe`); the auth
   module creates the user, as the only module allowed to open sessions.
4. **Every account still enrolls a RiskOS second factor**, Google sign-ins included.
5. **Production is gated**: a self-serve tenant's `production_enabled` is false until an
   operations admin approves its compliance onboarding in the admin realm. Console sessions
   cannot scope into production meanwhile (403 `PRODUCTION_LOCKED`); the test environment works
   at once. Operator-created tenants are unaffected.
6. **Abuse limits**: sign-up per IP (10/hour) and per email (5/hour), code checks per IP, and
   workspace creation per IP (5/day), all in Redis and failing open like sign-in throttling.

## Alternatives considered

- **Waitlist with staff approval before any access.** Safest, but slows evaluation to a crawl;
  the test environment carries no live money and is isolated per tenant.
- **Full access at sign-up.** Lets an unknown business run production traffic and live keys
  without any due diligence.
- **CAPTCHA on sign-up.** Deferred: rate limits plus proof of email stop bulk creation; a
  CAPTCHA can be added (Turnstile) if abuse appears, at the cost of a third-party script and a CSP
  change.
- **Google sign-in without a RiskOS second factor.** Rejected: one rule for every account is
  simpler to reason about and audit.

## Consequences

- Tenants now have an `origin` (operator or self-serve) and a production gate; the session
  lookup carries `production_enabled`.
- Users may have no password (Google-only); password sign-in treats them like unknown emails.
- A Google subject is bound to an account on its first Google sign-in; another Google account
  with the same email cannot take it.
- Sign-up with email needs SMTP; without it only Google sign-up is offered.
- Follow-ups: email the customer when onboarding is approved or sent back; consider a CAPTCHA and
  disposable-email checks if abuse appears; purge unfinished workspaces after a retention period.
