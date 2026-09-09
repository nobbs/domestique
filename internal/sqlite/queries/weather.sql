-- name: InsertActivityWeather :exec
INSERT INTO activity_weather (
  target_slot, workout_id, hour_unix, step_seconds, temperature_celsius, apparent_temperature_celsius,
  precipitation_millimetres, precipitation_probability_percent, wind_speed_kmh,
  wind_direction_degrees, weather_code, cloud_cover_percent
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id, hour_unix) DO UPDATE SET
  step_seconds = excluded.step_seconds,
  temperature_celsius = excluded.temperature_celsius,
  apparent_temperature_celsius = excluded.apparent_temperature_celsius,
  precipitation_millimetres = excluded.precipitation_millimetres,
  precipitation_probability_percent = excluded.precipitation_probability_percent,
  wind_speed_kmh = excluded.wind_speed_kmh,
  wind_direction_degrees = excluded.wind_direction_degrees,
  weather_code = excluded.weather_code,
  cloud_cover_percent = excluded.cloud_cover_percent;

-- name: DeleteActivityWeather :exec
DELETE FROM activity_weather WHERE target_slot = ? AND workout_id = ?;

-- name: RecordActivityWeatherRead :exec
INSERT INTO activity_weather_reads (target_slot, workout_id, read_at_unix, hours)
VALUES (?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  read_at_unix = excluded.read_at_unix,
  hours = excluded.hours;

-- Rides whose weather has never been asked about. A read that failed for good
-- left a row saying so, which is what keeps it out of this list.
-- name: ListActivitiesAwaitingWeather :many
SELECT a.workout_id, a.started_at_unix, a.elapsed_seconds
FROM activities AS a
LEFT JOIN activity_weather_reads AS r ON r.target_slot = a.target_slot AND r.workout_id = a.workout_id
WHERE a.target_slot = sqlc.arg(target_slot)
  AND a.records_state = 'stored'
  AND a.workout_type_id NOT IN (SELECT value FROM json_each(CAST(sqlc.arg(indoor_type_ids) AS TEXT)))
  AND r.workout_id IS NULL
ORDER BY a.started_at_unix DESC, a.workout_id DESC
LIMIT sqlc.arg(row_limit);

-- One row per ride rather than one per step: the listing wants a line about
-- each ride, and a rider with years of history has a step of weather for every
-- step they have ridden. The wind is a mean speed and never a mean direction: a
-- bearing does not average, so the summary carries none.
-- name: SummariseActivityWeather :many
SELECT workout_id,
  CAST(MIN(temperature_celsius) AS REAL) AS temperature_min_celsius,
  CAST(MAX(temperature_celsius) AS REAL) AS temperature_max_celsius,
  CAST(AVG(wind_speed_kmh) AS REAL) AS wind_speed_kmh,
  CAST(SUM(precipitation_millimetres) AS REAL) AS precipitation_millimetres,
  CAST(MAX(weather_code) AS INTEGER) AS weather_code
FROM activity_weather
WHERE target_slot = ?
GROUP BY workout_id
ORDER BY workout_id;

-- Aliased to at_unix rather than the stored hour_unix: the column keeps its
-- name so a preceding release can still read and write it, but every step of
-- a ride is not necessarily an hour any more.
-- name: ListActivityWeatherSteps :many
SELECT hour_unix AS at_unix, step_seconds, temperature_celsius, apparent_temperature_celsius,
  precipitation_millimetres, precipitation_probability_percent, wind_speed_kmh,
  wind_direction_degrees, weather_code, cloud_cover_percent
FROM activity_weather
WHERE target_slot = ? AND workout_id = ?
ORDER BY hour_unix;
