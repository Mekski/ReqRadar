DROP TABLE IF EXISTS events;        -- cascades to all month partitions + default
DROP TABLE IF EXISTS raw_signals;   -- cascades to all month partitions + default
DROP FUNCTION IF EXISTS reqradar_ensure_month_partition(text, date);
