-- +goose Up

-- Rotating the API key pepper (ADR-0008): a key that verified against the previous pepper is
-- rehashed under the current one in the same request. That happens during authentication,
-- before the tenant is known, so like auth_lookup_api_key it cannot go through RLS. This is the
-- second narrow exception: it replaces one key's hash, only when the stored hash is still the one
-- that was just verified (compare and swap), and changes nothing else. It runs with the owner's
-- rights (SECURITY DEFINER) and a pinned search_path; only riskos_app may execute it.
CREATE FUNCTION auth_rehash_api_key(p_id uuid, p_old bytea, p_new bytea)
  RETURNS boolean
  LANGUAGE sql VOLATILE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    WITH updated AS (
      UPDATE api_keys SET secret_hash = p_new
      WHERE id = p_id AND secret_hash = p_old
      RETURNING 1
    )
    SELECT EXISTS (SELECT 1 FROM updated)
  $$;
REVOKE ALL ON FUNCTION auth_rehash_api_key(uuid, bytea, bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_rehash_api_key(uuid, bytea, bytea) TO riskos_app;

-- +goose Down
DROP FUNCTION auth_rehash_api_key(uuid, bytea, bytea);
