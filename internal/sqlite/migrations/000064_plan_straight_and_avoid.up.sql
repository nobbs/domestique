-- The waypoints whose incoming leg is a straight line rather than routed, as a
-- JSON array of their indices, and the circles a plan's route keeps out of, as
-- a JSON array of [longitude, latitude, radius in metres].
ALTER TABLE plans ADD COLUMN straight TEXT NOT NULL DEFAULT '[]';
ALTER TABLE plans ADD COLUMN avoid TEXT NOT NULL DEFAULT '[]';
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (64, CAST(strftime('%s', 'now') AS INTEGER));
