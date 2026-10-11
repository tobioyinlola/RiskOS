-- +goose Up

-- Suspending or reactivating a tenant needs two staff members (ARCHITECTURE.md §16, ADR-0010): one
-- requests with a reason, a different one decides. Like support_access this is a staff-realm
-- table with no tenant isolation; the application role reaches it from the staff console only.
CREATE TABLE tenant_status_requests (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenants (id),
  action        text NOT NULL CHECK (action IN ('suspend', 'reactivate')),
  reason        text NOT NULL CHECK (char_length(reason) BETWEEN 10 AND 500),
  status        text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
  requested_by  uuid NOT NULL REFERENCES staff (id),
  created_at    timestamptz NOT NULL,
  decided_by    uuid REFERENCES staff (id),
  decided_at    timestamptz,
  decision_note text CHECK (char_length(decision_note) <= 500),
  -- Four eyes, enforced by the database: nobody decides their own request. The requester may
  -- withdraw it, which is not a decision.
  CHECK (decided_by IS NULL OR status = 'withdrawn' OR decided_by <> requested_by),
  CHECK (status = 'withdrawn' OR (status = 'pending') = (decided_by IS NULL))
);
-- One open request per tenant: a second would race the first.
CREATE UNIQUE INDEX tenant_status_requests_one_pending ON tenant_status_requests (tenant_id) WHERE status = 'pending';
CREATE INDEX tenant_status_requests_recent ON tenant_status_requests (created_at DESC);
GRANT SELECT, INSERT, UPDATE ON tenant_status_requests TO riskos_app;

-- Decides a request and, on approval, changes the tenant's status, as one step. The tenants table
-- is protected by row-level security, so the change is made here (SECURITY DEFINER, pinned
-- search_path, executable only by riskos_app) and only ever as the outcome of an approved
-- request: no caller can set a status directly. Returns the outcome instead of raising, so the
-- caller can tell "not found", "not pending", "own request" and "no longer applicable" apart.
-- +goose StatementBegin
CREATE FUNCTION admin_decide_tenant_status(
  p_request uuid, p_decider uuid, p_approve boolean, p_note text, p_now timestamptz
) RETURNS TABLE (outcome text, tenant_id uuid, action text)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  r tenant_status_requests%ROWTYPE;
  current_status text;
BEGIN
  SELECT * INTO r FROM tenant_status_requests WHERE id = p_request FOR UPDATE;
  IF NOT FOUND THEN
    RETURN QUERY SELECT 'not_found'::text, NULL::uuid, NULL::text;
    RETURN;
  END IF;
  IF r.status <> 'pending' THEN
    RETURN QUERY SELECT 'not_pending'::text, r.tenant_id, r.action;
    RETURN;
  END IF;
  IF r.requested_by = p_decider THEN
    RETURN QUERY SELECT 'own_request'::text, r.tenant_id, r.action;
    RETURN;
  END IF;
  IF NOT p_approve THEN
    UPDATE tenant_status_requests
      SET status = 'rejected', decided_by = p_decider, decided_at = p_now, decision_note = NULLIF(p_note, '')
      WHERE id = p_request;
    RETURN QUERY SELECT 'rejected'::text, r.tenant_id, r.action;
    RETURN;
  END IF;
  SELECT t.status INTO current_status FROM tenants t WHERE t.id = r.tenant_id FOR UPDATE;
  IF (r.action = 'suspend' AND current_status <> 'active')
     OR (r.action = 'reactivate' AND current_status <> 'suspended') THEN
    -- The tenant changed since the request: leave it pending so the decider can reject it.
    RETURN QUERY SELECT 'not_applicable'::text, r.tenant_id, r.action;
    RETURN;
  END IF;
  UPDATE tenants SET status = CASE r.action WHEN 'suspend' THEN 'suspended' ELSE 'active' END
    WHERE id = r.tenant_id;
  UPDATE tenant_status_requests
    SET status = 'approved', decided_by = p_decider, decided_at = p_now, decision_note = NULLIF(p_note, '')
    WHERE id = p_request;
  RETURN QUERY SELECT 'applied'::text, r.tenant_id, r.action;
END;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION admin_decide_tenant_status(uuid, uuid, boolean, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_decide_tenant_status(uuid, uuid, boolean, text, timestamptz) TO riskos_app;

-- The work queue counts requests waiting for the staff member (never their own). The result
-- gains a column, so the function is replaced rather than altered.
DROP FUNCTION admin_overview(uuid);
CREATE FUNCTION admin_overview(p_staff uuid)
  RETURNS TABLE (
    tenants_active bigint, tenants_suspended bigint, tenants_total bigint,
    onboarding_submitted bigint, support_pending bigint, support_active bigint, staff_active bigint,
    status_requests_pending bigint
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
      (SELECT count(*) FROM staff WHERE status = 'active'),
      (SELECT count(*) FROM tenant_status_requests WHERE status = 'pending' AND requested_by <> p_staff)
  $$;
REVOKE ALL ON FUNCTION admin_overview(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_overview(uuid) TO riskos_app;

-- +goose Down
DROP FUNCTION admin_overview(uuid);
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
DROP FUNCTION admin_decide_tenant_status(uuid, uuid, boolean, text, timestamptz);
DROP TABLE tenant_status_requests;
