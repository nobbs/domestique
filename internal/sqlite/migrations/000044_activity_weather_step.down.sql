DELETE FROM schema_migrations WHERE version = 44;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the same reason
-- migration 029's down migration is: a down migration is exactly the
-- disaster-recovery path where portability to whatever sqlite3 an operator
-- has on hand matters most.
CREATE TABLE activity_weather_pre_step (
  target_slot                       TEXT    NOT NULL,
  workout_id                        INTEGER NOT NULL,
  hour_unix                         INTEGER NOT NULL,
  temperature_celsius               REAL    NOT NULL,
  apparent_temperature_celsius      REAL    NOT NULL,
  precipitation_millimetres         REAL    NOT NULL,
  precipitation_probability_percent REAL,
  wind_speed_kmh                    REAL    NOT NULL,
  wind_direction_degrees            REAL    NOT NULL,
  weather_code                      INTEGER NOT NULL,
  cloud_cover_percent               REAL    NOT NULL,
  PRIMARY KEY (target_slot, workout_id, hour_unix),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_weather_pre_step
SELECT target_slot, workout_id, hour_unix, temperature_celsius, apparent_temperature_celsius,
  precipitation_millimetres, precipitation_probability_percent, wind_speed_kmh,
  wind_direction_degrees, weather_code, cloud_cover_percent
FROM activity_weather;
DROP TABLE activity_weather;
ALTER TABLE activity_weather_pre_step RENAME TO activity_weather;
