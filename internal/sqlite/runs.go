package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// retainedSyncRuns bounds the recorded run history. An hourly deployment writes
// about fifty rows a day, so this holds a little over a week.
const retainedSyncRuns = 500

// syncRunReferenceBytes is how much randomness names a run. Twelve hex characters
// are readable aloud and leave the retained window nowhere near a collision.
const syncRunReferenceBytes = 6

// LastSyncRun returns the most recently recorded terminal run, if any.
//
//nolint:gocritic // The primitive callback boundary keeps httpapi independent of SQLite record types.
func (s *Store) LastSyncRun(ctx context.Context) (completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int, found bool, err error) {
	var completedUnix int64
	err = s.database.QueryRowContext(ctx, `
		SELECT finished_at_unix, outcome, COALESCE(detail, ''), source_stages, created, updated, deleted
		FROM sync_runs ORDER BY id DESC LIMIT 1
	`).Scan(&completedUnix, &outcome, &detail, &sourceStages, &created, &updated, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, "", "", 0, 0, 0, 0, false, nil
	}
	if err != nil {
		return time.Time{}, "", "", 0, 0, 0, 0, false, fmt.Errorf("reading last sync run: %w", err)
	}

	return time.Unix(completedUnix, 0).UTC(), outcome, detail, sourceStages, created, updated, deleted, true, nil
}

// ForEachPhaseRun visits the most recent recorded run of each phase. The phases
// run and fail independently, so a single most recent run answers half the
// question. Runs recorded before phases existed carry none and are left out.
func (s *Store) ForEachPhaseRun(
	ctx context.Context,
	visit func(phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error,
) error {
	if visit == nil {
		return errors.New("phase run visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT phase, finished_at_unix, outcome, COALESCE(detail, ''),
			source_stages, created, updated, deleted
		FROM sync_runs
		WHERE phase <> '' AND id IN (SELECT MAX(id) FROM sync_runs WHERE phase <> '' GROUP BY phase)
		ORDER BY phase
	`)
	if err != nil {
		return fmt.Errorf("reading the last run of each phase: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var phase, outcome, detail string
		var completedUnix int64
		var sourceStages, created, updated, deleted int
		if err := rows.Scan(
			&phase, &completedUnix, &outcome, &detail, &sourceStages, &created, &updated, &deleted,
		); err != nil {
			return fmt.Errorf("reading a phase run: %w", err)
		}
		if err := visit(
			phase, time.Unix(completedUnix, 0).UTC(), outcome, detail, sourceStages, created, updated, deleted,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading the runs of each phase: %w", err)
	}

	return nil
}

// ForEachSyncRun visits one page of the recorded history, newest first, and
// returns the cursor for the page after it — empty when the history ends here. A
// cursor this store did not issue is reported as unusable rather than as a
// failure. A cursor is a position rather than a name, so it still resolves after
// the run it was taken from has been pruned.
func (s *Store) ForEachSyncRun(
	ctx context.Context,
	after string,
	limit int,
	visit func(reference, phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error,
) (next string, usable bool, err error) {
	if visit == nil {
		return "", false, errors.New("sync run visitor is required")
	}
	if limit <= 0 {
		return "", false, errors.New("a positive page size is required")
	}
	position := int64(math.MaxInt64)
	if after != "" {
		cursor, parseErr := strconv.ParseInt(after, 10, 64)
		if parseErr != nil {
			return "", false, nil
		}
		issued, readErr := s.lastSyncRunID(ctx)
		if readErr != nil {
			return "", false, readErr
		}
		// Positions are handed out from one upwards and the highest only grows,
		// because pruning never drops the newest run. A cursor outside that range
		// is one this store never issued.
		if cursor <= 0 || cursor > issued {
			return "", false, nil
		}
		position = cursor
	}
	// One row past the page, so "is there more" is read rather than guessed.
	// Rows predating the phase split carry none and are excluded here rather than
	// in the caller, so the lookahead counts what the page will contain.
	rows, err := s.database.QueryContext(ctx, `
		SELECT id, reference, phase, finished_at_unix, outcome, COALESCE(detail, ''),
			source_stages, created, updated, deleted
		FROM sync_runs
		WHERE id < ? AND phase <> ''
		ORDER BY id DESC
		LIMIT ?
	`, position, limit+1)
	if err != nil {
		return "", false, fmt.Errorf("reading sync runs: %w", err)
	}
	defer closeRows(rows)

	visited := 0
	for rows.Next() {
		var id, completedUnix int64
		var reference, phase, outcome, detail string
		var sourceStages, created, updated, deleted int
		if err := rows.Scan(
			&id, &reference, &phase, &completedUnix, &outcome, &detail,
			&sourceStages, &created, &updated, &deleted,
		); err != nil {
			return "", false, fmt.Errorf("reading a sync run: %w", err)
		}
		if visited == limit {
			return next, true, nil
		}
		visited++
		next = strconv.FormatInt(id, 10)
		if err := visit(
			reference, phase, time.Unix(completedUnix, 0).UTC(), outcome, detail,
			sourceStages, created, updated, deleted,
		); err != nil {
			return "", false, err
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("reading sync runs: %w", err)
	}

	// The page was not filled, so nothing follows it.
	return "", true, nil
}

// lastSyncRunID reports the highest position the store has issued to a recorded
// run, or zero when it has recorded none.
func (s *Store) lastSyncRunID(ctx context.Context) (int64, error) {
	var highest int64
	if err := s.database.QueryRowContext(
		ctx, `SELECT COALESCE(MAX(id), 0) FROM sync_runs`,
	).Scan(&highest); err != nil {
		return 0, fmt.Errorf("reading sync runs: %w", err)
	}

	return highest, nil
}

// RecordSyncRun stores one terminal synchronization result and returns the
// reference naming it. Its detail is a stable failure category, never provider
// text or a route name. Recording also prunes the history back to its bound.
func (s *Store) RecordSyncRun(
	ctx context.Context,
	phase string,
	startedAt, finishedAt time.Time,
	outcome, detail string,
	sourceStages, created, updated, deleted int,
) (string, error) {
	if phase == "" || startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) || outcome == "" ||
		sourceStages < 0 || created < 0 || updated < 0 || deleted < 0 {
		return "", errors.New("complete non-negative sync run metadata is required")
	}
	reference, err := newSyncRunReference()
	if err != nil {
		return "", err
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("recording sync run: %w", err)
	}
	defer rollback(transaction)

	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO sync_runs (
			reference, phase, started_at_unix, finished_at_unix, outcome, detail,
			source_stages, created, updated, deleted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, reference, phase, startedAt.Unix(), finishedAt.Unix(), outcome, detail,
		sourceStages, created, updated, deleted); err != nil {
		return "", fmt.Errorf("recording sync run: %w", err)
	}
	if err := pruneSyncRuns(ctx, transaction); err != nil {
		return "", err
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("committing sync run: %w", err)
	}

	return reference, nil
}

// newSyncRunReference mints the name one run is known by. Six random bytes are
// short enough to read back off a phone and far enough apart that the retained
// history will not hold two the same.
func newSyncRunReference() (string, error) {
	reference := make([]byte, syncRunReferenceBytes)
	if _, err := io.ReadFull(rand.Reader, reference); err != nil {
		return "", fmt.Errorf("naming sync run: %w", err)
	}

	return hex.EncodeToString(reference), nil
}

// pruneSyncRuns drops everything past the retained window, in the caller's
// transaction. The most recent run of each phase is kept whatever its age: the
// status response reads it as what that half last came to.
func pruneSyncRuns(ctx context.Context, transaction *sql.Tx) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM sync_runs
		WHERE id NOT IN (SELECT id FROM sync_runs ORDER BY id DESC LIMIT ?)
		  AND id NOT IN (SELECT MAX(id) FROM sync_runs GROUP BY phase)
	`, retainedSyncRuns); err != nil {
		return fmt.Errorf("pruning sync runs: %w", err)
	}

	return nil
}

// RecordTargetRun stores the terminal result of one target's reconciliation,
// replacing whatever that slot recorded before. Its detail is a stable failure
// category, never provider text, a route name, or a Wahoo identifier.
func (s *Store) RecordTargetRun(
	ctx context.Context,
	targetID string,
	finishedAt time.Time,
	outcome, detail string,
) error {
	if strings.TrimSpace(targetID) == "" || finishedAt.IsZero() || outcome == "" {
		return errors.New("target ID, finish time, and outcome are required")
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO target_runs (target_slot, finished_at_unix, outcome, detail)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(target_slot) DO UPDATE SET
			finished_at_unix = excluded.finished_at_unix,
			outcome = excluded.outcome,
			detail = excluded.detail
	`, targetID, finishedAt.Unix(), outcome, detail); err != nil {
		return fmt.Errorf("recording target run: %w", err)
	}

	return nil
}

// ForEachTargetRun visits the last recorded reconciliation of each target in
// stable slot order. A slot that has never been reconciled is not visited, which
// is how a reader tells "never attempted" from "attempted and failed".
func (s *Store) ForEachTargetRun(
	ctx context.Context,
	visit func(targetID string, finishedAt time.Time, outcome, detail string) error,
) error {
	if visit == nil {
		return errors.New("target run visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT target_slot, finished_at_unix, outcome, detail
		FROM target_runs ORDER BY target_slot
	`)
	if err != nil {
		return fmt.Errorf("listing target runs: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var targetID, outcome, detail string
		var finishedUnix int64
		if err := rows.Scan(&targetID, &finishedUnix, &outcome, &detail); err != nil {
			return fmt.Errorf("reading a target run: %w", err)
		}
		if err := visit(targetID, time.Unix(finishedUnix, 0).UTC(), outcome, detail); err != nil {
			return fmt.Errorf("visiting a target run: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating target runs: %w", err)
	}

	return nil
}

// LastPhaseOutcome returns the outcome of the most recent run recorded for one
// phase.
//
// Runs recorded before phases existed carry no phase and are not attributed to
// one, which is the same rule ForEachPhaseRun applies.
func (s *Store) LastPhaseOutcome(ctx context.Context, phase string) (outcome string, found bool, err error) {
	if phase == "" {
		return "", false, errors.New("phase is required")
	}
	if err := s.database.QueryRowContext(ctx, `
		SELECT outcome FROM sync_runs WHERE phase = ? ORDER BY id DESC LIMIT 1
	`, phase).Scan(&outcome); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("reading the last outcome of a phase: %w", err)
	}

	return outcome, true, nil
}

// LastSuccessfulPhaseCompletion returns when a phase last recorded a success,
// which is what its trusted inventory age is measured against: a failed or
// skipped run leaves that inventory exactly as an earlier success left it.
func (s *Store) LastSuccessfulPhaseCompletion(ctx context.Context, phase string) (completedAt time.Time, found bool, err error) {
	if phase == "" {
		return time.Time{}, false, errors.New("phase is required")
	}
	var completedUnix int64
	if err := s.database.QueryRowContext(ctx, `
		SELECT finished_at_unix FROM sync_runs WHERE phase = ? AND outcome = ? ORDER BY id DESC LIMIT 1
	`, phase, "succeeded").Scan(&completedUnix); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}

		return time.Time{}, false, fmt.Errorf("reading the last successful completion of a phase: %w", err)
	}

	return time.Unix(completedUnix, 0).UTC(), true, nil
}

// ForEachSuccessfulRunAfter visits the successful runs recorded after the given
// run, oldest first, carrying each id so the caller can move its window. It
// selects the counts a digest totals and not the detail column.
func (s *Store) ForEachSuccessfulRunAfter(
	ctx context.Context,
	runID int64,
	visit func(id int64, phase string, created, updated, deleted int) error,
) error {
	if visit == nil {
		return errors.New("successful run visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT id, phase, created, updated, deleted
		FROM sync_runs
		WHERE phase <> '' AND outcome = ? AND id > ?
		ORDER BY id
	`, "succeeded", runID)
	if err != nil {
		return fmt.Errorf("reading successful runs: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var id int64
		var phase string
		var created, updated, deleted int
		if err := rows.Scan(&id, &phase, &created, &updated, &deleted); err != nil {
			return fmt.Errorf("reading a successful run: %w", err)
		}
		if err := visit(id, phase, created, updated, deleted); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading the successful runs: %w", err)
	}

	return nil
}
