-- name: GetRiderProfile :one
SELECT max_heart_rate_bpm, resting_heart_rate_bpm, threshold_heart_rate_bpm,
  functional_threshold_power_watts, rider_mass_kg, bike_mass_kg
FROM rider_profiles
WHERE subject = ?;

-- name: UpsertRiderProfile :exec
INSERT INTO rider_profiles (
  subject, max_heart_rate_bpm, resting_heart_rate_bpm, threshold_heart_rate_bpm,
  functional_threshold_power_watts, rider_mass_kg, bike_mass_kg, updated_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(subject) DO UPDATE SET
  max_heart_rate_bpm = excluded.max_heart_rate_bpm,
  resting_heart_rate_bpm = excluded.resting_heart_rate_bpm,
  threshold_heart_rate_bpm = excluded.threshold_heart_rate_bpm,
  functional_threshold_power_watts = excluded.functional_threshold_power_watts,
  rider_mass_kg = excluded.rider_mass_kg,
  bike_mass_kg = excluded.bike_mass_kg,
  updated_at_unix = excluded.updated_at_unix;

-- name: ListActivitySensorSamples :many
SELECT r.workout_id, r.recorded_at_unix, r.heart_rate_bpm, r.power_watts
FROM activity_records AS r
JOIN activities AS a ON a.target_slot = r.target_slot AND a.workout_id = r.workout_id
WHERE r.target_slot = sqlc.arg(target_slot)
  AND a.started_at_unix >= sqlc.arg(since_unix)
  AND (r.heart_rate_bpm IS NOT NULL OR r.power_watts IS NOT NULL)
ORDER BY r.workout_id, r.record_index;
