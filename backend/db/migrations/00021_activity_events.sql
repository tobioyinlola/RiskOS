-- +goose Up
-- Activity events: account and platform activity that is neither a money movement nor a sign-in
-- (EVENTS.md#compliance).
ALTER TABLE events DROP CONSTRAINT events_type_check;
ALTER TABLE events ADD CONSTRAINT events_type_check CHECK (type IN (
  'login', 'transaction', 'beneficiary_added', 'device_change', 'session_change', 'profile_change', 'activity'));

-- +goose Down
-- Events are insert-only, so rolling back fails while any activity event exists.
ALTER TABLE events DROP CONSTRAINT events_type_check;
ALTER TABLE events ADD CONSTRAINT events_type_check CHECK (type IN (
  'login', 'transaction', 'beneficiary_added', 'device_change', 'session_change', 'profile_change'));
