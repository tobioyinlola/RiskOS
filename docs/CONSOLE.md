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
| Rules | `/rules` (tabs `presets`, `custom`, `references`), `/rules/new`, `/rules/{id}`, `/compliance/rules/{id}` | `rules:read` or `compliance:read` | One page for every rule, with no code anywhere. **Countries** (ADR-0007): a switcher above Preset rules and Deadlines and references picks the country (`?country=GH`), each named with its flag and where the workspace stands (default country, switched on, monitoring, not switched on); **All countries** shows every country's currency, time zone, regulators, rule counts and booking entities. **Preset rules:** the chosen country's pack, each rule in plain words with its type (required by law, risk signal, governance) and status, and for country libraries where it comes from (regulatory, industry practice or the library's design, explained in a legend; "From Nigeria's library"; "Based on ATO-01"; "Off until a threshold is confirmed"), with a filter by origin. Above a country's rules, its **activations** (one per booking entity): status (draft, monitoring, live, suspended), licence type, capabilities, the hour its business day starts, who proposed and approved going live, a readiness check listing what blocks going live (required settings, a missing exchange rate) and warnings (unverified open questions), the moves allowed (start monitoring, go live, back to monitoring, suspend, make default country, each with a rationale; going live, leaving live and changing the default wait for a second person with `rules:approve`, who approves or rejects them there), and the change history; below them, the country's **public holidays**, which its working-day clocks skip (entered and removed by `compliance:configure`; a readiness warning while none is entered for the coming year). **Activate {country}** opens a short form: booking entity, licence type, what the entity offers, day start, rationale; it creates a draft. Rules of every activated country are configured on their own pages (`compliance:configure`); a rule off until a setting is confirmed is switched on only with that setting entered; turning off a legal control asks why, and in production waits for a second person. A banner asks to confirm the institution type while the profile is the automatic one. A preset's page (`?jurisdiction=GH`) shows its wording, where it comes from (provenance, lineage and the library's basis verbatim), penalty exposure for regulatory rules, source, caveat, bounded settings in natural units, a 7-day backtest, history with restore, and approvals; never its engineering logic or machine definition. **Your rules:** first, the preset rules running for every activated country (live or monitoring), with booking entity and the settings each still needs, the same list developers read from `GET /api/v1/compliance/active-rules`; then the institution's own rules, built as When (event types) → If (conditions joined by and/or, in groups) → Then (risk points and severity), with a live plain-English sentence; new rules start by monitoring. Countries and currencies are picked by name. **Try it on recent events** runs the rule as it stands (saved or not) on the last 1, 7 or 30 days of events and shows how many it would have matched, with links to them. The tab counts active, monitoring and off rules, and each rule shows its matches over the last 7 days (a bar per day) and its last match; a rule's Activity tab shows matches per day over 7 or 30 days and its share of all events. Values are typed in natural units (naira, not kobo; countries by code, shown by name). A rule made through the API with logic the form cannot show is shown in words. In production saving proposes the change for another person with `rules:approve`. **Deadlines and references**, per country: deadlines labelled legal clock, internal policy, design placeholder or not found (never zero), open questions as conflicts or unverified items with how each is treated until settled (each can be marked verified against the original text, with a note and a link, shown on every rule it names), the **deadlines matrix**: the work running against each clock now (running, overdue, due within 24 hours; compliance-only work counted for compliance officers only), cross-border events (those that started a clock in a country other than the one that governed them, such as a Rwandan data subject's breach booked in Ghana), the retention in force per record type with the rule and country that set it, and the countries side by side (library section 6), glossary, sources, penalties and exchange rates, and downloads of the country's library (or every country's) as a spreadsheet or a PDF, audited. `/compliance` redirects here |
| Deny lists | `/deny-lists` | `rules:read` or `risk:configure` | Identifiers whose events are always blocked: add with a reason and an optional expiry, filter by kind, remove. User, account and device profiles can add their identifier directly |
| Risk settings | `/settings` | `risk:configure` | Band thresholds, signal weights, high-value amounts, fail policies, the alert threshold, automatic blocking (shadow evidence, switch on, kill switch) |
| Developer | `/developer`, `/developer/webhooks/{id}` | `api_keys:read` or `webhooks:read` | API keys (create, rotate, revoke), webhook endpoints, delivery logs, test deliveries and redelivery |
| Work queue | `/compliance/work` | `compliance:read` | What regulatory controls require in every activated country: reports (FTR, CTR, PEP, sanctions, STR, and Ghana's and Rwanda's reports to the FIC, NCSA and others) and actions (due diligence, freezing or closing an account) with their legal deadlines as countdowns, each with its country and authority, filterable by country, and marked when it is owed to a data subject in another country than the one that booked the event; where an approved report is filed (goAML for the NFIU, the institution's channel for an FIC); draft editing, approval by a compliance officer, export and the filing reference; alerts past their screening deadline (REG-14) and urgent alerts nobody picked up within the fraud response target (GOV-11); confirmed-fraud cases not yet reported, with one click to prepare the industry fraud report (GOV-12). Suspicious transaction reports and compliance-only signals appear only for `compliance:sensitive`; compliance officers confirm suspicion from a case, choosing the country the report is owed to when the workspace operates in several ([WORKFLOW.md](compliance/WORKFLOW.md)) |
| Single sign-on | `/sso` | `team:read` | The tenant's OpenID Connect provider: the redirect URI to register, issuer, client ID and secret, on or off, remove |
| Team | `/team` | `team:read` | Members, roles, invites (emailed when RiskOS has email configured; the link is also shown once to copy) |
| Audit log | `/audit-logs` | `audit_logs:read` | The tenant's audit trail |
| Account security | `/account` | signed in | Passkeys, authenticator app, recovery codes |
| Sign-up | `/signup`, `/signup/google` | no account | Create a workspace with email (a 6-digit emailed code) or Google, then a second factor ([CONSOLE_AUTH.md](CONSOLE_AUTH.md#sign-up)) |
| Onboarding | `/onboarding` | `team:read` (`team:write` to answer) | A self-serve workspace's compliance onboarding (below) |

A section appears in the navigation only when the user holds one of its permissions; the API
enforces the same permissions on every request.

Analytics read hourly rollups of events and signals for the hours that are finished, and the raw
events for the hour in progress; the worker keeps the rollups up to date every hour
([PERFORMANCE_BASELINE.md](PERFORMANCE_BASELINE.md#8-analytics-at-volume-rollups)). The numbers
are the same as aggregating the raw events, with one visible difference: an event that is
recorded into an hour that has already finished (a transaction that committed late) is counted
in that hour's bar from the next hourly run, not at once. The highest-risk users are read from
the events by score, never from the rollup.

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
