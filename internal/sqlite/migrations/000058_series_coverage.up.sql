-- Each series' own share of the ride's moving time it held a reading for,
-- kept beside the figures it describes rather than spent only on the
-- withhold decision in internal/trainingload. Nullable like every other
-- derived column: a ride with no strap or no meter has no coverage for it.
ALTER TABLE activity_metrics ADD COLUMN heart_rate_coverage REAL;
ALTER TABLE activity_metrics ADD COLUMN power_coverage REAL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (58, CAST(strftime('%s', 'now') AS INTEGER));
