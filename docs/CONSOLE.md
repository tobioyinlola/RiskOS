# Console

The RiskOS console is the dashboard where a tenant's people triage alerts, work cases, write
rules, tune risk settings and manage keys, webhooks and their team. It is a Next.js app in
`frontend/` built on the generated API client, and it follows the RiskOS design system
([design-system.md](design-system.md)). Sign-in, sessions and team management are described in
[CONSOLE_AUTH.md](CONSOLE_AUTH.md).

## Screens

| Section | Path | Needs | What it does |
|---|---|---|---|
| Overview | `/` | `events:read` or `alerts:read` | Open alerts and cases, the risk distribution and decisions of recent events |
| Live monitor | `/live` | `events:read` | Events as they arrive (checked every 2 seconds). New events wait above the table while you hover it, have an event open or pause, so rows never move under you; arrivals are announced politely to screen readers |
| Events | `/events` | `events:read` | Event explorer filtered by type, band, decision and user (in the URL, so a view can be shared); the drawer shows the evaluation (score, band, decision, reasons), links to the user, account and device, and the raw and normalized payloads |
| Transactions | `/transactions` | `events:read` | Transaction events with amount, direction, beneficiary and channel |
| Entities | `/entities`, `/entities/{kind}/{ref}` | `events:read` | Look up a user, account or device. Its profile shows activity, highest and mean score, decisions, and the users, accounts, devices, countries and IP addresses seen with it (from its most recent 5,000 events), then its events. Users in alerts, cases, events and transactions link here |
| Analytics | `/analytics` | `events:read` | Over 24 hours, 7 days or 30 days: events per hour or day by risk band (with a table view), mean score, decisions, highest-risk users and the signals that fired most |
| Alerts | `/alerts` | `alerts:read` | The triage queue, ordered by band, then score, then oldest first; the drawer shows each evaluation's reasons, the activity timeline and actions |
| Cases | `/cases`, `/cases/{id}` | `cases:read` | Investigations: linked alerts, notes, assignment, resolution |
| Rules | `/rules`, `/rules/new`, `/rules/{id}` | `rules:read` | Rules with versions, shadow mode, a test bench and rollback. In production, saving proposes a change; proposals awaiting approval are listed above the rules, and anyone with `rules:approve` other than the proposer can review the difference and approve or reject it |
| Risk settings | `/settings` | `risk:configure` | Band thresholds, signal weights, high-value amounts, fail policies, the alert threshold, automatic blocking (shadow evidence, switch on, kill switch) |
| Developer | `/developer`, `/developer/webhooks/{id}` | `api_keys:read` or `webhooks:read` | API keys (create, rotate, revoke), webhook endpoints, delivery logs, test deliveries and redelivery |
| Team | `/team` | `team:read` | Members, roles, invites |
| Audit log | `/audit-logs` | `audit_logs:read` | The tenant's audit trail |
| Account security | `/account` | signed in | Passkeys, authenticator app, recovery codes |

A section appears in the navigation only when the user holds one of its permissions; the API
enforces the same permissions on every request.

Analytics are computed from the events when you open the page. TODO(phase-9): precompute
hourly rollups if 30-day windows become slow at production volumes.

## Environments

One sign-in covers both of the tenant's environments. The switch in the top bar chooses one;
every request carries it in the `RiskOS-Environment` header, and every cached query is keyed by
it, so switching never shows one environment's data under the other. The choice is remembered
per browser. In production the console shows a PRODUCTION banner across the top.

## Layout and preferences

- **Navigation.** The left navigation (240px) collapses to an icon rail with the button at its
  foot or with `[`. In the rail, each section's name shows on hover or focus. Narrow screens get
  a drawer instead.
- **Command palette.** `Ctrl+K` (`⌘K` on a Mac), or the search box in the top bar, opens the
  palette. Type to find a section or an action (switch environment, theme, row density,
  navigation, shortcuts, sign out), then press Enter.
- **Preferences.** The theme (dark Console by default, light Daylight), table row density
  (Default 40px or Compact 32px) and the navigation state are kept per browser. The top bar has
  buttons for theme and density. The theme is applied before the page paints, so it never
  flashes.
- **Evidence.** IDs in drawers and headers have a copy button, and code and payload blocks can
  be copied in one click.
- **Danger actions.** Revoking a key, disabling a webhook endpoint and removing a passkey each
  open a confirmation that names the object.

## Keyboard

`?` lists the shortcuts on every screen. `Ctrl+K` opens the command palette; `[` collapses or
expands the navigation. `g` then a letter moves between sections (`g a` alerts,
`g e` events, `g c` cases, `g r` rules, `g o` overview). In the alert queue: `j`/`k` move,
`Enter` opens, `i` marks investigating, `f` false positive, `x` resolves, `c` opens a case.
Shortcuts are ignored while typing in a field.

## High-impact actions

Creating, rotating or revoking API keys, changing webhook endpoints or their secrets, changing
rules and risk settings, approving rule changes, inviting people and changing members require a
second factor within the last 10 minutes.
When the API answers `REAUTH_REQUIRED`, the console asks for a passkey, authenticator code or
recovery code and retries the action once.

Analysts never block by hand. BLOCK comes only from automatic blocking, which the Risk settings
page controls:
- start shadow mode, see how many events it would have blocked, and switch it on after 14 days;
- stop blocking at once (the kill switch).

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
