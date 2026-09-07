-- Power worked out from the track for the bicycles that carry no meter. Kept in
-- its own column, never in power_watts: an estimate and a measurement must
-- never be mistaken for one another, by a chart or by anything downstream.
-- Nullable, because a ride with no track, or one that already carries real
-- power, has no estimate rather than an estimate of zero.
ALTER TABLE activity_records ADD COLUMN estimated_power_watts REAL;
-- The ride's own average of the above, beside the other per-ride figures. Its
-- input is the rider's total mass, recorded here for the same reason the other
-- inputs are: a row worked out against a mass nobody holds is stale.
ALTER TABLE activity_metrics ADD COLUMN estimated_power_watts REAL;
ALTER TABLE activity_metrics ADD COLUMN input_total_mass REAL NOT NULL DEFAULT 0;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (41, CAST(strftime('%s', 'now') AS INTEGER));
