-- Firehose: broad "new internship" alerts for companies NOT on the watchlist
-- (job-watch parity). Deliberately lightweight — no entity resolution, no timing
-- history, no events rows. Just a seen-set so we alert once per new posting.
-- See DESIGN.md §3.2 (firehose note).
CREATE TABLE firehose_seen (
    source      TEXT        NOT NULL,
    external_id TEXT        NOT NULL,
    company     TEXT        NOT NULL,
    title       TEXT        NOT NULL,
    url         TEXT,
    category    TEXT,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source, external_id)
);
CREATE INDEX firehose_seen_first_seen_idx ON firehose_seen (first_seen);
