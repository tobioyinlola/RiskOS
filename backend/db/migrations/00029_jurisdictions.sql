-- +goose Up
-- Multi-jurisdiction compliance (ADR-0007).

-- The jurisdictions RiskOS has a pack for: reference data, seeded here and read-only to the
-- application. A fourth country needs a row here and a pack, nothing else. The pack loader
-- refuses a pack whose currency or time zone differs from its row.
CREATE TABLE jurisdictions (
  code       text PRIMARY KEY CHECK (code ~ '^[A-Z]{2}$'),
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  currency   text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  -- Digits after the decimal point in amounts: kobo and pesewas are 2, Rwandan francs 0.
  minor_unit smallint NOT NULL CHECK (minor_unit BETWEEN 0 AND 4),
  timezone   text NOT NULL CHECK (char_length(timezone) BETWEEN 1 AND 64),
  regulators text[] NOT NULL,
  pack_id    text NOT NULL UNIQUE CHECK (pack_id ~ '^[a-z]{2,16}$'),
  UNIQUE (code, pack_id)
);
INSERT INTO jurisdictions (code, name, currency, minor_unit, timezone, regulators, pack_id) VALUES
  ('NG', 'Nigeria', 'NGN', 2, 'Africa/Lagos', '{CBN,NFIU,NIBSS,NCC,SEC,FCCPC}', 'ng'),
  ('GH', 'Ghana',   'GHS', 2, 'Africa/Accra', '{FIC,BoG,SEC,CSA,DPC,NCA}', 'gh'),
  ('RW', 'Rwanda',  'RWF', 0, 'Africa/Kigali', '{FIC,BNR,CMA,NCSA,RURA}', 'rw');
GRANT SELECT ON jurisdictions TO riskos_app;

-- Rule IDs of country libraries (GH-REG-01) next to the first library's (REG-01).
ALTER TABLE pack_rules DROP CONSTRAINT pack_rules_rule_id_check;
ALTER TABLE pack_rules ADD CONSTRAINT pack_rules_rule_id_check
  CHECK (rule_id ~ '^([A-Z]{3}|[A-Z]{2}-(REG|TM|SEC|GOV))-[0-9]{2}$');

-- A tenant's compliance profile becomes its activations: one per jurisdiction and booking entity
-- (the licensed entity that holds the customer relationship). Existing profiles become the
-- tenant's Nigeria activation for the booking entity "default", flagged as the fallback: events
-- that name no booking entity keep being judged by the Nigeria pack, as before. The fallback is a
-- stated setting shown to the tenant, never a guess.
ALTER TABLE tenant_compliance RENAME TO jurisdiction_activations;
ALTER POLICY tenant_compliance_isolation ON jurisdiction_activations RENAME TO jurisdiction_activations_isolation;
ALTER TABLE jurisdiction_activations DROP CONSTRAINT tenant_compliance_pkey;
ALTER TABLE jurisdiction_activations
  ADD COLUMN id uuid,
  ADD COLUMN jurisdiction text,
  ADD COLUMN booking_entity text NOT NULL DEFAULT 'default'
    CHECK (booking_entity ~ '^[A-Za-z0-9._:-]{1,64}$'),
  ADD COLUMN status text NOT NULL DEFAULT 'live' CHECK (status IN ('draft', 'shadow', 'live', 'suspended')),
  ADD COLUMN fallback boolean NOT NULL DEFAULT false,
  ADD COLUMN effective_from timestamptz,
  ADD COLUMN activated_at timestamptz,
  ADD COLUMN activated_by text,
  ADD COLUMN rationale text CHECK (char_length(rationale) <= 1000),
  ADD COLUMN approved_by text;
UPDATE jurisdiction_activations a
SET id = gen_random_uuid(), fallback = true, activated_at = a.updated_at, activated_by = a.updated_by,
    jurisdiction = (SELECT p.jurisdiction FROM rule_packs p WHERE p.id = a.pack_id AND p.version = a.pack_version);
ALTER TABLE jurisdiction_activations
  ALTER COLUMN id SET NOT NULL,
  ALTER COLUMN jurisdiction SET NOT NULL,
  ADD PRIMARY KEY (id),
  ADD CONSTRAINT jurisdiction_activations_pack FOREIGN KEY (jurisdiction, pack_id) REFERENCES jurisdictions (code, pack_id);
CREATE UNIQUE INDEX jurisdiction_activations_entity ON jurisdiction_activations (tenant_id, booking_entity);
CREATE UNIQUE INDEX jurisdiction_activations_fallback ON jurisdiction_activations (tenant_id) WHERE fallback;
CREATE INDEX jurisdiction_activations_tenant ON jurisdiction_activations (tenant_id, jurisdiction);
GRANT DELETE ON jurisdiction_activations TO riskos_app;

-- New workspaces get the same automatic activation: Nigeria, the fallback, until a person
-- chooses otherwise.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tenant_compliance_default() RETURNS trigger
  LANGUAGE plpgsql SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
BEGIN
  INSERT INTO jurisdiction_activations (id, tenant_id, jurisdiction, pack_id, pack_version, licence_type, capabilities,
                                        booking_entity, status, fallback, updated_at, updated_by, signal_default,
                                        activated_at, activated_by)
  SELECT gen_random_uuid(), NEW.id, p.jurisdiction, p.id, p.version, 'other', '{}', 'default', 'live', true,
         now(), 'system:default', 'off', now(), 'system:default'
  FROM rule_packs p
  WHERE p.jurisdiction = 'NG'
  ORDER BY string_to_array(p.version, '.')::int[] DESC
  LIMIT 1
  ON CONFLICT DO NOTHING;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- Rule configuration is kept per pack: an inherited rule (ATO-04 in Ghana) is configured apart
-- from the same rule in Nigeria.
ALTER TABLE pack_rule_versions DROP CONSTRAINT pack_rule_versions_rule_id_check;
ALTER TABLE pack_rule_versions ADD CONSTRAINT pack_rule_versions_rule_id_check
  CHECK (rule_id ~ '^([A-Z]{3}|[A-Z]{2}-(REG|TM|SEC|GOV))-[0-9]{2}$');
ALTER TABLE pack_rule_versions DROP CONSTRAINT pack_rule_versions_pkey;
ALTER TABLE pack_rule_versions ADD PRIMARY KEY (tenant_id, environment_id, pack_id, rule_id, version);
ALTER TABLE pack_rule_changes DROP CONSTRAINT pack_rule_changes_rule_id_check;
ALTER TABLE pack_rule_changes ADD CONSTRAINT pack_rule_changes_rule_id_check
  CHECK (rule_id ~ '^([A-Z]{3}|[A-Z]{2}-(REG|TM|SEC|GOV))-[0-9]{2}$');
DROP INDEX pack_rule_changes_one_pending;
CREATE UNIQUE INDEX pack_rule_changes_one_pending ON pack_rule_changes (tenant_id, environment_id, pack_id, rule_id)
  WHERE status = 'pending';

-- Compliance work from the country packs: their rule IDs and the reports they prepare.
ALTER TABLE compliance_tasks DROP CONSTRAINT compliance_tasks_rule_id_check;
ALTER TABLE compliance_tasks ADD CONSTRAINT compliance_tasks_rule_id_check
  CHECK (rule_id ~ '^([A-Z]{3}|[A-Z]{2}-(REG|TM|SEC|GOV))-[0-9]{2}$');
ALTER TABLE compliance_tasks DROP CONSTRAINT compliance_tasks_kind_check;
ALTER TABLE compliance_tasks ADD CONSTRAINT compliance_tasks_kind_check CHECK (kind IN (
  'report.STR', 'report.CTR', 'report.FTR', 'report.PEP_monthly', 'report.sanctions', 'report.industry_fraud',
  'report.ECTR', 'report.PEP', 'report.cross_border_cash', 'report.BoG', 'report.BNR', 'report.CSA',
  'report.DPC', 'report.NCSA', 'report.RURA',
  'case.open', 'edd', 'source_of_funds', 'source_of_wealth', 'senior_approval',
  'freeze_account', 'close_account', 'escalate'));

-- +goose Down
-- Tasks are never deleted: country tasks already recorded stay, and only new rows are checked.
ALTER TABLE compliance_tasks DROP CONSTRAINT compliance_tasks_kind_check;
ALTER TABLE compliance_tasks ADD CONSTRAINT compliance_tasks_kind_check CHECK (kind IN (
  'report.STR', 'report.CTR', 'report.FTR', 'report.PEP_monthly', 'report.sanctions', 'report.industry_fraud',
  'case.open', 'edd', 'source_of_funds', 'source_of_wealth', 'senior_approval',
  'freeze_account', 'close_account', 'escalate')) NOT VALID;
ALTER TABLE compliance_tasks DROP CONSTRAINT compliance_tasks_rule_id_check;
ALTER TABLE compliance_tasks ADD CONSTRAINT compliance_tasks_rule_id_check CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$') NOT VALID;
DROP INDEX pack_rule_changes_one_pending;
-- Configuration of other packs' rules cannot be kept per rule alone, and history is immutable:
-- going down needs them gone first, which this migration will not do silently.
ALTER TABLE pack_rule_changes DROP CONSTRAINT pack_rule_changes_rule_id_check;
ALTER TABLE pack_rule_changes ADD CONSTRAINT pack_rule_changes_rule_id_check CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$') NOT VALID;
CREATE UNIQUE INDEX pack_rule_changes_one_pending ON pack_rule_changes (tenant_id, environment_id, rule_id)
  WHERE status = 'pending';
ALTER TABLE pack_rule_versions DROP CONSTRAINT pack_rule_versions_pkey;
ALTER TABLE pack_rule_versions ADD PRIMARY KEY (tenant_id, environment_id, rule_id, version);
ALTER TABLE pack_rule_versions DROP CONSTRAINT pack_rule_versions_rule_id_check;
ALTER TABLE pack_rule_versions ADD CONSTRAINT pack_rule_versions_rule_id_check CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$') NOT VALID;
-- Each tenant keeps one profile: its fallback activation.
DELETE FROM jurisdiction_activations WHERE NOT fallback;
REVOKE DELETE ON jurisdiction_activations FROM riskos_app;
DROP INDEX jurisdiction_activations_tenant;
DROP INDEX jurisdiction_activations_fallback;
DROP INDEX jurisdiction_activations_entity;
ALTER TABLE jurisdiction_activations DROP CONSTRAINT jurisdiction_activations_pack;
ALTER TABLE jurisdiction_activations DROP CONSTRAINT jurisdiction_activations_pkey;
ALTER TABLE jurisdiction_activations
  DROP COLUMN id, DROP COLUMN jurisdiction, DROP COLUMN booking_entity, DROP COLUMN status, DROP COLUMN fallback,
  DROP COLUMN effective_from, DROP COLUMN activated_at, DROP COLUMN activated_by, DROP COLUMN rationale,
  DROP COLUMN approved_by;
ALTER TABLE jurisdiction_activations ADD PRIMARY KEY (tenant_id);
ALTER POLICY jurisdiction_activations_isolation ON jurisdiction_activations RENAME TO tenant_compliance_isolation;
ALTER TABLE jurisdiction_activations RENAME TO tenant_compliance;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tenant_compliance_default() RETURNS trigger
  LANGUAGE plpgsql SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
BEGIN
  INSERT INTO tenant_compliance (tenant_id, pack_id, pack_version, licence_type, capabilities,
                                 updated_at, updated_by, signal_default)
  SELECT NEW.id, p.id, p.version, 'other', '{}', now(), 'system:default', 'off'
  FROM rule_packs p
  WHERE p.jurisdiction = 'NG'
  ORDER BY string_to_array(p.version, '.')::int[] DESC
  LIMIT 1
  ON CONFLICT (tenant_id) DO NOTHING;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
ALTER TABLE pack_rules DROP CONSTRAINT pack_rules_rule_id_check;
ALTER TABLE pack_rules ADD CONSTRAINT pack_rules_rule_id_check CHECK (rule_id ~ '^[A-Z]{3}-[0-9]{2}$') NOT VALID;
DROP TABLE jurisdictions;
