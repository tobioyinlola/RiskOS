-- The admin realm (staff). Platform tables: no tenant scope. See docs/ADMIN.md.

-- name: InsertStaff :exec
INSERT INTO staff (id, email, name, role, status, enroll_token_hash, enroll_expires_at, created_at, created_by)
VALUES (@id, @email, @name, @role, 'active', @enroll_token_hash, @enroll_expires_at, @now, @created_by);

-- name: GetStaff :one
SELECT * FROM staff WHERE id = @id;

-- name: StaffByEmail :one
SELECT * FROM staff WHERE email = @email;

-- name: StaffByEnrollToken :one
SELECT * FROM staff WHERE enroll_token_hash = @token_hash;

-- name: ListStaff :many
SELECT * FROM staff ORDER BY created_at;

-- name: SetEnrollToken :execrows
UPDATE staff SET enroll_token_hash = @token_hash, enroll_expires_at = @expires_at WHERE id = @id;

-- name: ClearEnrollToken :exec
UPDATE staff SET enroll_token_hash = NULL, enroll_expires_at = NULL WHERE id = @id;

-- name: SetStaffStatus :execrows
UPDATE staff SET status = @status WHERE id = @id;

-- name: StaffRoster :many
SELECT s.id::uuid AS id, s.email::text AS email, s.name::text AS name, s.role::text AS role,
       s.status::text AS status, s.created_at::timestamptz AS created_at,
       (SELECT count(*) FROM staff_passkeys p WHERE p.staff_id = s.id)::bigint AS passkeys,
       -- The epoch means "never signed in".
       COALESCE((SELECT max(ss.created_at) FROM staff_sessions ss WHERE ss.staff_id = s.id), 'epoch'::timestamptz)::timestamptz AS last_sign_in,
       COALESCE(s.enroll_token_hash IS NOT NULL AND s.enroll_expires_at > now(), false)::boolean AS enrollment_pending
FROM staff s ORDER BY s.created_at;

-- name: LockActiveSecurityAdmins :many
SELECT id FROM staff WHERE role = 'security_admin' AND status = 'active' FOR UPDATE;

-- name: InsertStaffPasskey :exec
INSERT INTO staff_passkeys (id, staff_id, name, public_key, attestation_type, aaguid, sign_count, transports, flags, created_at)
VALUES (@id, @staff_id, @name, @public_key, @attestation_type, @aaguid, @sign_count, @transports, @flags, @now);

-- name: StaffPasskeys :many
SELECT * FROM staff_passkeys WHERE staff_id = @staff_id ORDER BY created_at;

-- name: UpdateStaffPasskeyUse :exec
UPDATE staff_passkeys SET sign_count = @sign_count, flags = @flags, last_used_at = @now::timestamptz WHERE id = @id;

-- name: InsertStaffSession :exec
INSERT INTO staff_sessions (id, staff_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent)
VALUES (@id, @staff_id, @token_hash, @now, @now, @expires_at, @ip, @user_agent);

-- name: StaffSessionByToken :one
SELECT s.id, s.staff_id, s.created_at, s.last_seen_at, s.expires_at, s.revoked_at,
       st.email, st.name, st.role, st.status
FROM staff_sessions s JOIN staff st ON st.id = s.staff_id
WHERE s.token_hash = @token_hash;

-- name: TouchStaffSession :exec
UPDATE staff_sessions SET last_seen_at = @now WHERE id = @id;

-- name: RevokeStaffSession :exec
UPDATE staff_sessions SET revoked_at = @now::timestamptz WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeStaffSessions :exec
UPDATE staff_sessions SET revoked_at = @now::timestamptz WHERE staff_id = @staff_id AND revoked_at IS NULL;

-- name: InsertCeremony :exec
INSERT INTO staff_ceremonies (id_hash, kind, staff_id, data, expires_at)
VALUES (@id_hash, @kind, @staff_id, @data, @expires_at);

-- name: PurgeCeremonies :exec
DELETE FROM staff_ceremonies WHERE expires_at < @now;

-- name: TakeCeremony :one
DELETE FROM staff_ceremonies WHERE id_hash = @id_hash AND kind = @kind AND expires_at > @now
RETURNING staff_id, data;

-- name: InsertSupportAccess :exec
INSERT INTO support_access (id, tenant_id, requested_by, reason, duration_minutes, status, created_at)
VALUES (@id, @tenant_id, @requested_by, @reason, @duration_minutes, 'pending', @now);

-- name: GetSupportAccess :one
SELECT * FROM support_access WHERE id = @id;

-- name: GetSupportAccessForUpdate :one
SELECT * FROM support_access WHERE id = @id FOR UPDATE;

-- name: ListSupportAccess :many
SELECT * FROM support_access ORDER BY created_at DESC LIMIT 200;

-- name: DecideSupportAccess :exec
UPDATE support_access SET status = @status, decided_by = @decided_by, decided_at = @now::timestamptz, expires_at = @expires_at
WHERE id = @id;

-- name: RevokeSupportAccess :exec
UPDATE support_access SET status = 'revoked', revoked_by = @revoked_by, revoked_at = @now::timestamptz WHERE id = @id;

-- name: InsertStaffAudit :exec
INSERT INTO staff_audit_log (id, occurred_at, actor, action, tenant_id, resource_id, outcome, metadata,
                             seq, prev_hash, hash, metadata_text)
VALUES (@id, @now, @actor, @action, @tenant_id, @resource_id, @outcome, @metadata,
        @seq, @prev_hash, @hash, @metadata_text);

-- name: LockStaffAuditHead :one
SELECT seq, hash FROM staff_audit_chain_head WHERE singleton FOR UPDATE;

-- name: SetStaffAuditHead :exec
UPDATE staff_audit_chain_head SET seq = @seq, hash = @hash WHERE singleton;

-- name: GetStaffAuditHead :one
SELECT seq, hash FROM staff_audit_chain_head WHERE singleton;

-- name: ListChainedStaffAudit :many
SELECT id, occurred_at, actor, action, tenant_id, resource_id, outcome, seq, prev_hash, hash, metadata_text
FROM staff_audit_log WHERE seq IS NOT NULL ORDER BY seq;

-- name: ListStaffAudit :many
SELECT * FROM staff_audit_log ORDER BY occurred_at DESC, id DESC LIMIT @lim;

-- name: AdminTenants :many
SELECT id::uuid AS id, name::text AS name, status::text AS status, created_at::timestamptz AS created_at
FROM admin_tenants();

-- name: AdminTenantDirectory :many
SELECT id::uuid AS id, name::text AS name, region::text AS region, status::text AS status, origin::text AS origin,
       production_enabled::boolean AS production_enabled, created_at::timestamptz AS created_at,
       active_users::bigint AS active_users, active_api_keys::bigint AS active_api_keys,
       onboarding_status::text AS onboarding_status
FROM admin_tenant_directory();

-- name: AdminOverview :one
SELECT tenants_active::bigint AS tenants_active, tenants_suspended::bigint AS tenants_suspended,
       tenants_total::bigint AS tenants_total, onboarding_submitted::bigint AS onboarding_submitted,
       support_pending::bigint AS support_pending, support_active::bigint AS support_active,
       staff_active::bigint AS staff_active, status_requests_pending::bigint AS status_requests_pending,
       emergency_reviews_pending::bigint AS emergency_reviews_pending
FROM admin_overview(@staff_id);

-- name: EmergencySuspendTenant :one
SELECT outcome::text AS outcome, COALESCE(request_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS request_id
FROM admin_emergency_suspend(@tenant_id, @staff_id, @reason, @now::timestamptz);

-- name: ReviewEmergencySuspension :one
SELECT outcome::text AS outcome, COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS tenant_id
FROM admin_review_emergency(@request_id, @reviewer_id, @note, @now::timestamptz);

-- name: InsertTenantStatusRequest :exec
INSERT INTO tenant_status_requests (id, tenant_id, action, reason, status, requested_by, created_at)
VALUES (@id, @tenant_id, @action, @reason, 'pending', @requested_by, @now);

-- name: GetTenantStatusRequest :one
SELECT * FROM tenant_status_requests WHERE id = @id;

-- name: GetTenantStatusRequestForUpdate :one
SELECT * FROM tenant_status_requests WHERE id = @id FOR UPDATE;

-- name: ListTenantStatusRequests :many
SELECT * FROM tenant_status_requests ORDER BY created_at DESC LIMIT 200;

-- name: WithdrawTenantStatusRequest :execrows
UPDATE tenant_status_requests SET status = 'withdrawn', decided_by = NULL, decided_at = @now::timestamptz
WHERE id = @id AND requested_by = @staff_id AND status = 'pending';

-- name: DecideTenantStatusRequest :one
SELECT outcome::text AS outcome, tenant_id::uuid AS tenant_id, action::text AS action
FROM admin_decide_tenant_status(@request_id, @decider_id, @approve::boolean, @note, @now::timestamptz);

-- name: ListStaffAuditForTenant :many
SELECT * FROM staff_audit_log WHERE tenant_id = @tenant_id ORDER BY occurred_at DESC, id DESC LIMIT @lim;

-- name: SecurityOverview :many
SELECT tenant_id::uuid AS tenant_id, tenant_name::text AS tenant_name, tenant_status::text AS tenant_status,
       degraded_evaluations::bigint AS degraded_evaluations, open_security_alerts::bigint AS open_security_alerts,
       failed_logins::bigint AS failed_logins, disabled_webhooks::bigint AS disabled_webhooks
FROM admin_security_overview(@since);

-- name: GetTenantLimits :one
SELECT * FROM tenant_limits WHERE tenant_id = @tenant_id;

-- name: UpsertTenantLimits :exec
INSERT INTO tenant_limits (tenant_id, per_key_per_second, per_key_burst, per_tenant_per_second, per_tenant_burst,
                           reason, updated_by, updated_at)
VALUES (@tenant_id, @per_key_per_second, @per_key_burst, @per_tenant_per_second, @per_tenant_burst,
        @reason, @updated_by, @now)
ON CONFLICT (tenant_id) DO UPDATE SET
  per_key_per_second = EXCLUDED.per_key_per_second, per_key_burst = EXCLUDED.per_key_burst,
  per_tenant_per_second = EXCLUDED.per_tenant_per_second, per_tenant_burst = EXCLUDED.per_tenant_burst,
  reason = EXCLUDED.reason, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at;

-- name: DeleteTenantLimits :execrows
DELETE FROM tenant_limits WHERE tenant_id = @tenant_id;

-- name: GetTenantQuota :one
SELECT * FROM tenant_quotas WHERE tenant_id = @tenant_id;

-- name: UpsertTenantQuota :exec
-- A new quota starts a new measurement: the worker counts the month again on its next run.
INSERT INTO tenant_quotas (tenant_id, monthly_events, reason, updated_by, updated_at)
VALUES (@tenant_id, @monthly_events, @reason, @updated_by, @now)
ON CONFLICT (tenant_id) DO UPDATE SET
  monthly_events = EXCLUDED.monthly_events, reason = EXCLUDED.reason, updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at, usage_checked_at = NULL;

-- name: DeleteTenantQuota :execrows
DELETE FROM tenant_quotas WHERE tenant_id = @tenant_id;

-- name: ListTenantQuotas :many
SELECT * FROM tenant_quotas ORDER BY tenant_id;

-- name: CountTenantEvents :one
SELECT admin_count_events(@tenant_id::uuid, @since::timestamptz)::bigint;

-- name: SetTenantQuotaUsage :exec
UPDATE tenant_quotas SET usage_month = @usage_month, usage_events = @usage_events, usage_checked_at = @now
WHERE tenant_id = @tenant_id;

-- name: MarkTenantQuotaExceeded :execrows
-- Claims the announcement for the month: only the first caller gets a row, so an overage is
-- announced once even with several workers.
UPDATE tenant_quotas SET exceeded_month = @month
WHERE tenant_id = @tenant_id AND exceeded_month IS DISTINCT FROM @month;

-- name: InsertStaffAccountRequest :exec
INSERT INTO staff_account_requests (id, email, name, role, reason, status, requested_by, created_at)
VALUES (@id, @email, @name, @role, @reason, 'pending', @requested_by, @now);

-- name: GetStaffAccountRequestForUpdate :one
SELECT * FROM staff_account_requests WHERE id = @id FOR UPDATE;

-- name: GetStaffAccountRequest :one
SELECT * FROM staff_account_requests WHERE id = @id;

-- name: ListStaffAccountRequests :many
SELECT * FROM staff_account_requests ORDER BY created_at DESC LIMIT 200;

-- name: DecideStaffAccountRequest :exec
UPDATE staff_account_requests
SET status = @status, decided_by = @decided_by, decided_at = @now::timestamptz,
    decision_note = sqlc.narg('decision_note'), staff_id = sqlc.narg('staff_id')
WHERE id = @id;

-- name: WithdrawStaffAccountRequest :execrows
UPDATE staff_account_requests SET status = 'withdrawn', decided_at = @now::timestamptz
WHERE id = @id AND requested_by = @staff_id AND status = 'pending';

-- name: StaffSSOIdentity :one
SELECT * FROM staff_sso_identities WHERE staff_id = @staff_id;

-- name: StaffBySSOSubject :one
SELECT s.* FROM staff s JOIN staff_sso_identities i ON i.staff_id = s.id
WHERE i.issuer = @issuer AND i.subject = @subject;

-- name: BindStaffSSOIdentity :exec
INSERT INTO staff_sso_identities (staff_id, issuer, subject, bound_at)
VALUES (@staff_id, @issuer, @subject, @now);

-- name: UnbindStaffSSOIdentity :execrows
DELETE FROM staff_sso_identities WHERE staff_id = @staff_id;
