-- +goose Up

-- The staff audit log becomes tamper-evident like the tenant audit log: each new entry carries a
-- sequence number and a SHA-256 over its fields and the previous entry's hash, a modified,
-- removed or reordered entry breaks the chain, and the table is immutable for everyone including
-- the owner (ADR-0010 follow-up). Entries written before this migration have no chain fields:
-- they stay as they are and are outside the chain, which the docs say.
ALTER TABLE staff_audit_log
  ADD COLUMN seq           bigint,
  ADD COLUMN prev_hash     bytea,
  ADD COLUMN hash          bytea,
  -- The exact canonical bytes of the metadata that were hashed: jsonb re-formats its input, so the
  -- hash must be recomputable from this text.
  ADD COLUMN metadata_text text;

ALTER TABLE staff_audit_log
  ADD CONSTRAINT staff_audit_chained CHECK (
    (seq IS NULL) = (hash IS NULL) AND (seq IS NULL) = (prev_hash IS NULL) AND (seq IS NULL) = (metadata_text IS NULL)
  ),
  ADD CONSTRAINT staff_audit_hash_sizes CHECK (
    (hash IS NULL OR octet_length(hash) = 32) AND (prev_hash IS NULL OR octet_length(prev_hash) = 32)
  ),
  ADD CONSTRAINT staff_audit_seq_positive CHECK (seq IS NULL OR seq >= 1);
CREATE UNIQUE INDEX staff_audit_log_seq ON staff_audit_log (seq) WHERE seq IS NOT NULL;

-- The head of the chain: one row, locked (FOR UPDATE) by every append, which serializes them.
CREATE TABLE staff_audit_chain_head (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  seq       bigint NOT NULL CHECK (seq >= 0),
  hash      bytea NOT NULL CHECK (octet_length(hash) = 32)
);
INSERT INTO staff_audit_chain_head (singleton, seq, hash) VALUES (true, 0, '\x0000000000000000000000000000000000000000000000000000000000000000');
GRANT SELECT, UPDATE ON staff_audit_chain_head TO riskos_app;

-- Immutable for everyone, including the owner: corrections are new entries.
CREATE FUNCTION staff_audit_reject_change() RETURNS trigger
  LANGUAGE plpgsql
  AS $$ BEGIN RAISE EXCEPTION 'staff_audit_log is append-only'; END $$;
CREATE TRIGGER staff_audit_no_update_delete BEFORE UPDATE OR DELETE ON staff_audit_log
  FOR EACH ROW EXECUTE FUNCTION staff_audit_reject_change();
CREATE TRIGGER staff_audit_no_truncate BEFORE TRUNCATE ON staff_audit_log
  FOR EACH STATEMENT EXECUTE FUNCTION staff_audit_reject_change();

-- +goose Down
DROP TRIGGER staff_audit_no_truncate ON staff_audit_log;
DROP TRIGGER staff_audit_no_update_delete ON staff_audit_log;
DROP FUNCTION staff_audit_reject_change();
DROP TABLE staff_audit_chain_head;
DROP INDEX staff_audit_log_seq;
ALTER TABLE staff_audit_log
  DROP CONSTRAINT staff_audit_seq_positive, DROP CONSTRAINT staff_audit_hash_sizes, DROP CONSTRAINT staff_audit_chained,
  DROP COLUMN metadata_text, DROP COLUMN hash, DROP COLUMN prev_hash, DROP COLUMN seq;
