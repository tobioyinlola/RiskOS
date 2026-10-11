-- +goose Up

-- Analytics rollups (PERFORMANCE_BASELINE.md section 7). The console's analytics page aggregated a
-- tenant's raw events and evaluations on every request: with 600,000 events in 30 days the
-- 30-day window took about 3 s and sometimes missed the 5 s request budget
-- (PERFORMANCE_BASELINE.md section 8). The worker now keeps hourly aggregates
-- that the page reads for the hours that are complete, and the raw rows only for the hour still
-- in progress.
--
-- A rollup row is a pure function of the immutable events and evaluations of its hour, so the
-- worker recomputes (upserts) the last few hours on every run: that is what makes it idempotent
-- and what picks up a row that committed late. The readers use a rollup only up to
-- rolled_through and never before covered_from, and read raw rows for the rest, so a rollup that
-- is behind, or still back-filling, is slower but never wrong.

CREATE TABLE analytics_hourly (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  bucket         timestamptz NOT NULL,
  events         bigint NOT NULL,
  score_sum      bigint NOT NULL,
  minimal        bigint NOT NULL,
  low            bigint NOT NULL,
  elevated       bigint NOT NULL,
  high           bigint NOT NULL,
  critical       bigint NOT NULL,
  -- Decisions are columns, not a document, so rollups add up with plain sums.
  allow          bigint NOT NULL,
  fee            bigint NOT NULL,
  flag           bigint NOT NULL,
  step_up        bigint NOT NULL,
  review         bigint NOT NULL,
  restrict       bigint NOT NULL,
  hold           bigint NOT NULL,
  block_session  bigint NOT NULL,
  decline        bigint NOT NULL,
  block          bigint NOT NULL,
  PRIMARY KEY (tenant_id, environment_id, bucket),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
ALTER TABLE analytics_hourly ENABLE ROW LEVEL SECURITY;
CREATE POLICY analytics_hourly_isolation ON analytics_hourly TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON analytics_hourly TO riskos_app;

CREATE TABLE signals_hourly (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  bucket         timestamptz NOT NULL,
  code           text NOT NULL,
  label          text NOT NULL,
  hits           bigint NOT NULL,
  PRIMARY KEY (tenant_id, environment_id, bucket, code),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
ALTER TABLE signals_hourly ENABLE ROW LEVEL SECURITY;
CREATE POLICY signals_hourly_isolation ON signals_hourly TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON signals_hourly TO riskos_app;

-- How far each rollup has been computed for an environment: hours in [covered_from,
-- rolled_through) are complete. One row per rollup (name: 'analytics' or 'signals').
CREATE TABLE rollup_state (
  tenant_id      uuid NOT NULL,
  environment_id uuid NOT NULL,
  name           text NOT NULL CHECK (name IN ('analytics', 'signals')),
  covered_from   timestamptz NOT NULL,
  rolled_through timestamptz NOT NULL,
  updated_at     timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, environment_id, name),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id),
  CHECK (covered_from <= rolled_through)
);
ALTER TABLE rollup_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY rollup_state_isolation ON rollup_state TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON rollup_state TO riskos_app;

-- The environments of active tenants, for the worker to visit one by one inside each scope. Only
-- identifiers are returned; the function exists because listing them is not a tenant-scoped act.
CREATE FUNCTION rollup_environments() RETURNS TABLE (tenant_id uuid, environment_id uuid)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT e.tenant_id, e.id FROM environments e JOIN tenants t ON t.id = e.tenant_id WHERE t.status = 'active'
  $$;
REVOKE ALL ON FUNCTION rollup_environments() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION rollup_environments() TO riskos_app;

-- The highest-risk users of a window are read by score, not by aggregating every event: the
-- analytics page walks this index until it has the users it needs.
CREATE INDEX events_by_score ON events (tenant_id, environment_id, risk_score DESC, received_at DESC);

-- +goose Down
DROP INDEX events_by_score;
DROP FUNCTION rollup_environments();
DROP TABLE rollup_state;
DROP TABLE signals_hourly;
DROP TABLE analytics_hourly;
