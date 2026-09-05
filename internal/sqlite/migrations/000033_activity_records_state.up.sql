-- Which activities still owe a FIT download, kept on the summary row so the
-- poll finds them without scanning the per-sample table. 'pending', 'stored' or
-- 'unreadable', without a CHECK: a constraint added to an existing table is what
-- a rollback onto the previous release cannot satisfy.
ALTER TABLE activities ADD COLUMN records_state TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE activities ADD COLUMN fit_checksum_failed INTEGER NOT NULL DEFAULT 0;
-- Partial: once most activities are stored or unreadable, a poll's own query
-- only ever wants the pending few, and only they cost this index anything.
CREATE INDEX activities_pending_records_index ON activities(target_slot, started_at_unix)
  WHERE records_state = 'pending';
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (33, CAST(strftime('%s', 'now') AS INTEGER));
