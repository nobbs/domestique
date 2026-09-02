-- name: GetRuntimeSettings :one
SELECT allow_empty_source_deletion, stale_after_seconds, sync_initial_delay_seconds,
  notifications_enabled, pushover_base_url, surface_rebuild_interval_seconds,
  wahoo_api_base_url, wahoo_oauth_base_url, wahoo_client_id,
  ridemodel_coefficients_file, timezone
FROM runtime_settings
WHERE id = 1;

-- name: UpdateRuntimeSettings :exec
UPDATE runtime_settings
SET allow_empty_source_deletion = ?, stale_after_seconds = ?, sync_initial_delay_seconds = ?,
  notifications_enabled = ?, pushover_base_url = ?, surface_rebuild_interval_seconds = ?,
  wahoo_api_base_url = ?, wahoo_oauth_base_url = ?, wahoo_client_id = ?,
  ridemodel_coefficients_file = ?, timezone = ?, updated_at_unix = ?
WHERE id = 1;

-- name: DeleteRuntimeBasemaps :exec
DELETE FROM runtime_basemap;

-- name: InsertRuntimeBasemap :exec
INSERT INTO runtime_basemap (position, name, style_url, style_url_dark, dark_cartography)
VALUES (?, ?, ?, ?, ?);

-- name: DeleteRuntimeSurfaceRegions :exec
DELETE FROM runtime_surface_region;

-- name: InsertRuntimeSurfaceRegion :exec
INSERT INTO runtime_surface_region (position, region) VALUES (?, ?);

-- name: DeleteRuntimeSources :exec
DELETE FROM runtime_source;

-- name: InsertRuntimeSource :exec
INSERT INTO runtime_source (position, provider, base_url) VALUES (?, ?, ?);

-- name: ListRuntimeBasemaps :many
SELECT name, style_url, style_url_dark, dark_cartography
FROM runtime_basemap
ORDER BY position;

-- name: ListRuntimeSources :many
SELECT provider, base_url FROM runtime_source ORDER BY position;

-- name: ListRuntimeSecrets :many
SELECT name, value FROM runtime_secret;

-- name: DeleteRuntimeSecret :exec
DELETE FROM runtime_secret WHERE name = ?;

-- name: UpsertRuntimeSecret :exec
INSERT INTO runtime_secret (name, value, updated_at_unix)
VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET value = excluded.value,
  updated_at_unix = excluded.updated_at_unix;

-- name: ListRuntimeSurfaceRegions :many
SELECT region FROM runtime_surface_region ORDER BY position;
