-- Time-partitioned signal tables. The two append-heavy tables are the reason
-- the project earns "time-series / data systems" without a second database:
-- monthly range partitions make retention a cheap DROP of whole partitions and
-- let timing queries prune to relevant months. See DESIGN.md §4.

-- Helper: create the month partition of `parent` covering [start, start+1mo).
-- Idempotent. Reused by the ongoing partition-maintenance job (Milestone C).
CREATE FUNCTION reqradar_ensure_month_partition(parent text, start date)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    part      text := format('%s_%s', parent, to_char(start, 'YYYY_MM'));
    range_end date := (start + INTERVAL '1 month')::date;
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        part, parent, start, range_end
    );
END;
$$;

-- Raw signals = the replay buffer (reprocess after a parser fix). Partitioned by
-- observed_at (ingest time), NOT event_time, because retention is "drop what we
-- ingested >30 days ago" — backfilled rows carry old event_times but are ingested
-- now, so observed_at is the correct retention axis.
CREATE TABLE raw_signals (
    id           BIGINT GENERATED ALWAYS AS IDENTITY,
    source_id    BIGINT      NOT NULL REFERENCES sources (id),
    external_id  TEXT        NOT NULL,
    kind         TEXT        NOT NULL,
    event_time   TIMESTAMPTZ NOT NULL,            -- when it happened (backfill: years ago)
    observed_at  TIMESTAMPTZ NOT NULL,            -- when we ingested it (partition key, latency t0)
    content_hash TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    PRIMARY KEY (id, observed_at)                 -- partition key must be in the PK
) PARTITION BY RANGE (observed_at);
CREATE INDEX raw_signals_source_external_idx ON raw_signals (source_id, external_id);

-- Normalized events = the product. Partitioned by event_time because every timing
-- query groups by when-it-happened; kept forever. This table is the resume asset.
CREATE TABLE events (
    id          BIGINT GENERATED ALWAYS AS IDENTITY,
    entity_id   BIGINT      NOT NULL REFERENCES entities (id),
    type        TEXT        NOT NULL,             -- posting_opened|posting_closed|jd_changed|...
    event_time  TIMESTAMPTZ NOT NULL,
    ingest_time TIMESTAMPTZ NOT NULL,
    posting_id  BIGINT,                           -- soft ref; intentionally no FK (see note)
    data        JSONB       NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, event_time)
) PARTITION BY RANGE (event_time);
CREATE INDEX events_entity_time_idx ON events (entity_id, event_time);
CREATE INDEX events_type_time_idx ON events (type, event_time);
-- posting_id has no FK on purpose: the event log is the immutable source of truth
-- and must never be blocked by, or cascade from, changes to the mutable postings
-- table. Integrity is maintained by the processor, not the database.

-- Default catch-all partitions: a safety net so an INSERT never fails if the
-- monthly maintenance job lapses. Monthly partitions are pre-created below so
-- retention/pruning operate on whole months; the default should stay empty.
CREATE TABLE raw_signals_default PARTITION OF raw_signals DEFAULT;
CREATE TABLE events_default PARTITION OF events DEFAULT;

-- Pre-create monthly partitions:
--   events      from backfill start (2023-08) through 2027-12 — the timing-history span
--   raw_signals from 2026-01 through 2027-12 — only recent ingest needs months
DO $$
DECLARE d date;
BEGIN
    d := DATE '2023-08-01';
    WHILE d < DATE '2028-01-01' LOOP
        PERFORM reqradar_ensure_month_partition('events', d);
        d := (d + INTERVAL '1 month')::date;
    END LOOP;

    d := DATE '2026-01-01';
    WHILE d < DATE '2028-01-01' LOOP
        PERFORM reqradar_ensure_month_partition('raw_signals', d);
        d := (d + INTERVAL '1 month')::date;
    END LOOP;
END $$;
