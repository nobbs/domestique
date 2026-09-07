-- What the estimate's own shape says about whether to trust it, beside the
-- estimate itself. Nullable like every other derived column: a ride with no
-- estimate has no quality either, and one column can never carry both facts.
ALTER TABLE activity_metrics ADD COLUMN estimate_autocorrelation REAL;
ALTER TABLE activity_metrics ADD COLUMN estimate_delta_watts_per_second REAL;
ALTER TABLE activity_metrics ADD COLUMN estimate_clip_bias_watts REAL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (45, CAST(strftime('%s', 'now') AS INTEGER));
