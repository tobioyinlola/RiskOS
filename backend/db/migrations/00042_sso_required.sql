-- +goose Up

-- A tenant can require single sign-on: password and Google sign-in are then refused for its
-- people (CONSOLE_AUTH.md). Only meaningful while the connection is enabled, which the check
-- enforces so a switched-off connection cannot leave people locked out.
ALTER TABLE sso_connections
  ADD COLUMN required boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT sso_required_needs_enabled CHECK (NOT required OR enabled);

-- +goose Down
ALTER TABLE sso_connections
  DROP CONSTRAINT sso_required_needs_enabled,
  DROP COLUMN required;
