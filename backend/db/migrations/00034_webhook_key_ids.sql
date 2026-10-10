-- +goose Up

-- Webhook secrets are derived from a server-side key, the endpoint and a version, and are shown to
-- the customer only once. So rotating the server-side key cannot change a secret a customer holds:
-- each endpoint records which key its current (and, during an overlap, previous) secret was
-- derived with, and new secrets use the current key (ADR-0008, amended). Existing endpoints are
-- key 1, the key every deployment has used until now.
ALTER TABLE webhook_endpoints
  ADD COLUMN secret_key_id          smallint NOT NULL DEFAULT 1 CHECK (secret_key_id >= 1),
  ADD COLUMN previous_secret_key_id smallint CHECK (previous_secret_key_id >= 1);
UPDATE webhook_endpoints SET previous_secret_key_id = 1 WHERE previous_secret_version IS NOT NULL;

-- How many endpoints still depend on each key, across tenants and counts only. A key may be
-- removed from configuration only when none does; the API and worker refuse to start otherwise.
-- SECURITY DEFINER because the application role is confined to one tenant by row-level security.
CREATE FUNCTION webhook_key_usage()
  RETURNS TABLE (key_id integer, endpoints bigint)
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT k::integer, count(*)
    FROM (
      SELECT secret_key_id AS k FROM webhook_endpoints
      UNION ALL
      SELECT previous_secret_key_id FROM webhook_endpoints
      WHERE previous_secret_key_id IS NOT NULL AND previous_secret_valid_until > now()
    ) u
    GROUP BY k
    ORDER BY k
  $$;
REVOKE ALL ON FUNCTION webhook_key_usage() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION webhook_key_usage() TO riskos_app;

-- +goose Down
DROP FUNCTION webhook_key_usage();
ALTER TABLE webhook_endpoints DROP COLUMN previous_secret_key_id, DROP COLUMN secret_key_id;
