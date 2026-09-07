DELETE FROM schema_migrations WHERE version = 43;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason migration
-- 041's down migration gives: portability to whatever sqlite3 an operator has
-- on hand matters most on this path.
CREATE TABLE activity_metrics_pre_averages (
  target_slot                TEXT    NOT NULL,
  workout_id                 INTEGER NOT NULL,
  zone_1_seconds             REAL,
  zone_2_seconds             REAL,
  zone_3_seconds             REAL,
  zone_4_seconds             REAL,
  zone_5_seconds             REAL,
  trimp                      REAL,
  heart_rate_tss             REAL,
  normalized_power_watts     REAL,
  intensity_factor           REAL,
  power_tss                  REAL,
  estimated_power_watts      REAL,
  input_max_heart_rate       REAL    NOT NULL,
  input_resting_heart_rate   REAL    NOT NULL,
  input_threshold_heart_rate REAL    NOT NULL,
  input_threshold_power      REAL    NOT NULL,
  input_total_mass           REAL    NOT NULL DEFAULT 0,
  computed_at_unix           INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_metrics_pre_averages
SELECT target_slot, workout_id, zone_1_seconds, zone_2_seconds, zone_3_seconds, zone_4_seconds,
  zone_5_seconds, trimp, heart_rate_tss, normalized_power_watts, intensity_factor, power_tss,
  estimated_power_watts, input_max_heart_rate, input_resting_heart_rate, input_threshold_heart_rate,
  input_threshold_power, input_total_mass, computed_at_unix
FROM activity_metrics;
DROP TABLE activity_metrics;
ALTER TABLE activity_metrics_pre_averages RENAME TO activity_metrics;
