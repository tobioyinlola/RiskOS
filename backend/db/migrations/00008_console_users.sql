-- Console users, sessions, MFA and invites (build step 7). See docs/CONSOLE_AUTH.md.
--
-- The dashboard realm (ARCHITECTURE.md §9): people sign in with email, password and a mandatory
-- second factor (passkey or authenticator app), and hold a server-side session. Users belong to
-- exactly one tenant and see both of its environments. Passwords are Argon2id hashes; session
-- and invite tokens are stored as SHA-256 hashes; TOTP secrets are encrypted with a server key.

-- +goose Up

CREATE TABLE users (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenants (id),
  -- Lowercased. Globally unique: one account signs in to one tenant.
  email              text NOT NULL UNIQUE CHECK (email = lower(email) AND char_length(email) BETWEEN 3 AND 254),
  name               text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  role               text NOT NULL CHECK (role IN ('customer_admin', 'developer', 'risk_manager', 'analyst')),
  status             text NOT NULL CHECK (status IN ('active', 'disabled')),
  password_hash      text NOT NULL,
  -- AES-256-GCM ciphertext of the TOTP secret; NULL until enrolled.
  totp_secret_enc    bytea,
  -- Last accepted TOTP time step, so a code cannot be replayed.
  totp_last_step     bigint,
  -- SHA-256 hashes of unused recovery codes.
  recovery_hashes    bytea[] NOT NULL DEFAULT '{}',
  created_at         timestamptz NOT NULL,
  created_by         text NOT NULL,
  updated_at         timestamptz NOT NULL,
  last_login_at      timestamptz
);
CREATE INDEX users_tenant ON users (tenant_id, id);

CREATE TABLE webauthn_credentials (
  id                bytea PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenants (id),
  user_id           uuid NOT NULL REFERENCES users (id),
  name              text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  public_key        bytea NOT NULL,
  attestation_type  text NOT NULL,
  aaguid            bytea NOT NULL,
  sign_count        bigint NOT NULL,
  transports        text[] NOT NULL,
  flags             jsonb NOT NULL,
  created_at        timestamptz NOT NULL,
  last_used_at      timestamptz
);
CREATE INDEX webauthn_credentials_user ON webauthn_credentials (user_id);

CREATE TABLE console_sessions (
  id                  uuid PRIMARY KEY,
  tenant_id           uuid NOT NULL REFERENCES tenants (id),
  user_id             uuid NOT NULL REFERENCES users (id),
  token_hash          bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  -- mfa_required: password verified, second factor pending. enrollment_required: no second
  -- factor yet (new users). active: fully signed in.
  state               text NOT NULL CHECK (state IN ('mfa_required', 'enrollment_required', 'active')),
  created_at          timestamptz NOT NULL,
  last_seen_at        timestamptz NOT NULL,
  expires_at          timestamptz NOT NULL,
  -- Last password or second-factor confirmation; high-impact actions need a recent one.
  reauth_at           timestamptz,
  revoked_at          timestamptz,
  -- In-flight WebAuthn ceremony (challenge and options), cleared when used.
  webauthn_ceremony   jsonb,
  ip                  inet,
  user_agent          text CHECK (char_length(user_agent) <= 256)
);
CREATE INDEX console_sessions_user ON console_sessions (user_id, created_at DESC);

CREATE TABLE user_invites (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants (id),
  email        text NOT NULL CHECK (email = lower(email) AND char_length(email) BETWEEN 3 AND 254),
  role         text NOT NULL CHECK (role IN ('customer_admin', 'developer', 'risk_manager', 'analyst')),
  token_hash   bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  created_at   timestamptz NOT NULL,
  created_by   text NOT NULL,
  expires_at   timestamptz NOT NULL,
  accepted_at  timestamptz,
  revoked_at   timestamptz
);
CREATE INDEX user_invites_tenant ON user_invites (tenant_id, created_at DESC);

-- Tenant-level tables: isolated by tenant (a user sees both environments).
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY users_isolation ON users TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON users TO riskos_app;

ALTER TABLE webauthn_credentials ENABLE ROW LEVEL SECURITY;
CREATE POLICY webauthn_credentials_isolation ON webauthn_credentials TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON webauthn_credentials TO riskos_app;

ALTER TABLE console_sessions ENABLE ROW LEVEL SECURITY;
CREATE POLICY console_sessions_isolation ON console_sessions TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON console_sessions TO riskos_app;

ALTER TABLE user_invites ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_invites_isolation ON user_invites TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON user_invites TO riskos_app;

-- Pre-authentication lookups. Signing in, resuming a session and accepting an invite happen
-- before the tenant is known, so they cannot go through RLS. Like auth_lookup_api_key, each
-- function returns at most one row by an exact, unguessable or credential-bound key, runs with
-- the owner's rights and a pinned search_path, and is executable only by riskos_app.

CREATE FUNCTION auth_user_by_email(p_email text)
  RETURNS TABLE (id uuid, tenant_id uuid, status text, tenant_status text, password_hash text)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT u.id, u.tenant_id, u.status, t.status, u.password_hash
    FROM users u JOIN tenants t ON t.id = u.tenant_id
    WHERE u.email = lower(p_email)
  $$;
REVOKE ALL ON FUNCTION auth_user_by_email(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_user_by_email(text) TO riskos_app;

CREATE FUNCTION auth_session_by_token(p_token_hash bytea)
  RETURNS TABLE (
    id uuid, tenant_id uuid, user_id uuid, state text, created_at timestamptz,
    last_seen_at timestamptz, expires_at timestamptz, reauth_at timestamptz, revoked_at timestamptz,
    user_status text, user_role text, tenant_status text, tenant_name text, test_env_id uuid, production_env_id uuid
  )
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT s.id, s.tenant_id, s.user_id, s.state, s.created_at, s.last_seen_at,
           s.expires_at, s.reauth_at, s.revoked_at, u.status, u.role, t.status, t.name,
           (SELECT e.id FROM environments e WHERE e.tenant_id = s.tenant_id AND e.kind = 'test'),
           (SELECT e.id FROM environments e WHERE e.tenant_id = s.tenant_id AND e.kind = 'production')
    FROM console_sessions s
    JOIN users u ON u.id = s.user_id
    JOIN tenants t ON t.id = s.tenant_id
    WHERE s.token_hash = p_token_hash
  $$;
REVOKE ALL ON FUNCTION auth_session_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_session_by_token(bytea) TO riskos_app;

CREATE FUNCTION auth_invite_by_token(p_token_hash bytea)
  RETURNS TABLE (id uuid, tenant_id uuid, email text, role text, expires_at timestamptz,
                 accepted_at timestamptz, revoked_at timestamptz, tenant_status text)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT i.id, i.tenant_id, i.email, i.role, i.expires_at, i.accepted_at, i.revoked_at, t.status
    FROM user_invites i JOIN tenants t ON t.id = i.tenant_id
    WHERE i.token_hash = p_token_hash
  $$;
REVOKE ALL ON FUNCTION auth_invite_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_invite_by_token(bytea) TO riskos_app;

-- An email can only be taken once, across tenants; checking it before RLS applies avoids
-- revealing which tenant holds it.
CREATE FUNCTION auth_email_taken(p_email text) RETURNS boolean
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$ SELECT EXISTS (SELECT 1 FROM users WHERE email = lower(p_email)) $$;
REVOKE ALL ON FUNCTION auth_email_taken(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_email_taken(text) TO riskos_app;

-- +goose Down
DROP FUNCTION auth_email_taken(text);
DROP FUNCTION auth_invite_by_token(bytea);
DROP FUNCTION auth_session_by_token(bytea);
DROP FUNCTION auth_user_by_email(text);
DROP TABLE user_invites;
DROP TABLE console_sessions;
DROP TABLE webauthn_credentials;
DROP TABLE users;
