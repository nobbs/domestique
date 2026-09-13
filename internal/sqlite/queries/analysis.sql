-- The no-op update makes RETURNING answer with the instant already standing.
-- name: RecordAnalysisEnabled :one
INSERT INTO analysis_state (id, enabled_since_unix) VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET enabled_since_unix = analysis_state.enabled_since_unix
RETURNING enabled_since_unix;

-- Derived rides with no analysis that started at or after the instant the
-- analysis was enabled, oldest first so each answer can read the ones before it.
-- A head unit's ride of a held type that ended at or after held_since waits
-- for the Zwift copy that may replace it.
-- name: ListActivitiesAwaitingAnalysis :many
SELECT a.workout_id, a.started_at_unix
FROM activities AS a
JOIN activity_metrics AS m ON m.target_slot = a.target_slot AND m.workout_id = a.workout_id
LEFT JOIN activity_analyses AS x ON x.target_slot = a.target_slot AND x.workout_id = a.workout_id
WHERE a.target_slot = sqlc.arg(target_slot)
  AND a.started_at_unix >= sqlc.arg(enabled_since_unix)
  AND x.workout_id IS NULL
  AND NOT (a.provider = 'wahoo'
    AND a.started_at_unix + CAST(a.elapsed_seconds AS INTEGER) >= sqlc.arg(held_since_unix)
    AND a.workout_type_id IN (SELECT value FROM json_each(CAST(sqlc.arg(held_type_ids) AS TEXT))))
ORDER BY a.started_at_unix, a.workout_id
LIMIT sqlc.arg(row_limit);

-- name: UpsertActivityAnalysis :exec
INSERT INTO activity_analyses (target_slot, workout_id, text, model, prompt_revision, analysed_at_unix)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(target_slot, workout_id) DO UPDATE SET
  text = excluded.text,
  model = excluded.model,
  prompt_revision = excluded.prompt_revision,
  analysed_at_unix = excluded.analysed_at_unix;

-- The analyses of rides that started before one, newest first.
-- name: ListAnalysesBefore :many
SELECT x.text, x.model, x.prompt_revision, x.analysed_at_unix, a.started_at_unix
FROM activity_analyses AS x
JOIN activities AS a ON a.target_slot = x.target_slot AND a.workout_id = x.workout_id
WHERE x.target_slot = sqlc.arg(target_slot)
  AND a.started_at_unix < sqlc.arg(started_before_unix)
ORDER BY a.started_at_unix DESC, a.workout_id DESC
LIMIT sqlc.arg(row_limit);

-- name: DeleteActivityAnalysis :exec
DELETE FROM activity_analyses WHERE target_slot = ? AND workout_id = ?;

-- name: ClearActivityAnalyses :exec
DELETE FROM activity_analyses WHERE target_slot = ?;
