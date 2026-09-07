-- name: ListActivitiesAwaitingRouteMatch :many
-- Rides whose samples are stored and whose match was never worked out, or was
-- worked out against a library that has since changed.
SELECT a.workout_id
FROM activities AS a
LEFT JOIN activity_route_match AS m ON m.target_slot = a.target_slot AND m.workout_id = a.workout_id
WHERE a.target_slot = sqlc.arg(target_slot)
  AND a.records_state = 'stored'
  AND (m.workout_id IS NULL OR m.library_hash <> sqlc.arg(library_hash))
ORDER BY a.started_at_unix DESC, a.workout_id DESC;

-- name: UpsertActivityRouteMatch :exec
INSERT INTO activity_route_match (
  target_slot, workout_id, provider, route_id, stage_order,
  route_coverage, ride_coverage, direction, library_hash, matched_at_unix
) VALUES (
  sqlc.arg(target_slot), sqlc.arg(workout_id), sqlc.narg(provider), sqlc.narg(route_id), sqlc.narg(stage_order),
  sqlc.narg(route_coverage), sqlc.narg(ride_coverage), sqlc.narg(direction),
  sqlc.arg(library_hash), sqlc.arg(matched_at_unix)
)
ON CONFLICT (target_slot, workout_id) DO UPDATE SET
  provider = excluded.provider,
  route_id = excluded.route_id,
  stage_order = excluded.stage_order,
  route_coverage = excluded.route_coverage,
  ride_coverage = excluded.ride_coverage,
  direction = excluded.direction,
  library_hash = excluded.library_hash,
  matched_at_unix = excluded.matched_at_unix;

-- name: DeleteActivityRouteMatch :exec
DELETE FROM activity_route_match
WHERE target_slot = sqlc.arg(target_slot) AND workout_id = sqlc.arg(workout_id);

-- name: ListActivityRouteMatches :many
-- Every match one target holds, for the ride list and the ride page.
SELECT workout_id, provider, route_id, stage_order, route_coverage, ride_coverage, direction
FROM activity_route_match
WHERE target_slot = sqlc.arg(target_slot) AND provider IS NOT NULL;

-- name: ListRouteActivities :many
-- The rides one target rode on one route, newest first.
SELECT m.workout_id, m.route_coverage, m.ride_coverage, m.direction
FROM activity_route_match AS m
JOIN activities AS a ON a.target_slot = m.target_slot AND a.workout_id = m.workout_id
WHERE m.target_slot = sqlc.arg(target_slot)
  AND m.provider = sqlc.arg(provider)
  AND m.route_id = sqlc.arg(route_id)
  AND m.stage_order = sqlc.arg(stage_order)
ORDER BY a.started_at_unix DESC, m.workout_id DESC;

-- name: ListLibraryStageGeometry :many
-- Every route a ride may be matched to, with the hash that says whether its
-- line has moved since a match was worked out against it.
SELECT provider, route_id, stage_order, content_hash, coordinates
FROM stage_geometry
ORDER BY provider, route_id, stage_order;
