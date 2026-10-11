-- +goose Up

-- Staff single sign-on (ADR-0010 follow-up): the platform's identity provider vouches for the
-- person, and the staff member's passkey is still required. The provider's subject is bound to
-- the staff member at the first SSO sign-in, which also needs the provider to vouch for the
-- account's email; later sign-ins must present the same subject, so an email reassigned at the
-- provider does not reach the account.
CREATE TABLE staff_sso_identities (
  staff_id  uuid PRIMARY KEY REFERENCES staff (id),
  issuer    text NOT NULL CHECK (issuer <> ''),
  subject   text NOT NULL CHECK (subject <> ''),
  bound_at  timestamptz NOT NULL,
  UNIQUE (issuer, subject)
);
GRANT SELECT, INSERT, DELETE ON staff_sso_identities TO riskos_app;

-- SSO requests in flight, and the passkey step that follows a verified SSO sign-in.
ALTER TABLE staff_ceremonies DROP CONSTRAINT staff_ceremonies_kind_check;
ALTER TABLE staff_ceremonies ADD CONSTRAINT staff_ceremonies_kind_check
  CHECK (kind IN ('enroll', 'login', 'sso_start', 'sso_login'));

-- +goose Down

DELETE FROM staff_ceremonies WHERE kind IN ('sso_start', 'sso_login');
ALTER TABLE staff_ceremonies DROP CONSTRAINT staff_ceremonies_kind_check;
ALTER TABLE staff_ceremonies ADD CONSTRAINT staff_ceremonies_kind_check
  CHECK (kind IN ('enroll', 'login'));
DROP TABLE staff_sso_identities;
