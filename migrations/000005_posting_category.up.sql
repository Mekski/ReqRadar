-- Add category to postings so seasonality/timing can filter to SWE-intern roles
-- (the question Mark actually cares about) vs. all internship types blended.
ALTER TABLE postings ADD COLUMN category TEXT;

-- Backfill from each posting's latest version's parsed payload (no re-ingest needed).
UPDATE postings p SET category = v.cat
FROM (
    SELECT DISTINCT ON (posting_id) posting_id, parsed->>'category' AS cat
    FROM posting_versions ORDER BY posting_id, captured_at DESC
) v
WHERE v.posting_id = p.id;

CREATE INDEX postings_category_idx ON postings (category);
