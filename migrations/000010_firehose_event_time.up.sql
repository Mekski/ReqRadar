-- firehose_seen recorded only first_seen (when WE saw it), not the job's own
-- posting date — so the feed couldn't show a posting's real age or sort by it.
-- Add event_time (the posting's date_posted) so the firehose reads as a genuine
-- "newly posted" feed. Nullable: rows that predate this (incl. the backfill-
-- polluted ones being cleared) have no known posting date.
ALTER TABLE firehose_seen ADD COLUMN event_time TIMESTAMPTZ;
