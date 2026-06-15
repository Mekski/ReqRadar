-- Posted pay: 000003 already created pay_min/pay_max (INTEGER) + pay_currency,
-- unused until now. This migration (1) widens the amounts to NUMERIC so hourly
-- intern rates keep their cents (e.g. $72.50/hr), and (2) adds pay_period to
-- distinguish the dominant intern case (hourly) from annual/monthly. The
-- processor extracts these from JD text (posted ranges only — DESIGN legal lines).
ALTER TABLE postings
  ALTER COLUMN pay_min TYPE NUMERIC(12, 2),
  ALTER COLUMN pay_max TYPE NUMERIC(12, 2),
  ADD COLUMN pay_period TEXT;          -- 'hourly' | 'annual' | 'monthly'
