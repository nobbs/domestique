package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecordTaskRun records no provider text, no route name, and no upstream
// identifier: the detail is a stable category and the reference is random.
func (s *Store) RecordTaskRun(
	ctx context.Context,
	task, argument string,
	startedAt, finishedAt time.Time,
	outcome, detail, reference string,
	retain int,
) error {
	if task == "" || outcome == "" || reference == "" ||
		startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return errors.New("complete task run metadata is required")
	}
	if retain < 1 {
		return errors.New("a task run history must retain at least one attempt")
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("recording task run: %w", err)
	}
	defer rollback(transaction)

	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO task_runs (task, argument, started_at_unix, finished_at_unix, outcome, detail, reference)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, task, argument, startedAt.Unix(), finishedAt.Unix(), outcome, detail, reference); err != nil {
		return fmt.Errorf("recording task run: %w", err)
	}
	if err := pruneTaskRuns(ctx, transaction, task, retain); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing task run: %w", err)
	}

	return nil
}

// pruneTaskRuns drops everything past one task's retained window, in the
// caller's transaction. The most recent attempt per argument is kept
// regardless of age. Recency is (finished_at_unix, id): a refusal recorded
// off the caller's goroutine can commit after a later attempt's row, so id
// alone no longer tracks insertion order against real event time.
func pruneTaskRuns(ctx context.Context, transaction *sql.Tx, task string, retain int) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM task_runs
		WHERE task = ?
		  AND id NOT IN (
			SELECT id FROM task_runs WHERE task = ? ORDER BY finished_at_unix DESC, id DESC LIMIT ?
		  )
		  AND EXISTS (
			SELECT 1 FROM task_runs newer
			WHERE newer.task = task_runs.task AND newer.argument = task_runs.argument
			  AND (newer.finished_at_unix, newer.id) > (task_runs.finished_at_unix, task_runs.id)
		  )
	`, task, task, retain); err != nil {
		return fmt.Errorf("pruning task runs: %w", err)
	}

	return nil
}

// ForEachTaskRun visits one task's recorded attempts, most recent first. It is
// how a status surface reads back what a task has been doing.
func (s *Store) ForEachTaskRun(
	ctx context.Context,
	task string,
	visit func(argument string, startedAt, finishedAt time.Time, outcome, detail string) error,
) error {
	if task == "" || visit == nil {
		return errors.New("a task name and a run visitor are required")
	}

	rows, err := s.database.QueryContext(ctx, `
		SELECT argument, started_at_unix, finished_at_unix, outcome, detail
		FROM task_runs WHERE task = ? ORDER BY finished_at_unix DESC, id DESC
	`, task)
	if err != nil {
		return fmt.Errorf("reading task runs: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var argument, outcome, detail string
		var startedAt, finishedAt int64
		if err := rows.Scan(&argument, &startedAt, &finishedAt, &outcome, &detail); err != nil {
			return fmt.Errorf("reading a task run: %w", err)
		}
		if err := visit(
			argument, time.Unix(startedAt, 0).UTC(), time.Unix(finishedAt, 0).UTC(), outcome, detail,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading task runs: %w", err)
	}

	return nil
}

// LastTaskOutcome is what a success is compared against, to tell a routine one
// from the one that ends an incident.
func (s *Store) LastTaskOutcome(
	ctx context.Context, task, argument string,
) (outcome string, found bool, err error) {
	if task == "" {
		return "", false, errors.New("task is required")
	}
	if err := s.database.QueryRowContext(ctx, `
		SELECT outcome FROM task_runs WHERE task = ? AND argument = ?
		ORDER BY finished_at_unix DESC, id DESC LIMIT 1
	`, task, argument).Scan(&outcome); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("reading the last outcome of a task: %w", err)
	}

	return outcome, true, nil
}

// LastTaskSuccess is what staleness is measured against: a failed or skipped
// attempt leaves whatever the task keeps exactly as an earlier success left it.
func (s *Store) LastTaskSuccess(
	ctx context.Context, task, argument string,
) (finishedAt time.Time, found bool, err error) {
	if task == "" {
		return time.Time{}, false, errors.New("task is required")
	}
	var finishedUnix int64
	if err := s.database.QueryRowContext(ctx, `
		SELECT finished_at_unix FROM task_runs
		WHERE task = ? AND argument = ? AND outcome = ?
		ORDER BY finished_at_unix DESC, id DESC LIMIT 1
	`, task, argument, "succeeded").Scan(&finishedUnix); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}

		return time.Time{}, false, fmt.Errorf("reading the last success of a task: %w", err)
	}

	return time.Unix(finishedUnix, 0).UTC(), true, nil
}

// backoffScan bounds how far back a fault streak is counted. A task that has
// failed this many times running is as backed off as the cap allows, so reading
// further would change nothing.
const backoffScan = 64

// TaskFaultStreak counts consecutive faults at the tail of the history. A
// success ends the streak; anything else — a refusal, a run that found nothing
// to do — is passed over, because neither says the task is broken.
func (s *Store) TaskFaultStreak(
	ctx context.Context, task, argument string,
) (faults int, lastAt time.Time, err error) {
	if task == "" {
		return 0, time.Time{}, errors.New("task is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT outcome, finished_at_unix FROM task_runs
		WHERE task = ? AND argument = ?
		ORDER BY finished_at_unix DESC, id DESC LIMIT ?
	`, task, argument, backoffScan)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading a task's fault streak: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			outcome    string
			finishedAt int64
		)
		if err := rows.Scan(&outcome, &finishedAt); err != nil {
			return 0, time.Time{}, fmt.Errorf("reading a task's fault streak: %w", err)
		}
		if outcome == "succeeded" {
			break
		}
		if outcome != "failed" && outcome != "blocked" {
			continue
		}
		if faults == 0 {
			lastAt = time.Unix(finishedAt, 0).UTC()
		}
		faults++
	}
	if err := rows.Err(); err != nil {
		return 0, time.Time{}, fmt.Errorf("reading a task's fault streak: %w", err)
	}

	return faults, lastAt, nil
}
