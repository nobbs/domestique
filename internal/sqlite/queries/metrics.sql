-- name: UpsertActivityMetrics :exec
INSERT INTO activity_metrics (
  target_slot, workout_id,
  zone_1_seconds, zone_2_seconds, zone_3_seconds, zone_4_seconds, zone_5_seconds,
  trimp, heart_rate_tss, normalized_power_watts, intensity_factor, power_tss,
  estimated_power_watts,
  average_heart_rate_bpm, max_heart_rate_bpm, average_cadence_rpm, average_power_watts,
  input_max_heart_rate, input_resting_heart_rate, input_threshold_heart_rate, input_threshold_power,
  input_total_mass, derivation_version, computed_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
  average_heart_rate_bpm = excluded.average_heart_rate_bpm,
  max_heart_rate_bpm = excluded.max_heart_rate_bpm,
  average_cadence_rpm = excluded.average_cadence_rpm,
  average_power_watts = excluded.average_power_watts,
  input_max_heart_rate = excluded.input_max_heart_rate,
  input_resting_heart_rate = excluded.input_resting_heart_rate,
  input_threshold_heart_rate = excluded.input_threshold_heart_rate,
  input_threshold_power = excluded.input_threshold_power,
  input_total_mass = excluded.input_total_mass,
  derivation_version = excluded.derivation_version,
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
  estimated_power_watts,
  average_heart_rate_bpm, max_heart_rate_bpm, average_cadence_rpm, average_power_watts
FROM activity_metrics
WHERE target_slot = ?
ORDER BY workout_id;

-- Rides whose stored samples could still yield something this derivation now
-- allows: those with no metrics row at all, those whose row was worked out
-- against different profile values, and those whose row an earlier derivation
-- wrote and so cannot hold every figure this one produces. A ride still
-- awaiting its FIT has nothing to derive from and is left for the download to
-- bring in.
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
    OR m.input_total_mass <> sqlc.arg(total_mass)
    OR m.derivation_version <> sqlc.arg(derivation_version))
ORDER BY a.started_at_unix DESC, a.workout_id DESC;

-- Every record a derivation can do something with: one carrying a sensor, or
-- one carrying a whole track sample. A record that is neither is skipped here
-- rather than scanned and discarded in Go. The track test is latitude and
-- longitude together, which is what ListActivityTrack calls a positioned
-- sample: a record the track would not serve must not shape an estimate.
-- name: ListActivitySensorRecords :many
SELECT record_index, recorded_at_unix, heart_rate_bpm, cadence_rpm, power_watts,
  distance_metres, altitude_metres, latitude, longitude
FROM activity_records
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id)
  AND (heart_rate_bpm IS NOT NULL
    OR cadence_rpm IS NOT NULL
    OR power_watts IS NOT NULL
    OR (latitude IS NOT NULL AND longitude IS NOT NULL
      AND altitude_metres IS NOT NULL AND distance_metres IS NOT NULL))
ORDER BY record_index;

-- name: GetTargetOwner :one
SELECT COALESCE(owner_subject, '') AS owner_subject FROM targets WHERE slot = ?;

-- name: ClearEstimatedPower :exec
UPDATE activity_records SET estimated_power_watts = NULL
WHERE target_slot = ? AND workout_id = ?;

-- Every derived ride of one target with the day it was ridden, which the
-- fitness timeline is a fold over. Ordered so a fold reads it once.
-- name: ListActivityRideLoads :many
SELECT a.started_at_unix,
  m.trimp, m.heart_rate_tss, m.power_tss,
  m.zone_1_seconds, m.zone_2_seconds, m.zone_3_seconds, m.zone_4_seconds, m.zone_5_seconds
FROM activity_metrics AS m
JOIN activities AS a ON a.target_slot = m.target_slot AND a.workout_id = m.workout_id
WHERE m.target_slot = ?
ORDER BY a.started_at_unix;
