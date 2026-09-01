-- name: ListTrustedInventory :many
SELECT
  source_stages.provider,
  source_stages.route_id,
  source_stages.stage_order,
  source_stages.source_revision,
  source_stages.content_hash,
  stage_geometry.content_hash AS geometry_hash,
  stage_geometry.route_name,
  stage_geometry.stage_name,
  stage_geometry.coordinates
FROM source_stages
LEFT JOIN stage_geometry
  ON stage_geometry.provider = source_stages.provider
  AND stage_geometry.route_id = source_stages.route_id
  AND stage_geometry.stage_order = source_stages.stage_order
ORDER BY source_stages.provider, source_stages.route_id, source_stages.stage_order;

-- name: UpsertTargetStage :exec
INSERT INTO target_stages (
  target_slot, provider, route_id, stage_order, wahoo_route_id, content_hash, source_revision
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, provider, route_id, stage_order) DO UPDATE SET
  wahoo_route_id = excluded.wahoo_route_id,
  content_hash = excluded.content_hash,
  source_revision = excluded.source_revision;

-- name: UpsertStageSurface :exec
INSERT INTO stage_surface (
  provider, route_id, stage_order, content_hash, index_generation, ranges, matched_metres, updated_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET
  content_hash = excluded.content_hash,
  index_generation = excluded.index_generation,
  ranges = excluded.ranges,
  matched_metres = excluded.matched_metres,
  updated_at_unix = excluded.updated_at_unix;
