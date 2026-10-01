-- Feature-store state reset detection. See docs/DISASTER_RECOVERY.md#losing-redis-state.
--
-- Redis holds behavioural history that is not backed up. A marker (an epoch) is kept both in
-- Redis and here: when Redis loses its data, its marker disappears and the next check records a
-- reset. A change of the feature hash key also makes every stored history unreachable, so the
-- key's fingerprint (a keyed hash, never the key) is recorded too. Until history has rebuilt, the
-- engine holds back history signals and lists them in warming_signals.
--
-- One row for the whole platform: Redis is shared by every tenant. No tenant data, no RLS.

-- +goose Up
CREATE TABLE feature_store_state (
  id               smallint PRIMARY KEY CHECK (id = 1),
  epoch            text NOT NULL CHECK (char_length(epoch) BETWEEN 1 AND 64),
  key_fingerprint  text NOT NULL CHECK (char_length(key_fingerprint) BETWEEN 1 AND 64),
  started_at       timestamptz NOT NULL,
  -- When the last reset was detected; NULL if the state has never been lost.
  reset_at         timestamptz
);
GRANT SELECT, INSERT, UPDATE ON feature_store_state TO riskos_app;

ALTER TABLE evaluations ADD COLUMN warming_signals text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE evaluations DROP COLUMN warming_signals;
DROP TABLE feature_store_state;
