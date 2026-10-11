-- +goose Up

-- Just-in-time accounts (CONSOLE_AUTH.md#just-in-time-accounts). A workspace can let people whose
-- email is at a domain it has proved it controls sign in through its identity provider without an
-- invite: the first sign-in creates the account with a fixed role chosen by the workspace, never
-- an administrator.
ALTER TABLE sso_connections
  ADD COLUMN jit_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN jit_role text NOT NULL DEFAULT 'auditor'
    CHECK (jit_role IN ('developer', 'risk_manager', 'analyst', 'compliance_officer', 'auditor')),
  ADD CONSTRAINT sso_jit_needs_enabled CHECK (NOT jit_enabled OR enabled);

-- Email domains a workspace claims. A claim is proved by a DNS TXT record the workspace controls;
-- only a verified claim is used, and only one workspace can hold a domain verified.
CREATE TABLE sso_domains (
  tenant_id   uuid NOT NULL REFERENCES tenants (id),
  domain      text NOT NULL CHECK (domain = lower(domain) AND char_length(domain) BETWEEN 4 AND 253),
  created_at  timestamptz NOT NULL,
  created_by  text NOT NULL,
  verified_at timestamptz,
  PRIMARY KEY (tenant_id, domain)
);
CREATE UNIQUE INDEX sso_domains_verified ON sso_domains (domain) WHERE verified_at IS NOT NULL;
ALTER TABLE sso_domains ENABLE ROW LEVEL SECURITY;
CREATE POLICY sso_domains_isolation ON sso_domains TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sso_domains TO riskos_app;

-- A sign-in in flight for someone with no account yet: the email they asked to sign in with.
ALTER TABLE sso_logins
  ALTER COLUMN user_id DROP NOT NULL,
  ADD COLUMN email text,
  ADD CONSTRAINT sso_logins_who CHECK (user_id IS NOT NULL OR email IS NOT NULL);

-- The workspace that has verified p_domain and lets its people in just in time (pre-authentication).
CREATE FUNCTION auth_sso_for_domain(p_domain text)
  RETURNS TABLE (tenant_id uuid, tenant_status text, sso_enabled boolean, jit_enabled boolean, jit_role text)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT d.tenant_id, t.status, c.enabled, c.jit_enabled, c.jit_role
    FROM sso_domains d
    JOIN tenants t ON t.id = d.tenant_id
    JOIN sso_connections c ON c.tenant_id = d.tenant_id
    WHERE d.domain = lower(p_domain) AND d.verified_at IS NOT NULL
  $$;
REVOKE ALL ON FUNCTION auth_sso_for_domain(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_sso_for_domain(text) TO riskos_app;

-- +goose Down
DROP FUNCTION auth_sso_for_domain(text);
ALTER TABLE sso_logins DROP CONSTRAINT sso_logins_who, DROP COLUMN email;
DELETE FROM sso_logins WHERE user_id IS NULL;
ALTER TABLE sso_logins ALTER COLUMN user_id SET NOT NULL;
DROP TABLE sso_domains;
ALTER TABLE sso_connections
  DROP CONSTRAINT sso_jit_needs_enabled, DROP COLUMN jit_role, DROP COLUMN jit_enabled;
