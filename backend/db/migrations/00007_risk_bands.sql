-- Five risk bands (build step 7, docs/design-system.md and RISK_ENGINE.md).
--
-- Bands become minimal / low / elevated / high / critical with tenant-adjustable thresholds
-- (defaults 20/50/70/90). Alert severities and case priorities use the same scale.
--
-- Stored evaluations and events are insert-only. A band is a pure function of the stored score,
-- so this migration, which runs before any production data exists, recomputes the band of
-- existing rows from their score with the default thresholds (the triggers are suspended for
-- the duration of this transaction only). Stored decisions are not changed.

-- +goose Up

ALTER TABLE risk_settings ADD COLUMN thresholds jsonb NOT NULL
  DEFAULT '{"low": 20, "elevated": 50, "high": 70, "critical": 90}';

CREATE FUNCTION band_for_score(score integer) RETURNS text
  LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN score >= 90 THEN 'critical' WHEN score >= 70 THEN 'high'
                WHEN score >= 50 THEN 'elevated' WHEN score >= 20 THEN 'low' ELSE 'minimal' END
  $$;

ALTER TABLE evaluations DROP CONSTRAINT evaluations_risk_level_check;
ALTER TABLE evaluations DISABLE TRIGGER evaluations_insert_only;
UPDATE evaluations SET risk_level = band_for_score(risk_score);
ALTER TABLE evaluations ENABLE TRIGGER evaluations_insert_only;
ALTER TABLE evaluations ADD CONSTRAINT evaluations_risk_level_check
  CHECK (risk_level IN ('minimal', 'low', 'elevated', 'high', 'critical'));

ALTER TABLE events DISABLE TRIGGER events_insert_only;
UPDATE events SET risk_level = band_for_score(risk_score);
ALTER TABLE events ENABLE TRIGGER events_insert_only;

ALTER TABLE alert_evaluations DISABLE TRIGGER alert_evaluations_insert_only;
UPDATE alert_evaluations SET level = band_for_score(score);
ALTER TABLE alert_evaluations ENABLE TRIGGER alert_evaluations_insert_only;

ALTER TABLE alerts DROP CONSTRAINT alerts_severity_check;
UPDATE alerts SET severity = CASE WHEN kind = 'risk' THEN band_for_score(max_score)
                                  WHEN severity = 'medium' THEN 'elevated' ELSE severity END;
ALTER TABLE alerts ADD CONSTRAINT alerts_severity_check
  CHECK (severity IN ('minimal', 'low', 'elevated', 'high', 'critical'));
UPDATE alerts SET title = initcap(severity) || '-risk activity' WHERE kind = 'risk';

ALTER TABLE cases DROP CONSTRAINT cases_priority_check;
UPDATE cases SET priority = 'elevated' WHERE priority = 'medium';
ALTER TABLE cases ADD CONSTRAINT cases_priority_check
  CHECK (priority IN ('minimal', 'low', 'elevated', 'high', 'critical'));

DROP FUNCTION band_for_score(integer);

-- +goose Down
ALTER TABLE cases DROP CONSTRAINT cases_priority_check;
UPDATE cases SET priority = 'medium' WHERE priority IN ('elevated', 'minimal');
ALTER TABLE cases ADD CONSTRAINT cases_priority_check CHECK (priority IN ('low', 'medium', 'high', 'critical'));
ALTER TABLE alerts DROP CONSTRAINT alerts_severity_check;
UPDATE alerts SET severity = 'medium' WHERE severity IN ('elevated', 'minimal');
ALTER TABLE alerts ADD CONSTRAINT alerts_severity_check CHECK (severity IN ('low', 'medium', 'high', 'critical'));
ALTER TABLE evaluations DROP CONSTRAINT evaluations_risk_level_check;
ALTER TABLE evaluations DISABLE TRIGGER evaluations_insert_only;
UPDATE evaluations SET risk_level = 'medium' WHERE risk_level IN ('elevated', 'minimal');
ALTER TABLE evaluations ENABLE TRIGGER evaluations_insert_only;
ALTER TABLE evaluations ADD CONSTRAINT evaluations_risk_level_check CHECK (risk_level IN ('low', 'medium', 'high', 'critical'));
ALTER TABLE risk_settings DROP COLUMN thresholds;
