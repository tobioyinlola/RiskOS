-- Rules and risk settings (build step 4). See docs/RISK_ENGINE.md.
--
-- A rule is a stable identity (rules) with an append-only history of versions (rule_versions).
-- Editing or rolling back a rule adds a version and moves current_version; old versions are never
-- changed, so every stored evaluation can name the exact rule versions it used.

-- +goose Up

CREATE TABLE rules (
  tenant_id        uuid NOT NULL,
  environment_id   uuid NOT NULL,
  id               uuid PRIMARY KEY,
  -- Signal code the rule emits; unique per environment, never reused for another rule.
  code             text NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{2,63}$'),
  current_version  integer NOT NULL CHECK (current_version >= 1),
  created_at       timestamptz NOT NULL DEFAULT now(),
  created_by       text NOT NULL,
  UNIQUE (tenant_id, environment_id, code),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);

ALTER TABLE rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY rules_isolation ON rules TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON rules TO riskos_app;

CREATE TABLE rule_versions (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  rule_id         uuid NOT NULL REFERENCES rules (id),
  version         integer NOT NULL CHECK (version >= 1),
  name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  description     text NOT NULL CHECK (char_length(description) <= 1000),
  severity        text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
  weight          smallint NOT NULL CHECK (weight BETWEEN 0 AND 100),
  -- active rules add their weight to the score; shadow rules are evaluated and recorded only.
  mode            text NOT NULL CHECK (mode IN ('active', 'shadow')),
  enabled         boolean NOT NULL,
  condition       jsonb NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  created_by      text NOT NULL,
  -- Set when the version restores an earlier one (one-step rollback).
  restored_from   integer,
  PRIMARY KEY (rule_id, version),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);

ALTER TABLE rule_versions ENABLE ROW LEVEL SECURITY;
CREATE POLICY rule_versions_isolation ON rule_versions TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON rule_versions TO riskos_app;

CREATE TRIGGER rule_versions_insert_only BEFORE UPDATE OR DELETE ON rule_versions
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

-- Risk settings ----------------------------------------------------------------------------

-- One row per environment; absent means product defaults (RISK_ENGINE.md).
CREATE TABLE risk_settings (
  tenant_id              uuid NOT NULL,
  environment_id         uuid NOT NULL,
  -- Built-in signal weight overrides: {"new_device": 25, ...}
  weights                jsonb NOT NULL,
  high_value_default     bigint NOT NULL CHECK (high_value_default > 0),
  -- {"USD": 500000, ...} in minor units
  high_value_by_currency jsonb NOT NULL,
  -- {"transaction": "closed", "login": "open", ...}
  fail_policies          jsonb NOT NULL,
  updated_at             timestamptz NOT NULL,
  updated_by             text NOT NULL,
  PRIMARY KEY (tenant_id, environment_id),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);

ALTER TABLE risk_settings ENABLE ROW LEVEL SECURITY;
CREATE POLICY risk_settings_isolation ON risk_settings TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON risk_settings TO riskos_app;

-- Evaluations record the policy's reasons and shadow-rule matches next to the features.
ALTER TABLE evaluations ADD COLUMN policy_reasons text[] NOT NULL DEFAULT '{}';
ALTER TABLE evaluations ADD COLUMN shadow_signals jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE evaluations DROP COLUMN shadow_signals;
ALTER TABLE evaluations DROP COLUMN policy_reasons;
DROP TABLE risk_settings;
DROP TABLE rule_versions;
DROP TABLE rules;
