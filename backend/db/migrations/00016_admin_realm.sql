-- The internal admin realm: RiskOS staff. See docs/ADMIN.md.
--
-- Staff are a separate set of accounts (not tenant users), created only by the operator CLI and
-- signing in with passkeys only. Staff hold no access to tenant data. Support access to one
-- tenant is requested by one staff member, approved by another, read-only, time-boxed, and
-- recorded in the tenant's own audit log. These are platform tables (no tenant): no RLS.

-- +goose Up
CREATE TABLE staff (
  id                 uuid PRIMARY KEY,
  email              text NOT NULL UNIQUE CHECK (email = lower(email) AND char_length(email) BETWEEN 3 AND 254),
  name               text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  role               text NOT NULL CHECK (role IN ('operations_admin', 'security_admin')),
  status             text NOT NULL CHECK (status IN ('active', 'disabled')),
  -- Single-use link to register the first passkey; cleared once used.
  enroll_token_hash  bytea UNIQUE CHECK (octet_length(enroll_token_hash) = 32),
  enroll_expires_at  timestamptz,
  created_at         timestamptz NOT NULL,
  created_by         text NOT NULL
);
GRANT SELECT, INSERT, UPDATE ON staff TO riskos_app;

CREATE TABLE staff_passkeys (
  id                bytea PRIMARY KEY,
  staff_id          uuid NOT NULL REFERENCES staff (id),
  name              text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  public_key        bytea NOT NULL,
  attestation_type  text NOT NULL,
  aaguid            bytea,
  sign_count        bigint NOT NULL,
  transports        text[] NOT NULL,
  flags             jsonb NOT NULL,
  created_at        timestamptz NOT NULL,
  last_used_at      timestamptz
);
CREATE INDEX staff_passkeys_staff ON staff_passkeys (staff_id);
GRANT SELECT, INSERT, UPDATE ON staff_passkeys TO riskos_app;

CREATE TABLE staff_sessions (
  id            uuid PRIMARY KEY,
  staff_id      uuid NOT NULL REFERENCES staff (id),
  token_hash    bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  created_at    timestamptz NOT NULL,
  last_seen_at  timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL,
  revoked_at    timestamptz,
  ip            inet,
  user_agent    text NOT NULL
);
CREATE INDEX staff_sessions_staff ON staff_sessions (staff_id);
GRANT SELECT, INSERT, UPDATE ON staff_sessions TO riskos_app;

-- WebAuthn ceremonies in flight (two minutes); the ID is returned to the browser, stored hashed.
CREATE TABLE staff_ceremonies (
  id_hash     bytea PRIMARY KEY CHECK (octet_length(id_hash) = 32),
  kind        text NOT NULL CHECK (kind IN ('enroll', 'login')),
  staff_id    uuid REFERENCES staff (id),
  data        jsonb NOT NULL,
  expires_at  timestamptz NOT NULL
);
CREATE INDEX staff_ceremonies_expiry ON staff_ceremonies (expires_at);
GRANT SELECT, INSERT, DELETE ON staff_ceremonies TO riskos_app;

CREATE TABLE support_access (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenants (id),
  requested_by      uuid NOT NULL REFERENCES staff (id),
  reason            text NOT NULL CHECK (char_length(reason) BETWEEN 10 AND 500),
  duration_minutes  integer NOT NULL CHECK (duration_minutes BETWEEN 15 AND 240),
  status            text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
  created_at        timestamptz NOT NULL,
  decided_by        uuid REFERENCES staff (id),
  decided_at        timestamptz,
  -- Set on approval: access runs from the approval for duration_minutes.
  expires_at        timestamptz,
  revoked_by        uuid REFERENCES staff (id),
  revoked_at        timestamptz,
  -- Four eyes, enforced by the database too: nobody approves their own request.
  CHECK (decided_by IS NULL OR decided_by <> requested_by),
  CHECK ((status = 'approved') = (expires_at IS NOT NULL) OR status = 'revoked')
);
CREATE INDEX support_access_recent ON support_access (created_at DESC);
GRANT SELECT, INSERT, UPDATE ON support_access TO riskos_app;

-- Every staff action, append-only for the application role (no UPDATE or DELETE grant).
CREATE TABLE staff_audit_log (
  id           uuid PRIMARY KEY,
  occurred_at  timestamptz NOT NULL,
  actor        text NOT NULL,
  action       text NOT NULL,
  tenant_id    uuid,
  resource_id  text NOT NULL,
  outcome      text NOT NULL CHECK (outcome IN ('success', 'failure')),
  metadata     jsonb NOT NULL
);
CREATE INDEX staff_audit_log_recent ON staff_audit_log (occurred_at DESC);
GRANT SELECT, INSERT ON staff_audit_log TO riskos_app;

-- Tenants as staff may see them: identity and status only.
CREATE FUNCTION admin_tenants()
  RETURNS TABLE (id uuid, name text, status text, created_at timestamptz)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$ SELECT t.id, t.name, t.status, t.created_at FROM tenants t ORDER BY t.created_at DESC $$;
REVOKE ALL ON FUNCTION admin_tenants() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants() TO riskos_app;

-- The cross-tenant security view: counts per tenant, never tenant data.
CREATE FUNCTION admin_security_overview(p_since timestamptz)
  RETURNS TABLE (tenant_id uuid, tenant_name text, tenant_status text,
                 degraded_evaluations bigint, open_security_alerts bigint,
                 failed_logins bigint, disabled_webhooks bigint)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT t.id, t.name, t.status,
      (SELECT count(*) FROM evaluations e
        WHERE e.tenant_id = t.id AND e.created_at >= p_since AND NOT e.dry_run
          AND cardinality(e.degraded_signals) > 0),
      (SELECT count(*) FROM alerts a
        WHERE a.tenant_id = t.id AND a.kind = 'security' AND a.status IN ('open', 'investigating', 'escalated')),
      (SELECT count(*) FROM audit_log l
        WHERE l.tenant_id = t.id AND l.occurred_at >= p_since AND l.action IN ('user.login_failed', 'user.mfa_failed', 'user.sso_failed')),
      (SELECT count(*) FROM webhook_endpoints w WHERE w.tenant_id = t.id AND w.status = 'disabled')
    FROM tenants t
  $$;
REVOKE ALL ON FUNCTION admin_security_overview(timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_security_overview(timestamptz) TO riskos_app;

-- +goose Down
DROP FUNCTION admin_security_overview(timestamptz);
DROP FUNCTION admin_tenants();
DROP TABLE staff_audit_log;
DROP TABLE support_access;
DROP TABLE staff_ceremonies;
DROP TABLE staff_sessions;
DROP TABLE staff_passkeys;
DROP TABLE staff;
