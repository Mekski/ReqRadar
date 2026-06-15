-- resolution_decisions becomes a resolution CACHE: one decision per raw company
-- string, DB-enforced. Previously dedup lived only in the processor's in-memory
-- set, so every restart re-appended a row for every string in the feed (unbounded
-- growth in a "kept forever" table) and, once an LLM resolution step lands, would
-- re-trigger calls — violating "one call per unique string, ever".
--
-- Collapse any existing duplicates (keep the earliest decision per string), then
-- replace the non-unique index with a unique one. RecordResolution now inserts
-- with ON CONFLICT (raw_text) DO NOTHING, so the first decision wins and stands.
DELETE FROM resolution_decisions a
USING resolution_decisions b
WHERE a.id > b.id AND a.raw_text = b.raw_text;

DROP INDEX IF EXISTS resolution_decisions_raw_text_idx;
CREATE UNIQUE INDEX resolution_decisions_raw_text_key ON resolution_decisions (raw_text);
