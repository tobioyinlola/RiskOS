-- Single sign-on with OpenID Connect, per tenant. See docs/CONSOLE_AUTH.md#single-sign-on.
--
-- A tenant connects one identity provider. SSO signs in people who already have an account
-- (invited as usual); it never creates accounts. After SSO, RiskOS still asks for its own second
-- factor.

-- +goose Up
CREATE TABLE sso_connections (
  tenant_id          uuid PRIMARY KEY REFERENCES tenants (id),
  issuer             text NOT NULL CHECK (char_length(issuer) BETWEEN 8 AND 512),
  client_id          text NOT NULL CHECK (char_length(client_id) BETWEEN 1 AND 256),
  -- AES-256-GCM under the console secret, bound to the tenant.
  client_secret_enc  bytea NOT NULL,
  enabled            boolean NOT NULL,
  updated_at         timestamptz NOT NULL,
  updated_by         text NOT NULL
);
ALTER TABLE sso_connections ENABLE ROW LEVEL SECURITY;
CREATE POLICY sso_connections_isolation ON sso_connections TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sso_connections TO riskos_app;

-- The identity provider's subject for the user, bound at the first SSO sign-in. Later sign-ins
-- must present the same subject, so an email reassigned at the provider cannot take the account.
ALTER TABLE users ADD COLUMN sso_subject text CHECK (char_length(sso_subject) BETWEEN 1 AND 255);
CREATE UNIQUE INDEX users_sso_subject ON users (tenant_id, sso_subject) WHERE sso_subject IS NOT NULL;

-- Authorization requests in flight: single use, ten minutes. The state token is stored hashed;
-- browser_hash binds the request to the browser that started it (login CSRF). The PKCE verifier
-- and the nonce are derived from the state with a server key, so they are not stored.
-- Pre-authentication data with no tenant scope yet: no RLS.
CREATE TABLE sso_logins (
  state_hash    bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
  browser_hash  bytea NOT NULL CHECK (octet_length(browser_hash) = 32),
  tenant_id     uuid NOT NULL,
  user_id       uuid NOT NULL,
  created_at    timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL
);
CREATE INDEX sso_logins_expiry ON sso_logins (expires_at);
GRANT SELECT, INSERT, DELETE ON sso_logins TO riskos_app;

-- Which identity provider, if any, signs in the user with this email (pre-authentication).
CREATE FUNCTION auth_sso_for_email(p_email text)
  RETURNS TABLE (user_id uuid, tenant_id uuid, user_status text, tenant_status text, sso_enabled boolean)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT u.id, u.tenant_id, u.status, t.status, coalesce(c.enabled, false)
    FROM users u
    JOIN tenants t ON t.id = u.tenant_id
    LEFT JOIN sso_connections c ON c.tenant_id = u.tenant_id
    WHERE u.email = lower(p_email)
  $$;
REVOKE ALL ON FUNCTION auth_sso_for_email(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_sso_for_email(text) TO riskos_app;

-- +goose Down
DROP FUNCTION auth_sso_for_email(text);
DROP TABLE sso_logins;
DROP INDEX users_sso_subject;
ALTER TABLE users DROP COLUMN sso_subject;
DROP TABLE sso_connections;
