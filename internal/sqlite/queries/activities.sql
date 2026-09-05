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

-- name: ListActivitiesBetween :many
SELECT workout_id, workout_type_id, workout_type_location_id, started_at_unix,
  distance_metres, moving_seconds, elapsed_seconds, ascent_metres
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
