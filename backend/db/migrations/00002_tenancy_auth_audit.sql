-- Tenants, environments, API keys and the audit log (build step 2).
--
-- Isolation (TENANCY.md): every tenant table has row-level security keyed to transaction-local
-- settings app.tenant_id / app.environment_id, which the application sets with SET LOCAL
-- semantics in each transaction. The application connects as riskos_app, which owns nothing and
-- has no BYPASSRLS, so the policies always apply to it. Migrations run as the owner.

-- +goose Up

-- The application role. In deployed environments it is created by the platform with LOGIN and a
-- password from the secrets manager; this only guarantees the role exists so grants succeed.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'riskos_app') THEN
    CREATE ROLE riskos_app NOLOGIN;
  END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO riskos_app;

-- Current scope. NULLIF turns an unset setting into NULL, so every policy comparison is false and
-- a query without a scope sees nothing (fail closed).
CREATE FUNCTION app_tenant_id() RETURNS uuid
  LANGUAGE sql STABLE
  AS $$ SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid $$;

CREATE FUNCTION app_environment_id() RETURNS uuid
  LANGUAGE sql STABLE
  AS $$ SELECT NULLIF(current_setting('app.environment_id', true), '')::uuid $$;

-- Tenants -----------------------------------------------------------------------------------

CREATE TABLE tenants (
  id          uuid PRIMARY KEY,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  -- Regional cell the tenant is pinned to (ARCHITECTURE.md §16.6). Never changes.
  region      text NOT NULL CHECK (region ~ '^[a-z0-9-]{2,32}$'),
  status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'disabled')),
  created_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenants_isolation ON tenants TO riskos_app
  USING (id = app_tenant_id())
  WITH CHECK (id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON tenants TO riskos_app;

-- Environments ------------------------------------------------------------------------------

CREATE TABLE environments (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants (id),
  kind        text NOT NULL CHECK (kind IN ('test', 'production')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, kind),
  UNIQUE (tenant_id, id)
);

ALTER TABLE environments ENABLE ROW LEVEL SECURITY;
CREATE POLICY environments_isolation ON environments TO riskos_app
  USING (tenant_id = app_tenant_id())
  WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT ON environments TO riskos_app;

-- API keys ----------------------------------------------------------------------------------

CREATE TABLE api_keys (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  environment_id  uuid NOT NULL,
  name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  -- Public lookup part of the key. Globally unique; not a secret.
  lookup          text NOT NULL UNIQUE,
  -- HMAC-SHA256 of the secret with a server-side pepper. The secret itself is never stored.
  secret_hash     bytea NOT NULL CHECK (octet_length(secret_hash) = 32),
  scopes          text[] NOT NULL,
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  expires_at      timestamptz,
  revoked_at      timestamptz,
  -- Set when the key is rotated: the key that replaces it.
  replaced_by     uuid REFERENCES api_keys (id),
  FOREIGN KEY (tenant_id, environment_id) REFERENCES environments (tenant_id, id)
);

CREATE INDEX api_keys_by_scope ON api_keys (tenant_id, environment_id, id DESC);

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY api_keys_isolation ON api_keys TO riskos_app
  USING (tenant_id = app_tenant_id() AND environment_id = app_environment_id())
  WITH CHECK (tenant_id = app_tenant_id() AND environment_id = app_environment_id());
GRANT SELECT, INSERT, UPDATE ON api_keys TO riskos_app;

-- Authentication happens before the tenant is known, so key lookup cannot go through RLS. This
-- function is the single, narrow exception: it returns at most one key by its lookup value,
-- with the minimum needed to verify it. It runs with the owner's rights (SECURITY DEFINER) and a
-- pinned search_path; only riskos_app may execute it.
CREATE FUNCTION auth_lookup_api_key(p_lookup text)
  RETURNS TABLE (
    id               uuid,
    tenant_id        uuid,
    environment_id   uuid,
    environment_kind text,
    secret_hash      bytea,
    scopes           text[],
    expires_at       timestamptz,
    revoked_at       timestamptz,
    tenant_status    text
  )
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT k.id, k.tenant_id, k.environment_id, e.kind, k.secret_hash, k.scopes,
           k.expires_at, k.revoked_at, t.status
    FROM api_keys k
    JOIN environments e ON e.id = k.environment_id
    JOIN tenants t ON t.id = k.tenant_id
    WHERE k.lookup = p_lookup
  $$;
REVOKE ALL ON FUNCTION auth_lookup_api_key(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_lookup_api_key(text) TO riskos_app;

-- Audit log ---------------------------------------------------------------------------------

-- Append-only and hash-chained per tenant: hash = SHA-256(canonical entry || prev_hash), and
-- seq increases by one per tenant, so a modified, removed or reordered entry is detectable.
CREATE TABLE audit_log (
  tenant_id       uuid NOT NULL REFERENCES tenants (id),
  seq             bigint NOT NULL CHECK (seq >= 1),
  id              uuid NOT NULL UNIQUE,
  -- NULL for tenant-level actions; otherwise the environment the action happened in.
  environment_id  uuid,
  occurred_at     timestamptz NOT NULL,
  actor_type      text NOT NULL,
  actor_id        text NOT NULL,
  action          text NOT NULL,
  resource_type   text NOT NULL,
  resource_id     text NOT NULL,
  outcome         text NOT NULL CHECK (outcome IN ('success', 'failure', 'denied')),
  request_id      text NOT NULL,
  -- Canonical JSON stored as text, not jsonb: jsonb re-formats its input, and the hash must be
  -- recomputable from the exact bytes that were hashed.
  metadata        text NOT NULL CHECK (metadata IS JSON OBJECT),
  prev_hash       bytea NOT NULL CHECK (octet_length(prev_hash) = 32),
  hash            bytea NOT NULL CHECK (octet_length(hash) = 32),
  PRIMARY KEY (tenant_id, seq)
);

CREATE INDEX audit_log_by_id ON audit_log (tenant_id, id DESC);

-- Immutability for everyone, including the owner: corrections are new entries.
CREATE FUNCTION audit_log_reject_change() RETURNS trigger
  LANGUAGE plpgsql
  AS $$ BEGIN RAISE EXCEPTION 'audit_log is append-only'; END $$;

CREATE TRIGGER audit_log_no_update_delete BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_reject_change();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
  FOR EACH STATEMENT EXECUTE FUNCTION audit_log_reject_change();

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_read ON audit_log FOR SELECT TO riskos_app
  USING (tenant_id = app_tenant_id()
         AND (environment_id IS NULL OR environment_id = app_environment_id()));
CREATE POLICY audit_log_append ON audit_log FOR INSERT TO riskos_app
  WITH CHECK (tenant_id = app_tenant_id()
              AND (environment_id IS NULL OR environment_id = app_environment_id()));
GRANT SELECT, INSERT ON audit_log TO riskos_app;

-- Head of each tenant's chain. Entries are visible per environment, but the chain spans the whole
-- tenant, so appends read and advance the head here. Locking the head row (FOR UPDATE)
-- serializes concurrent appends for a tenant. Verification compares the recomputed chain with it.
CREATE TABLE audit_chain_heads (
  tenant_id  uuid PRIMARY KEY REFERENCES tenants (id),
  seq        bigint NOT NULL CHECK (seq >= 0),
  hash       bytea NOT NULL CHECK (octet_length(hash) = 32)
);

ALTER TABLE audit_chain_heads ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_chain_heads_isolation ON audit_chain_heads TO riskos_app
  USING (tenant_id = app_tenant_id())
  WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON audit_chain_heads TO riskos_app;

-- +goose Down
DROP TABLE audit_chain_heads;
DROP TABLE audit_log;
DROP FUNCTION audit_log_reject_change();
DROP FUNCTION auth_lookup_api_key(text);
DROP TABLE api_keys;
DROP TABLE environments;
DROP TABLE tenants;
DROP FUNCTION app_environment_id();
DROP FUNCTION app_tenant_id();
REVOKE USAGE ON SCHEMA public FROM riskos_app;
