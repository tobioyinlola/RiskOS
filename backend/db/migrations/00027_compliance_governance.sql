-- +goose Up
-- Phase 6 of the rule pack (ADR-0006).
--
-- GOV-12: confirmed fraud is reported to the industry channel (NIBSS) by a person, from a draft
-- RiskOS prepares in one click; nothing is submitted automatically. One report per case, unless
-- it was dismissed.
ALTER TABLE compliance_tasks DROP CONSTRAINT compliance_tasks_kind_check;
ALTER TABLE compliance_tasks ADD CONSTRAINT compliance_tasks_kind_check CHECK (kind IN (
  'report.STR', 'report.CTR', 'report.FTR', 'report.PEP_monthly', 'report.sanctions', 'report.industry_fraud',
  'case.open', 'edd', 'source_of_funds', 'source_of_wealth', 'senior_approval',
  'freeze_account', 'close_account', 'escalate'));
CREATE UNIQUE INDEX compliance_tasks_case_report ON compliance_tasks (tenant_id, environment_id, case_id, kind)
  WHERE kind = 'report.industry_fraud' AND status <> 'dismissed';

-- The account a task concerns (the event's account reference), when known.
ALTER TABLE compliance_tasks ADD COLUMN account_ref text CHECK (char_length(account_ref) <= 128);
-- The daily check (REG-16) opens one task per account, whatever became of it.
CREATE UNIQUE INDEX compliance_tasks_account_once ON compliance_tasks (tenant_id, environment_id, rule_id, kind, account_ref)
  WHERE account_ref IS NOT NULL AND evaluation_id IS NULL;
CREATE INDEX compliance_tasks_account ON compliance_tasks (tenant_id, environment_id, account_ref)
  WHERE account_ref IS NOT NULL;

-- REG-16: the account register the daily check reads. It holds what events said about each
-- account's BVN (account.has_bvn), never the BVN itself.
CREATE TABLE compliance_accounts (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  account_ref    text NOT NULL CHECK (char_length(account_ref) BETWEEN 1 AND 128),
  subject_ref    text CHECK (char_length(subject_ref) <= 128),
  has_bvn        boolean NOT NULL,
  -- The earliest event about the account: REG-16's clock runs from it.
  first_seen_at  timestamptz NOT NULL,
  -- The event time of the latest BVN fact; an older event arriving late changes nothing.
  observed_at    timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, environment_id, account_ref),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX compliance_accounts_no_bvn ON compliance_accounts (tenant_id, environment_id, first_seen_at)
  WHERE NOT has_bvn;
ALTER TABLE compliance_accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY compliance_accounts_isolation ON compliance_accounts TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON compliance_accounts TO riskos_app;

-- The environments the daily check visits. Identifiers only: the job then works inside each
-- tenant's scope, under row-level security.
CREATE FUNCTION compliance_account_scopes()
  RETURNS TABLE (tenant_id uuid, environment_id uuid)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT DISTINCT a.tenant_id, a.environment_id FROM compliance_accounts a
  $$;
REVOKE ALL ON FUNCTION compliance_account_scopes() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION compliance_account_scopes() TO riskos_app;

-- +goose Down
DROP FUNCTION compliance_account_scopes();
DROP TABLE compliance_accounts;
DROP INDEX compliance_tasks_account;
DROP INDEX compliance_tasks_account_once;
ALTER TABLE compliance_tasks DROP COLUMN account_ref;
DROP INDEX compliance_tasks_case_report;
-- Tasks are never deleted: industry reports already recorded stay, and only new rows are checked.
ALTER TABLE compliance_tasks DROP CONSTRAINT compliance_tasks_kind_check;
ALTER TABLE compliance_tasks ADD CONSTRAINT compliance_tasks_kind_check CHECK (kind IN (
  'report.STR', 'report.CTR', 'report.FTR', 'report.PEP_monthly', 'report.sanctions',
  'case.open', 'edd', 'source_of_funds', 'source_of_wealth', 'senior_approval',
  'freeze_account', 'close_account', 'escalate')) NOT VALID;
