-- The device's own instantaneous speed, gradient and cumulative counters,
-- beside the estimated ones this service derives -- never substituted for the
-- derivation, and served nowhere it is not asked for by name. Wahoo's own
-- cumulative ascent and descent arrive as developer fields rather than a FIT
-- profile field, resolved by field_description name rather than a fixed
-- number; a file without them, or one from any other device, leaves the
-- columns null like every other sensor a ride was not fitted with.
ALTER TABLE activity_records ADD COLUMN speed_ms REAL;
ALTER TABLE activity_records ADD COLUMN grade_percent REAL;
ALTER TABLE activity_records ADD COLUMN calories_kcal REAL;
ALTER TABLE activity_records ADD COLUMN ascent_metres REAL;
ALTER TABLE activity_records ADD COLUMN descent_metres REAL;
-- Which record schema a ride's samples were last decoded under, beside
-- records_state: a ride stored before this column existed, or before the
-- schema it names grew a field, is stale rather than missing, and a poll
-- re-reads it once more, at the pace its first download did.
ALTER TABLE activities ADD COLUMN records_version INTEGER NOT NULL DEFAULT 0;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (50, CAST(strftime('%s', 'now') AS INTEGER));
