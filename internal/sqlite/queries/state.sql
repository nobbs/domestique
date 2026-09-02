-- name: ListAlertToggles :many
SELECT task, scope, alert, enabled FROM alert_toggle ORDER BY task, scope, alert;

-- name: UpsertAlertToggle :exec
INSERT INTO alert_toggle (task, scope, alert, enabled, updated_at_unix)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (task, scope, alert) DO UPDATE SET
  enabled = excluded.enabled,
  updated_at_unix = excluded.updated_at_unix;

-- name: EnsureTargetOwner :exec
-- A self-service target's slot is its owning subject's own value, so this is
-- the one creation path: idempotent, safe on a rider's first "Connect" click
-- and every one after.
INSERT INTO targets (slot, owner_subject, authorization_state, updated_at_unix)
VALUES (?, ?, ?, ?)
ON CONFLICT(slot) DO NOTHING;

-- name: ListTargetStates :many
SELECT slot, authorization_state, COALESCE(owner_subject, '') AS owner_subject
FROM targets ORDER BY slot;

-- name: GetTarget :one
SELECT slot, COALESCE(wahoo_user_id, '') AS wahoo_user_id, authorization_state,
  COALESCE(owner_subject, '') AS owner_subject
FROM targets
WHERE slot = ?;

-- name: ListTaskSchedule :many
SELECT task, enabled FROM task_schedule;

-- name: UpsertTaskSchedule :exec
INSERT INTO task_schedule (task, enabled, updated_at_unix) VALUES (?, ?, ?)
ON CONFLICT(task) DO UPDATE SET enabled = excluded.enabled, updated_at_unix = excluded.updated_at_unix;

-- name: GetNotificationSentAt :one
SELECT last_sent_at_unix FROM notification_state WHERE kind = ?;

-- name: DeleteNotification :exec
DELETE FROM notification_state WHERE kind = ?;

-- name: UpsertNotification :exec
INSERT INTO notification_state (kind, last_sent_at_unix) VALUES (?, ?)
ON CONFLICT(kind) DO UPDATE SET last_sent_at_unix = excluded.last_sent_at_unix;

-- name: GetStageDurationFingerprint :one
SELECT content_hash, surface_generation, coefficient_fingerprint
FROM stage_duration WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: UpsertStageDuration :exec
INSERT INTO stage_duration (
  provider, route_id, stage_order, content_hash, surface_generation, coefficient_fingerprint,
  moving_seconds, cumulative_seconds, updated_at_unix
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET
  content_hash = excluded.content_hash,
  surface_generation = excluded.surface_generation,
  coefficient_fingerprint = excluded.coefficient_fingerprint,
  moving_seconds = excluded.moving_seconds,
  cumulative_seconds = excluded.cumulative_seconds,
  updated_at_unix = excluded.updated_at_unix;

-- name: DeleteStageDurationsWithDifferentFingerprint :exec
DELETE FROM stage_duration WHERE coefficient_fingerprint != ?;

-- name: PruneStageDuration :exec
DELETE FROM stage_duration
WHERE NOT EXISTS (
  SELECT 1 FROM stage_geometry
  WHERE stage_geometry.provider = stage_duration.provider
    AND stage_geometry.route_id = stage_duration.route_id
    AND stage_geometry.stage_order = stage_duration.stage_order
    AND stage_geometry.content_hash = stage_duration.content_hash
);

-- name: UpsertStageEnrichmentFailure :exec
INSERT INTO stage_enrichment_failure (provider, route_id, stage_order, pass, reason, failed_at_unix)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, route_id, stage_order, pass) DO UPDATE SET
  reason = excluded.reason,
  failed_at_unix = excluded.failed_at_unix;

-- name: ListStageEnrichmentFailures :many
SELECT provider, route_id, stage_order, pass, reason, failed_at_unix
FROM stage_enrichment_failure
ORDER BY provider, route_id, stage_order, pass;

-- name: CountStageEnrichmentFailures :one
SELECT COUNT(*) FROM (
  SELECT 1 FROM stage_enrichment_failure GROUP BY provider, route_id, stage_order
);

-- name: PruneStageEnrichmentFailures :exec
DELETE FROM stage_enrichment_failure
WHERE NOT EXISTS (
  SELECT 1 FROM stage_geometry
  WHERE stage_geometry.provider = stage_enrichment_failure.provider
    AND stage_geometry.route_id = stage_enrichment_failure.route_id
    AND stage_geometry.stage_order = stage_enrichment_failure.stage_order
);

-- name: DeleteStageEnrichmentFailuresByPass :exec
DELETE FROM stage_enrichment_failure WHERE pass = ?;

-- name: DeleteStageEnrichmentFailure :exec
DELETE FROM stage_enrichment_failure
WHERE provider = ? AND route_id = ? AND stage_order = ? AND pass = ?;

-- name: CountTrustedInventory :one
SELECT COUNT(*) FROM source_stages WHERE provider = ?;

-- name: DeleteSourceStagesByProvider :exec
DELETE FROM source_stages WHERE provider = ?;

-- name: InsertSourceStage :exec
INSERT INTO source_stages (provider, route_id, stage_order, source_revision, content_hash)
VALUES (?, ?, ?, ?, ?);

-- name: ListTargetStages :many
SELECT provider, route_id, stage_order, source_revision, content_hash, wahoo_route_id
FROM target_stages
WHERE target_slot = ?
ORDER BY provider, route_id, stage_order;

-- name: DeleteTargetStage :execresult
DELETE FROM target_stages
WHERE target_slot = ? AND provider = ? AND route_id = ? AND stage_order = ?;

-- name: GetStageSurface :one
SELECT ranges, matched_metres
FROM stage_surface
WHERE provider = ? AND route_id = ? AND stage_order = ? AND content_hash = ?;

-- name: GetStageSurfaceHash :one
SELECT content_hash, index_generation
FROM stage_surface WHERE provider = ? AND route_id = ? AND stage_order = ?;

-- name: PruneStageSurface :exec
DELETE FROM stage_surface
WHERE NOT EXISTS (
  SELECT 1 FROM stage_geometry
  WHERE stage_geometry.provider = stage_surface.provider
    AND stage_geometry.route_id = stage_surface.route_id
    AND stage_geometry.stage_order = stage_surface.stage_order
    AND stage_geometry.content_hash = stage_surface.content_hash
);

-- name: GetSurfaceCoverage :one
SELECT
  (SELECT COUNT(*) FROM source_stages) AS total,
  (SELECT COUNT(*)
    FROM stage_surface
    JOIN source_stages
      ON source_stages.provider = stage_surface.provider
      AND source_stages.route_id = stage_surface.route_id
      AND source_stages.stage_order = stage_surface.stage_order
    WHERE stage_surface.content_hash = source_stages.content_hash) AS classified;

-- name: GetSurfaceIndexBuild :one
SELECT built_at_unix, generation FROM surface_index WHERE id = 1;

-- name: UpdateSurfaceIndexBuild :exec
UPDATE surface_index SET built_at_unix = ?, generation = ? WHERE id = 1;
