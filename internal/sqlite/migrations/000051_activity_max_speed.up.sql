-- The ride's highest speed, in km/h: from the device's own speed reading
-- where any record carried one, otherwise from distance over time. Nullable
-- like every other derived sensor figure -- a ride with no speed series at
-- all holds none.
ALTER TABLE activity_metrics ADD COLUMN max_speed_kmh REAL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (51, CAST(strftime('%s', 'now') AS INTEGER));
