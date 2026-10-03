-- +goose Up
-- Jurisdiction rule packs (ADR-0006). A pack version is global reference data: the migrate
-- command loads it as the schema owner, the application only reads it, and a loaded version
-- never changes (its checksum is the SHA-256 of the pack's files).
CREATE TABLE rule_packs (
  id           text NOT NULL CHECK (id ~ '^[a-z]{2,16}$'),
  version      text NOT NULL CHECK (version ~ '^[0-9]{4}\.[0-9]+\.[0-9]+$'),
  jurisdiction text NOT NULL CHECK (jurisdiction ~ '^[A-Z]{2}$'),
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  checksum     text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
  -- Groups, sources, clocks, gaps, glossary, licence types and capabilities.
  meta         jsonb NOT NULL CHECK (jsonb_typeof(meta) = 'object'),
  loaded_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id, version)
);
CREATE INDEX rule_packs_latest ON rule_packs (jurisdiction, loaded_at DESC);

CREATE TABLE pack_rules (
  pack_id      text NOT NULL,
  pack_version text NOT NULL,
  rule_id      text NOT NULL CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$'),
  kind         text NOT NULL CHECK (kind IN ('control', 'signal', 'governance')),
  -- The machine definition (validated when loaded; interpreted, never executed).
  definition   jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
  -- The library's wording, verbatim: sections 7 and 7B.
  wording      jsonb NOT NULL CHECK (jsonb_typeof(wording) = 'object'),
  PRIMARY KEY (pack_id, pack_version, rule_id),
  FOREIGN KEY (pack_id, pack_version) REFERENCES rule_packs (id, version)
);

-- +goose StatementBegin
CREATE FUNCTION rule_packs_immutable() RETURNS trigger
  LANGUAGE plpgsql
  AS $$
  BEGIN
    RAISE EXCEPTION 'a loaded rule pack version cannot change; load a new version instead';
  END;
  $$;
-- +goose StatementEnd
CREATE TRIGGER rule_packs_immutable BEFORE UPDATE OR DELETE ON rule_packs
  FOR EACH ROW EXECUTE FUNCTION rule_packs_immutable();
CREATE TRIGGER pack_rules_immutable BEFORE UPDATE OR DELETE ON pack_rules
  FOR EACH ROW EXECUTE FUNCTION rule_packs_immutable();

GRANT SELECT ON rule_packs, pack_rules TO riskos_app;

-- A tenant's compliance profile: the pack version it is on and what kind of institution it is,
-- which decides the rules that apply to it.
CREATE TABLE tenant_compliance (
  tenant_id    uuid PRIMARY KEY REFERENCES tenants (id),
  pack_id      text NOT NULL,
  pack_version text NOT NULL,
  licence_type text NOT NULL CHECK (licence_type ~ '^[a-z_]{1,64}$'),
  capabilities text[] NOT NULL DEFAULT '{}' CHECK (cardinality(capabilities) <= 32),
  updated_at   timestamptz NOT NULL,
  updated_by   text NOT NULL,
  FOREIGN KEY (pack_id, pack_version) REFERENCES rule_packs (id, version)
);
ALTER TABLE tenant_compliance ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_compliance_isolation ON tenant_compliance TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON tenant_compliance TO riskos_app;

-- Two customer roles: compliance officers and (read-only) auditors.
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
  CHECK (role IN ('customer_admin', 'developer', 'risk_manager', 'analyst', 'compliance_officer', 'auditor'));
ALTER TABLE user_invites DROP CONSTRAINT user_invites_role_check;
ALTER TABLE user_invites ADD CONSTRAINT user_invites_role_check
  CHECK (role IN ('customer_admin', 'developer', 'risk_manager', 'analyst', 'compliance_officer', 'auditor'));

-- +goose Down
-- People in the new roles fall back to the least-privileged role that existed before.
UPDATE users SET role = 'analyst' WHERE role IN ('compliance_officer', 'auditor');
DELETE FROM user_invites WHERE role IN ('compliance_officer', 'auditor');
ALTER TABLE user_invites DROP CONSTRAINT user_invites_role_check;
ALTER TABLE user_invites ADD CONSTRAINT user_invites_role_check
  CHECK (role IN ('customer_admin', 'developer', 'risk_manager', 'analyst'));
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
  CHECK (role IN ('customer_admin', 'developer', 'risk_manager', 'analyst'));
DROP TABLE tenant_compliance;
DROP TABLE pack_rules;
DROP TABLE rule_packs;
DROP FUNCTION rule_packs_immutable();
