-- Invite emails. See docs/CONSOLE.md#invites.
--
-- The worker emails an invite's link when SMTP is configured, and records when it did. The link's
-- token is derived from the invite ID with a server key, so it never sits in the job queue.

-- +goose Up
ALTER TABLE user_invites ADD COLUMN email_sent_at timestamptz;

-- +goose Down
ALTER TABLE user_invites DROP COLUMN email_sent_at;
