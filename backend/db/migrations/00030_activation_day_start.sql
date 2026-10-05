-- +goose Up
-- The hour a business day starts in an activation's jurisdiction (ADR-0007): calendar windows
-- ("the wallet's daily limit") run from it, in the jurisdiction's time zone. Midnight unless the
-- institution's day starts later.
ALTER TABLE jurisdiction_activations
  ADD COLUMN day_starts_at smallint NOT NULL DEFAULT 0 CHECK (day_starts_at BETWEEN 0 AND 23);

-- +goose Down
ALTER TABLE jurisdiction_activations DROP COLUMN day_starts_at;
