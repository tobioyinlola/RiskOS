-- Alerts and cases (build step 5). See docs/ALERTS_AND_CASES.md.
--
-- Alerts are raised asynchronously from evaluations (a job enqueued in the decision transaction)
-- and grouped per subject within a window. Their status, assignee and counters change; every
-- change is also written to an insert-only activity log. Case notes and activity are
-- insert-only. Nothing here can change a decision already returned to the customer.

-- +goose Up

CREATE TABLE cases (
  tenant_id        uuid NOT NULL,
  environment_id   uuid NOT NULL,
  id               uuid PRIMARY KEY,
  title            text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  description      text NOT NULL CHECK (char_length(description) <= 5000),
  status           text NOT NULL CHECK (status IN ('open', 'investigating', 'resolved')),
  priority         text NOT NULL CHECK (priority IN ('low', 'medium', 'high', 'critical')),
  -- Free-text reference to the analyst (dashboard user accounts arrive in build step 7).
  assignee         text CHECK (char_length(assignee) BETWEEN 1 AND 254),
  resolution       text CHECK (resolution IN ('confirmed_fraud', 'false_positive', 'legitimate', 'inconclusive')),
  resolution_note  text CHECK (char_length(resolution_note) <= 5000),
  resolved_at      timestamptz,
  resolved_by      text,
  created_at       timestamptz NOT NULL,
  created_by       text NOT NULL,
  updated_at       timestamptz NOT NULL,
  CHECK ((status = 'resolved') = (resolution IS NOT NULL AND resolved_at IS NOT NULL)),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX cases_list ON cases (tenant_id, environment_id, status, id DESC);

CREATE TABLE alerts (
  tenant_id         uuid NOT NULL,
  environment_id    uuid NOT NULL,
  id                uuid PRIMARY KEY,
  -- risk: an evaluation reached the tenant's alert threshold; security: evaluations ran with
  -- missing signals (possibly attacker-induced, ARCHITECTURE.md §16.8).
  kind              text NOT NULL CHECK (kind IN ('risk', 'security')),
  -- What the alert is about: the customer's user reference for risk alerts, the environment for
  -- security alerts. Grouping key together with kind.
  subject_type      text NOT NULL CHECK (subject_type IN ('user', 'environment')),
  subject_ref       text NOT NULL CHECK (char_length(subject_ref) BETWEEN 1 AND 128),
  title             text NOT NULL,
  severity          text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
  status            text NOT NULL CHECK (status IN ('open', 'investigating', 'escalated', 'false_positive', 'resolved')),
  assignee          text CHECK (char_length(assignee) BETWEEN 1 AND 254),
  case_id           uuid REFERENCES cases (id),
  max_score         smallint NOT NULL CHECK (max_score BETWEEN 0 AND 100),
  evaluation_count  integer NOT NULL CHECK (evaluation_count >= 1),
  first_seen_at     timestamptz NOT NULL,
  last_seen_at      timestamptz NOT NULL,
  created_at        timestamptz NOT NULL,
  updated_at        timestamptz NOT NULL,
  CHECK ((status = 'escalated') = (case_id IS NOT NULL) OR status IN ('resolved', 'false_positive')),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX alerts_list ON alerts (tenant_id, environment_id, id DESC);
-- Finds the open alert an evaluation should be grouped into.
CREATE INDEX alerts_grouping ON alerts (tenant_id, environment_id, kind, subject_ref, last_seen_at DESC)
  WHERE status IN ('open', 'investigating');
CREATE INDEX alerts_case ON alerts (case_id) WHERE case_id IS NOT NULL;

-- The evaluations an alert groups, with the snapshot needed to triage without other modules.
CREATE TABLE alert_evaluations (
  tenant_id         uuid NOT NULL,
  environment_id    uuid NOT NULL,
  alert_id          uuid NOT NULL REFERENCES alerts (id),
  alert_kind        text NOT NULL,
  evaluation_id     uuid NOT NULL,
  event_id          uuid NOT NULL,
  event_type        text NOT NULL,
  score             smallint NOT NULL,
  level             text NOT NULL,
  decision          text NOT NULL,
  signals           jsonb NOT NULL,
  degraded_signals  text[] NOT NULL,
  evaluated_at      timestamptz NOT NULL,
  -- An evaluation joins at most one alert of each kind: this makes the raise job idempotent.
  PRIMARY KEY (tenant_id, environment_id, evaluation_id, alert_kind),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX alert_evaluations_alert ON alert_evaluations (alert_id, evaluated_at DESC);

CREATE TABLE alert_activity (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  id              uuid PRIMARY KEY,
  alert_id        uuid NOT NULL REFERENCES alerts (id),
  action          text NOT NULL,
  actor           text NOT NULL,
  detail          jsonb NOT NULL,
  created_at      timestamptz NOT NULL,
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX alert_activity_alert ON alert_activity (alert_id, id);

CREATE TABLE case_notes (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  id              uuid PRIMARY KEY,
  case_id         uuid NOT NULL REFERENCES cases (id),
  body            text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 10000),
  author          text NOT NULL,
  created_at      timestamptz NOT NULL,
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX case_notes_case ON case_notes (case_id, id);

CREATE TABLE case_activity (
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  id              uuid PRIMARY KEY,
  case_id         uuid NOT NULL REFERENCES cases (id),
  action          text NOT NULL,
  actor           text NOT NULL,
  detail          jsonb NOT NULL,
  created_at      timestamptz NOT NULL,
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);
CREATE INDEX case_activity_case ON case_activity (case_id, id);

-- Row-level security and grants: mutable tables get UPDATE; history tables are insert-only.
ALTER TABLE cases ENABLE ROW LEVEL SECURITY;
CREATE POLICY cases_isolation ON cases TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON cases TO riskos_app;

ALTER TABLE alerts ENABLE ROW LEVEL SECURITY;
CREATE POLICY alerts_isolation ON alerts TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON alerts TO riskos_app;

ALTER TABLE alert_evaluations ENABLE ROW LEVEL SECURITY;
CREATE POLICY alert_evaluations_isolation ON alert_evaluations TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON alert_evaluations TO riskos_app;
CREATE TRIGGER alert_evaluations_insert_only BEFORE UPDATE OR DELETE ON alert_evaluations
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

ALTER TABLE alert_activity ENABLE ROW LEVEL SECURITY;
CREATE POLICY alert_activity_isolation ON alert_activity TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON alert_activity TO riskos_app;
CREATE TRIGGER alert_activity_insert_only BEFORE UPDATE OR DELETE ON alert_activity
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

ALTER TABLE case_notes ENABLE ROW LEVEL SECURITY;
CREATE POLICY case_notes_isolation ON case_notes TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON case_notes TO riskos_app;
CREATE TRIGGER case_notes_insert_only BEFORE UPDATE OR DELETE ON case_notes
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

ALTER TABLE case_activity ENABLE ROW LEVEL SECURITY;
CREATE POLICY case_activity_isolation ON case_activity TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT ON case_activity TO riskos_app;
CREATE TRIGGER case_activity_insert_only BEFORE UPDATE OR DELETE ON case_activity
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();

-- Which decisions raise an alert (RISK_ENGINE.md): 'review' (default) or 'step_up'.
ALTER TABLE risk_settings ADD COLUMN alert_min_decision text NOT NULL DEFAULT 'review'
  CHECK (alert_min_decision IN ('step_up', 'review'));

-- +goose Down
ALTER TABLE risk_settings DROP COLUMN alert_min_decision;
DROP TABLE case_activity;
DROP TABLE case_notes;
DROP TABLE alert_activity;
DROP TABLE alert_evaluations;
DROP TABLE alerts;
DROP TABLE cases;
