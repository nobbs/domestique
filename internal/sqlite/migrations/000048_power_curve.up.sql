-- The best mean power each ride held over the power-duration curve's durations,
-- so the curve is a fold over stored numbers rather than a rescan of every
-- sensor sample the rider has ever recorded.
--
-- One column per duration, named by it in seconds, the way the zone columns are
-- named by their zone. Nullable like every other derived column: a ride shorter
-- than a duration holds no best for it, and a ride with no meter holds none at
-- all. Measured power only -- an estimate never enters the curve.
ALTER TABLE activity_metrics ADD COLUMN best_power_5s REAL;
ALTER TABLE activity_metrics ADD COLUMN best_power_30s REAL;
ALTER TABLE activity_metrics ADD COLUMN best_power_60s REAL;
ALTER TABLE activity_metrics ADD COLUMN best_power_300s REAL;
ALTER TABLE activity_metrics ADD COLUMN best_power_1200s REAL;
ALTER TABLE activity_metrics ADD COLUMN best_power_3600s REAL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (48, CAST(strftime('%s', 'now') AS INTEGER));
