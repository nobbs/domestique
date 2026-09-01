-- name: GetLastSyncRun :one
SELECT finished_at_unix, outcome, COALESCE(detail, '') AS detail,
  source_stages, created, updated, deleted
FROM sync_runs ORDER BY id DESC LIMIT 1;

-- name: ListLastPhaseRuns :many
SELECT phase, finished_at_unix, outcome, COALESCE(detail, '') AS detail,
  source_stages, created, updated, deleted
FROM sync_runs
WHERE phase <> '' AND id IN (SELECT MAX(id) FROM sync_runs WHERE phase <> '' GROUP BY phase)
ORDER BY phase;

-- name: ListSyncRunsPage :many
SELECT id, reference, phase, finished_at_unix, outcome, COALESCE(detail, '') AS detail,
  source_stages, created, updated, deleted
FROM sync_runs
WHERE id < ? AND phase <> ''
ORDER BY id DESC
LIMIT ?;

-- name: GetLastSyncRunID :one
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) AS id FROM sync_runs;

-- name: InsertSyncRun :exec
INSERT INTO sync_runs (
  reference, phase, started_at_unix, finished_at_unix, outcome, detail,
  source_stages, created, updated, deleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: PruneSyncRuns :exec
DELETE FROM sync_runs
WHERE id NOT IN (SELECT id FROM sync_runs ORDER BY id DESC LIMIT ?)
  AND id NOT IN (SELECT MAX(id) FROM sync_runs GROUP BY phase);

-- name: UpsertTargetRun :exec
INSERT INTO target_runs (target_slot, finished_at_unix, outcome, detail)
VALUES (?, ?, ?, ?)
ON CONFLICT(target_slot) DO UPDATE SET
  finished_at_unix = excluded.finished_at_unix,
  outcome = excluded.outcome,
  detail = excluded.detail;

-- name: ListTargetRuns :many
SELECT target_slot, finished_at_unix, outcome, detail
FROM target_runs ORDER BY target_slot;

-- name: GetLastSuccessfulPhaseCompletion :one
SELECT finished_at_unix FROM sync_runs
WHERE phase = ? AND outcome = ?
ORDER BY id DESC LIMIT 1;
