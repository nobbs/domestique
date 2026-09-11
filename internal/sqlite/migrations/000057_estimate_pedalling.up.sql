-- The share of the ride's own samples the rider was pedalling through, which
-- the estimate above is averaged over, and the two bicycle numbers the
-- estimate was worked out against. Nullable/zero-defaulted like every other
-- derived and input column: a ride with no estimate has no share, and a row
-- worked out before a rider entered a bicycle held the built-in numbers.
ALTER TABLE activity_metrics ADD COLUMN estimated_pedalling_share REAL;
ALTER TABLE activity_metrics ADD COLUMN input_drag_area REAL NOT NULL DEFAULT 0;
ALTER TABLE activity_metrics ADD COLUMN input_rolling_resistance REAL NOT NULL DEFAULT 0;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (57, CAST(strftime('%s', 'now') AS INTEGER));
