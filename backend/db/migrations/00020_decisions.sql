-- +goose Up
-- Regulatory controls set the action the law requires (ADR-0006): decline, hold, block a
-- session, restrict, flag or charge a fee, next to allow, review, step_up and block.
ALTER TABLE evaluations DROP CONSTRAINT evaluations_decision_check;
ALTER TABLE evaluations ADD CONSTRAINT evaluations_decision_check CHECK (decision IN (
  'allow', 'fee', 'flag', 'step_up', 'review', 'restrict', 'hold', 'block_session', 'decline', 'block'));

-- +goose Down
-- Evaluations are insert-only, so rows with the new decisions cannot be rewritten: rolling back
-- fails while any exist.
ALTER TABLE evaluations DROP CONSTRAINT evaluations_decision_check;
ALTER TABLE evaluations ADD CONSTRAINT evaluations_decision_check
  CHECK (decision IN ('allow', 'review', 'step_up', 'block'));
