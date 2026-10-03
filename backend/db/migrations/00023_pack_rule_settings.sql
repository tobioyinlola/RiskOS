-- +goose Up
-- Per-environment configuration of rule pack rules (ADR-0006): a rule's mode and its settings.
-- Every change is a new immutable version; a rule's configuration is its latest version, and a
-- rule without one runs with the library's defaults (version 0).
CREATE TABLE pack_rule_versions (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  pack_id        text NOT NULL,
  rule_id        text NOT NULL CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$'),
  version        integer NOT NULL CHECK (version >= 1),
  mode           text NOT NULL CHECK (mode IN ('live', 'shadow', 'off')),
  -- Settings that differ from the library's defaults, by name; values are checked against the
  -- pack's bounds before they are stored.
  settings       jsonb NOT NULL CHECK (jsonb_typeof(settings) = 'object' AND octet_length(settings::text) <= 4096),
  reason         text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
  created_at     timestamptz NOT NULL,
  created_by     text NOT NULL,
  -- The approved change that produced this version, when one was needed.
  change_id      uuid,
  restored_from  integer,
  PRIMARY KEY (tenant_id, environment_id, rule_id, version),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE TRIGGER pack_rule_versions_no_update BEFORE UPDATE ON pack_rule_versions
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
CREATE TRIGGER pack_rule_versions_no_delete BEFORE DELETE ON pack_rule_versions
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
ALTER TABLE pack_rule_versions ENABLE ROW LEVEL SECURITY;
CREATE POLICY pack_rule_versions_isolation ON pack_rule_versions TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON pack_rule_versions TO riskos_app;

-- Changes that need a second person: in production, any settings change to a regulatory control
-- and any move of one away from live. Approval applies the change as the rule's next version,
-- only if the rule is still at base_version.
CREATE TABLE pack_rule_changes (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  id             uuid PRIMARY KEY,
  pack_id        text NOT NULL,
  rule_id        text NOT NULL CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$'),
  base_version   integer NOT NULL CHECK (base_version >= 0),
  mode           text NOT NULL CHECK (mode IN ('live', 'shadow', 'off')),
  settings       jsonb NOT NULL CHECK (jsonb_typeof(settings) = 'object' AND octet_length(settings::text) <= 4096),
  reason         text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
  restored_from  integer,
  status         text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
  proposed_by    text NOT NULL,
  proposed_at    timestamptz NOT NULL,
  decided_by     text,
  decided_at     timestamptz,
  decision_note  text CHECK (char_length(decision_note) <= 500),
  applied_version integer,
  CHECK ((status = 'pending') = (decided_at IS NULL AND decided_by IS NULL)),
  CHECK ((status = 'approved') = (applied_version IS NOT NULL)),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX pack_rule_changes_by_scope ON pack_rule_changes (tenant_id, environment_id, proposed_at DESC);
CREATE UNIQUE INDEX pack_rule_changes_one_pending ON pack_rule_changes (tenant_id, environment_id, rule_id)
  WHERE status = 'pending';

-- A proposal's content never changes, and a decision is final.
-- +goose StatementBegin
CREATE FUNCTION pack_rule_change_decide_once() RETURNS trigger
  LANGUAGE plpgsql
  AS $$
BEGIN
  IF OLD.status <> 'pending' THEN
    RAISE EXCEPTION 'pack rule change % is already decided', OLD.id;
  END IF;
  IF (NEW.tenant_id, NEW.environment_id, NEW.id, NEW.pack_id, NEW.rule_id, NEW.base_version, NEW.mode,
      NEW.settings, NEW.reason, NEW.restored_from, NEW.proposed_by, NEW.proposed_at)
     IS DISTINCT FROM
     (OLD.tenant_id, OLD.environment_id, OLD.id, OLD.pack_id, OLD.rule_id, OLD.base_version, OLD.mode,
      OLD.settings, OLD.reason, OLD.restored_from, OLD.proposed_by, OLD.proposed_at) THEN
    RAISE EXCEPTION 'pack rule change % content is immutable', OLD.id;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pack_rule_changes_decide_once BEFORE UPDATE ON pack_rule_changes
  FOR EACH ROW EXECUTE FUNCTION pack_rule_change_decide_once();
CREATE TRIGGER pack_rule_changes_no_delete BEFORE DELETE ON pack_rule_changes
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
ALTER TABLE pack_rule_changes ENABLE ROW LEVEL SECURITY;
CREATE POLICY pack_rule_changes_isolation ON pack_rule_changes TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON pack_rule_changes TO riskos_app;

-- +goose Down
DROP TABLE pack_rule_changes;
DROP FUNCTION pack_rule_change_decide_once();
DROP TABLE pack_rule_versions;
