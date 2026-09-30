-- Baseline migration.
--
-- Establishes goose version history so every later migration is an ordinary, reviewable step.
-- It intentionally changes no schema: domain tables (tenants, environments, api keys, events, ...)
-- are designed in phase-0 and added as new numbered migrations. Rules for writing migrations
-- live in CONTRIBUTING.md.

-- +goose Up

-- +goose Down
