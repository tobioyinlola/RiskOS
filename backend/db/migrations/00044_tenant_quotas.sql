-- +goose Up

-- A tenant's monthly event-volume quota, set by RiskOS staff (ADMIN.md#event-volume-quotas). It is
-- a soft limit: events are never refused for it. The worker measures usage hourly and announces
-- an overage once a month, to staff and in the tenant's own audit log. Staff-realm table, like
-- tenant_limits: written from the staff console and the worker, never by tenants.
CREATE TABLE tenant_quotas (
  tenant_id        uuid PRIMARY KEY REFERENCES tenants (id),
  monthly_events   bigint NOT NULL CHECK (monthly_events BETWEEN 1 AND 1000000000000),
  reason           text NOT NULL CHECK (char_length(reason) BETWEEN 10 AND 500),
  updated_by       uuid NOT NULL REFERENCES staff (id),
  updated_at       timestamptz NOT NULL,
  -- What the worker last measured: events received in usage_month (the first instant of the month,
  -- UTC).
  usage_month      timestamptz,
  usage_events     bigint NOT NULL DEFAULT 0,
  usage_checked_at timestamptz,
  -- The month an overage was last announced for, so it is announced once per month.
  exceeded_month   timestamptz
);
GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_quotas TO riskos_app;

-- How many events a tenant received since p_since, across its environments. The worker has no
-- tenant scope to count under, and events are protected by row-level security, so this runs with
-- the owner's rights and only counts.
CREATE FUNCTION admin_count_events(p_tenant uuid, p_since timestamptz) RETURNS bigint
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT count(*) FROM events WHERE tenant_id = p_tenant AND received_at >= p_since
  $$;
REVOKE ALL ON FUNCTION admin_count_events(uuid, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_count_events(uuid, timestamptz) TO riskos_app;

-- +goose Down
DROP FUNCTION admin_count_events(uuid, timestamptz);
DROP TABLE tenant_quotas;
