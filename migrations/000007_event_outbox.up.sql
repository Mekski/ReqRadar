-- Transactional outbox (alert-loss-trio H1/H2): events are staged here in the
-- same transaction as their posting/firehose writes, so a NATS publish can never
-- be lost relative to a committed DB write. The processor publishes inline right
-- after commit (happy path stays sub-second) and a relay sweeps any rows a failed
-- inline publish left behind. published_at IS NULL means "not yet on NATS".
CREATE TABLE event_outbox (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  subject      TEXT NOT NULL,          -- NATS subject, e.g. events.posting_opened
  payload      BYTEA NOT NULL,         -- exact bytes to publish (a marshaled signal.Event)
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ             -- NULL until successfully published to NATS
);

-- The relay only ever scans unpublished rows; a partial index keeps that cheap
-- and the index tiny (published rows drop out of it).
CREATE INDEX event_outbox_unpublished ON event_outbox (id) WHERE published_at IS NULL;
