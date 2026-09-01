package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// RecordTaskRun records no provider text, no route name, and no upstream
// identifier: the detail is a stable category and the reference is random.
func (s *Store) RecordTaskRun(
	ctx context.Context,
	task, argument, trigger string,
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

	queries := s.queries.WithTx(transaction)
	if err := queries.InsertTaskRun(ctx, sqlcgen.InsertTaskRunParams{
		Task: task, Argument: argument, Trigger: trigger, StartedAtUnix: startedAt.Unix(),
		FinishedAtUnix: finishedAt.Unix(), Outcome: outcome, Detail: detail, Reference: reference,
	}); err != nil {
		return fmt.Errorf("recording task run: %w", err)
	}
	if err := pruneTaskRuns(ctx, queries, task, retain); err != nil {
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
func pruneTaskRuns(ctx context.Context, queries *sqlcgen.Queries, task string, retain int) error {
	if err := queries.PruneTaskRuns(ctx, sqlcgen.PruneTaskRunsParams{
		TaskName: task, Retain: int64(retain),
	}); err != nil {
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

	rows, err := s.queries.ListTaskRuns(ctx, task)
	if err != nil {
		return fmt.Errorf("reading task runs: %w", err)
	}
	for _, row := range rows {
		if err := visit(
			row.Argument, time.Unix(row.StartedAtUnix, 0).UTC(), time.Unix(row.FinishedAtUnix, 0).UTC(),
			row.Outcome, row.Detail,
		); err != nil {
			return err
		}
	}

	return nil
}

// ForEachTaskRunPage visits one page of recorded attempts, newest first, and
// returns the cursor for the page after it — empty when the history ends here.
// An empty task is every task. A cursor outside the range of positions this
// store has issued is reported as unusable rather than as a failure; one inside
// that range resolves as a position, which lets it outlive its own row's
// pruning — a position no attempt ever held reads as a history that ends there.
func (s *Store) ForEachTaskRunPage(
	ctx context.Context,
	task, after string,
	limit int,
	visit func(task, argument, trigger string, startedAt, finishedAt time.Time, outcome, detail, reference string) error,
) (next string, usable bool, err error) {
	if visit == nil {
		return "", false, errors.New("task run visitor is required")
	}
	if limit <= 0 {
		return "", false, errors.New("a positive page size is required")
	}
	finishedBefore, idBefore := int64(math.MaxInt64), int64(math.MaxInt64)
	if after != "" {
		finishedAt, id, ok := parseTaskRunCursor(after)
		if !ok {
			return "", false, nil
		}
		latestFinished, issued, readErr := s.lastTaskRun(ctx)
		if readErr != nil {
			return "", false, readErr
		}
		// Bounded against the impossible at both ends: an instant ahead of
		// everything recorded, or at or before the epoch, was never a finish.
		if id <= 0 || id > issued || finishedAt <= 0 || finishedAt > latestFinished {
			return "", false, nil
		}
		finishedBefore, idBefore = finishedAt, id
	}
	// One row past the page, so "is there more" is read rather than guessed.
	rows, err := s.queries.ListTaskRunsPage(ctx, sqlcgen.ListTaskRunsPageParams{
		FinishedBefore: finishedBefore, IDBefore: idBefore, TaskFilter: task, RowLimit: int64(limit + 1),
	})
	if err != nil {
		return "", false, fmt.Errorf("reading task runs: %w", err)
	}
	visited := 0
	for _, row := range rows {
		if visited == limit {
			return next, true, nil
		}
		visited++
		next = formatTaskRunCursor(row.FinishedAtUnix, row.ID)
		if err := visit(
			row.Task, row.Argument, row.Trigger,
			time.Unix(row.StartedAtUnix, 0).UTC(), time.Unix(row.FinishedAtUnix, 0).UTC(),
			row.Outcome, row.Detail, row.Reference,
		); err != nil {
			return "", false, err
		}
	}
	// The page was not filled, so nothing follows it.
	return "", true, nil
}

// formatTaskRunCursor names a position in the history. Both halves of the
// recency tuple travel, because a refusal can commit its row after a later
// attempt's and an id alone would then skip it.
func formatTaskRunCursor(finishedUnix, id int64) string {
	return strconv.FormatInt(finishedUnix, 10) + ":" + strconv.FormatInt(id, 10)
}

// parseTaskRunCursor reads a position back, reporting a malformed one as
// unusable rather than as a failure: it is the caller's input.
func parseTaskRunCursor(cursor string) (finishedUnix, id int64, ok bool) {
	finishedText, idText, found := strings.Cut(cursor, ":")
	if !found {
		return 0, 0, false
	}
	finishedUnix, err := strconv.ParseInt(finishedText, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	id, err = strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return finishedUnix, id, true
}

// lastTaskRun reports the furthest position the store has issued a cursor for:
// the latest instant it has recorded and the highest id. Both are zero when it
// has recorded nothing. Neither half falls, so a position beyond either is one
// this store never handed out — a pruned row still sits within both.
func (s *Store) lastTaskRun(ctx context.Context) (finishedUnix, id int64, err error) {
	row, err := s.queries.GetLastTaskRunPosition(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("reading task runs: %w", err)
	}
	return row.FinishedAtUnix, row.ID, nil
}

// LastTaskOutcome is what a success is compared against, to tell a routine one
// from the one that ends an incident.
func (s *Store) LastTaskOutcome(
	ctx context.Context, task, argument string,
) (outcome string, found bool, err error) {
	if task == "" {
		return "", false, errors.New("task is required")
	}
	outcome, err = s.queries.GetLastTaskOutcome(ctx, sqlcgen.GetLastTaskOutcomeParams{
		Task: task, Argument: argument,
	})
	if err != nil {
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
	finishedUnix, err := s.queries.GetLastTaskSuccess(ctx, sqlcgen.GetLastTaskSuccessParams{
		Task: task, Argument: argument, Outcome: "succeeded",
	})
	if err != nil {
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
	rows, err := s.queries.ListTaskOutcomesForFaultStreak(ctx, sqlcgen.ListTaskOutcomesForFaultStreakParams{
		Task: task, Argument: argument, Limit: backoffScan,
	})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading a task's fault streak: %w", err)
	}
	for _, row := range rows {
		if row.Outcome == "succeeded" {
			break
		}
		if row.Outcome != "failed" && row.Outcome != "blocked" {
			continue
		}
		if faults == 0 {
			lastAt = time.Unix(row.FinishedAtUnix, 0).UTC()
		}
		faults++
	}
	return faults, lastAt, nil
}
