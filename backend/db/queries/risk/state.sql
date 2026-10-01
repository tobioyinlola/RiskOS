-- Feature-store state (platform-wide, no tenant scope): see migration 00013.

-- name: SyncFeatureStoreState :exec
-- Records the epoch and key fingerprint in use. A change of either is a reset.
INSERT INTO feature_store_state (id, epoch, key_fingerprint, started_at)
VALUES (1, @epoch, @key_fingerprint, @now)
ON CONFLICT (id) DO UPDATE
  SET epoch = EXCLUDED.epoch, key_fingerprint = EXCLUDED.key_fingerprint, reset_at = EXCLUDED.started_at
  WHERE feature_store_state.epoch <> EXCLUDED.epoch
     OR feature_store_state.key_fingerprint <> EXCLUDED.key_fingerprint;

-- name: GetFeatureStoreReset :one
SELECT reset_at FROM feature_store_state WHERE id = 1;
