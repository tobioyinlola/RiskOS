-- Self-serve sign-up, Google sign-in and compliance onboarding. See docs/CONSOLE_AUTH.md#sign-up
-- and docs/adr/ADR-0005-self-serve-signup.md.
--
-- Anyone can create a workspace: with email, business name and password (the email proven by a
-- one-time code), or with Google. The new tenant can use its test environment at once; its
-- production environment stays locked until RiskOS staff approve its compliance onboarding.
-- Tenants created by operators keep production as before.

-- +goose Up

ALTER TABLE tenants
  ADD COLUMN production_enabled boolean NOT NULL DEFAULT true,
  ADD COLUMN origin text NOT NULL DEFAULT 'operator' CHECK (origin IN ('operator', 'self_serve'));

-- People who sign up with Google have no RiskOS password until they set one.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
-- Google's subject for the user, bound at the first Google sign-in (like sso_subject).
ALTER TABLE users ADD COLUMN google_subject text CHECK (char_length(google_subject) BETWEEN 1 AND 255);
CREATE UNIQUE INDEX users_google_subject ON users (google_subject) WHERE google_subject IS NOT NULL;

-- Sign-ups in flight, before any tenant exists: no RLS. A row is found only by the hash of a
-- 256-bit token held in the browser's cookie, or by its ID from the email job. The one-time code
-- is derived from the ID and code_sent_count with a server key, so it is never stored.
CREATE TABLE signups (
  id                 uuid PRIMARY KEY,
  token_hash         bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  email              text NOT NULL CHECK (email = lower(email) AND char_length(email) BETWEEN 3 AND 254),
  name               text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  -- NULL until known: Google sign-ups name their business after returning from Google.
  business_name      text CHECK (char_length(business_name) BETWEEN 1 AND 200),
  -- Argon2id; NULL for Google sign-ups.
  password_hash      text,
  google_subject     text CHECK (char_length(google_subject) BETWEEN 1 AND 255),
  -- The email already belongs to an account: the email says so instead of carrying a code, and
  -- no code verifies (the response never reveals it).
  existing_account   boolean NOT NULL,
  email_verified_at  timestamptz,
  code_sent_count    integer NOT NULL DEFAULT 0 CHECK (code_sent_count BETWEEN 0 AND 10),
  code_sent_at       timestamptz,
  failed_attempts    integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
  created_at         timestamptz NOT NULL,
  expires_at         timestamptz NOT NULL,
  completed_at       timestamptz,
  CHECK (password_hash IS NOT NULL OR google_subject IS NOT NULL)
);
CREATE INDEX signups_expiry ON signups (expires_at);
GRANT SELECT, INSERT, UPDATE, DELETE ON signups TO riskos_app;

-- Google authorization requests in flight: single use, ten minutes; like sso_logins, without a
-- tenant (the person may not have an account yet).
CREATE TABLE google_logins (
  state_hash    bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
  browser_hash  bytea NOT NULL CHECK (octet_length(browser_hash) = 32),
  created_at    timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL
);
CREATE INDEX google_logins_expiry ON google_logins (expires_at);
GRANT SELECT, INSERT, DELETE ON google_logins TO riskos_app;

-- Compliance onboarding (know your business), one per tenant. answers is validated by the
-- tenants module; staff decide through the admin realm.
CREATE TABLE tenant_onboarding (
  tenant_id     uuid PRIMARY KEY REFERENCES tenants (id),
  answers       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(answers) = 'object' AND octet_length(answers::text) <= 32768),
  status        text NOT NULL CHECK (status IN ('draft', 'submitted', 'approved', 'rejected')),
  updated_at    timestamptz NOT NULL,
  updated_by    text NOT NULL,
  submitted_at  timestamptz,
  submitted_by  text,
  reviewed_at   timestamptz,
  reviewed_by   text,
  review_note   text CHECK (char_length(review_note) <= 1000)
);
ALTER TABLE tenant_onboarding ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_onboarding_isolation ON tenant_onboarding TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON tenant_onboarding TO riskos_app;
CREATE INDEX tenant_onboarding_queue ON tenant_onboarding (status, submitted_at);

-- The session lookup also reports whether the tenant's production environment is open.
DROP FUNCTION auth_session_by_token(bytea);
CREATE FUNCTION auth_session_by_token(p_token_hash bytea)
  RETURNS TABLE (
    id uuid, tenant_id uuid, user_id uuid, state text, created_at timestamptz,
    last_seen_at timestamptz, expires_at timestamptz, reauth_at timestamptz, revoked_at timestamptz,
    user_status text, user_role text, tenant_status text, tenant_name text, test_env_id uuid, production_env_id uuid,
    production_enabled boolean
  )
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT s.id, s.tenant_id, s.user_id, s.state, s.created_at, s.last_seen_at,
           s.expires_at, s.reauth_at, s.revoked_at, u.status, u.role, t.status, t.name,
           (SELECT e.id FROM environments e WHERE e.tenant_id = s.tenant_id AND e.kind = 'test'),
           (SELECT e.id FROM environments e WHERE e.tenant_id = s.tenant_id AND e.kind = 'production'),
           t.production_enabled
    FROM console_sessions s
    JOIN users u ON u.id = s.user_id
    JOIN tenants t ON t.id = s.tenant_id
    WHERE s.token_hash = p_token_hash
  $$;
REVOKE ALL ON FUNCTION auth_session_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_session_by_token(bytea) TO riskos_app;

-- Which user, if any, has this Google subject (pre-authentication).
CREATE FUNCTION auth_user_by_google_subject(p_subject text)
  RETURNS TABLE (id uuid, tenant_id uuid, status text, tenant_status text)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT u.id, u.tenant_id, u.status, t.status
    FROM users u JOIN tenants t ON t.id = u.tenant_id
    WHERE u.google_subject = p_subject
  $$;
REVOKE ALL ON FUNCTION auth_user_by_google_subject(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_user_by_google_subject(text) TO riskos_app;

-- The staff review queue: identity, status and dates of every self-serve onboarding, never the
-- answers (staff read those for one tenant at a time, through a tenant scope, audited).
CREATE FUNCTION admin_onboarding_queue()
  RETURNS TABLE (tenant_id uuid, tenant_name text, tenant_status text, created_at timestamptz,
                 status text, submitted_at timestamptz, reviewed_at timestamptz)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT t.id, t.name, t.status, t.created_at, o.status, o.submitted_at, o.reviewed_at
    FROM tenant_onboarding o JOIN tenants t ON t.id = o.tenant_id
    ORDER BY (o.status = 'submitted') DESC, o.submitted_at DESC NULLS LAST, t.created_at DESC
  $$;
REVOKE ALL ON FUNCTION admin_onboarding_queue() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_onboarding_queue() TO riskos_app;

-- +goose Down
DROP FUNCTION admin_onboarding_queue();
DROP FUNCTION auth_user_by_google_subject(text);
DROP FUNCTION auth_session_by_token(bytea);
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
DROP TABLE tenant_onboarding;
DROP TABLE google_logins;
DROP TABLE signups;
DROP INDEX users_google_subject;
ALTER TABLE users DROP COLUMN google_subject;
-- Google-only users get an unusable password before the column becomes required again.
UPDATE users SET password_hash = '!' WHERE password_hash IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
ALTER TABLE tenants DROP COLUMN origin, DROP COLUMN production_enabled;
