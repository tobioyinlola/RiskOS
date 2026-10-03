-- +goose Up
-- Rule pack results stored with each evaluation (ADR-0006): the regulatory controls that applied
-- and the triggered rules that could not be evaluated, with what they missed.
ALTER TABLE evaluations
  ADD COLUMN controls jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(controls) = 'array'),
  ADD COLUMN not_evaluated jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(not_evaluated) = 'array');

-- +goose Down
ALTER TABLE evaluations DROP COLUMN not_evaluated, DROP COLUMN controls;
