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

-- name: ListActivitiesAwaitingRecords :many
SELECT workout_id, raw_summary_json
FROM activities
WHERE target_slot = sqlc.arg(target_slot) AND records_state = 'pending'
ORDER BY started_at_unix, workout_id
LIMIT sqlc.arg(row_limit);

-- name: DeleteActivityRecords :exec
DELETE FROM activity_records WHERE target_slot = ? AND workout_id = ?;

-- name: MarkActivityRecordsStored :exec
UPDATE activities SET records_state = 'stored', fit_checksum_failed = sqlc.arg(fit_checksum_failed)
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id);

-- name: MarkActivityRecordsUnreadable :exec
UPDATE activities SET records_state = 'unreadable' WHERE target_slot = ? AND workout_id = ?;

-- name: ListActivityRides :many
SELECT started_at_unix, distance_metres, moving_seconds, ascent_metres
FROM activities
ORDER BY started_at_unix;

-- name: ListPendingActivityListings :many
SELECT workout_id, started_at_unix, workout_type_id, workout_type_location_id
FROM activity_listings
WHERE target_slot = ?
ORDER BY started_at_unix, workout_id;

-- name: DeleteActivityListings :exec
DELETE FROM activity_listings WHERE target_slot = ?;

-- name: InsertActivityListing :exec
INSERT INTO activity_listings (
  target_slot, workout_id, started_at_unix, workout_type_id, workout_type_location_id
) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  started_at_unix = excluded.started_at_unix,
  workout_type_id = excluded.workout_type_id,
  workout_type_location_id = excluded.workout_type_location_id;

-- name: DeleteActivityListing :exec
DELETE FROM activity_listings WHERE target_slot = ? AND workout_id = ?;
