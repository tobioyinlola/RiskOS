# Compliance workflow

**Purpose:** how the work regulatory controls require reaches compliance staff, with its legal
deadlines, and how reports are prepared, approved and filed. Design:
[ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md). Status: **built (rule pack phases 5 and 6).**

## Tasks

When a live control fires, the decision transaction enqueues a job (as alerts do); the worker
records a **compliance task** for each task the control names that RiskOS tracks:

| Task | From | Deadline |
|---|---|---|
| `report.FTR` | REG-10, international transfers of US$10,000 or more | REG-10's `ftr_due` (1 day) from the transfer |
| `report.CTR` | REG-17, cash at or above the threshold | none in the pack |
| `report.PEP_monthly` | REG-18, PEP transactions | none in the pack |
| `report.sanctions` | REG-11, confirmed sanctions matches | none in the pack |
| `edd`, `source_of_funds`, `source_of_wealth`, `senior_approval` | REG-12, politically exposed persons | none in the pack |
| `case.open`, `freeze_account`, `close_account` | REG-05, REG-11, REG-13, REG-16 | `close_after` for REG-16 |
| `close_account` | REG-16's daily check of accounts without a BVN | `close_after` (30 days) from when the account was first seen |
| `report.STR` | A compliance officer confirming suspicion on a case (REG-14) | REG-14's `str_due` (24 hours) from confirmation |
| `report.industry_fraud` | A person preparing it for a confirmed-fraud case (GOV-12) | none: the source states none |

Deadlines use the rule's settings in the environment. A retried job creates nothing twice (one
task per evaluation, rule and kind). Actions for the customer's own systems (fresh
authentication, enhanced MFA, routing through a financial institution) are not tasks: they are
returned with the decision in `controls`.

**Escalations.** The work queue also lists, for callers who read alerts:

- **Screening (REG-14):** alerts still `open` past REG-14's `screen_within` (72 hours) from
  their creation.
- **Fraud response (GOV-11):** `high` and `critical` alerts still `open` (nobody picked them up)
  past GOV-11's `respond_within` (30 minutes, configurable from 5 minutes to 4 hours). RiskOS
  reads "confirmed-fraud alerts" in the CBN's target as the urgent alerts it raises, because a
  case is confirmed only after someone acted on it; the escalation is the visible breach.

Both are computed when the queue is read, so they are always current.

**Unreported fraud (GOV-12).** For callers who read cases, the queue lists cases resolved as
`confirmed_fraud` without an industry fraud report (at most 200, newest first). **Prepare
report** (`POST /api/v1/compliance/fraud-reports`) opens a draft prefilled from the case (title,
when it was confirmed, the resolution note as the narrative); it then follows the report flow
below. A dismissed report puts the case back on the list. NIBSS's reporting format and deadline
were not found, so the export is a neutral record and no deadline is set.

**Accounts without a BVN (REG-16).** An event whose `compliance.facts` say
`account.has_bvn` for its `actor.account_id` updates an account register (the fact, never the
BVN; a late older event does not overwrite a newer one). A daily job then, in every environment
where REG-16 is live:

- opens one `close_account` task per account without a BVN that never had one, due
  `close_after` from when the account was first seen;
- marks open tasks done ("A BVN is now on file") for accounts that now have one.

It is idempotent and handles at most 20,000 accounts per environment per day. Accounts opened
before an institution adopted the pack are covered once any event about them carries the fact;
send it on `account_opened` and on profile changes.

## Reports

```
open (a draft) ──edit──▶ approved ──filed (with the regulator's reference)──▶ filed
       └──────────────── dismissed (with a note) ◀──────────────┘
```

- **Approval (GOV-06).** Only compliance officers (`compliance:sensitive`) approve a report.
- **Completeness (REG-19).** A suspicious transaction report is approved only when goAML would
  accept it: a predicate offence, at least one indicator, and a category goAML accepts. The pack
  rule itself decides; the refusal says what is missing.
- **Filing.** RiskOS never submits a report. A person files it in the regulator's portal (goAML
  for the NFIU), using the export, and records the reference the portal gave.
- Other tasks are marked done, or dismissed with a note.

## No tipping off (GOV-09)

- Suspicious transaction reports are visible only to compliance officers: listing omits them and
  reading one by ID answers 404 for anyone else, administrators included.
- The audit log records that compliance work happened (`compliance.task_created`,
  `compliance.report_approved`...) without saying, for a suspicious transaction report, what it is.
- Compliance-only signals are masked everywhere else ([RISK_ENGINE.md](../RISK_ENGINE.md#rule-packs)).

## API

`GET /api/v1/compliance/tasks`, `GET …/tasks/{id}`, `PATCH …/tasks/{id}/report`,
`POST …/tasks/{id}/approve`, `POST …/tasks/{id}/file`, `POST …/tasks/{id}/close`,
`GET …/tasks/{id}/export`, `POST /api/v1/compliance/suspicion`,
`POST /api/v1/compliance/fraud-reports`. Approving, filing and confirming suspicion need a recent
second factor in the console. Every change is audited.

Tested in `backend/test/integration/compliance_tasks_test.go`, `compliance_governance_test.go`
and `gov09_test.go`.
