-- Postings + version history + operational tables. See DESIGN.md §4.

-- Current state of each posting (one row per posting, updated on each sighting).
CREATE TABLE postings (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_id    BIGINT      NOT NULL REFERENCES entities (id),
    source_id    BIGINT      NOT NULL REFERENCES sources (id),
    external_id  TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    url          TEXT,
    locations    TEXT[],
    pay_min      INTEGER,                         -- posted ranges only (transparency laws)
    pay_max      INTEGER,
    pay_currency TEXT,
    first_seen   TIMESTAMPTZ NOT NULL,
    last_seen    TIMESTAMPTZ NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'open',
    UNIQUE (source_id, external_id)               -- dedupe key
);
CREATE INDEX postings_entity_idx ON postings (entity_id);
CREATE INDEX postings_status_idx ON postings (status);

-- Immutable version history: a new row each time a posting's content hash changes.
-- This is the JD-diffing feature's storage.
CREATE TABLE posting_versions (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    posting_id   BIGINT      NOT NULL REFERENCES postings (id) ON DELETE CASCADE,
    content_hash TEXT        NOT NULL,
    raw_text     TEXT        NOT NULL,
    parsed       JSONB       NOT NULL,
    captured_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX posting_versions_posting_idx ON posting_versions (posting_id, captured_at);

-- One row per alert sent. detect_to_alert_ms is the instrumented latency number
-- behind the "sub-minute detection-to-alert" claim.
CREATE TABLE alerts (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id            BIGINT      NOT NULL REFERENCES users (id),
    event_id           BIGINT      NOT NULL,      -- no FK: events is partitioned (composite PK)
    channel            TEXT        NOT NULL DEFAULT 'telegram',
    sent_at            TIMESTAMPTZ NOT NULL,
    detect_to_alert_ms INTEGER     NOT NULL
);
CREATE INDEX alerts_sent_idx ON alerts (sent_at);

-- Per-collector-run audit: powers source-health observability and alerting.
CREATE TABLE collector_runs (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id    BIGINT      NOT NULL REFERENCES sources (id),
    started_at   TIMESTAMPTZ NOT NULL,
    finished_at  TIMESTAMPTZ,
    status       TEXT        NOT NULL,            -- 'running' | 'ok' | 'error'
    signal_count INTEGER     NOT NULL DEFAULT 0,
    error        TEXT
);
CREATE INDEX collector_runs_source_idx ON collector_runs (source_id, started_at);
