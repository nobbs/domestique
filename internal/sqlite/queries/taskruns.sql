-- name: InsertTaskRun :exec
INSERT INTO task_runs (task, argument, trigger, started_at_unix, finished_at_unix, outcome, detail, reference)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: PruneTaskRuns :exec
DELETE FROM task_runs
WHERE task_runs.task = sqlc.arg(task_name)
  AND id NOT IN (
    SELECT id FROM task_runs
    WHERE task_runs.task = sqlc.arg(task_name)
    ORDER BY finished_at_unix DESC, id DESC
    LIMIT sqlc.arg(retain)
  )
  AND EXISTS (
    SELECT 1 FROM task_runs newer
    WHERE newer.task = task_runs.task AND newer.argument = task_runs.argument
      AND (newer.finished_at_unix, newer.id) > (task_runs.finished_at_unix, task_runs.id)
  );

-- name: ListTaskRuns :many
SELECT argument, started_at_unix, finished_at_unix, outcome, detail
FROM task_runs WHERE task = ? ORDER BY finished_at_unix DESC, id DESC;

-- name: ListTaskRunsPage :many
SELECT id, task, argument, trigger, started_at_unix, finished_at_unix, outcome, detail, reference
FROM task_runs
WHERE (finished_at_unix, id) < (sqlc.arg(finished_before), sqlc.arg(id_before))
  AND (CAST(sqlc.arg(task_filter) AS TEXT) = '' OR task = sqlc.arg(task_filter))
ORDER BY finished_at_unix DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetLastTaskRunPosition :one
SELECT CAST(COALESCE(MAX(finished_at_unix), 0) AS INTEGER) AS finished_at_unix,
  CAST(COALESCE(MAX(id), 0) AS INTEGER) AS id
FROM task_runs;

-- name: GetLastTaskOutcome :one
SELECT outcome FROM task_runs WHERE task = ? AND argument = ?
ORDER BY finished_at_unix DESC, id DESC LIMIT 1;

-- name: GetLastTaskSuccess :one
SELECT finished_at_unix FROM task_runs
WHERE task = ? AND argument = ? AND outcome = ?
ORDER BY finished_at_unix DESC, id DESC LIMIT 1;

-- name: ListTaskOutcomesForFaultStreak :many
SELECT outcome, finished_at_unix FROM task_runs
WHERE task = ? AND argument = ?
ORDER BY finished_at_unix DESC, id DESC LIMIT ?;
