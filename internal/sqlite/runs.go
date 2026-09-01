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

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
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
	row, err := s.queries.GetLastSyncRun(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, "", "", 0, 0, 0, 0, false, nil
	}
	if err != nil {
		return time.Time{}, "", "", 0, 0, 0, 0, false, fmt.Errorf("reading last sync run: %w", err)
	}
	if !row.FinishedAtUnix.Valid {
		return time.Time{}, "", "", 0, 0, 0, 0, false, errors.New("reading last sync run: finish time is null")
	}

	return time.Unix(row.FinishedAtUnix.Int64, 0).UTC(), row.Outcome, row.Detail,
		int(row.SourceStages), int(row.Created), int(row.Updated), int(row.Deleted), true, nil
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
	rows, err := s.queries.ListLastPhaseRuns(ctx)
	if err != nil {
		return fmt.Errorf("reading the last run of each phase: %w", err)
	}
	for _, row := range rows {
		if !row.FinishedAtUnix.Valid {
			return errors.New("reading a phase run: finish time is null")
		}
		if err := visit(
			row.Phase, time.Unix(row.FinishedAtUnix.Int64, 0).UTC(), row.Outcome, row.Detail,
			int(row.SourceStages), int(row.Created), int(row.Updated), int(row.Deleted),
		); err != nil {
			return err
		}
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
			//nolint:nilerr // Malformed cursors are deliberately reported as unusable.
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
	rows, err := s.queries.ListSyncRunsPage(ctx, sqlcgen.ListSyncRunsPageParams{
		ID: position, Limit: int64(limit + 1),
	})
	if err != nil {
		return "", false, fmt.Errorf("reading sync runs: %w", err)
	}
	visited := 0
	for _, row := range rows {
		if visited == limit {
			return next, true, nil
		}
		if !row.FinishedAtUnix.Valid {
			return "", false, errors.New("reading a sync run: finish time is null")
		}
		visited++
		next = strconv.FormatInt(row.ID, 10)
		if err := visit(
			row.Reference, row.Phase, time.Unix(row.FinishedAtUnix.Int64, 0).UTC(), row.Outcome, row.Detail,
			int(row.SourceStages), int(row.Created), int(row.Updated), int(row.Deleted),
		); err != nil {
			return "", false, err
		}
	}

	// The page was not filled, so nothing follows it.
	return "", true, nil
}

// lastSyncRunID reports the highest position the store has issued to a recorded
// run, or zero when it has recorded none.
func (s *Store) lastSyncRunID(ctx context.Context) (int64, error) {
	highest, err := s.queries.GetLastSyncRunID(ctx)
	if err != nil {
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

	queries := s.queries.WithTx(transaction)
	if err := queries.InsertSyncRun(ctx, sqlcgen.InsertSyncRunParams{
		Reference: reference, Phase: phase, StartedAtUnix: startedAt.Unix(),
		FinishedAtUnix: sql.NullInt64{Int64: finishedAt.Unix(), Valid: true},
		Outcome:        outcome, Detail: sql.NullString{String: detail, Valid: true},
		SourceStages: int64(sourceStages), Created: int64(created), Updated: int64(updated), Deleted: int64(deleted),
	}); err != nil {
		return "", fmt.Errorf("recording sync run: %w", err)
	}
	if err := pruneSyncRuns(ctx, queries); err != nil {
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
func pruneSyncRuns(ctx context.Context, queries *sqlcgen.Queries) error {
	if err := queries.PruneSyncRuns(ctx, retainedSyncRuns); err != nil {
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
	if err := s.queries.UpsertTargetRun(ctx, sqlcgen.UpsertTargetRunParams{
		TargetSlot: targetID, FinishedAtUnix: finishedAt.Unix(), Outcome: outcome, Detail: detail,
	}); err != nil {
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
	rows, err := s.queries.ListTargetRuns(ctx)
	if err != nil {
		return fmt.Errorf("listing target runs: %w", err)
	}
	for _, row := range rows {
		if err := visit(row.TargetSlot, time.Unix(row.FinishedAtUnix, 0).UTC(), row.Outcome, row.Detail); err != nil {
			return fmt.Errorf("visiting a target run: %w", err)
		}
	}

	return nil
}

// LastSuccessfulPhaseCompletion returns when a phase last recorded a success,
// which is what its trusted inventory age is measured against: a failed or
// skipped run leaves that inventory exactly as an earlier success left it.
func (s *Store) LastSuccessfulPhaseCompletion(ctx context.Context, phase string) (completedAt time.Time, found bool, err error) {
	if phase == "" {
		return time.Time{}, false, errors.New("phase is required")
	}
	completedUnix, err := s.queries.GetLastSuccessfulPhaseCompletion(ctx, sqlcgen.GetLastSuccessfulPhaseCompletionParams{
		Phase: phase, Outcome: "succeeded",
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}

		return time.Time{}, false, fmt.Errorf("reading the last successful completion of a phase: %w", err)
	}
	if !completedUnix.Valid {
		return time.Time{}, false, errors.New("reading the last successful completion of a phase: finish time is null")
	}

	return time.Unix(completedUnix.Int64, 0).UTC(), true, nil
}
