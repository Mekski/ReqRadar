-- Mark each posting as summer-cohort or not, so "expected open" reflects the
-- Summer SWE-intern cycle (not fall/off-season roles that skew the month).
-- Derived from the listing's `terms` (e.g. ["Summer 2026"]).
ALTER TABLE postings ADD COLUMN is_summer BOOLEAN NOT NULL DEFAULT false;

UPDATE postings p SET is_summer = v.summer
FROM (
    SELECT DISTINCT ON (posting_id) posting_id,
        EXISTS (
            SELECT 1 FROM jsonb_array_elements_text(parsed -> 'terms') AS t WHERE t LIKE 'Summer%'
        ) AS summer
    FROM posting_versions ORDER BY posting_id, captured_at DESC
) v
WHERE v.posting_id = p.id;

CREATE INDEX postings_summer_idx ON postings (is_summer);
