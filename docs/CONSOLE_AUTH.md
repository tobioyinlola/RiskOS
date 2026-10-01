# Console authentication

**Purpose:** how people sign in to the RiskOS console, how their sessions are protected, and how
tenants manage their team. Design: [ARCHITECTURE.md §9 and §16.7](ARCHITECTURE.md#9-security-architecture).
Status: **built (build step 7).** API keys (machine access) are unchanged ([TENANCY.md](TENANCY.md)).

## Accounts

- A user belongs to exactly one tenant and sees both of its environments (test and production).
  Emails are unique across RiskOS; one email signs in to one tenant.
- Roles: `customer_admin`, `developer`, `risk_manager`, `analyst`, with the same permissions as
  API key scopes (`platform/authz`).
- **Getting an account.** RiskOS does not send email yet. A tenant's first administrator is
  invited by an operator (`riskosctl user invite --tenant ten_… --email … --operator …`), and
  administrators invite others from the console (Team). Either way the result is a single-use
  link, valid for 72 hours, shown once, with the token in the URL fragment so it never reaches
  server logs or `Referer` headers. The invited person sets their name and password and must then
  enroll a second factor before they can do anything else.

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

## Deviations and pending work

- Single sign-on (OIDC or SAML) for customer administrators is not built. TODO(phase-8).
- No email delivery: invites are links the administrator passes on. TODO(phase-8).
- Passkey ceremonies are verified by the go-webauthn library; the integration tests cover their
  request and rejection paths. The stack browser test (`make test-e2e-stack`, in CI) registers a
  passkey with a virtual authenticator and signs in with it against the real API
  ([CONSOLE.md](CONSOLE.md#tests)).
