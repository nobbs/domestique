-- Which upstream recorded a ride. Defaulted rather than back-filled: every row
-- that exists today came from Wahoo. No CHECK, for migration 33's reason - a
-- constraint added to an existing table is what a rollback cannot satisfy.
ALTER TABLE activities ADD COLUMN provider TEXT NOT NULL DEFAULT 'wahoo';
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (54, CAST(strftime('%s', 'now') AS INTEGER));
