-- The staff security overview (docs/ADMIN.md) ran one correlated scan per tenant over all of a
-- tenant's evaluations: seconds with a few hundred thousand rows, and it grows with every event.
-- It now aggregates each source once, through partial indexes that hold only the rows it counts
-- (degraded evaluations and failed sign-ins are a small fraction of their tables).

-- +goose Up
CREATE INDEX evaluations_degraded ON evaluations (tenant_id, created_at)
  WHERE cardinality(degraded_signals) > 0 AND NOT dry_run;
CREATE INDEX audit_log_sign_in_failures ON audit_log (tenant_id, occurred_at)
  WHERE action IN ('user.login_failed', 'user.mfa_failed', 'user.sso_failed');
CREATE INDEX alerts_open_security ON alerts (tenant_id)
  WHERE kind = 'security' AND status IN ('open', 'investigating', 'escalated');

CREATE OR REPLACE FUNCTION admin_security_overview(p_since timestamptz)
  RETURNS TABLE (tenant_id uuid, tenant_name text, tenant_status text,
                 degraded_evaluations bigint, open_security_alerts bigint,
                 failed_logins bigint, disabled_webhooks bigint)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    WITH degraded AS (
      SELECT e.tenant_id, count(*) AS n FROM evaluations e
      WHERE e.created_at >= p_since AND cardinality(e.degraded_signals) > 0 AND NOT e.dry_run
      GROUP BY e.tenant_id),
    security AS (
      SELECT a.tenant_id, count(*) AS n FROM alerts a
      WHERE a.kind = 'security' AND a.status IN ('open', 'investigating', 'escalated')
      GROUP BY a.tenant_id),
    failures AS (
      SELECT l.tenant_id, count(*) AS n FROM audit_log l
      WHERE l.occurred_at >= p_since AND l.action IN ('user.login_failed', 'user.mfa_failed', 'user.sso_failed')
      GROUP BY l.tenant_id),
    hooks AS (
      SELECT w.tenant_id, count(*) AS n FROM webhook_endpoints w WHERE w.status = 'disabled'
      GROUP BY w.tenant_id)
    SELECT t.id, t.name, t.status,
           coalesce(d.n, 0), coalesce(s.n, 0), coalesce(f.n, 0), coalesce(h.n, 0)
    FROM tenants t
    LEFT JOIN degraded d ON d.tenant_id = t.id
    LEFT JOIN security s ON s.tenant_id = t.id
    LEFT JOIN failures f ON f.tenant_id = t.id
    LEFT JOIN hooks h ON h.tenant_id = t.id
  $$;

-- +goose Down
DROP INDEX alerts_open_security;
DROP INDEX audit_log_sign_in_failures;
DROP INDEX evaluations_degraded;
-- The previous function body is in 00016_admin_realm.sql; this version returns the same rows.
