-- +goose Up
-- Compliance-only signals as they fired (GOV-09, no tipping off). The evaluation's signals list
-- them as "compliance_review"; the rule itself is kept here, read only through the compliance
-- API by people holding compliance:sensitive.
ALTER TABLE evaluations
  ADD COLUMN restricted_signals jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(restricted_signals) = 'array');

-- +goose Down
ALTER TABLE evaluations DROP COLUMN restricted_signals;
