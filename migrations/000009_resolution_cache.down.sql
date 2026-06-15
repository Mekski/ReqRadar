-- Revert to the non-unique index. (Collapsed duplicate rows are not restored —
-- forward-only philosophy; the cache shape is the intended state.)
DROP INDEX IF EXISTS resolution_decisions_raw_text_key;
CREATE INDEX resolution_decisions_raw_text_idx ON resolution_decisions (raw_text);
