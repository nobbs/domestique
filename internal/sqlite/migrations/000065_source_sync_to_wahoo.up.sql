-- Whether a run writes this library's routes to riders' Wahoo accounts. Off
-- still reads it; its routes simply leave the targets.
ALTER TABLE runtime_source ADD COLUMN sync_to_wahoo INTEGER NOT NULL DEFAULT 1;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (65, CAST(strftime('%s', 'now') AS INTEGER));
