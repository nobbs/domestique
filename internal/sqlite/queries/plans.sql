-- name: InsertPlan :exec
INSERT INTO plans (
  id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  pushing, turns, cues, straight, avoid, published, version, created_at_unix_nano, updated_at_unix_nano
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPlan :one
SELECT id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano, pushing, turns, cues, straight, avoid
FROM plans WHERE id = ?;

-- name: ListPlans :many
SELECT id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano, pushing, turns, cues, straight, avoid
FROM plans ORDER BY id;

-- name: ListPublishedPlans :many
SELECT id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano, pushing, turns, cues, straight, avoid
FROM plans WHERE published = 1 ORDER BY id;

-- name: UpdatePlan :execrows
UPDATE plans SET
  name = ?, profile = ?, waypoints = ?, coordinates = ?, distance_metres = ?,
  ascent_metres = ?, pushing = ?, turns = ?, cues = ?, straight = ?, avoid = ?, published = ?, version = ?, updated_at_unix_nano = ?
WHERE id = ? AND version = ?;

-- name: DeletePlan :execrows
DELETE FROM plans WHERE id = ? AND version = ?;
