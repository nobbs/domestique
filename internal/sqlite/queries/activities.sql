-- name: ListActivityIDs :many
SELECT workout_id FROM activities WHERE target_slot = ? ORDER BY workout_id;

-- name: UpsertActivity :exec
INSERT INTO activities (
  target_slot, workout_id, workout_type_id, workout_type_location_id, started_at_unix,
  distance_metres, moving_seconds, elapsed_seconds, ascent_metres, raw_summary_json, updated_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  workout_type_id = excluded.workout_type_id,
  workout_type_location_id = excluded.workout_type_location_id,
  started_at_unix = excluded.started_at_unix,
  distance_metres = excluded.distance_metres,
  moving_seconds = excluded.moving_seconds,
  elapsed_seconds = excluded.elapsed_seconds,
  ascent_metres = excluded.ascent_metres,
  raw_summary_json = excluded.raw_summary_json,
  updated_at_unix = excluded.updated_at_unix;
