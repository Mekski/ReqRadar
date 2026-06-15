DROP INDEX IF EXISTS postings_category_idx;
ALTER TABLE postings DROP COLUMN IF EXISTS category;
