-- Single sign-on (OIDC). Every tenant-scoped query takes tenant_id explicitly (TENANCY.md).

-- name: GetSSOConnection :one
SELECT * FROM sso_connections WHERE tenant_id = @tenant_id;

-- name: UpsertSSOConnection :exec
INSERT INTO sso_connections (tenant_id, issuer, client_id, client_secret_enc, enabled, required, updated_at, updated_by)
VALUES (@tenant_id, @issuer, @client_id, @client_secret_enc, @enabled, @required, @now, @updated_by)
ON CONFLICT (tenant_id) DO UPDATE
  SET issuer = EXCLUDED.issuer, client_id = EXCLUDED.client_id, client_secret_enc = EXCLUDED.client_secret_enc,
      enabled = EXCLUDED.enabled, required = EXCLUDED.required, updated_at = EXCLUDED.updated_at,
      updated_by = EXCLUDED.updated_by;

-- name: SetSSORequired :execrows
-- The operator's way out when an identity provider is unavailable (riskosctl sso optional).
UPDATE sso_connections SET required = false, updated_at = @now, updated_by = @updated_by
WHERE tenant_id = @tenant_id AND required;

-- name: DeleteSSOConnection :execrows
DELETE FROM sso_connections WHERE tenant_id = @tenant_id;

-- name: ClearSSOSubjects :exec
-- A different identity provider has different subjects: bindings start again.
UPDATE users SET sso_subject = NULL WHERE tenant_id = @tenant_id AND sso_subject IS NOT NULL;

-- sso_logins is read and written before authentication: see store/sso.go.

-- name: BindSSOSubject :execrows
-- Binds the provider's subject on the first SSO sign-in; afterwards it must match.
UPDATE users SET sso_subject = @subject
WHERE tenant_id = @tenant_id AND id = @id AND (sso_subject IS NULL OR sso_subject = @subject);
