-- +goose Up

-- Creating a staff account from the console needs two security administrators (ADR-0010,
-- amended): one requests with a reason, a different one approves, and the enrollment link goes
-- only to the new person's mailbox, never to either session. Staff-realm table, like
-- support_access and tenant_status_requests.
CREATE TABLE staff_account_requests (
  id            uuid PRIMARY KEY,
  email         text NOT NULL CHECK (email = lower(email) AND char_length(email) BETWEEN 3 AND 254),
  name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  role          text NOT NULL CHECK (role IN ('operations_admin', 'security_admin')),
  reason        text NOT NULL CHECK (char_length(reason) BETWEEN 10 AND 500),
  status        text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
  requested_by  uuid NOT NULL REFERENCES staff (id),
  created_at    timestamptz NOT NULL,
  decided_by    uuid REFERENCES staff (id),
  decided_at    timestamptz,
  decision_note text CHECK (char_length(decision_note) <= 500),
  -- The account created on approval.
  staff_id      uuid REFERENCES staff (id),
  -- Four eyes, enforced by the database: nobody decides their own request.
  CONSTRAINT sar_four_eyes CHECK (decided_by IS NULL OR status = 'withdrawn' OR decided_by <> requested_by),
  CONSTRAINT sar_decided CHECK (status = 'withdrawn' OR (status = 'pending') = (decided_by IS NULL)),
  CONSTRAINT sar_created CHECK ((status = 'approved') = (staff_id IS NOT NULL))
);
-- One open request per address.
CREATE UNIQUE INDEX staff_account_requests_one_pending ON staff_account_requests (email) WHERE status = 'pending';
CREATE INDEX staff_account_requests_recent ON staff_account_requests (created_at DESC);
GRANT SELECT, INSERT, UPDATE ON staff_account_requests TO riskos_app;

-- +goose Down
DROP TABLE staff_account_requests;
