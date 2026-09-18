-- The routing engine's turn instructions along a plan, as a JSON array of
-- {turn, metres, exit} objects measured against the stored line, and whether
-- the plan's course carries them to a device.
ALTER TABLE plans ADD COLUMN turns TEXT NOT NULL DEFAULT '[]';
ALTER TABLE plans ADD COLUMN cues INTEGER NOT NULL DEFAULT 0;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (63, CAST(strftime('%s', 'now') AS INTEGER));
