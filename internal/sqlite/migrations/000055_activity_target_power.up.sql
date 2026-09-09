-- The power a structured workout prescribed for this record, from a Zwift
-- FIT's own developer field. Nullable: a free ride and every other provider
-- carry none.
ALTER TABLE activity_records ADD COLUMN target_power_watts REAL;

-- A Zwift ride's structured workout identity and outcome, from the
-- single-activity response. All nullable: absent for a free ride and every
-- other provider.
ALTER TABLE activities ADD COLUMN workout_name TEXT;
ALTER TABLE activities ADD COLUMN workout_hash INTEGER;
ALTER TABLE activities ADD COLUMN workout_completion REAL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (55, CAST(strftime('%s', 'now') AS INTEGER));
