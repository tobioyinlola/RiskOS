-- +goose Up
-- Compliance work (ADR-0006, phase 5): what fired regulatory controls require of the institution
-- (reports to the NFIU, enhanced due diligence, freezing or closing an account) and suspicious
-- transaction reports, each with its legal deadline. Reports are prepared here, approved by a
-- compliance officer (GOV-06) and filed by a person outside RiskOS: nothing is submitted
-- automatically.
CREATE TABLE compliance_tasks (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  id             uuid PRIMARY KEY,
  kind           text NOT NULL CHECK (kind IN (
    'report.STR', 'report.CTR', 'report.FTR', 'report.PEP_monthly', 'report.sanctions',
    'case.open', 'edd', 'source_of_funds', 'source_of_wealth', 'senior_approval',
    'freeze_account', 'close_account', 'escalate')),
  -- The pack rule that requires it (REG-10, REG-14...).
  rule_id        text NOT NULL CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$'),
  evaluation_id  uuid,
  event_id       uuid,
  case_id        uuid,
  -- The customer it concerns (the event's user reference), when known.
  subject_ref    text CHECK (char_length(subject_ref) <= 128),
  -- Suspicious transaction reports: only compliance officers see them (GOV-09).
  sensitive      boolean NOT NULL,
  -- Reports: open (a draft) -> approved -> filed; other tasks: open -> done. Both may be
  -- dismissed with a note.
  status         text NOT NULL CHECK (status IN ('open', 'approved', 'filed', 'done', 'dismissed')),
  due_at         timestamptz,
  report         jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(report) = 'object' AND octet_length(report::text) <= 16384),
  created_at     timestamptz NOT NULL,
  created_by     text NOT NULL,
  updated_at     timestamptz NOT NULL,
  approved_by    text,
  approved_at    timestamptz,
  closed_by      text,
  closed_at      timestamptz,
  close_note     text CHECK (char_length(close_note) <= 1000),
  -- The reference the regulator's portal gave when a person filed the report.
  filing_ref     text CHECK (char_length(filing_ref) <= 128),
  CHECK ((status = 'approved' OR status = 'filed') = (approved_at IS NOT NULL AND approved_by IS NOT NULL) OR status = 'dismissed'),
  CHECK ((status = 'filed') = (filing_ref IS NOT NULL)),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
-- A retried job creates nothing twice: one task per evaluation, rule and kind.
CREATE UNIQUE INDEX compliance_tasks_once ON compliance_tasks (tenant_id, environment_id, evaluation_id, rule_id, kind)
  WHERE evaluation_id IS NOT NULL;
CREATE INDEX compliance_tasks_queue ON compliance_tasks (tenant_id, environment_id, status, due_at);
CREATE TRIGGER compliance_tasks_no_delete BEFORE DELETE ON compliance_tasks
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
ALTER TABLE compliance_tasks ENABLE ROW LEVEL SECURITY;
CREATE POLICY compliance_tasks_isolation ON compliance_tasks TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON compliance_tasks TO riskos_app;

-- +goose Down
DROP TABLE compliance_tasks;
