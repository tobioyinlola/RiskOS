-- +goose Up

-- Break-glass suspension (ADR-0010, amended): in an incident one security administrator may
-- suspend a tenant at once. It is never silent and never final on one person's word: the request
-- is recorded as an emergency, applied immediately, and then waits for a different staff member to
-- review it. Reactivation never has an emergency path.
ALTER TABLE tenant_status_requests
  ADD COLUMN emergency   boolean NOT NULL DEFAULT false,
  ADD COLUMN reviewed_by uuid REFERENCES staff (id),
  ADD COLUMN reviewed_at timestamptz,
  ADD COLUMN review_note text CHECK (char_length(review_note) <= 500);

-- The four-eyes rules, restated for the new column: an emergency is decided by its own requester
-- (that is the point) but must be reviewed by someone else.
-- +goose StatementBegin
DO $$
DECLARE c record;
BEGIN
  FOR c IN SELECT conname FROM pg_constraint
           WHERE conrelid = 'tenant_status_requests'::regclass AND contype = 'c'
             AND pg_get_constraintdef(oid) LIKE '%decided_by%' LOOP
    EXECUTE format('ALTER TABLE tenant_status_requests DROP CONSTRAINT %I', c.conname);
  END LOOP;
END
$$;
-- +goose StatementEnd
ALTER TABLE tenant_status_requests
  ADD CONSTRAINT tsr_four_eyes CHECK (decided_by IS NULL OR status = 'withdrawn' OR emergency OR decided_by <> requested_by),
  ADD CONSTRAINT tsr_decided CHECK (status = 'withdrawn' OR (status = 'pending') = (decided_by IS NULL)),
  ADD CONSTRAINT tsr_emergency_is_suspension CHECK (NOT emergency OR (action = 'suspend' AND status = 'approved')),
  ADD CONSTRAINT tsr_review CHECK (
    (reviewed_by IS NULL) = (reviewed_at IS NULL)
    AND (reviewed_by IS NULL OR (emergency AND reviewed_by <> requested_by))
  );

-- Suspends a tenant on one security administrator's authority and records it as an emergency.
-- The role is checked here as well as by the caller, so no code path can skip it.
-- +goose StatementBegin
CREATE FUNCTION admin_emergency_suspend(p_tenant uuid, p_staff uuid, p_reason text, p_now timestamptz)
RETURNS TABLE (outcome text, request_id uuid)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  who staff%ROWTYPE;
  current_status text;
  new_id uuid := gen_random_uuid();
BEGIN
  SELECT * INTO who FROM staff WHERE id = p_staff;
  IF NOT FOUND OR who.status <> 'active' OR who.role <> 'security_admin' THEN
    RETURN QUERY SELECT 'forbidden'::text, NULL::uuid;
    RETURN;
  END IF;
  SELECT t.status INTO current_status FROM tenants t WHERE t.id = p_tenant FOR UPDATE;
  IF NOT FOUND THEN
    RETURN QUERY SELECT 'not_found'::text, NULL::uuid;
    RETURN;
  END IF;
  IF current_status <> 'active' THEN
    RETURN QUERY SELECT 'not_applicable'::text, NULL::uuid;
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM tenant_status_requests WHERE tenant_id = p_tenant AND status = 'pending') THEN
    -- Decide or withdraw the waiting request first: two live requests would race.
    RETURN QUERY SELECT 'pending_exists'::text, NULL::uuid;
    RETURN;
  END IF;
  INSERT INTO tenant_status_requests
    (id, tenant_id, action, reason, status, requested_by, created_at, decided_by, decided_at, emergency)
  VALUES (new_id, p_tenant, 'suspend', p_reason, 'approved', p_staff, p_now, p_staff, p_now, true);
  UPDATE tenants SET status = 'suspended' WHERE id = p_tenant;
  RETURN QUERY SELECT 'applied'::text, new_id;
END;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION admin_emergency_suspend(uuid, uuid, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_emergency_suspend(uuid, uuid, text, timestamptz) TO riskos_app;

-- A different staff member confirms (or notes a problem with) an emergency suspension.
-- +goose StatementBegin
CREATE FUNCTION admin_review_emergency(p_request uuid, p_reviewer uuid, p_note text, p_now timestamptz)
RETURNS TABLE (outcome text, tenant_id uuid)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE r tenant_status_requests%ROWTYPE;
BEGIN
  SELECT * INTO r FROM tenant_status_requests WHERE id = p_request FOR UPDATE;
  IF NOT FOUND THEN
    RETURN QUERY SELECT 'not_found'::text, NULL::uuid;
    RETURN;
  END IF;
  IF NOT r.emergency THEN
    RETURN QUERY SELECT 'not_emergency'::text, r.tenant_id;
    RETURN;
  END IF;
  IF r.reviewed_by IS NOT NULL THEN
    RETURN QUERY SELECT 'already_reviewed'::text, r.tenant_id;
    RETURN;
  END IF;
  IF r.requested_by = p_reviewer THEN
    RETURN QUERY SELECT 'own_request'::text, r.tenant_id;
    RETURN;
  END IF;
  UPDATE tenant_status_requests
    SET reviewed_by = p_reviewer, reviewed_at = p_now, review_note = NULLIF(p_note, '')
    WHERE id = p_request;
  RETURN QUERY SELECT 'reviewed'::text, r.tenant_id;
END;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION admin_review_emergency(uuid, uuid, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_review_emergency(uuid, uuid, text, timestamptz) TO riskos_app;

-- The work queue also counts emergency suspensions waiting for this staff member's review.
DROP FUNCTION admin_overview(uuid);
CREATE FUNCTION admin_overview(p_staff uuid)
  RETURNS TABLE (
    tenants_active bigint, tenants_suspended bigint, tenants_total bigint,
    onboarding_submitted bigint, support_pending bigint, support_active bigint, staff_active bigint,
    status_requests_pending bigint, emergency_reviews_pending bigint
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
      (SELECT count(*) FROM tenant_status_requests WHERE status = 'pending' AND requested_by <> p_staff),
      (SELECT count(*) FROM tenant_status_requests
        WHERE emergency AND reviewed_by IS NULL AND requested_by <> p_staff)
  $$;
REVOKE ALL ON FUNCTION admin_overview(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_overview(uuid) TO riskos_app;

-- +goose Down
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
DROP FUNCTION admin_review_emergency(uuid, uuid, text, timestamptz);
DROP FUNCTION admin_emergency_suspend(uuid, uuid, text, timestamptz);
DELETE FROM tenant_status_requests WHERE emergency;
ALTER TABLE tenant_status_requests
  DROP CONSTRAINT tsr_review, DROP CONSTRAINT tsr_emergency_is_suspension,
  DROP CONSTRAINT tsr_decided, DROP CONSTRAINT tsr_four_eyes,
  DROP COLUMN review_note, DROP COLUMN reviewed_at, DROP COLUMN reviewed_by, DROP COLUMN emergency;
ALTER TABLE tenant_status_requests
  ADD CHECK (decided_by IS NULL OR status = 'withdrawn' OR decided_by <> requested_by),
  ADD CHECK (status = 'withdrawn' OR (status = 'pending') = (decided_by IS NULL));
