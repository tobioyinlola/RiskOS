-- Dashboard realm. Tenant-level tables: every query names tenant_id (RLS is layer 2).

-- name: InsertUser :exec
INSERT INTO users (id, tenant_id, email, name, role, status, password_hash, created_at, created_by, updated_at)
VALUES (@id, @tenant_id, @email, @name, @role, 'active', @password_hash, @now, @created_by, @now);

-- name: GetUser :one
SELECT * FROM users WHERE tenant_id = @tenant_id AND id = @id;

-- name: GetUserForUpdate :one
SELECT * FROM users WHERE tenant_id = @tenant_id AND id = @id FOR UPDATE;

-- name: ListUsers :many
SELECT * FROM users WHERE tenant_id = @tenant_id ORDER BY created_at, id;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users WHERE tenant_id = @tenant_id AND role = 'customer_admin' AND status = 'active';

-- name: UpdateUserAccess :exec
UPDATE users SET role = @role, status = @status, updated_at = @now WHERE tenant_id = @tenant_id AND id = @id;

-- name: SetUserTOTP :exec
UPDATE users SET totp_secret_enc = sqlc.narg('totp_secret_enc'), totp_last_step = sqlc.narg('totp_last_step'), updated_at = @now
WHERE tenant_id = @tenant_id AND id = @id;

-- name: SetTOTPStep :exec
UPDATE users SET totp_last_step = @step WHERE tenant_id = @tenant_id AND id = @id;

-- name: SetRecoveryHashes :exec
UPDATE users SET recovery_hashes = @recovery_hashes, updated_at = @now WHERE tenant_id = @tenant_id AND id = @id;

-- name: TouchLogin :exec
UPDATE users SET last_login_at = @now WHERE tenant_id = @tenant_id AND id = @id;

-- name: InsertSession :exec
INSERT INTO console_sessions (id, tenant_id, user_id, token_hash, state, created_at, last_seen_at, expires_at, reauth_at, ip, user_agent)
VALUES (@id, @tenant_id, @user_id, @token_hash, @state, @now, @now, @expires_at, @now, sqlc.narg('ip'), @user_agent);

-- name: SetSessionState :exec
UPDATE console_sessions SET state = @state, reauth_at = @now, last_seen_at = @now, expires_at = @expires_at
WHERE tenant_id = @tenant_id AND id = @id AND revoked_at IS NULL;

-- name: TouchSession :exec
UPDATE console_sessions SET last_seen_at = @now WHERE tenant_id = @tenant_id AND id = @id;

-- name: SetSessionReauth :exec
UPDATE console_sessions SET reauth_at = @now WHERE tenant_id = @tenant_id AND id = @id AND revoked_at IS NULL;

-- name: SetSessionCeremony :exec
UPDATE console_sessions SET webauthn_ceremony = sqlc.narg('ceremony') WHERE tenant_id = @tenant_id AND id = @id;

-- name: GetSessionCeremony :one
SELECT webauthn_ceremony FROM console_sessions WHERE tenant_id = @tenant_id AND id = @id;

-- name: RevokeSession :exec
UPDATE console_sessions SET revoked_at = @now WHERE tenant_id = @tenant_id AND id = @id AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE console_sessions SET revoked_at = @now
WHERE tenant_id = @tenant_id AND user_id = @user_id AND revoked_at IS NULL AND id <> @except_id;

-- name: InsertInvite :exec
INSERT INTO user_invites (id, tenant_id, email, role, token_hash, created_at, created_by, expires_at)
VALUES (@id, @tenant_id, @email, @role, @token_hash, @now, @created_by, @expires_at);

-- name: ListInvites :many
SELECT id, email, role, created_at, created_by, expires_at, accepted_at, revoked_at, email_sent_at
FROM user_invites WHERE tenant_id = @tenant_id ORDER BY created_at DESC LIMIT 200;

-- name: AcceptInvite :execrows
UPDATE user_invites SET accepted_at = @now
WHERE tenant_id = @tenant_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > @now;

-- name: MarkInviteEmailed :exec
UPDATE user_invites SET email_sent_at = @now::timestamptz WHERE tenant_id = @tenant_id AND id = @id;

-- name: TenantName :one
SELECT name FROM tenants WHERE id = @tenant_id;

-- name: RevokeInvite :execrows
UPDATE user_invites SET revoked_at = @now
WHERE tenant_id = @tenant_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: InsertCredential :exec
INSERT INTO webauthn_credentials (id, tenant_id, user_id, name, public_key, attestation_type, aaguid, sign_count, transports, flags, created_at)
VALUES (@id, @tenant_id, @user_id, @name, @public_key, @attestation_type, @aaguid, @sign_count, @transports, @flags, @now);

-- name: ListCredentials :many
SELECT * FROM webauthn_credentials WHERE tenant_id = @tenant_id AND user_id = @user_id ORDER BY created_at;

-- name: UpdateCredentialUse :exec
UPDATE webauthn_credentials SET sign_count = @sign_count, flags = @flags, last_used_at = @now
WHERE tenant_id = @tenant_id AND id = @id;

-- name: DeleteCredential :execrows
DELETE FROM webauthn_credentials WHERE tenant_id = @tenant_id AND user_id = @user_id AND id = @id;

-- name: BindGoogleSubject :execrows
-- Binds Google's subject on the first Google sign-in; afterwards it must match.
UPDATE users SET google_subject = @subject, updated_at = @now
WHERE tenant_id = @tenant_id AND id = @id AND (google_subject IS NULL OR google_subject = @subject);
