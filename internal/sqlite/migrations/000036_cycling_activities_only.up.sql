-- A poll now stores only Wahoo's biking family, so the runs, walks, swims and
-- strength sessions earlier polls kept are removed: they were never predicted
-- from, and a calibration counting them fits a rider against another sport.
-- Records go first so the rows are gone whether or not the cascade is enforced.
DELETE FROM activity_records WHERE (target_slot, workout_id) IN (
  SELECT target_slot, workout_id FROM activities
  WHERE workout_type_id NOT IN (0, 11, 12, 13, 14, 15, 16, 17, 49, 61, 64, 68, 70)
);
DELETE FROM activities
WHERE workout_type_id NOT IN (0, 11, 12, 13, 14, 15, 16, 17, 49, 61, 64, 68, 70);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (36, CAST(strftime('%s', 'now') AS INTEGER));
