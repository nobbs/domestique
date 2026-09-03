-- name: ListSourceStages :many
SELECT provider, route_id, stage_order, source_revision, content_hash
FROM source_stages ORDER BY provider, route_id, stage_order;

-- name: ListStageSummaries :many
SELECT
  source_stages.provider,
  source_stages.route_id,
  source_stages.stage_order,
  source_stages.source_revision,
  source_stages.content_hash,
  COALESCE(stage_geometry.route_name, '') AS source_route_name,
  COALESCE(stage_geometry.stage_name, '') AS route_name,
  COALESCE(stage_geometry.point_count, 0) AS point_count,
  COALESCE(stage_geometry.distance_metres, 0) AS distance_metres,
  COALESCE(stage_geometry.ascent_metres, 0) AS ascent_metres,
  COALESCE(stage_geometry.descent_metres, 0) AS descent_metres,
  COALESCE(stage_geometry.max_gradient_percent, 0) AS max_gradient_percent,
  stage_duration.moving_seconds,
  COALESCE(stage_geometry.min_longitude, 0) AS min_longitude,
  COALESCE(stage_geometry.min_latitude, 0) AS min_latitude,
  COALESCE(stage_geometry.max_longitude, 0) AS max_longitude,
  COALESCE(stage_geometry.max_latitude, 0) AS max_latitude
FROM source_stages
LEFT JOIN stage_geometry
  ON stage_geometry.provider = source_stages.provider
  AND stage_geometry.route_id = source_stages.route_id
  AND stage_geometry.stage_order = source_stages.stage_order
LEFT JOIN stage_duration
  ON stage_duration.provider = source_stages.provider
  AND stage_duration.route_id = source_stages.route_id
  AND stage_duration.stage_order = source_stages.stage_order
  AND stage_duration.content_hash = source_stages.content_hash
ORDER BY source_stages.provider, source_stages.route_id, source_stages.stage_order;

-- name: GetStageGeometry :one
SELECT
  stage_geometry.provider,
  stage_geometry.route_id,
  stage_geometry.stage_order,
  COALESCE(source_stages.source_revision, '') AS source_revision,
  stage_geometry.content_hash,
  stage_geometry.route_name AS source_route_name,
  stage_geometry.stage_name AS route_name,
  stage_geometry.point_count,
  stage_geometry.distance_metres,
  stage_geometry.ascent_metres,
  stage_geometry.descent_metres,
  stage_geometry.max_gradient_percent,
  stage_duration.moving_seconds,
  stage_geometry.min_longitude,
  stage_geometry.min_latitude,
  stage_geometry.max_longitude,
  stage_geometry.max_latitude,
  stage_geometry.coordinates,
  stage_duration.cumulative_seconds
FROM stage_geometry
LEFT JOIN source_stages
  ON source_stages.provider = stage_geometry.provider
  AND source_stages.route_id = stage_geometry.route_id
  AND source_stages.stage_order = stage_geometry.stage_order
LEFT JOIN stage_duration
  ON stage_duration.provider = stage_geometry.provider
  AND stage_duration.route_id = stage_geometry.route_id
  AND stage_duration.stage_order = stage_geometry.stage_order
  AND stage_duration.content_hash = stage_geometry.content_hash
WHERE stage_geometry.provider = ? AND stage_geometry.route_id = ? AND stage_geometry.stage_order = ?;

-- name: GetSourceStageExists :one
SELECT 1 FROM source_stages WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: UpsertStageReprocess :exec
INSERT INTO stage_reprocess (provider, route_id, stage_order, requested_at_unix)
VALUES (?, ?, ?, ?)
ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET requested_at_unix = excluded.requested_at_unix;

-- name: MarkTargetStagesForReprocess :exec
UPDATE target_stages SET source_revision = ?, content_hash = ?
WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: DeleteStageSurface :exec
DELETE FROM stage_surface WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: DeleteStageDuration :exec
DELETE FROM stage_duration WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: GetStageGeometryHash :one
SELECT content_hash FROM stage_geometry WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: UpsertStageGeometry :exec
INSERT INTO stage_geometry (
  provider, route_id, stage_order, content_hash, route_name, stage_name,
  point_count, distance_metres, ascent_metres, descent_metres, max_gradient_percent,
  min_longitude, min_latitude, max_longitude, max_latitude,
  coordinates, updated_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET
  content_hash = excluded.content_hash,
  route_name = excluded.route_name,
  stage_name = excluded.stage_name,
  point_count = excluded.point_count,
  distance_metres = excluded.distance_metres,
  ascent_metres = excluded.ascent_metres,
  descent_metres = excluded.descent_metres,
  max_gradient_percent = excluded.max_gradient_percent,
  min_longitude = excluded.min_longitude,
  min_latitude = excluded.min_latitude,
  max_longitude = excluded.max_longitude,
  max_latitude = excluded.max_latitude,
  coordinates = excluded.coordinates,
  updated_at_unix = excluded.updated_at_unix;

-- name: PruneStageGeometry :exec
DELETE FROM stage_geometry
WHERE NOT EXISTS (
  SELECT 1 FROM source_stages
  WHERE source_stages.provider = stage_geometry.provider
    AND source_stages.route_id = stage_geometry.route_id
    AND source_stages.stage_order = stage_geometry.stage_order
);

-- name: DeleteStageReprocessByProvider :exec
DELETE FROM stage_reprocess WHERE provider = ?;

-- name: ListStageReprocess :many
SELECT provider, route_id, stage_order FROM stage_reprocess;
