# Console

The RiskOS console is the dashboard where a tenant's people triage alerts, work cases, write
rules, tune risk settings and manage keys, webhooks and their team. It is a Next.js app in
`frontend/` built on the generated API client, and it follows the Instrument design system
([design-system.md](design-system.md)). Sign-in, sessions and team management are described in
[CONSOLE_AUTH.md](CONSOLE_AUTH.md).

## Screens

| Section | Path | Needs | What it does |
|---|---|---|---|
| Overview | `/` | `events:read` or `alerts:read` | Open alerts and cases, the risk distribution and decisions of recent events |
| Events | `/events` | `events:read` | Event explorer with filters; the drawer shows the evaluation: score, band, decision, reasons, raw and normalized payloads |
| Alerts | `/alerts` | `alerts:read` | The triage queue, ordered by band, then score, then oldest first; the drawer shows each evaluation's reasons, the activity timeline and actions |
| Cases | `/cases`, `/cases/{id}` | `cases:read` | Investigations: linked alerts, notes, assignment, resolution |
| Rules | `/rules`, `/rules/new`, `/rules/{id}` | `rules:read` | Rules with versions, shadow mode, a test bench and rollback. In production, saving proposes a change; proposals awaiting approval are listed above the rules, and anyone with `rules:approve` other than the proposer can review the difference and approve or reject it |
| Risk settings | `/settings` | `risk:configure` | Band thresholds, signal weights, high-value amounts, fail policies, the alert threshold |
| Developer | `/developer`, `/developer/webhooks/{id}` | `api_keys:read` or `webhooks:read` | API keys (create, rotate, revoke), webhook endpoints, delivery logs, test deliveries and redelivery |
| Team | `/team` | `team:read` | Members, roles, invites |
| Audit log | `/audit-logs` | `audit_logs:read` | The tenant's audit trail |
| Account security | `/account` | signed in | Passkeys, authenticator app, recovery codes |

A section appears in the navigation only when the user holds one of its permissions; the API
enforces the same permissions on every request.

Not built yet: the live risk monitor, entity pages (users, accounts, devices, transactions)
and analytics beyond the overview's counts. TODO(phase-8).

## Environments

One sign-in covers both of the tenant's environments. The switch in the top bar chooses one;
every request carries it in the `RiskOS-Environment` header, and every cached query is keyed by
it, so switching never shows one environment's data under the other. The choice is remembered
per browser. In production the console shows a PRODUCTION banner across the top.

## Keyboard

`?` lists the shortcuts on every screen. `g` then a letter moves between sections (`g a` alerts,
`g e` events, `g c` cases, `g r` rules, `g o` overview). In the alert queue: `j`/`k` move,
`Enter` opens, `i` marks investigating, `f` false positive, `x` resolves, `c` opens a case.
Shortcuts are ignored while typing in a field.

## High-impact actions

Creating, rotating or revoking API keys, changing webhook endpoints or their secrets, changing
rules and risk settings, approving rule changes, inviting people and changing members require a
second factor within the last 10 minutes.
When the API answers `REAUTH_REQUIRED`, the console asks for a passkey, authenticator code or
recovery code and retries the action once.

The console never offers a Block action: the engine does not return BLOCK until auto-block
ships (build step 8).

## How it talks to the API

- The browser calls `/api/v1/*` on the console's own origin. A route handler
  (`app/api/v1/[...path]/route.ts`) forwards those calls to `RISKOS_API_URL`. It forwards only
  cookies and the console's own headers, and passes `Set-Cookie` back. The session cookie can
  therefore stay HttpOnly and `SameSite=Strict`, and the API's address stays server-side.
- The client is `openapi-fetch` over the types generated from the contract
  (`lib/api/schema.gen.ts`). Middleware adds the environment header and the CSRF token to every
  request.
- Server state lives in TanStack Query (`lib/queries.ts`); actions invalidate the queries they
  affect (`lib/action.ts`).

## Browser security

| Control | Where |
|---|---|
| Content-Security-Policy with a fresh nonce per page: `script-src 'self' 'nonce-…' 'strict-dynamic'`, `frame-ancestors 'none'`, `object-src 'none'`, `base-uri 'none'`, `connect-src 'self'`. Styles allow inline attributes because React renders `style` props into the HTML | `frontend/proxy.ts` |
| `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, a strict referrer policy and a restrictive permissions policy | `frontend/next.config.ts` |
| Pages are rendered per request, so every script carries that request's nonce | `frontend/app/layout.tsx` |
| Redirects after sign-in follow only same-site relative paths | `features/auth/login.tsx` |
| The invite token travels in the URL fragment, which never reaches a server log | `features/auth/login.tsx` |

## Tests

| Suite | What it covers | Command |
|---|---|---|
| Unit and component (`frontend/tests/unit`) | Risk bands and readouts, formatting, passkey encoding, navigation permissions, the core components, queue order | `make test-frontend` |
| Browser, mocked API (`tests/e2e/console.spec.ts`) | Sign-in redirect, wrong password, passkey enrollment and sign-in with a virtual authenticator, recovery codes, every section on desktop and mobile, the environment switch, sign-out, the keyboard queue, security headers and the CSP nonce. Fails on any CSP violation or a request without the CSRF token or environment | `make test-e2e` |
| Browser, real stack (`tests/e2e/stack.spec.ts`) | Against `make up`, with no mocks: accepts a fresh invite, registers a passkey the API verifies, opens every section in both environments with no error state, signs out and back in with the passkey | `make test-e2e-stack` |

CI runs all three.
