-- Events, evaluations and idempotency (build step 3). See docs/EVENTS.md and docs/RISK_ENGINE.md.
--
-- events and evaluations are range-partitioned by month (ARCHITECTURE.md §7): retention drops
-- whole partitions. Their time column is the timestamp embedded in the row's UUIDv7 id, so a
-- lookup by id also knows its partition. Both tables are insert-only.
-- Idempotency lives in its own, unpartitioned table, because a unique constraint on a
-- partitioned table must include the partition key and could not span months.

-- +goose Up

-- Events -----------------------------------------------------------------------------------

CREATE TABLE events (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  id              uuid NOT NULL,
  -- Time from id (UUIDv7): the partition key.
  received_at     timestamptz NOT NULL,
  -- The customer's event_id (idempotency key).
  event_key       text NOT NULL,
  type            text NOT NULL CHECK (type IN ('login', 'transaction', 'beneficiary_added', 'device_change', 'session_change', 'profile_change')),
  occurred_at     timestamptz NOT NULL,
  user_ref        text NOT NULL,
  account_ref     text,
  device_ref      text,
  session_ref     text,
  ip              inet,
  country         text CHECK (country ~ '^[A-Z]{2}$'),
  -- The body exactly as received, and the canonical form the engine evaluated.
  raw_payload     text NOT NULL,
  normalized      jsonb NOT NULL,
  payload_hash    bytea NOT NULL CHECK (octet_length(payload_hash) = 32),
  -- The evaluation (owned by the risk module) and a copy of its outcome, so event lists need no
  -- cross-module join. Written once, with the event.
  evaluation_id   uuid NOT NULL,
  risk_score      smallint NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
  risk_level      text NOT NULL,
  decision        text NOT NULL,
  request_id      text NOT NULL,
  PRIMARY KEY (received_at, id),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
) PARTITION BY RANGE (received_at);

CREATE INDEX events_by_scope ON events (tenant_id, environment_id, id DESC);

ALTER TABLE events ENABLE ROW LEVEL SECURITY;
CREATE POLICY events_isolation ON events TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON events TO riskos_app;

-- Evaluations ------------------------------------------------------------------------------

CREATE TABLE evaluations (
  tenant_id         uuid NOT NULL,
  environment_id    uuid NOT NULL,
  id                uuid NOT NULL,
  -- Time from id (UUIDv7): the partition key.
  created_at        timestamptz NOT NULL,
  -- NULL for dry runs (POST /api/v1/risk/evaluate), which store no event.
  event_id          uuid,
  event_key         text NOT NULL,
  dry_run           boolean NOT NULL,
  -- Reproducibility (RISK_ENGINE.md): engine and rule versions, the features used, the result.
  engine_version    text NOT NULL,
  rule_versions     text[] NOT NULL,
  features          jsonb NOT NULL,
  risk_score        smallint NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
  risk_level        text NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
  decision          text NOT NULL CHECK (decision IN ('allow', 'review', 'step_up', 'block')),
  signals           jsonb NOT NULL,
  degraded_signals  text[] NOT NULL,
  request_id        text NOT NULL,
  duration_us       integer NOT NULL CHECK (duration_us >= 0),
  PRIMARY KEY (created_at, id),
  CHECK (dry_run = (event_id IS NULL)),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
) PARTITION BY RANGE (created_at);

CREATE INDEX evaluations_by_scope ON evaluations (tenant_id, environment_id, id DESC);

ALTER TABLE evaluations ENABLE ROW LEVEL SECURITY;
CREATE POLICY evaluations_isolation ON evaluations TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON evaluations TO riskos_app;

-- Insert-only for everyone, including the owner; retention drops partitions (DDL), never rows.
CREATE FUNCTION reject_row_change() RETURNS trigger
  LANGUAGE plpgsql
  AS $$ BEGIN RAISE EXCEPTION '% is insert-only', TG_TABLE_NAME; END $$;

CREATE TRIGGER events_insert_only BEFORE UPDATE OR DELETE ON events
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
CREATE TRIGGER evaluations_insert_only BEFORE UPDATE OR DELETE ON evaluations
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

-- Idempotency -------------------------------------------------------------------------------

-- One row per (tenant, environment, event_key). The primary key is the source of truth for
-- idempotency: a concurrent duplicate blocks on it until the first transaction commits, then
-- reads the stored response.
CREATE TABLE event_idempotency (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  event_key       text NOT NULL,
  payload_hash    bytea NOT NULL CHECK (octet_length(payload_hash) = 32),
  event_id        uuid NOT NULL,
  -- The exact response body returned for the first request; NULL only inside the transaction
  -- that claims the key.
  response        text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, environment_id, event_key),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);

ALTER TABLE event_idempotency ENABLE ROW LEVEL SECURITY;
CREATE POLICY event_idempotency_isolation ON event_idempotency TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON event_idempotency TO riskos_app;

-- Partitions --------------------------------------------------------------------------------

-- Creates monthly partitions of events and evaluations from the current month through
-- p_months_ahead months ahead. Idempotent. Runs with the owner's rights so the worker (as
-- riskos_app) can call it daily; there is deliberately no default partition, so rows never land
-- in a catch-all that would block creating the right partition later.
-- +goose StatementBegin
CREATE FUNCTION ensure_time_partitions(p_months_ahead integer) RETURNS integer
  LANGUAGE plpgsql SECURITY DEFINER
  SET search_path = pg_catalog, public
  SET timezone = 'UTC'
  AS $$
DECLARE
  month_start date;
  parent text;
  partition text;
  created integer := 0;
BEGIN
  IF p_months_ahead < 0 OR p_months_ahead > 24 THEN
    RAISE EXCEPTION 'months ahead must be between 0 and 24';
  END IF;
  FOR i IN 0..p_months_ahead LOOP
    month_start := (date_trunc('month', now()) + make_interval(months => i))::date;
    FOREACH parent IN ARRAY ARRAY['events', 'evaluations'] LOOP
      partition := format('%s_p%s', parent, to_char(month_start, 'YYYYMM'));
      IF to_regclass('public.' || partition) IS NULL THEN
        EXECUTE format('CREATE TABLE public.%I PARTITION OF public.%I FOR VALUES FROM (%L) TO (%L)',
                       partition, parent, month_start, (month_start + interval '1 month')::date);
        created := created + 1;
      END IF;
    END LOOP;
  END LOOP;
  RETURN created;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION ensure_time_partitions(integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_time_partitions(integer) TO riskos_app;

SELECT ensure_time_partitions(2);

-- +goose Down
DROP FUNCTION ensure_time_partitions(integer);
DROP TABLE event_idempotency;
DROP TABLE evaluations;
DROP TABLE events;
DROP FUNCTION reject_row_change();
