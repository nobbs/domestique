DELETE FROM schema_migrations WHERE version = 41;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason migration
-- 029's down migration gives: portability to whatever sqlite3 an operator has
-- on hand matters most on this path. activity_records is referenced by nothing,
-- so a rebuild is safe here.
CREATE TABLE activity_records_pre_estimate (
  target_slot         TEXT    NOT NULL,
  workout_id          INTEGER NOT NULL,
  record_index        INTEGER NOT NULL,
  recorded_at_unix    INTEGER NOT NULL,
  distance_metres     REAL,
  latitude            REAL,
  longitude           REAL,
  altitude_metres     REAL,
  cadence_rpm         REAL,
  heart_rate_bpm      REAL,
  power_watts         REAL,
  temperature_celsius REAL,
  PRIMARY KEY (target_slot, workout_id, record_index),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_records_pre_estimate
SELECT target_slot, workout_id, record_index, recorded_at_unix, distance_metres, latitude,
  longitude, altitude_metres, cadence_rpm, heart_rate_bpm, power_watts, temperature_celsius
FROM activity_records;
DROP TABLE activity_records;
ALTER TABLE activity_records_pre_estimate RENAME TO activity_records;

CREATE TABLE activity_metrics_pre_estimate (
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
  input_max_heart_rate       REAL    NOT NULL,
  input_resting_heart_rate   REAL    NOT NULL,
  input_threshold_heart_rate REAL    NOT NULL,
  input_threshold_power      REAL    NOT NULL,
  computed_at_unix           INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_metrics_pre_estimate
SELECT target_slot, workout_id, zone_1_seconds, zone_2_seconds, zone_3_seconds, zone_4_seconds,
  zone_5_seconds, trimp, heart_rate_tss, normalized_power_watts, intensity_factor, power_tss,
  input_max_heart_rate, input_resting_heart_rate, input_threshold_heart_rate, input_threshold_power,
  computed_at_unix
FROM activity_metrics;
DROP TABLE activity_metrics;
ALTER TABLE activity_metrics_pre_estimate RENAME TO activity_metrics;
