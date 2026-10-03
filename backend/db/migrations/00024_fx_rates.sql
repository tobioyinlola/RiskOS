-- +goose Up
-- Exchange rates an institution sets for its rule pack (ADR-0006): US dollars per one unit of a
-- currency, so rules on US dollar amounts (REG-10) can convert an event's amount.
CREATE TABLE compliance_fx_rates (
  tenant_id    uuid NOT NULL REFERENCES tenants (id),
  currency     text NOT NULL CHECK (currency ~ '^[A-Z]{3}$' AND currency <> 'USD'),
  usd_per_unit numeric(20, 10) NOT NULL CHECK (usd_per_unit > 0 AND usd_per_unit <= 1000000),
  updated_at   timestamptz NOT NULL,
  updated_by   text NOT NULL,
  PRIMARY KEY (tenant_id, currency)
);
ALTER TABLE compliance_fx_rates ENABLE ROW LEVEL SECURITY;
CREATE POLICY compliance_fx_rates_isolation ON compliance_fx_rates TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON compliance_fx_rates TO riskos_app;

-- +goose Down
DROP TABLE compliance_fx_rates;
