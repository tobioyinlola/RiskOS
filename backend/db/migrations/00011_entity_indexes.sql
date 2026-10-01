-- Entity pages, filtered event listings and analytics (build step 9). See docs/CONSOLE.md.
--
-- Per-entity listings walk these indexes newest first; analytics groups by received_at within a
-- window. All are created on the partitioned table, so every partition (and every future one)
-- gets them.

-- +goose Up
CREATE INDEX events_by_user ON events (tenant_id, environment_id, user_ref, id DESC);
CREATE INDEX events_by_account ON events (tenant_id, environment_id, account_ref, id DESC)
  WHERE account_ref IS NOT NULL;
CREATE INDEX events_by_device ON events (tenant_id, environment_id, device_ref, id DESC)
  WHERE device_ref IS NOT NULL;
CREATE INDEX events_by_time ON events (tenant_id, environment_id, received_at);

-- +goose Down
DROP INDEX events_by_time;
DROP INDEX events_by_device;
DROP INDEX events_by_account;
DROP INDEX events_by_user;
