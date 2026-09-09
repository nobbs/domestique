-- name: ListActivityIDs :many
SELECT workout_id FROM activities WHERE target_slot = ? AND provider = ? ORDER BY workout_id;

-- name: ActivityExists :one
SELECT EXISTS(SELECT 1 FROM activities WHERE target_slot = ? AND workout_id = ? AND provider = ?);

-- Zero rows means the stored row belongs to another provider: two id spaces
-- share this key, so a collision fails loudly rather than overwriting a ride.
-- name: UpsertActivity :execrows
INSERT INTO activities (
  target_slot, workout_id, workout_type_id, workout_type_location_id, started_at_unix,
  distance_metres, moving_seconds, elapsed_seconds, ascent_metres, raw_summary_json, updated_at_unix,
  provider
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  workout_type_id = excluded.workout_type_id,
  workout_type_location_id = excluded.workout_type_location_id,
  started_at_unix = excluded.started_at_unix,
  distance_metres = excluded.distance_metres,
  moving_seconds = excluded.moving_seconds,
  elapsed_seconds = excluded.elapsed_seconds,
  ascent_metres = excluded.ascent_metres,
  raw_summary_json = excluded.raw_summary_json,
  updated_at_unix = excluded.updated_at_unix
WHERE activities.provider = excluded.provider;

-- name: ListActivitiesBetween :many
SELECT workout_id, workout_type_id, workout_type_location_id, started_at_unix,
  distance_metres, moving_seconds, elapsed_seconds, ascent_metres, provider
FROM activities
WHERE target_slot = sqlc.arg(target_slot) AND started_at_unix >= sqlc.arg(from_unix) AND started_at_unix < sqlc.arg(to_unix)
ORDER BY started_at_unix DESC
LIMIT sqlc.arg(row_limit);

-- name: ListActivitySkips :many
SELECT workout_id, attempts, last_attempt_unix FROM activity_skips WHERE target_slot = ? ORDER BY workout_id;

-- name: UpsertActivitySkip :exec
INSERT INTO activity_skips (target_slot, workout_id, attempts, last_attempt_unix, observed)
VALUES (?, ?, 1, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  attempts = attempts + 1,
  last_attempt_unix = excluded.last_attempt_unix,
  observed = excluded.observed;

-- name: DeleteActivitySkip :exec
DELETE FROM activity_skips WHERE target_slot = ? AND workout_id = ?;

-- name: ListActivitiesAwaitingRecords :many
SELECT workout_id, raw_summary_json
FROM activities
WHERE target_slot = sqlc.arg(target_slot)
  AND provider = sqlc.arg(provider)
  AND (records_state = 'pending'
    OR (records_state = 'stored' AND records_version < sqlc.arg(records_version)))
ORDER BY records_state <> 'pending', started_at_unix DESC, workout_id DESC
LIMIT sqlc.arg(row_limit);

-- name: DeleteActivityRecords :exec
DELETE FROM activity_records WHERE target_slot = ? AND workout_id = ?;

-- name: MarkActivityRecordsStored :exec
UPDATE activities SET records_state = 'stored', fit_checksum_failed = sqlc.arg(fit_checksum_failed),
  records_version = sqlc.arg(records_version)
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id);

-- name: MarkActivityRecordsUnreadable :exec
UPDATE activities SET records_state = 'unreadable' WHERE target_slot = ? AND workout_id = ?;

-- name: ListActivityRides :many
SELECT target_slot, started_at_unix, distance_metres, moving_seconds, ascent_metres
FROM activities
WHERE started_at_unix >= sqlc.arg(since_unix)
  AND workout_type_id IN (sqlc.slice(workout_type_ids))
ORDER BY started_at_unix;

-- name: ListRecordedActivities :many
SELECT target_slot, workout_id, ascent_metres, distance_metres, moving_seconds
FROM activities
WHERE records_state = 'stored'
ORDER BY target_slot, workout_id;

-- name: GetActivityRawSummary :one
SELECT raw_summary_json
FROM activities
WHERE target_slot = ? AND workout_id = ?;

-- name: GetActivityProviderSummary :one
SELECT provider, raw_summary_json
FROM activities
WHERE target_slot = ? AND workout_id = ?;

-- name: ListActivityListings :many
SELECT workout_id, started_at_unix, workout_type_id, workout_type_location_id, read_at_unix
FROM activity_listings
WHERE target_slot = ?
ORDER BY started_at_unix, workout_id;

-- name: DeleteActivityListings :exec
DELETE FROM activity_listings WHERE target_slot = ?;

-- name: InsertActivityListing :exec
INSERT INTO activity_listings (
  target_slot, workout_id, started_at_unix, workout_type_id, workout_type_location_id, read_at_unix
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  started_at_unix = excluded.started_at_unix,
  workout_type_id = excluded.workout_type_id,
  workout_type_location_id = excluded.workout_type_location_id,
  read_at_unix = excluded.read_at_unix;

-- name: GetActivityRecordsState :one
SELECT records_state, workout_type_id FROM activities WHERE target_slot = ? AND workout_id = ?;

-- name: ListActivityTrack :many
SELECT recorded_at_unix, latitude, longitude, altitude_metres, estimated_power_watts
FROM activity_records
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id)
  AND latitude IS NOT NULL AND longitude IS NOT NULL
ORDER BY record_index;

-- name: ListActivitySeries :many
SELECT recorded_at_unix, distance_metres, altitude_metres,
  heart_rate_bpm, cadence_rpm, power_watts, temperature_celsius,
  speed_ms, grade_percent, calories_kcal, ascent_metres, descent_metres
FROM activity_records
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id)
  AND latitude IS NOT NULL AND longitude IS NOT NULL
ORDER BY record_index;

-- name: UpsertActivitySession :exec
INSERT INTO activity_session (
  target_slot, workout_id, max_speed_kmh, average_speed_kmh, distance_metres,
  timer_seconds, elapsed_seconds, ascent_metres, descent_metres, calories_kcal,
  average_heart_rate_bpm, max_heart_rate_bpm, min_heart_rate_bpm,
  average_cadence_rpm, max_cadence_rpm,
  average_power_watts, max_power_watts, normalized_power_watts, threshold_power_watts,
  average_temperature_celsius, max_temperature_celsius,
  average_grade_percent, max_positive_grade_percent, max_negative_grade_percent,
  min_altitude_metres, max_altitude_metres, average_altitude_metres,
  sport, sub_sport,
  heart_rate_zone_seconds_json, heart_rate_zone_high_bpm_json,
  power_zone_seconds_json, power_zone_high_watts_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  max_speed_kmh = excluded.max_speed_kmh,
  average_speed_kmh = excluded.average_speed_kmh,
  distance_metres = excluded.distance_metres,
  timer_seconds = excluded.timer_seconds,
  elapsed_seconds = excluded.elapsed_seconds,
  ascent_metres = excluded.ascent_metres,
  descent_metres = excluded.descent_metres,
  calories_kcal = excluded.calories_kcal,
  average_heart_rate_bpm = excluded.average_heart_rate_bpm,
  max_heart_rate_bpm = excluded.max_heart_rate_bpm,
  min_heart_rate_bpm = excluded.min_heart_rate_bpm,
  average_cadence_rpm = excluded.average_cadence_rpm,
  max_cadence_rpm = excluded.max_cadence_rpm,
  average_power_watts = excluded.average_power_watts,
  max_power_watts = excluded.max_power_watts,
  normalized_power_watts = excluded.normalized_power_watts,
  threshold_power_watts = excluded.threshold_power_watts,
  average_temperature_celsius = excluded.average_temperature_celsius,
  max_temperature_celsius = excluded.max_temperature_celsius,
  average_grade_percent = excluded.average_grade_percent,
  max_positive_grade_percent = excluded.max_positive_grade_percent,
  max_negative_grade_percent = excluded.max_negative_grade_percent,
  min_altitude_metres = excluded.min_altitude_metres,
  max_altitude_metres = excluded.max_altitude_metres,
  average_altitude_metres = excluded.average_altitude_metres,
  sport = excluded.sport,
  sub_sport = excluded.sub_sport,
  heart_rate_zone_seconds_json = excluded.heart_rate_zone_seconds_json,
  heart_rate_zone_high_bpm_json = excluded.heart_rate_zone_high_bpm_json,
  power_zone_seconds_json = excluded.power_zone_seconds_json,
  power_zone_high_watts_json = excluded.power_zone_high_watts_json;

-- name: DeleteActivitySession :exec
DELETE FROM activity_session WHERE target_slot = ? AND workout_id = ?;

-- name: ListActivitySessions :many
SELECT workout_id, max_speed_kmh, average_speed_kmh, distance_metres,
  timer_seconds, elapsed_seconds, ascent_metres, descent_metres, calories_kcal,
  average_heart_rate_bpm, max_heart_rate_bpm, min_heart_rate_bpm,
  average_cadence_rpm, max_cadence_rpm,
  average_power_watts, max_power_watts, normalized_power_watts, threshold_power_watts,
  average_temperature_celsius, max_temperature_celsius,
  average_grade_percent, max_positive_grade_percent, max_negative_grade_percent,
  min_altitude_metres, max_altitude_metres, average_altitude_metres,
  sport, sub_sport,
  heart_rate_zone_seconds_json, heart_rate_zone_high_bpm_json,
  power_zone_seconds_json, power_zone_high_watts_json
FROM activity_session
WHERE target_slot = ?
ORDER BY workout_id;

-- name: ApplyActivitySessionTotals :exec
UPDATE activities SET
  distance_metres = COALESCE(sqlc.narg(distance_metres), distance_metres),
  moving_seconds  = COALESCE(sqlc.narg(moving_seconds), moving_seconds),
  elapsed_seconds = COALESCE(sqlc.narg(elapsed_seconds), elapsed_seconds),
  ascent_metres   = COALESCE(sqlc.narg(ascent_metres), ascent_metres)
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id);

-- Zwift rides alone: a Wahoo listing starting within a minute of one of these
-- is the head unit's copy of that same indoor ride.
-- name: ListIndoorRideStarts :many
SELECT started_at_unix FROM activities
WHERE target_slot = ? AND provider = 'zwift'
ORDER BY started_at_unix;

-- The head unit's copy of an indoor ride Zwift also recorded. Every derived row
-- goes with it through the existing cascades; the skip and listing rows do not,
-- and must not: those mirror the account rather than what is stored.
-- name: DeleteTrainerCopyActivity :execrows
DELETE FROM activities
WHERE target_slot = sqlc.arg(target_slot) AND provider = 'wahoo'
  AND workout_type_id IN (SELECT value FROM json_each(CAST(sqlc.arg(indoor_type_ids) AS TEXT)))
  AND started_at_unix >= sqlc.arg(from_unix) AND started_at_unix <= sqlc.arg(to_unix);
