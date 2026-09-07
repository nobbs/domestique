-- The weather one ride was actually ridden through, asked of Open-Meteo once
-- and kept. Hourly rows, keyed as the ride is and cascading with it: weather
-- outliving the ride it describes describes nothing.
--
-- precipitation_probability is nullable alone among the series: the reanalysis
-- that answers for an older ride records what fell rather than what might have,
-- so a ride from last year has every other column and not that one.
CREATE TABLE activity_weather (
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

-- What a ride's weather read came to, so a ride is asked about once rather than
-- on every run. A row here with no rows above is a read that failed for good:
-- the provider has no data for that place and time, and asking again spends a
-- request to be told so again. Nothing about the failure is recorded but the
-- fact and when, because a provider's own words are not this service's to keep.
CREATE TABLE activity_weather_reads (
  target_slot   TEXT    NOT NULL,
  workout_id    INTEGER NOT NULL,
  read_at_unix  INTEGER NOT NULL,
  hours         INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (42, CAST(strftime('%s', 'now') AS INTEGER));
