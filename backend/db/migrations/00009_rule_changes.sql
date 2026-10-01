-- Two-person approval of production rule changes (build step 8). See docs/RISK_ENGINE.md.
--
-- In a production environment, creating, changing or rolling back a rule records a proposed
-- change here instead of a new version. A different person approves it (which applies it as the
-- next version, if the rule has not changed since) or rejects it. Content is fixed at proposal;
-- only the decision columns change, once.

-- +goose Up

CREATE TABLE rule_changes (
  tenant_id        uuid NOT NULL,
  environment_id   uuid NOT NULL,
  id               uuid PRIMARY KEY,
  -- For a new rule, the ID it will get when approved.
  rule_id          uuid NOT NULL,
  code             text NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{2,63}$'),
  kind             text NOT NULL CHECK (kind IN ('create', 'update', 'rollback')),
  -- The rule's current version when proposed (NULL for a new rule). Approval applies the change
  -- only if the rule is still at this version.
  base_version     integer,
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  description      text NOT NULL CHECK (char_length(description) <= 1000),
  severity         text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
  weight           smallint NOT NULL CHECK (weight BETWEEN 0 AND 100),
  mode             text NOT NULL CHECK (mode IN ('active', 'shadow')),
  enabled          boolean NOT NULL,
  condition        jsonb NOT NULL,
  restored_from    integer,
  status           text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
  proposed_by      text NOT NULL,
  proposed_at      timestamptz NOT NULL,
  decided_by       text,
  decided_at       timestamptz,
  reason           text CHECK (char_length(reason) <= 500),
  applied_version  integer,
  CHECK ((kind = 'create') = (base_version IS NULL)),
  CHECK ((status = 'pending') = (decided_at IS NULL AND decided_by IS NULL)),
  CHECK ((status = 'approved') = (applied_version IS NOT NULL)),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX rule_changes_by_scope ON rule_changes (tenant_id, environment_id, id DESC);
-- At most one pending change per rule, and per code (so two proposals cannot claim a new code).
CREATE UNIQUE INDEX rule_changes_one_pending_rule ON rule_changes (tenant_id, environment_id, rule_id)
  WHERE status = 'pending';
CREATE UNIQUE INDEX rule_changes_one_pending_code ON rule_changes (tenant_id, environment_id, code)
  WHERE status = 'pending';

-- A proposal's content never changes, and a decision is final.
-- +goose StatementBegin
CREATE FUNCTION rule_change_decide_once() RETURNS trigger
  LANGUAGE plpgsql
  AS $$
BEGIN
  IF OLD.status <> 'pending' THEN
    RAISE EXCEPTION 'rule change % is already decided', OLD.id;
  END IF;
  IF (NEW.tenant_id, NEW.environment_id, NEW.id, NEW.rule_id, NEW.code, NEW.kind, NEW.base_version,
      NEW.name, NEW.description, NEW.severity, NEW.weight, NEW.mode, NEW.enabled, NEW.condition,
      NEW.restored_from, NEW.proposed_by, NEW.proposed_at)
     IS DISTINCT FROM
     (OLD.tenant_id, OLD.environment_id, OLD.id, OLD.rule_id, OLD.code, OLD.kind, OLD.base_version,
      OLD.name, OLD.description, OLD.severity, OLD.weight, OLD.mode, OLD.enabled, OLD.condition,
      OLD.restored_from, OLD.proposed_by, OLD.proposed_at) THEN
    RAISE EXCEPTION 'rule change % content is immutable', OLD.id;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER rule_changes_decide_once BEFORE UPDATE ON rule_changes
  FOR EACH ROW EXECUTE FUNCTION rule_change_decide_once();
CREATE TRIGGER rule_changes_no_delete BEFORE DELETE ON rule_changes
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

ALTER TABLE rule_changes ENABLE ROW LEVEL SECURITY;
CREATE POLICY rule_changes_isolation ON rule_changes TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON rule_changes TO riskos_app;

-- +goose Down
DROP TABLE rule_changes;
DROP FUNCTION rule_change_decide_once();
