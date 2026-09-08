-- What a ride says about riding warm and about drifting away from its own
-- effort. Nullable like every other derived column: decoupling needs measured
-- power over a long enough ride, and a heat-drift reading needs the rider's
-- threshold power to place the band it is taken in.
--
-- The heat-drift reading is a pair rather than a single figure. One ride is a
-- point -- a heart rate held at a temperature -- and the drift is those points
-- over a season, so both halves of the pair are stored and neither is a trend
-- on its own.
ALTER TABLE activity_metrics ADD COLUMN decoupling_percent REAL;
ALTER TABLE activity_metrics ADD COLUMN heat_drift_heart_rate_bpm REAL;
ALTER TABLE activity_metrics ADD COLUMN heat_drift_temperature_celsius REAL;
ALTER TABLE activity_metrics ADD COLUMN heat_drift_samples INTEGER;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (47, CAST(strftime('%s', 'now') AS INTEGER));
