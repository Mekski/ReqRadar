DROP INDEX IF EXISTS postings_summer_idx;
ALTER TABLE postings DROP COLUMN IF EXISTS is_summer;
