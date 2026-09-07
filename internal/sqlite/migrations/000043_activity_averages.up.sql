-- The plain figures a rider reads before any training load: what the ride's own
-- sensors averaged, and the highest heart rate it reached. Nullable like every
-- other derived column — a ride carries the sensors it carries — and worked out
-- from the same stored samples, so no upstream request is involved.
--
-- Average speed is deliberately not here: it is distance over moving time, both
-- already on activities, and is therefore known for a ride whose file never
-- became readable at all.
ALTER TABLE activity_metrics ADD COLUMN average_heart_rate_bpm REAL;
ALTER TABLE activity_metrics ADD COLUMN max_heart_rate_bpm REAL;
ALTER TABLE activity_metrics ADD COLUMN average_cadence_rpm REAL;
ALTER TABLE activity_metrics ADD COLUMN average_power_watts REAL;
-- Which derivation wrote the row. A row from an earlier one is stale even when
-- the profile behind it is current, which is the only way to tell a figure this
-- derivation has yet to work out from one the ride simply has no sensor for.
ALTER TABLE activity_metrics ADD COLUMN derivation_version INTEGER NOT NULL DEFAULT 0;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (43, CAST(strftime('%s', 'now') AS INTEGER));
