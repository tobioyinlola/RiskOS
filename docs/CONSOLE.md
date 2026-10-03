# Console

The RiskOS console is the dashboard where a tenant's people triage alerts, work cases, write
rules, tune risk settings and manage keys, webhooks and their team. It is a Next.js app in
`frontend/` built on the generated API client, and it follows the RiskOS design system
([design-system.md](design-system.md)). Sign-in, sessions and team management are described in
[CONSOLE_AUTH.md](CONSOLE_AUTH.md).

## Screens

| Section | Path | Needs | What it does |
|---|---|---|---|
| Overview | `/` | `events:read` or `alerts:read` | A greeting and quick actions; the mean score of recent events with its trend and a chart, interventions and the risk mix, open alerts and cases, and recent activity filtered by pill tabs |
| Live monitor | `/live` | `events:read` | Events as they arrive (checked every 2 seconds). New events wait above the table while you hover it, have an event open or pause, so rows never move under you; arrivals are announced politely to screen readers |
| Events | `/events` | `events:read` | Event explorer filtered by type, band, decision and user (in the URL, so a view can be shared); the drawer shows the evaluation (score, band, decision, reasons), links to the user, account and device, and the raw and normalized payloads |
| Transactions | `/transactions` | `events:read` | Transaction events with amount, direction, beneficiary and channel |
| Entities | `/entities`, `/entities/{kind}/{ref}` | `events:read` | Look up a user, account or device. Its profile shows activity, highest and mean score, decisions, and the users, accounts, devices, countries and IP addresses seen with it (from its most recent 5,000 events), then its events. Users in alerts, cases, events and transactions link here |
| Analytics | `/analytics` | `events:read` | Over 24 hours, 7 days or 30 days: events per hour or day by risk band (with a table view), mean score, decisions, highest-risk users and the signals that fired most |
| Alerts | `/alerts` | `alerts:read` | The triage queue, ordered by band, then score, then oldest first; the drawer shows each evaluation's reasons, the activity timeline and actions |
| Cases | `/cases`, `/cases/{id}` | `cases:read` | Investigations: linked alerts, notes, assignment, resolution |
| Rules | `/rules`, `/rules/new`, `/rules/{id}` | `rules:read` | Rules with versions, shadow mode, a test bench and rollback. In production, saving proposes a change; proposals awaiting approval are listed above the rules, and anyone with `rules:approve` other than the proposer can review the difference and approve or reject it |
| Deny lists | `/deny-lists` | `rules:read` or `risk:configure` | Identifiers whose events are always blocked: add with a reason and an optional expiry, filter by kind, remove. User, account and device profiles can add their identifier directly |
| Risk settings | `/settings` | `risk:configure` | Band thresholds, signal weights, high-value amounts, fail policies, the alert threshold, automatic blocking (shadow evidence, switch on, kill switch) |
| Developer | `/developer`, `/developer/webhooks/{id}` | `api_keys:read` or `webhooks:read` | API keys (create, rotate, revoke), webhook endpoints, delivery logs, test deliveries and redelivery |
| Rules library | `/compliance`, `/compliance/rules/{id}` | `compliance:read` (`compliance:write` to set the licence type) | The jurisdiction rule pack (ADR-0006): every rule in the library's own words with its source, effective date, verification status and caveat; filters by type, group, regulator, verification and what applies; bounded settings with their defaults; legal deadlines, open questions, glossary and sources. Engineering logic and the machine definition show only with `compliance:engineering`. Per environment: each rule's mode and settings in natural units (naira, minutes to days) within their bounds, with a 7-day backtest of new values against current ones, a version history with restore, and changes awaiting a second person (approve, reject, withdraw); exchange rates for US dollar rules. Configuring needs `compliance:configure`, approving `rules:approve`, rates `compliance:write` |
| Work queue | `/compliance/work` | `compliance:read` | What regulatory controls require: reports (FTR, CTR, PEP, sanctions, STR) and actions (due diligence, freezing or closing an account) with their legal deadlines as countdowns; draft editing, approval by a compliance officer, export and the filing reference; alerts past their screening deadline (REG-14) and urgent alerts nobody picked up within the fraud response target (GOV-11); confirmed-fraud cases not yet reported, with one click to prepare the industry fraud report (GOV-12). Suspicious transaction reports and compliance-only signals appear only for `compliance:sensitive`; compliance officers confirm suspicion from a case ([WORKFLOW.md](compliance/WORKFLOW.md)) |
| Single sign-on | `/sso` | `team:read` | The tenant's OpenID Connect provider: the redirect URI to register, issuer, client ID and secret, on or off, remove |
| Team | `/team` | `team:read` | Members, roles, invites (emailed when RiskOS has email configured; the link is also shown once to copy) |
| Audit log | `/audit-logs` | `audit_logs:read` | The tenant's audit trail |
| Account security | `/account` | signed in | Passkeys, authenticator app, recovery codes |
| Sign-up | `/signup`, `/signup/google` | no account | Create a workspace with email (a 6-digit emailed code) or Google, then a second factor ([CONSOLE_AUTH.md](CONSOLE_AUTH.md#sign-up)) |
| Onboarding | `/onboarding` | `team:read` (`team:write` to answer) | A self-serve workspace's compliance onboarding (below) |

A section appears in the navigation only when the user holds one of its permissions; the API
enforces the same permissions on every request.

Analytics are computed from the events when you open the page. TODO(phase-9): precompute
hourly rollups if 30-day windows become slow at production volumes.

## Onboarding

A self-serve workspace answers a compliance onboarding before its production environment opens.
`/onboarding` is a five-step page outside the console shell: business (legal name, country,
registration number, website, industry), regulation (regulator and licence, AML/CFT programme,
compliance officer, screening provider), use case (events, monthly volume, countries served,
go-live), data and privacy (data protection laws, residency, DPO, terms and data processing
agreement), then review and submit. Answers are saved at each step (`PUT /api/v1/onboarding`);
`missing` lists what submitting still needs. Submitted answers are frozen while RiskOS reviews
them ([ADMIN.md](ADMIN.md#onboarding-reviews)); a rejected onboarding shows the reviewer's note and
can be changed and submitted again. Until approval the overview shows a "Production is locked"
banner and the Production switch is disabled.

## Environments

One sign-in covers both of the tenant's environments. The switch in the top bar chooses one;
every request carries it in the `RiskOS-Environment` header, and every cached query is keyed by
it, so switching never shows one environment's data under the other. The choice is remembered
per browser. In production the console shows a PRODUCTION banner across the top.

## Layout and preferences

- **Navigation.** The left navigation (248px) has the workspace switcher on top, the main
  sections, then the Workflows and Organization groups and an onboarding card. It collapses to
  an icon rail with the button at its foot or with `[`; in the rail, each section's name shows
  on hover or focus. Narrow screens get a drawer instead.
- **Command palette.** `Ctrl+K` (`⌘K` on a Mac), or the search box in the top bar, opens the
  palette. Type to find a section or an action (switch environment, row density,
  navigation, shortcuts, sign out), then press Enter.
- **Preferences.** Table row density (Default 52px or Compact 40px) and the navigation state
  are kept per browser; the top bar has a density button. The console is light only.
- **Alerts bell.** The bell in the top bar opens the alert queue and shows a dot while alerts
  are open.
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
rules, risk settings and deny lists, approving rule changes, inviting people and changing members require a
second factor within the last 10 minutes.
When the API answers `REAUTH_REQUIRED`, the console asks for a passkey, authenticator code or
recovery code and retries the action once.

Analysts never block single events by hand. BLOCK comes from the deny list (for identifiers a
risk manager has listed) or from automatic blocking, which the Risk settings page controls:
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
