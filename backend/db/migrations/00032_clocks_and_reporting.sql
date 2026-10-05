-- +goose Up
-- Phase 5 of ADR-0007: clocks and reporting across countries.

-- Tasks carry their jurisdiction: the country whose rule requires the work (its deadline, its
-- authority) and the country that governed the event. They differ when a duty follows the
-- data subject (RW-SEC-04 on a Ghana-booked event); such events form the cross-border view.
ALTER TABLE compliance_tasks
  ADD COLUMN jurisdiction text CHECK (jurisdiction ~ '^[A-Z]{2}$'),
  ADD COLUMN event_jurisdiction text CHECK (event_jurisdiction ~ '^[A-Z]{2}$'),
  -- The body the report goes to (NFIU, FIC, NCSA), from the rule's task.
  ADD COLUMN authority text CHECK (char_length(authority) BETWEEN 1 AND 32);
-- Earlier tasks: a country rule's own prefix, else the tenant's fallback activation (the only
-- pack that judged events before ADR-0007), else Nigeria.
UPDATE compliance_tasks t SET jurisdiction = CASE
  WHEN t.rule_id ~ '^[A-Z]{2}-(REG|TM|SEC|GOV)-' THEN left(t.rule_id, 2)
  ELSE coalesce((SELECT a.jurisdiction FROM jurisdiction_activations a WHERE a.tenant_id = t.tenant_id AND a.fallback), 'NG')
END;
UPDATE compliance_tasks SET event_jurisdiction = jurisdiction WHERE event_id IS NOT NULL;
ALTER TABLE compliance_tasks ALTER COLUMN jurisdiction SET NOT NULL;
CREATE INDEX compliance_tasks_cross_border ON compliance_tasks (tenant_id, environment_id, created_at DESC)
  WHERE event_jurisdiction IS NOT NULL AND event_jurisdiction <> jurisdiction;

-- Public holidays an institution entered for a country it operates in: working-day clocks
-- (GH-REG-11's 7 working days) skip them, with weekends. RiskOS ships no holiday dates.
CREATE TABLE jurisdiction_holidays (
  tenant_id    uuid NOT NULL,
  jurisdiction text NOT NULL REFERENCES jurisdictions (code),
  day          date NOT NULL,
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  created_at   timestamptz NOT NULL,
  created_by   text NOT NULL,
  PRIMARY KEY (tenant_id, jurisdiction, day)
);
ALTER TABLE jurisdiction_holidays ENABLE ROW LEVEL SECURITY;
CREATE POLICY jurisdiction_holidays_isolation ON jurisdiction_holidays TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, DELETE ON jurisdiction_holidays TO riskos_app;

-- +goose Down
DROP TABLE jurisdiction_holidays;
DROP INDEX compliance_tasks_cross_border;
ALTER TABLE compliance_tasks DROP COLUMN authority, DROP COLUMN event_jurisdiction, DROP COLUMN jurisdiction;
