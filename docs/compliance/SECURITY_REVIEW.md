# Rule pack security review

**Purpose:** the security review of the Nigeria rule pack and compliance workflow
([ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md)), rule pack phase 6. Each control
names the evidence that keeps it true. Reviewed 2026-10-03 against `staging`.

## Findings fixed in this review

| Finding | Severity | Fix |
|---|---|---|
| A busy entity made every event scan its whole window in Redis (up to 20,000 entries per window, per query). One tenant's popular biller or a deliberate flood of payments to one account slowed decisions for every tenant sharing Redis; the pack saturated at 250 events/s on the benchmark laptop | High (availability, cross-tenant) | Windows keep indexes: counts and distinct counts are one `ZCOUNT`, sums add per-bucket totals and scan only the edges, empty windows answer at once ([RISK_ENGINE.md](../RISK_ENGINE.md#pack-windows), [PERFORMANCE_BASELINE.md](../PERFORMANCE_BASELINE.md) §6). Exactness against the backtest windows and the cost on a 20,000-entry window are tested (`internal/risk/internal/store/windows_integration_test.go`) |
| CodeQL's high-severity gate failed on the sign-up path: a `"password"` literal passed to `createWorkspace` made the new session token look like a password hashed with SHA-256 | False positive, but the gate blocked merges | The method is derived inside the function (commit `d22ff88`) |

## Controls

| Control | How | Evidence |
|---|---|---|
| Tenant isolation | Every pack table (`tenant_compliance`, `pack_rule_versions`, `pack_rule_changes`, `compliance_fx_rates`, `compliance_tasks`, `compliance_accounts`) has row-level security with a policy on the authenticated tenant and environment; no query takes a tenant ID from the client. The daily check's cross-tenant listing is a `SECURITY DEFINER` function returning identifiers only, called by the worker | `TestEveryTenantTableIsIsolated` fails for any tenant table without RLS (three documented exceptions outside the pack); `tenancy_test.go` |
| Mandatory controls are not loosened silently | In production, any settings change to a control and any move away from live waits for a second person; versions and changes are append-only and audited; settings stay within each parameter's bounds | `pack_config_test.go` |
| No tipping off (GOV-09) | Suspicious transaction reports and compliance-only signals are visible only with `compliance:sensitive` (compliance officers; never administrators or API keys); elsewhere they read as `compliance_review`; the audit log omits what an STR is about | `gov09_test.go`, `compliance_tasks_test.go` |
| Nothing is submitted automatically | Reports (NFIU via goAML, industry fraud via NIBSS) are exported and filed by a person, who records the reference. The rules module makes no outbound calls (its only `net/http` use is status codes) | Code review; `compliance_governance_test.go` (filing needs approval and a reference) |
| People, not keys, act on compliance work | Confirming suspicion, approving, filing and preparing industry reports require a console user; approving, filing and confirming suspicion also require a recent second factor | `compliance_tasks_test.go`, `compliance_governance_test.go` (API key gets 403) |
| Compliance work is not destroyed | `compliance_tasks`, `pack_rule_versions`, `pack_rule_changes` and the audit log have no DELETE grant for the application role; tasks also reject deletes by trigger | `TestEveryTenantTableIsIsolated` |
| No raw personal data | Event `refs` refuse values that look like BVNs or phone numbers (`422 SENSITIVE_DATA`); the account register stores whether an account has a BVN, never the BVN; identifiers in Redis are HMACs; rule examples use references (`usr_4812`, `acc_…`) | `compliance_events_test.go`; library review |
| No scripting from customers | Rules are JSON interpreted by Go (`internal/rules/internal/interp`); nothing a customer sends is evaluated as code. The Redis Lua scripts are fixed server code | Code review |
| Missing data is not "no risk" | A rule whose data is missing is `not_evaluated`; a window with an unchecked filter is unknown; a late event never makes a distinct count smaller | `interp` tests; `windows_integration_test.go` |
| Bounded work | Queue listings are capped (500 tasks, 200 escalations, 200 unreported cases); the daily check handles 500 accounts per transaction and 20,000 per environment per day; report fields and notes have length limits; windows are capped at 20,000 entries | Code review; migration constraints |

## Accepted risks

- **Account references.** `actor.account_id` is the customer's reference for an account. If an
  institution sends raw account numbers there, they are stored as they already are on every
  event. The API documentation asks for references.
- **Distinct counts past the window cap** may include values seen only in evicted entries (more
  risk, never less). The cap (20,000 entries in a window) is far above the pack's settings.
- **Industry fraud reporting deadline.** The source does not state one, so none is set; the queue
  tracks unreported cases instead.
- **Design-peak throughput** is not demonstrated on the benchmark laptop
  ([PERFORMANCE_BASELINE.md](../PERFORMANCE_BASELINE.md) §6); re-measure on separated hosts
  (TODO(phase-8)).
- **Overload raises an alert per degraded decision.** When Redis is overloaded, decisions degrade
  (fail closed) and each raises a security alert job, which adds PostgreSQL load during the
  overload (seen at 250 events/s before the window fix). Grouping still yields one alert per
  subject; the job volume is the cost. Revisit with rate-based alerting on separated hosts.

## Several jurisdictions (ADR-0007)

The review of everything ADR-0007 added (Ghana and Rwanda packs, activations and their
approvals, open-question verification, the library export, working-day clocks and holidays,
data-subject duties, country tasks, the clock load, the cross-border view and retention).
Reviewed 2026-10-05 against `staging`.

### Findings fixed

| Finding | Severity | Fix |
|---|---|---|
| `days_over` (GH-TM-07: days a wallet's debits reached its daily limit) read and parsed every entry of its 7-day window, three times per event. An agent wallet with 20,000 debits in a week cost 75 to 91 ms of Redis time per event: one busy wallet would stall decisions for every tenant sharing Redis | High (availability, cross-tenant) | Bucket sizes divide an hour or a day, so no bucket straddles a local day; days are totalled from whole buckets plus the two partial edges, once per window and day offset; misaligned offsets and small windows scan. 1.4 to 1.7 ms per event, the write included; exact against the backtest on random streams (`windows_integration_test.go`, PERFORMANCE_BASELINE.md §7) |
| A compliance-only control (GH-REG-11, an FIC suspension or freeze) was returned with its rule, reason (`fic_order`) and tasks in the decision, the stored evaluation and the work queue: telling the institution's systems, and anyone reading them, that the FIC had acted (tipping off is an offence under Act 1044 s.38(4)) | High (confidentiality, legal) | Fixed in phase 5 (`8ca485a`): the control reads as `compliance_review` with its decision and hold kept, its work is sensitive, and a test fires every compliance-only rule of the Ghana and Rwanda packs and walks every payload others can read (`clocks_reporting_test.go`) |
| The retention check ignored retention rules switched off in the environment. Switching off a governance preset needs no second person, so one person could make every deletion check answer "allowed" | Medium (integrity of records) | Retention minimums count whatever their mode; the API shows each rule's mode (`clocks_reporting_test.go`) |
| Library export cells starting with `=`, `+`, `-` or `@` would be read as formulas when the CSV is opened in a spreadsheet. The cells hold the library's own text today, not tenant input | Low (defence in depth) | Such cells are prefixed with an apostrophe (`TestSpreadsheetSafe`) |
| Public holidays had no date bound: every pack compile loads them, and a far-future date could only stretch a hold | Low | A holiday is within a year back and three years ahead |

### Controls

| Control | How | Evidence |
|---|---|---|
| Tenant isolation | `jurisdiction_activations`, `activation_changes`, `gap_verifications` and `jurisdiction_holidays` have row-level security on the authenticated tenant; `jurisdictions` and the packs are reference data the application can only read | `TestEveryTenantTableIsIsolated`; `jurisdictions_test.go` (another tenant's booking entity resolves to nothing) |
| No country is guessed | One governing pack per event: the booking entity's activation, else the stated fallback, else none; a draft or suspended entity is judged by no other country. Data-subject duties run only from the tenant's own activation in that country | `jurisdictions_test.go`, `clocks_reporting_test.go` |
| Going live is never one person's act | Activations start as drafts; going live, leaving live, editing a live activation and changing the fallback need another person with `rules:approve`, readiness is checked again on approval, and every change is kept (decided once, never deleted) and audited | `activation_workflow_test.go` |
| People, not keys, configure countries | Activations, approvals, open-question verification and holidays need a console user (`compliance:configure` or `rules:approve`); API keys get 403 | `activation_workflow_test.go`, `clocks_reporting_test.go` |
| No tipping off in any country | Compliance-only signals and controls of every pack read as `compliance_review`; their work, suspicious transaction reports and the counts of both are seen only with `compliance:sensitive`, also in the clock load and the cross-border view | `gov09_test.go`, `clocks_reporting_test.go` |
| Nothing is filed or deleted by RiskOS | Reports to the FIC and NCSA are exported and filed by a person; the retention check answers and audits, it deletes nothing | Code review; `clocks_reporting_test.go` |
| Exports are safe to open and audited | Filenames come from validated country codes; CSV cells cannot start a formula; the PDF writer escapes its strings; each export is audited | `TestSpreadsheetSafe`, `library_countries_test.go` |
| Links stay links | An open question's verification link must be `http(s)`, at most 500 characters; the console opens it with `rel="noreferrer noopener"` | `activation_workflow_test.go`; code review |
| Bounded work | The cross-border view lists 100 events; the clock load is one grouped query; holidays are bounded by date; days are totalled from at most about 1,024 buckets per window | Code review; `windows_integration_test.go` |

### Accepted risks

- **Holidays lengthen holds.** A person with `compliance:configure` can enter a holiday that is
  not one, holding funds a day longer than the law allows. Every holiday is audited and shown
  with who entered it.
- **Holidays are entered, not shipped.** A country whose holidays nobody entered counts only
  weekends, so a hold may end before a public holiday it should skip. Readiness warns while
  none is entered for the coming year.
- **Retention years are 365 days.** The packs state retention in seconds (5 years =
  157,680,000 s), about one to three days short of calendar years. Institutions that need the
  calendar date can raise the setting.
- **A retention check per call loads the activated packs' configuration.** API keys are rate
  limited; heavy disposal jobs should batch and pace their calls.
