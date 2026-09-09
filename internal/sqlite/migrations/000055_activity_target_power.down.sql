DELETE FROM schema_migrations WHERE version = 55;

ALTER TABLE activities DROP COLUMN workout_completion;
ALTER TABLE activities DROP COLUMN workout_hash;
ALTER TABLE activities DROP COLUMN workout_name;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason migration
-- 050's down migration gives: portability to whatever sqlite3 an operator has
-- on hand matters most on this path.
CREATE TABLE activity_records_pre_target_power (
  target_slot           TEXT    NOT NULL,
  workout_id            INTEGER NOT NULL,
  record_index          INTEGER NOT NULL,
  recorded_at_unix      INTEGER NOT NULL,
  distance_metres       REAL,
  latitude              REAL,
  longitude             REAL,
  altitude_metres       REAL,
  cadence_rpm           REAL,
  heart_rate_bpm        REAL,
  power_watts           REAL,
  temperature_celsius   REAL,
  estimated_power_watts REAL,
  speed_ms              REAL,
  grade_percent         REAL,
  calories_kcal         REAL,
  ascent_metres         REAL,
  descent_metres        REAL,
  PRIMARY KEY (target_slot, workout_id, record_index),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_records_pre_target_power
SELECT target_slot, workout_id, record_index, recorded_at_unix, distance_metres, latitude,
  longitude, altitude_metres, cadence_rpm, heart_rate_bpm, power_watts, temperature_celsius,
  estimated_power_watts, speed_ms, grade_percent, calories_kcal, ascent_metres, descent_metres
FROM activity_records;
DROP TABLE activity_records;
ALTER TABLE activity_records_pre_target_power RENAME TO activity_records;
