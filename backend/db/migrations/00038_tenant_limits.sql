-- +goose Up

-- Per-tenant rate limits set by RiskOS staff (ADR-0010). A row replaces the platform's per-key and
-- per-tenant limits for that tenant; no row means the platform's. Staff-realm table: written only
-- from the staff console, read by the API on the request path (cached), never by tenants.
CREATE TABLE tenant_limits (
  tenant_id             uuid PRIMARY KEY REFERENCES tenants (id),
  per_key_per_second    double precision NOT NULL CHECK (per_key_per_second > 0 AND per_key_per_second <= 100000),
  per_key_burst         integer NOT NULL CHECK (per_key_burst BETWEEN 1 AND 1000000),
  per_tenant_per_second double precision NOT NULL CHECK (per_tenant_per_second > 0 AND per_tenant_per_second <= 100000),
  per_tenant_burst      integer NOT NULL CHECK (per_tenant_burst BETWEEN 1 AND 1000000),
  reason                text NOT NULL CHECK (char_length(reason) BETWEEN 10 AND 500),
  updated_by            uuid NOT NULL REFERENCES staff (id),
  updated_at            timestamptz NOT NULL
);
GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_limits TO riskos_app;

-- +goose Down
DROP TABLE tenant_limits;
