-- Automatic blocking, per environment, behind shadow evidence (build step 8; ARCHITECTURE.md
-- decision 12, docs/RISK_ENGINE.md). Off by default. In shadow mode Critical evaluations are
-- marked would_block without changing the decision; it can be switched on only after 14 days in
-- shadow, and switched off at once (the kill switch).

-- +goose Up
ALTER TABLE risk_settings
  ADD COLUMN auto_block_mode text NOT NULL DEFAULT 'off' CHECK (auto_block_mode IN ('off', 'shadow', 'on')),
  -- When shadow evidence started (kept while blocking is on); NULL when off.
  ADD COLUMN auto_block_shadow_since timestamptz,
  ADD CONSTRAINT risk_settings_auto_block_since CHECK ((auto_block_mode = 'off') = (auto_block_shadow_since IS NULL));

-- Counting would_block evaluations for the evidence shown before blocking is enabled.
CREATE INDEX evaluations_would_block ON evaluations (tenant_id, environment_id, created_at)
  WHERE 'would_block' = ANY (policy_reasons);

-- +goose Down
DROP INDEX evaluations_would_block;
ALTER TABLE risk_settings
  DROP CONSTRAINT risk_settings_auto_block_since,
  DROP COLUMN auto_block_shadow_since,
  DROP COLUMN auto_block_mode;
