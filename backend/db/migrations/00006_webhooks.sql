-- Webhooks (build step 6). See docs/WEBHOOKS.md.
--
-- An endpoint subscribes to event types. Publishing an event (in the transaction of the change
-- that caused it) inserts one delivery per subscribed endpoint and enqueues its job. Deliveries
-- change state as they are attempted; every attempt is logged in an insert-only table. Secrets
-- are never stored: they are derived from a server-side key, the endpoint ID and a version.

-- +goose Up

CREATE TABLE webhook_endpoints (
  tenant_id                    uuid NOT NULL,
  environment_id               uuid NOT NULL,
  id                           uuid PRIMARY KEY,
  url                          text NOT NULL CHECK (char_length(url) BETWEEN 1 AND 2048),
  description                  text NOT NULL CHECK (char_length(description) <= 500),
  event_types                  text[] NOT NULL CHECK (cardinality(event_types) >= 1),
  status                       text NOT NULL CHECK (status IN ('active', 'disabled')),
  disabled_reason              text,
  secret_version               integer NOT NULL CHECK (secret_version >= 1),
  -- During a rotation overlap, deliveries are also signed with the previous secret.
  previous_secret_version      integer,
  previous_secret_valid_until  timestamptz,
  -- First failed attempt since the last success; drives auto-disable.
  failing_since                timestamptz,
  created_at                   timestamptz NOT NULL,
  created_by                   text NOT NULL,
  updated_at                   timestamptz NOT NULL,
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX webhook_endpoints_env ON webhook_endpoints (tenant_id, environment_id, id DESC);

CREATE TABLE webhook_deliveries (
  tenant_id         uuid NOT NULL,
  environment_id    uuid NOT NULL,
  id                uuid PRIMARY KEY,
  endpoint_id       uuid NOT NULL REFERENCES webhook_endpoints (id),
  -- The event's ID, the same for every endpoint it is delivered to; receivers deduplicate on it.
  event_id          uuid NOT NULL,
  event_type        text NOT NULL,
  payload           jsonb NOT NULL,
  status            text NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
  -- Attempts in the current delivery cycle (a redelivery starts a new cycle).
  attempts          integer NOT NULL DEFAULT 0,
  next_attempt_at   timestamptz,
  last_attempt_at   timestamptz,
  last_status_code  integer,
  last_error        text,
  created_at        timestamptz NOT NULL,
  completed_at      timestamptz,
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX webhook_deliveries_endpoint ON webhook_deliveries (endpoint_id, id DESC);

CREATE TABLE webhook_attempts (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  id              uuid PRIMARY KEY,
  delivery_id     uuid NOT NULL REFERENCES webhook_deliveries (id),
  attempt         integer NOT NULL,
  started_at      timestamptz NOT NULL,
  duration_ms     integer NOT NULL,
  status_code     integer,
  -- A short, sanitized reason (never a response body).
  error           text,
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX webhook_attempts_delivery ON webhook_attempts (delivery_id, id);

ALTER TABLE webhook_endpoints ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_endpoints_isolation ON webhook_endpoints TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON webhook_endpoints TO riskos_app;

ALTER TABLE webhook_deliveries ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_deliveries_isolation ON webhook_deliveries TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON webhook_deliveries TO riskos_app;

ALTER TABLE webhook_attempts ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_attempts_isolation ON webhook_attempts TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON webhook_attempts TO riskos_app;
CREATE TRIGGER webhook_attempts_insert_only BEFORE UPDATE OR DELETE ON webhook_attempts
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

-- Security alerts can be about a webhook endpoint (auto-disabled after failing), without
-- evaluations.
ALTER TABLE alerts DROP CONSTRAINT alerts_subject_type_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_subject_type_check
  CHECK (subject_type IN ('user', 'environment', 'webhook_endpoint'));
ALTER TABLE alerts DROP CONSTRAINT alerts_evaluation_count_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_evaluation_count_check CHECK (evaluation_count >= 0);

-- +goose Down
ALTER TABLE alerts DROP CONSTRAINT alerts_evaluation_count_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_evaluation_count_check CHECK (evaluation_count >= 1);
ALTER TABLE alerts DROP CONSTRAINT alerts_subject_type_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_subject_type_check CHECK (subject_type IN ('user', 'environment'));
DROP TABLE webhook_attempts;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_endpoints;
