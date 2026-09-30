-- Every query takes tenant_id and environment_id explicitly (TENANCY.md, layer 1).

-- name: LockAlertSubject :exec
-- Serializes raising alerts for one subject, so concurrent evaluations group into one alert.
-- Transaction-scoped: released at commit or rollback.
SELECT pg_advisory_xact_lock(hashtextextended(@lock_key::text, 0));

-- name: EvaluationAlerted :one
SELECT EXISTS (
  SELECT 1 FROM alert_evaluations
  WHERE tenant_id = @tenant_id AND environment_id = @environment_id
    AND evaluation_id = @evaluation_id AND alert_kind = @alert_kind
);

-- name: FindGroupingAlert :one
SELECT id, severity, max_score, first_seen_at, last_seen_at
FROM alerts
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND kind = @kind AND subject_ref = @subject_ref
  AND status IN ('open', 'investigating') AND last_seen_at >= @since
ORDER BY last_seen_at DESC
LIMIT 1
FOR UPDATE;

-- name: InsertAlert :exec
INSERT INTO alerts (
  tenant_id, environment_id, id, kind, subject_type, subject_ref, title, severity, status,
  max_score, evaluation_count, first_seen_at, last_seen_at, created_at, updated_at
) VALUES (
  @tenant_id, @environment_id, @id, @kind, @subject_type, @subject_ref, @title, @severity, 'open',
  @max_score, @evaluation_count, @seen_at, @seen_at, @now, @now
);

-- name: AddToAlert :exec
UPDATE alerts SET
  severity = @severity,
  title = @title,
  max_score = GREATEST(max_score, @score::smallint),
  evaluation_count = evaluation_count + 1,
  first_seen_at = LEAST(first_seen_at, @seen_at),
  last_seen_at = GREATEST(last_seen_at, @seen_at),
  updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: InsertAlertEvaluation :exec
INSERT INTO alert_evaluations (
  tenant_id, environment_id, alert_id, alert_kind, evaluation_id, event_id, event_type,
  score, level, decision, signals, degraded_signals, evaluated_at
) VALUES (
  @tenant_id, @environment_id, @alert_id, @alert_kind, @evaluation_id, @event_id, @event_type,
  @score, @level, @decision, @signals, @degraded_signals, @evaluated_at
);

-- name: GetAlert :one
SELECT * FROM alerts
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: GetAlertForUpdate :one
SELECT * FROM alerts
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id
FOR UPDATE;

-- name: ListAlerts :many
SELECT * FROM alerts
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('before_id')::uuid IS NULL OR id < sqlc.narg('before_id')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- name: ListAlertsByCase :many
SELECT * FROM alerts
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND case_id = @case_id
ORDER BY id;

-- name: ListAlertEvaluations :many
SELECT * FROM alert_evaluations
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND alert_id = @alert_id
ORDER BY evaluated_at DESC
LIMIT @row_limit;

-- name: UpdateAlertTriage :exec
UPDATE alerts SET status = @status, assignee = sqlc.narg('assignee'), updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: EscalateAlert :exec
UPDATE alerts SET status = 'escalated', case_id = @case_id, updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND id = @id;

-- name: CloseCaseAlerts :many
UPDATE alerts SET status = @status, updated_at = @now
WHERE tenant_id = @tenant_id AND environment_id = @environment_id
  AND case_id = @case_id AND status = 'escalated'
RETURNING id;

-- name: InsertAlertActivity :exec
INSERT INTO alert_activity (tenant_id, environment_id, id, alert_id, action, actor, detail, created_at)
VALUES (@tenant_id, @environment_id, @id, @alert_id, @action, @actor, @detail, @created_at);

-- name: ListAlertActivity :many
SELECT id, action, actor, detail, created_at FROM alert_activity
WHERE tenant_id = @tenant_id AND environment_id = @environment_id AND alert_id = @alert_id
ORDER BY id;
