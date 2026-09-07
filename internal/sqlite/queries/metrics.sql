-- name: UpsertActivityMetrics :exec
INSERT INTO activity_metrics (
  target_slot, workout_id,
  zone_1_seconds, zone_2_seconds, zone_3_seconds, zone_4_seconds, zone_5_seconds,
  trimp, heart_rate_tss, normalized_power_watts, intensity_factor, power_tss,
  estimated_power_watts,
  input_max_heart_rate, input_resting_heart_rate, input_threshold_heart_rate, input_threshold_power,
  input_total_mass, computed_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  zone_1_seconds = excluded.zone_1_seconds,
  zone_2_seconds = excluded.zone_2_seconds,
  zone_3_seconds = excluded.zone_3_seconds,
  zone_4_seconds = excluded.zone_4_seconds,
  zone_5_seconds = excluded.zone_5_seconds,
  trimp = excluded.trimp,
  heart_rate_tss = excluded.heart_rate_tss,
  normalized_power_watts = excluded.normalized_power_watts,
  intensity_factor = excluded.intensity_factor,
  power_tss = excluded.power_tss,
  estimated_power_watts = excluded.estimated_power_watts,
  input_max_heart_rate = excluded.input_max_heart_rate,
  input_resting_heart_rate = excluded.input_resting_heart_rate,
  input_threshold_heart_rate = excluded.input_threshold_heart_rate,
  input_threshold_power = excluded.input_threshold_power,
  input_total_mass = excluded.input_total_mass,
  computed_at_unix = excluded.computed_at_unix;

-- name: DeleteActivityMetrics :exec
DELETE FROM activity_metrics WHERE target_slot = ? AND workout_id = ?;

-- Every row one target holds, for a rider who has cleared the profile the rows
-- were worked out from. Reports how many went, so a derivation can tell a
-- clearing from a rider who never had a profile at all.
-- name: ClearActivityMetrics :execrows
DELETE FROM activity_metrics WHERE target_slot = ?;

-- name: ListActivityMetrics :many
SELECT workout_id,
  zone_1_seconds, zone_2_seconds, zone_3_seconds, zone_4_seconds, zone_5_seconds,
  trimp, heart_rate_tss, normalized_power_watts, intensity_factor, power_tss,
  estimated_power_watts
FROM activity_metrics
WHERE target_slot = ?
ORDER BY workout_id;

-- Rides whose stored samples could still yield something the profile now
-- allows: those with no metrics row at all, and those whose row was worked out
-- against different profile values. A ride still awaiting its FIT has nothing
-- to derive from and is left for the download to bring in.
-- name: ListActivitiesAwaitingDerivation :many
SELECT a.workout_id
FROM activities AS a
LEFT JOIN activity_metrics AS m ON m.target_slot = a.target_slot AND m.workout_id = a.workout_id
WHERE a.target_slot = sqlc.arg(target_slot)
  AND a.records_state = 'stored'
  AND (m.workout_id IS NULL
    OR m.input_max_heart_rate <> sqlc.arg(max_heart_rate)
    OR m.input_resting_heart_rate <> sqlc.arg(resting_heart_rate)
    OR m.input_threshold_heart_rate <> sqlc.arg(threshold_heart_rate)
    OR m.input_threshold_power <> sqlc.arg(threshold_power)
    OR m.input_total_mass <> sqlc.arg(total_mass))
ORDER BY a.started_at_unix DESC, a.workout_id DESC;

-- name: ListActivitySensorRecords :many
SELECT record_index, recorded_at_unix, heart_rate_bpm, power_watts,
  distance_metres, altitude_metres, latitude
FROM activity_records
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id)
ORDER BY record_index;

-- name: GetTargetOwner :one
SELECT COALESCE(owner_subject, '') AS owner_subject FROM targets WHERE slot = ?;

-- name: ClearEstimatedPower :exec
UPDATE activity_records SET estimated_power_watts = NULL
WHERE target_slot = ? AND workout_id = ?;

-- name: SetEstimatedPower :exec
UPDATE activity_records SET estimated_power_watts = ?
WHERE target_slot = ? AND workout_id = ? AND record_index = ?;
