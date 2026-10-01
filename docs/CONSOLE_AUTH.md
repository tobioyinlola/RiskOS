# Console authentication

**Purpose:** how people sign in to the RiskOS console, how their sessions are protected, and how
tenants manage their team. Design: [ARCHITECTURE.md §9 and §16.7](ARCHITECTURE.md#9-security-architecture).
Status: **built (build step 7).** API keys (machine access) are unchanged ([TENANCY.md](TENANCY.md)).

## Accounts

- A user belongs to exactly one tenant and sees both of its environments (test and production).
  Emails are unique across RiskOS; one email signs in to one tenant.
- Roles: `customer_admin`, `developer`, `risk_manager`, `analyst`, with the same permissions as
  API key scopes (`platform/authz`).
- **Getting an account.** A tenant's first administrator is invited by an operator
  (`riskosctl user invite --tenant ten_… --email … --operator …`, which prints the link), and
  administrators invite others from the console (Team). Either way the result is a single-use
  link, valid for 72 hours, shown once, with the token in the URL fragment so it never reaches
  server logs or `Referer` headers. When email is configured, console invites are also emailed
  ([Invite emails](#invite-emails)). The invited person sets their name and password and must
  then enroll a second factor before they can do anything else.

## Sign-in

1. Email and password (`POST /api/v1/auth/login`). Passwords: 12 to 128 characters, not the
   email address, not found in known data breaches (below), stored only as Argon2id hashes
   (m=64 MiB, t=3, p=2). Unknown emails, wrong
   passwords and disabled users get the same `401 INVALID_CREDENTIALS`, and take the same time.
2. A mandatory second factor: a **passkey** (WebAuthn, user verification required, preferred)
   or an **authenticator-app code** (TOTP, RFC 6238, each code accepted once). Ten single-use
   **recovery codes** are issued at the first enrollment (shown once, stored hashed).
3. The session becomes active for 12 hours, ending after 30 minutes without activity.

Attempts are throttled in Redis: 10 per email and 60 per client address per 15 minutes for
passwords, 5 per user per 5 minutes for second factors (`429` with `Retry-After`). The client
address is the browser's, read through the console's server and any load balancer listed in
`RISKOS_TRUSTED_PROXIES` (only addresses those proxies added are believed). Every failed
authentication also counts against the address's API-wide budget (see
[SECURITY.md](SECURITY.md)).

**Breached passwords.** A new password is checked against the Pwned Passwords range API with
k-anonymity: only the first five hexadecimal characters of its SHA-1 hash leave RiskOS, and
responses are padded. A password found there is refused. If the service cannot be reached
within 3 seconds, the password is accepted and the failure logged, so an outside outage cannot
stop people from joining. `RISKOS_BREACHED_PASSWORD_CHECK=off` disables the check (for
deployments without outbound internet access); `RISKOS_BREACHED_PASSWORD_URL` points it at a
mirror. Failures,
sign-ins, enrollments, re-authentications and sign-outs are in the tenant's audit log.

## Sessions

- The session token is a 256-bit random value in an `HttpOnly`, `SameSite=Strict` cookie
  (`Secure` everywhere except local development over http, which configuration refuses in
  production). Only its SHA-256 hash is stored, with the user agent and address.
- **CSRF:** every unsafe cookie-authenticated request must carry `X-CSRF-Token` (an HMAC of the
  session, returned by `GET /api/v1/auth/session`) and, when the browser sends one, an `Origin`
  from the configured console origins.
- **Environment:** a session request chooses `RiskOS-Environment: test|production` (default
  `test`); the console shows a production banner.
- **Re-authentication:** creating, rotating or revoking API keys, changing webhooks, risk
  settings or rules, and inviting or changing team members require a second-factor confirmation
  within the last 10 minutes (`403 REAUTH_REQUIRED` otherwise). API keys are not affected.
- Changing a user's role or status ends all their sessions, so new permissions apply at once.
- The tenant always keeps an active administrator, and nobody can change their own access.

## Secrets and configuration

| Setting | Purpose |
|---|---|
| `RISKOS_CONSOLE_SECRET` | At least 32 bytes. Encrypts TOTP secrets (AES-256-GCM, bound to the user) and keys CSRF tokens. Rotating it invalidates enrolled authenticator apps. |
| `RISKOS_CONSOLE_URL` | The console's base URL (invite links); https in production. |
| `RISKOS_CONSOLE_ORIGINS` | Origins allowed to make cookie-authenticated requests (default: the URL's origin). |
| `RISKOS_WEBAUTHN_RP_ID` | Passkey relying party ID (default: the URL's host name). |
| `RISKOS_SESSION_COOKIE_SECURE` | `true` by default and required in production. |

## Single sign-on

A tenant can connect one OpenID Connect identity provider (Team → Single sign-on, or
`PUT /api/v1/sso`). People then choose "Sign in with single sign-on" and enter their work email.

- **Existing accounts only.** SSO signs in people who already have an account (invited as usual).
  It never creates accounts or changes roles. Password sign-in keeps working.
- **RiskOS's second factor still applies.** After the provider, the session needs a passkey,
  authenticator code or recovery code, exactly as after a password.
- **The flow.** Authorization code with PKCE (S256), `state` and `nonce`. RiskOS verifies the ID
  token: its signature against the provider's published keys, the issuer, the audience (the client
  ID), the expiry and the nonce. The PKCE verifier and the nonce are derived from the state with a
  key derived from `RISKOS_CONSOLE_SECRET`, so only the state's hash is stored, for ten minutes, and
  it works once.
- **Login CSRF.** Starting SSO sets a ten-minute `riskos_sso` cookie (HttpOnly, SameSite=Lax,
  scoped to the SSO endpoints); the callback must come from the browser that holds it.
- **Account binding.** The first SSO sign-in needs the provider to confirm the account's email
  (`email_verified: true` and the same address). RiskOS then binds the provider's subject (`sub`)
  to the account, and later sign-ins must present that subject: an email reassigned at the provider
  cannot take over the account. A subject belongs to one account. Changing the issuer or removing
  SSO forgets the bindings.
- **The client secret** is stored encrypted (AES-256-GCM under the console secret, bound to the
  tenant) and never returned. Changes are audited (`sso.configured`, `sso.removed`) without it and
  need re-authentication; sign-ins are audited (`user.sso_authenticated`, `user.sso_failed` with a
  reason).
- **Reaching providers.** Discovery, keys and token requests go through an SSRF-safe client: https
  only, no private or special-purpose addresses, no redirects, 5-second timeouts, and responses
  capped at 1 MiB. Saving a
  connection reads the issuer's discovery document first and refuses an issuer it cannot read.
  `RISKOS_SSO_ALLOW_PRIVATE=true` allows http and private addresses for local providers; production
  refuses it.
- **Known trade-off.** Starting SSO answers differently for an email with SSO (a redirect) and one
  without (404 `SSO_NOT_CONFIGURED`), so it reveals that an account uses SSO. Attempts are
  throttled per email and per IP, like password sign-in.

## Invite emails

With `RISKOS_SMTP_HOST` set, each console invite queues a job, in the invite's transaction, that
the worker runs to email the link. Any SMTP provider works; locally, `docker compose` runs
Mailpit, which catches every message (inbox at http://localhost:8025).

- **No token in the queue.** Job arguments hold the tenant and invite IDs only. The token is
  derived from the invite ID with a key derived from `RISKOS_CONSOLE_SECRET` (HMAC-SHA256, 256
  bits), and the worker derives it again to build the link. The worker therefore needs
  `RISKOS_CONSOLE_SECRET` when email is on.
- **Only pending invites.** An invite accepted, revoked or expired before the job runs is not
  emailed, and neither is one whose tenant is suspended.
- **At least once.** A failed send is retried by the queue. A send whose confirmation was lost can
  deliver the same link twice; the link still works only once.
- **Recorded.** The invite shows when it was emailed (`email_sent_at`), and the tenant's audit log
  records `user.invite_emailed`.
- **Plain text** only, so nothing in the message is rendered as HTML. Header values are encoded,
  so a tenant's name cannot inject headers.

| Setting | Purpose |
|---|---|
| `RISKOS_SMTP_HOST` | SMTP server; empty (the default) turns email off. |
| `RISKOS_SMTP_PORT` | Default 587. |
| `RISKOS_SMTP_USERNAME`, `RISKOS_SMTP_PASSWORD` | Optional credentials (sent only over TLS, except to localhost). |
| `RISKOS_SMTP_FROM` | The sender, e.g. `RiskOS <no-reply@example.com>`. Required when email is on. |
| `RISKOS_SMTP_TLS` | `starttls` (default), `tls` (implicit, port 465) or `none` (local catchers; refused in production). |
| `RISKOS_SMTP_TIMEOUT` | Bounds one send; default 10 s. |

## Deviations and pending work

- SAML is not supported; SSO is OpenID Connect only.
- Requiring SSO (turning off password sign-in for a tenant) and provisioning accounts from the
  provider (SCIM or just-in-time) are not built. TODO(phase-9)
- Operator invites (`riskosctl`) print the link and are not emailed.
- Passkey ceremonies are verified by the go-webauthn library; the integration tests cover their
  request and rejection paths. The stack browser test (`make test-e2e-stack`, in CI) registers a
  passkey with a virtual authenticator and signs in with it against the real API
  ([CONSOLE.md](CONSOLE.md#tests)).
