package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecordTaskRun stores what one attempt came to, under the name a message about
// it can carry, and prunes that task's history back to its bound. It records no
// provider text, no route name, and no upstream identifier: the detail is a
// stable category and the reference is random.
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
// caller's transaction. The most recent attempt over each argument is kept
// whatever its age: it is what that argument last came to, and a status page
// reads it as such.
func pruneTaskRuns(ctx context.Context, transaction *sql.Tx, task string, retain int) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM task_runs
		WHERE task = ?
		  AND id NOT IN (SELECT id FROM task_runs WHERE task = ? ORDER BY id DESC LIMIT ?)
		  AND id NOT IN (SELECT MAX(id) FROM task_runs WHERE task = ? GROUP BY argument)
	`, task, task, retain, task); err != nil {
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
		FROM task_runs WHERE task = ? ORDER BY id DESC
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
