-- Deny lists with hard blocks (build step 9). See docs/RISK_ENGINE.md.
--
-- An entry names one identifier the tenant never wants to transact with. An event whose actor,
-- device, IP address, beneficiary or country matches an active entry is blocked, whatever its
-- score. Entries can expire; removing one is audited. Values are stored as the tenant sends them
-- (IP addresses in canonical form, countries upper-case).

-- +goose Up
CREATE TABLE deny_list_entries (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  id              uuid PRIMARY KEY,
  kind            text NOT NULL CHECK (kind IN ('user', 'account', 'device', 'ip', 'beneficiary', 'country')),
  value           text NOT NULL CHECK (char_length(value) BETWEEN 1 AND 128),
  reason          text NOT NULL CHECK (char_length(reason) <= 200),
  expires_at      timestamptz,
  created_at      timestamptz NOT NULL,
  created_by      text NOT NULL,
  UNIQUE (tenant_id, environment_id, kind, value),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX deny_list_by_scope ON deny_list_entries (tenant_id, environment_id, id DESC);

ALTER TABLE deny_list_entries ENABLE ROW LEVEL SECURITY;
CREATE POLICY deny_list_entries_isolation ON deny_list_entries TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, DELETE ON deny_list_entries TO riskos_app;

-- +goose Down
DROP TABLE deny_list_entries;
