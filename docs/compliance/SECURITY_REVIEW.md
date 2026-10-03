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
