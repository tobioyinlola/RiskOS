# Compliance workflow

**Purpose:** how the work regulatory controls require reaches compliance staff, with its legal
deadlines, and how reports are prepared, approved and filed. Design:
[ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md). Status: **built (rule pack phase 5).**

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
| `report.STR` | A compliance officer confirming suspicion on a case (REG-14) | REG-14's `str_due` (24 hours) from confirmation |

Deadlines use the rule's settings in the environment. A retried job creates nothing twice (one
task per evaluation, rule and kind). Actions for the customer's own systems (fresh
authentication, enhanced MFA, routing through a financial institution) are not tasks: they are
returned with the decision in `controls`.

**Screening (REG-14).** The work queue also lists alerts still `open` past REG-14's
`screen_within` (72 hours) from their creation, for compliance roles to escalate. It is computed
when the queue is read, so it is always current.

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
`GET …/tasks/{id}/export`, `POST /api/v1/compliance/suspicion`. Approving, filing and confirming
suspicion need a recent second factor in the console. Every change is audited.

Tested in `backend/test/integration/compliance_tasks_test.go` and `gov09_test.go`.
