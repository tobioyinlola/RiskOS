-- +goose Up

-- The staff console's tenant directory and work queue (ADR-0010). Like admin_tenants() and
-- admin_security_overview(), they return identity, status and counts only, never tenant data, and
-- exist because the application role is confined to one tenant by row-level security.

CREATE FUNCTION admin_tenant_directory()
  RETURNS TABLE (
    id uuid, name text, region text, status text, origin text, production_enabled boolean,
    created_at timestamptz, active_users bigint, active_api_keys bigint, onboarding_status text
  )
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT t.id, t.name, t.region, t.status, t.origin, t.production_enabled, t.created_at,
      (SELECT count(*) FROM users u WHERE u.tenant_id = t.id AND u.status = 'active'),
      (SELECT count(*) FROM api_keys k
        WHERE k.tenant_id = t.id AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at > now())),
      -- Empty when the tenant has no onboarding (operator-created tenants).
      COALESCE((SELECT o.status FROM tenant_onboarding o WHERE o.tenant_id = t.id), '')
    FROM tenants t
    ORDER BY t.created_at DESC
  $$;
REVOKE ALL ON FUNCTION admin_tenant_directory() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenant_directory() TO riskos_app;

-- What waits for the staff member p_staff: counts, each of which a page clears. Support requests
-- count only those someone else made, because nobody may decide their own.
CREATE FUNCTION admin_overview(p_staff uuid)
  RETURNS TABLE (
    tenants_active bigint, tenants_suspended bigint, tenants_total bigint,
    onboarding_submitted bigint, support_pending bigint, support_active bigint, staff_active bigint
  )
  LANGUAGE sql STABLE SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
    SELECT
      (SELECT count(*) FROM tenants WHERE status = 'active'),
      (SELECT count(*) FROM tenants WHERE status = 'suspended'),
      (SELECT count(*) FROM tenants),
      (SELECT count(*) FROM tenant_onboarding WHERE status = 'submitted'),
      (SELECT count(*) FROM support_access WHERE status = 'pending' AND requested_by <> p_staff),
      (SELECT count(*) FROM support_access WHERE status = 'approved' AND expires_at > now()),
      (SELECT count(*) FROM staff WHERE status = 'active')
  $$;
REVOKE ALL ON FUNCTION admin_overview(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_overview(uuid) TO riskos_app;

-- +goose Down
DROP FUNCTION admin_overview(uuid);
DROP FUNCTION admin_tenant_directory();
