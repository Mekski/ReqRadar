-- Reverts to the 000003 shape: drop pay_period, narrow amounts back to INTEGER.
-- pay_min/pay_max/pay_currency themselves predate this migration, so they stay.
ALTER TABLE postings
  DROP COLUMN IF EXISTS pay_period,
  ALTER COLUMN pay_min TYPE INTEGER,
  ALTER COLUMN pay_max TYPE INTEGER;
