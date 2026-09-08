DELETE FROM schema_migrations WHERE version = 50;

-- ALTER TABLE ... DROP COLUMN for activities, as migration 030's down
-- migration explains: activity_records and other tables reference it by
-- foreign key, so a rebuild's DROP TABLE would fail against any stored ride.
ALTER TABLE activities DROP COLUMN records_version;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN for activity_records, for
-- the reason migration 041's down migration gives: it is referenced by
-- nothing, so a rebuild is safe and stays portable to whatever sqlite3 an
-- operator has on hand.
CREATE TABLE activity_records_pre_series (
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
  PRIMARY KEY (target_slot, workout_id, record_index),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_records_pre_series
SELECT target_slot, workout_id, record_index, recorded_at_unix, distance_metres, latitude,
  longitude, altitude_metres, cadence_rpm, heart_rate_bpm, power_watts, temperature_celsius,
  estimated_power_watts
FROM activity_records;
DROP TABLE activity_records;
ALTER TABLE activity_records_pre_series RENAME TO activity_records;
