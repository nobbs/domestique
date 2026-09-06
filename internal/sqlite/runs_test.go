package sqlite

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreRecordsRunsAndFailureNotificationState(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	reference, err := store.RecordSyncRun(
		t.Context(),
		"targets",
		startedAt,
		finishedAt,
		"succeeded",
		"",
		3,
		2,
		1,
		0,
	)
	require.NoError(t, err, "RecordSyncRun()")
	assert.Len(t, reference, 2*syncRunReferenceBytes, "the reference naming the recorded run")

	var (
		outcome      string
		detail       string
		sourceStages int
		created      int
		updated      int
		deleted      int
	)
	require.NoError(t, store.database.QueryRowContext(t.Context(), `
		SELECT outcome, detail, source_stages, created, updated, deleted FROM sync_runs
	`).Scan(&outcome, &detail, &sourceStages, &created, &updated, &deleted), "querying sync run")
	assert.Equal(t, "succeeded//3/2/1/0", fmt.Sprintf("%s/%s/%d/%d/%d/%d", outcome, detail, sourceStages, created, updated, deleted), "stored sync run")
	_, found, err := store.LastFailureNotification(t.Context(), "destination")
	require.NoError(t, err, "LastFailureNotification()")
	assert.False(t, found, "a notification was recorded before one was sent")
	require.NoError(t, store.RecordFailureNotification(t.Context(), "destination", finishedAt), "RecordFailureNotification()")
	sentAt, found, err := store.LastFailureNotification(t.Context(), "destination")
	require.NoError(t, err, "LastFailureNotification()")
	require.True(t, found, "the notification that was recorded is not readable")
	assert.WithinDuration(t, finishedAt, sentAt, 0, "LastFailureNotification()")
}

// Each phase's own last run is what an operator reads; the newest run of the
// other phase answers a different question.
func TestStoreReportsTheLastRunOfEachPhase(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	record := func(phase, outcome string, minute int, sourceStages, created int) {
		t.Helper()
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		_, err := store.RecordSyncRun(
			t.Context(), phase, began, began.Add(time.Second), outcome, "", sourceStages, created, 0, 0,
		)
		require.NoError(t, err, "RecordSyncRun()")
	}
	record("source", "failed", 0, 0, 0)
	record("source", "succeeded", 1, 12, 0)
	record("targets", "succeeded", 2, 12, 3)

	outcomes := make(map[string]string)
	counts := make(map[string]int)
	require.NoError(t, store.ForEachPhaseRun(t.Context(), func(
		phase string, _ time.Time, outcome, _ string, sourceStages, created, _, _ int,
	) error {
		outcomes[phase] = outcome
		counts[phase] = sourceStages + created

		return nil
	}), "ForEachPhaseRun()")
	assert.Equal(t, "succeeded", outcomes["source"], "source outcome")
	assert.Equal(t, "succeeded", outcomes["targets"], "targets outcome")
	assert.Equal(t, 15, counts["targets"], "target run counts")
}

// The history is what an operator reads back after a notification, so a run and
// the record of it must be the same run, and the page must come back newest
// first with a cursor that continues where it stopped.
func TestStoreReadsTheRecordedHistoryOnePageAtATime(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	references := make([]string, 0, 5)
	for minute := range 5 {
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		reference, err := store.RecordSyncRun(
			t.Context(), "source", began, began.Add(time.Second), "succeeded", "", minute, 0, 0, 0,
		)
		require.NoError(t, err, "RecordSyncRun()")
		references = append(references, reference)
	}

	page, next := readSyncRunPage(t, store, "", 2)
	assert.Equal(t, []string{references[4], references[3]}, page, "the newest page, newest first")
	require.NotEmpty(t, next, "a cursor for the runs before that page")

	page, next = readSyncRunPage(t, store, next, 2)
	assert.Equal(t, []string{references[2], references[1]}, page, "the page after the cursor")
	require.NotEmpty(t, next, "a cursor for the runs before that page")

	page, next = readSyncRunPage(t, store, next, 2)
	assert.Equal(t, []string{references[0]}, page, "the oldest page")
	assert.Empty(t, next, "a cursor past the oldest recorded run")
}

// A cursor this store did not issue is a client mistake rather than an empty
// history, and it is reported as one so the caller can say so. A number is not
// enough to be one: a position past the newest run would silently serve the
// first page again, which reads as a history that starts over.
func TestStoreRefusesACursorItDidNotIssue(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err := store.RecordSyncRun(
		t.Context(), "source", startedAt, startedAt.Add(time.Second), "succeeded", "", 3, 0, 0, 0,
	)
	require.NoError(t, err, "RecordSyncRun()")

	for _, cursor := range []string{"the-newest-one", "0", "-1", "999999999"} {
		visited := 0
		next, usable, err := store.ForEachSyncRun(t.Context(), cursor, 10, func(
			string, string, time.Time, string, string, int, int, int, int,
		) error {
			visited++

			return nil
		})
		require.NoError(t, err, "ForEachSyncRun(%q)", cursor)
		assert.False(t, usable, "ForEachSyncRun(%q) accepted a cursor it did not issue", cursor)
		assert.Empty(t, next, "a cursor served under %q", cursor)
		assert.Zero(t, visited, "runs visited under %q", cursor)
	}
}

// The visitor is this method's entire output and the page size decides how much
// of the table it reads, so a caller supplying neither is answered rather than
// served an empty history. A visitor that fails partway stops the page: a
// swallowed failure would serve half a page as a whole one.
func TestStoreStopsReadingTheHistoryOnVisitorFailure(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err := store.RecordSyncRun(
		t.Context(), "source", startedAt, startedAt.Add(time.Second), "succeeded", "", 3, 0, 0, 0,
	)
	require.NoError(t, err, "RecordSyncRun()")

	_, _, err = store.ForEachSyncRun(t.Context(), "", 10, nil)
	require.Error(t, err, "ForEachSyncRun() without a visitor")

	_, _, err = store.ForEachSyncRun(t.Context(), "", 0, func(
		string, string, time.Time, string, string, int, int, int, int,
	) error {
		return nil
	})
	require.Error(t, err, "ForEachSyncRun() without a page size")

	visitErr := errors.New("visiting sync run")
	_, _, err = store.ForEachSyncRun(t.Context(), "", 10, func(
		string, string, time.Time, string, string, int, int, int, int,
	) error {
		return visitErr
	})
	assert.ErrorIs(t, err, visitErr, "ForEachSyncRun() with a failing visitor")
}

// A run is what the history and the status response are both read from, so a
// record that could not describe one is refused rather than stored as a row that
// reports nothing.
func TestStoreRefusesAnIncompleteSyncRun(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)

	_, err := store.RecordSyncRun(t.Context(), "", startedAt, finishedAt, "succeeded", "", 0, 0, 0, 0)
	require.Error(t, err, "RecordSyncRun() without a phase")

	_, err = store.RecordSyncRun(t.Context(), "source", startedAt, startedAt.Add(-time.Second), "succeeded", "", 0, 0, 0, 0)
	require.Error(t, err, "RecordSyncRun() finishing before it started")

	_, err = store.RecordSyncRun(t.Context(), "source", startedAt, finishedAt, "", "", 0, 0, 0, 0)
	require.Error(t, err, "RecordSyncRun() without an outcome")

	_, err = store.RecordSyncRun(t.Context(), "source", startedAt, finishedAt, "succeeded", "", 0, -1, 0, 0)
	require.Error(t, err, "RecordSyncRun() with a negative count")
}

func TestSyncRunReadersRejectIncompleteRuns(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	_, err := store.database.ExecContext(t.Context(), `
		INSERT INTO sync_runs (
			reference, phase, started_at_unix, finished_at_unix, outcome, detail,
			source_stages, created, updated, deleted
		) VALUES ('run', 'source', 1, NULL, 'succeeded', '', 0, 0, 0, 0)
	`)
	require.NoError(t, err)

	_, _, _, _, _, _, _, _, err = store.LastSyncRun(t.Context())
	require.ErrorContains(t, err, "finish time is null")
	require.ErrorContains(t, store.ForEachPhaseRun(t.Context(), func(
		string, time.Time, string, string, int, int, int, int,
	) error {
		return nil
	}), "finish time is null")
	_, _, err = store.ForEachSyncRun(t.Context(), "", 1, func(
		string, string, time.Time, string, string, int, int, int, int,
	) error {
		return nil
	})
	require.ErrorContains(t, err, "finish time is null")
	_, _, err = store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.ErrorContains(t, err, "finish time is null")
}

// Runs are recorded forever on a service that is deployed forever, so the
// history is bounded. What it must never drop is the newest run of a half: the
// status response reads that as what the half last came to, and a half switched
// off while the other keeps running would otherwise lose its answer.
func TestStoreBoundsTheRecordedHistoryAndKeepsEachPhasesLastRun(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	record := func(phase string, minute int) {
		t.Helper()
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		_, err := store.RecordSyncRun(
			t.Context(), phase, began, began.Add(time.Second), "succeeded", "", 0, 0, 0, 0,
		)
		require.NoError(t, err, "RecordSyncRun()")
	}
	record("targets", 0)
	for minute := 1; minute <= retainedSyncRuns+10; minute++ {
		record("source", minute)
	}

	var runs, targetRuns int
	require.NoError(t, store.database.QueryRowContext(t.Context(), `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE phase = 'targets') FROM sync_runs
	`).Scan(&runs, &targetRuns), "counting retained runs")
	assert.Equal(t, retainedSyncRuns+1, runs, "retained runs, plus the target half's last one")
	assert.Equal(t, 1, targetRuns, "the target half's last run was pruned with the rest")
}

// readSyncRunPage collects one page of the recorded history as the references it
// names, newest first, with the cursor for the page after it. Runs recorded
// before the history was split by phase keep an empty phase and are not served.
// The page still fills to its limit: the exclusion happens where the page is read.
func TestStoreLeavesPrePhaseRunsOutOfTheHistory(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	legacy, err := store.RecordSyncRun(
		t.Context(), "source", startedAt, startedAt.Add(time.Second), "succeeded", "", 1, 0, 0, 0,
	)
	require.NoError(t, err, "RecordSyncRun()")
	// The state the phase migration leaves a pre-existing row in.
	_, err = store.database.ExecContext(
		t.Context(), "UPDATE sync_runs SET phase = '' WHERE reference = ?", legacy,
	)
	require.NoError(t, err, "ageing a run back to the pre-phase shape")

	recent := make([]string, 0, 2)
	for minute := range 2 {
		began := startedAt.Add(time.Duration(minute+1) * time.Minute)
		reference, recordErr := store.RecordSyncRun(
			t.Context(), "targets", began, began.Add(time.Second), "succeeded", "", 0, minute, 0, 0,
		)
		require.NoError(t, recordErr, "RecordSyncRun()")
		recent = append(recent, reference)
	}

	page, next := readSyncRunPage(t, store, "", 10)
	assert.Equal(t, []string{recent[1], recent[0]}, page, "the phased runs, newest first")
	assert.NotContains(t, page, legacy, "a run from before the history was split by phase")
	assert.Empty(t, next, "a cursor past the oldest servable run")
}

func readSyncRunPage(t *testing.T, store *Store, after string, limit int) (references []string, next string) {
	t.Helper()

	next, usable, err := store.ForEachSyncRun(t.Context(), after, limit, func(
		reference, _ string, _ time.Time, _, _ string, _, _, _, _ int,
	) error {
		references = append(references, reference)

		return nil
	})
	require.NoError(t, err, "ForEachSyncRun()")
	require.True(t, usable, "ForEachSyncRun() rejected a cursor it issued")

	return references, next
}

// Convergence is answered from the last attempt per slot, so a second run
// replaces what the first recorded rather than accumulating a history nobody
// reads.
func TestStoreKeepsOnlyTheLastRunOfEachTarget(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(9))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")

	first := time.Date(2026, time.August, 18, 6, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	require.NoError(t, store.RecordTargetRun(t.Context(), "rider-a", first, "failed", "destination"))
	require.NoError(t, store.RecordTargetRun(t.Context(), "rider-b", first, "succeeded", ""))
	require.NoError(t, store.RecordTargetRun(t.Context(), "rider-a", second, "succeeded", ""))

	type recorded struct {
		finishedAt time.Time
		id         string
		outcome    string
		detail     string
	}
	var runs []recorded
	require.NoError(t, store.ForEachTargetRun(
		t.Context(),
		func(targetID string, finishedAt time.Time, outcome, detail string) error {
			runs = append(runs, recorded{finishedAt: finishedAt, id: targetID, outcome: outcome, detail: detail})

			return nil
		},
	))
	assert.Equal(t, []recorded{
		{finishedAt: second, id: "rider-a", outcome: "succeeded"},
		{finishedAt: first, id: "rider-b", outcome: "succeeded"},
	}, runs)
}

// A slot that has never been reconciled is absent rather than reported as a run
// that succeeded with nothing to do.
func TestStoreReportsNoRunForAnUnreconciledTarget(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(10))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	visits := 0
	require.NoError(t, store.ForEachTargetRun(t.Context(), func(string, time.Time, string, string) error {
		visits++

		return nil
	}))
	assert.Zero(t, visits)
}

func TestStoreRefusesAnIncompleteTargetRun(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(11))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	finishedAt := time.Date(2026, time.August, 18, 6, 0, 0, 0, time.UTC)

	require.Error(t, store.RecordTargetRun(t.Context(), " ", finishedAt, "succeeded", ""))
	require.Error(t, store.RecordTargetRun(t.Context(), "rider-a", time.Time{}, "succeeded", ""))
	require.Error(t, store.RecordTargetRun(t.Context(), "rider-a", finishedAt, "", ""))
}

func TestStoreLastSuccessfulPhaseCompletionIgnoresFailuresAndOtherPhases(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	_, found, err := store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.NoError(t, err, "LastSuccessfulPhaseCompletion()")
	assert.False(t, found, "a completion was reported before any run")

	firstSuccess := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err = store.RecordSyncRun(t.Context(), "source", firstSuccess, firstSuccess, "succeeded", "", 3, 0, 0, 0)
	require.NoError(t, err, "RecordSyncRun()")
	_, err = store.RecordSyncRun(t.Context(), "targets", firstSuccess, firstSuccess, "succeeded", "", 3, 0, 1, 0)
	require.NoError(t, err, "RecordSyncRun()")

	laterFailure := firstSuccess.Add(time.Hour)
	_, err = store.RecordSyncRun(t.Context(), "source", laterFailure, laterFailure, "failed", "state", 0, 0, 0, 0)
	require.NoError(t, err, "RecordSyncRun()")

	completedAt, found, err := store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.NoError(t, err, "LastSuccessfulPhaseCompletion()")
	require.True(t, found, "the recorded success was not reported")
	assert.WithinDuration(t, firstSuccess, completedAt, 0, "LastSuccessfulPhaseCompletion() kept the failed run's time")
}

func TestStoreLastSuccessfulPhaseCompletionRequiresAPhase(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	_, _, err := store.LastSuccessfulPhaseCompletion(t.Context(), "")
	require.Error(t, err, "LastSuccessfulPhaseCompletion() accepted an empty phase")
}

func TestStoreLastSuccessfulPhaseCompletionReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, _, err := store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.Error(t, err, "LastSuccessfulPhaseCompletion() on a closed database")
}
