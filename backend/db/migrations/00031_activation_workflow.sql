-- +goose Up
-- Phase 4 of ADR-0007: country activations are created as drafts and moved through shadow and
-- live with a rationale; going live, leaving live and changing the fallback need a second
-- person. Every change is kept (activation_changes), applied at once or pending approval.
CREATE TABLE activation_changes (
  tenant_id      uuid NOT NULL,
  id             uuid PRIMARY KEY,
  activation_id  uuid NOT NULL REFERENCES jurisdiction_activations (id),
  -- create (a new draft), update (licence, capabilities, day start), status, or fallback (make
  -- this activation the one that judges events naming no booking entity).
  action         text NOT NULL CHECK (action IN ('create', 'update', 'status', 'fallback')),
  status_from    text CHECK (status_from IN ('draft', 'shadow', 'live', 'suspended')),
  status_to      text CHECK (status_to IN ('draft', 'shadow', 'live', 'suspended')),
  details        jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 4096),
  rationale      text NOT NULL CHECK (char_length(rationale) BETWEEN 1 AND 1000),
  state          text NOT NULL CHECK (state IN ('applied', 'pending', 'approved', 'rejected')),
  proposed_by    text NOT NULL,
  proposed_at    timestamptz NOT NULL,
  decided_by     text,
  decided_at     timestamptz,
  decision_note  text CHECK (char_length(decision_note) <= 500),
  CHECK ((state IN ('approved', 'rejected')) = (decided_at IS NOT NULL AND decided_by IS NOT NULL)),
  CHECK (action <> 'status' OR status_to IS NOT NULL)
);
CREATE INDEX activation_changes_by_tenant ON activation_changes (tenant_id, proposed_at DESC);
CREATE UNIQUE INDEX activation_changes_one_pending ON activation_changes (tenant_id, activation_id)
  WHERE state = 'pending';

-- A change's content never changes; only a pending one is decided, once.
-- +goose StatementBegin
CREATE FUNCTION activation_change_decide_once() RETURNS trigger
  LANGUAGE plpgsql
  AS $$
BEGIN
  IF OLD.state <> 'pending' THEN
    RAISE EXCEPTION 'activation change % is already decided', OLD.id;
  END IF;
  IF (NEW.tenant_id, NEW.id, NEW.activation_id, NEW.action, NEW.status_from, NEW.status_to, NEW.details,
      NEW.rationale, NEW.proposed_by, NEW.proposed_at)
     IS DISTINCT FROM
     (OLD.tenant_id, OLD.id, OLD.activation_id, OLD.action, OLD.status_from, OLD.status_to, OLD.details,
      OLD.rationale, OLD.proposed_by, OLD.proposed_at) THEN
    RAISE EXCEPTION 'activation change % content is immutable', OLD.id;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER activation_changes_decide_once BEFORE UPDATE ON activation_changes
  FOR EACH ROW EXECUTE FUNCTION activation_change_decide_once();
CREATE TRIGGER activation_changes_no_delete BEFORE DELETE ON activation_changes
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
ALTER TABLE activation_changes ENABLE ROW LEVEL SECURITY;
CREATE POLICY activation_changes_isolation ON activation_changes TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE ON activation_changes TO riskos_app;

-- An institution's record that it checked an open question (a conflict or unverified item of a
-- pack's library) against the original text. Kept per pack; never edited or deleted.
CREATE TABLE gap_verifications (
  tenant_id    uuid NOT NULL,
  pack_id      text NOT NULL,
  gap_id       text NOT NULL CHECK (gap_id ~ '^[A-Za-z0-9._-]{1,32}$'),
  verified_by  text NOT NULL,
  verified_at  timestamptz NOT NULL,
  note         text NOT NULL CHECK (char_length(note) BETWEEN 1 AND 1000),
  link         text CHECK (char_length(link) <= 500 AND link ~ '^https?://'),
  PRIMARY KEY (tenant_id, pack_id, gap_id)
);
CREATE TRIGGER gap_verifications_no_update BEFORE UPDATE OR DELETE ON gap_verifications
  FOR EACH ROW EXECUTE FUNCTION reject_row_change();
ALTER TABLE gap_verifications ENABLE ROW LEVEL SECURITY;
CREATE POLICY gap_verifications_isolation ON gap_verifications TO riskos_app
  USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT ON gap_verifications TO riskos_app;

-- +goose Down
DROP TABLE gap_verifications;
DROP TABLE activation_changes;
DROP FUNCTION activation_change_decide_once();
