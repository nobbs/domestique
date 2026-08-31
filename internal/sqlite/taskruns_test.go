package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordTaskRunReadsBackWhatItWrote(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "source", startedAt, startedAt.Add(time.Minute), "succeeded", "", "abc123def456", 10,
	), "RecordTaskRun()")

	assert.Equal(t, []taskRun{
		{argument: "source", startedAt: startedAt, finishedAt: startedAt.Add(time.Minute), outcome: "succeeded"},
	}, readTaskRuns(t, store, "sync"), "recorded runs")
}

func TestRecordTaskRunRefusesIncompleteMetadata(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		startedAt  time.Time
		finishedAt time.Time
		task       string
		outcome    string
		reference  string
		retain     int
	}{
		"no task":         {outcome: "succeeded", startedAt: at, finishedAt: at, reference: "r", retain: 1},
		"no outcome":      {task: "sync", startedAt: at, finishedAt: at, reference: "r", retain: 1},
		"no reference":    {task: "sync", outcome: "succeeded", startedAt: at, finishedAt: at, retain: 1},
		"no start":        {task: "sync", outcome: "succeeded", finishedAt: at, reference: "r", retain: 1},
		"no finish":       {task: "sync", outcome: "succeeded", startedAt: at, reference: "r", retain: 1},
		"finished first":  {task: "sync", outcome: "succeeded", startedAt: at, finishedAt: at.Add(-time.Hour), reference: "r", retain: 1},
		"retains nothing": {task: "sync", outcome: "succeeded", startedAt: at, finishedAt: at, reference: "r", retain: 0},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Error(t, store.RecordTaskRun(
				t.Context(), test.task, "", test.startedAt, test.finishedAt,
				test.outcome, "", test.reference, test.retain,
			), "RecordTaskRun()")
		})
	}
}

func TestRecordTaskRunBoundsEachTaskSeparately(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	for run := range 8 {
		require.NoError(t, store.RecordTaskRun(t.Context(), "chatty", "", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", "reference", 3,
		), "RecordTaskRun(chatty)")
	}
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "quiet", "", at, at, "succeeded", "", "reference", 3,
	), "RecordTaskRun(quiet)")

	assert.Len(t, readTaskRuns(t, store, "chatty"), 3, "the chatty task kept more than its bound")
	assert.Len(t, readTaskRuns(t, store, "quiet"), 1, "a chatty task evicted a quiet one's history")
}

func TestRecordTaskRunKeepsTheLatestAttemptOverEachArgument(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-a", at, at, "failed", "authorization", "reference", 1,
	), "RecordTaskRun(rider-a)")
	for run := range 4 {
		require.NoError(t, store.RecordTaskRun(
			t.Context(), "sync:target", "rider-b", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", "reference", 1,
		), "RecordTaskRun(rider-b)")
	}

	runs := readTaskRuns(t, store, "sync:target")
	arguments := make([]string, 0, len(runs))
	for _, run := range runs {
		arguments = append(arguments, run.argument)
	}
	assert.ElementsMatch(t, []string{"rider-b", "rider-a"}, arguments,
		"the last attempt over an argument was evicted by another argument's history")
}

func TestForEachTaskRunRefusesAnIncompleteRequest(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))

	require.Error(t, store.ForEachTaskRun(t.Context(), "", func(string, time.Time, time.Time, string, string) error {
		return nil
	}), "ForEachTaskRun() with no task")
	require.Error(t, store.ForEachTaskRun(t.Context(), "sync", nil), "ForEachTaskRun() with no visitor")
}

type taskRun struct {
	startedAt  time.Time
	finishedAt time.Time
	argument   string
	outcome    string
	detail     string
}

func readTaskRuns(t *testing.T, store *Store, task string) []taskRun {
	t.Helper()

	var runs []taskRun
	require.NoError(t, store.ForEachTaskRun(t.Context(), task,
		func(argument string, startedAt, finishedAt time.Time, outcome, detail string) error {
			runs = append(runs, taskRun{
				argument: argument, startedAt: startedAt, finishedAt: finishedAt, outcome: outcome, detail: detail,
			})

			return nil
		}), "ForEachTaskRun()")

	return runs
}

func TestRecordTaskRunReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.RecordTaskRun(
		t.Context(), "sync", "", at, at, "succeeded", "", "reference", 1,
	), "RecordTaskRun() on a closed database")
}

func TestForEachTaskRunReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.ForEachTaskRun(t.Context(), "sync",
		func(string, time.Time, time.Time, string, string) error { return nil },
	), "ForEachTaskRun() on a closed database")
}

// A visitor that gives up stops the read rather than being called again for
// every remaining row.
func TestForEachTaskRunStopsWhenTheVisitorDoes(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	for run := range 3 {
		require.NoError(t, store.RecordTaskRun(t.Context(), "sync", "", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", "reference", 5,
		), "RecordTaskRun()")
	}

	visits := 0
	giveUp := errors.New("enough")
	err := store.ForEachTaskRun(t.Context(), "sync", func(string, time.Time, time.Time, string, string) error {
		visits++

		return giveUp
	})

	require.ErrorIs(t, err, giveUp, "ForEachTaskRun()")
	assert.Equal(t, 1, visits, "the visitor was called again after giving up")
}

func TestLastTaskOutcomeReadsTheMostRecentAttemptOverAnArgument(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	for run, outcome := range []string{"failed", "succeeded"} {
		require.NoError(t, store.RecordTaskRun(t.Context(), "sync", "rider-a",
			at.Add(time.Duration(run)*time.Minute), at.Add(time.Duration(run)*time.Minute),
			outcome, "", "reference", 5,
		), "RecordTaskRun(rider-a)")
	}
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "rider-b", at, at, "blocked", "", "reference", 5,
	), "RecordTaskRun(rider-b)")

	outcome, found, err := store.LastTaskOutcome(t.Context(), "sync", "rider-a")
	require.NoError(t, err, "LastTaskOutcome()")
	assert.True(t, found, "found")
	assert.Equal(t, "succeeded", outcome, "the outcome")

	other, found, err := store.LastTaskOutcome(t.Context(), "sync", "rider-b")
	require.NoError(t, err, "LastTaskOutcome(rider-b)")
	assert.True(t, found, "found")
	assert.Equal(t, "blocked", other, "one argument's history answered for another")
}

func TestLastTaskSuccessSkipsWhatDidNotSucceed(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "", at, at, "succeeded", "", "reference", 5,
	), "RecordTaskRun(succeeded)")
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "", at.Add(time.Hour), at.Add(time.Hour), "failed", "", "reference", 5,
	), "RecordTaskRun(failed)")

	finishedAt, found, err := store.LastTaskSuccess(t.Context(), "sync", "")
	require.NoError(t, err, "LastTaskSuccess()")
	assert.True(t, found, "found")
	assert.Equal(t, at, finishedAt, "the last success")
}

func TestTaskHistoryLookupsRefuseAnIncompleteRequestAndReportAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	_, _, err := store.LastTaskOutcome(t.Context(), "", "")
	require.Error(t, err, "LastTaskOutcome() with no task")
	_, _, err = store.LastTaskSuccess(t.Context(), "", "")
	require.Error(t, err, "LastTaskSuccess() with no task")

	require.NoError(t, store.Close(), "Close()")
	_, _, err = store.LastTaskOutcome(t.Context(), "sync", "")
	require.Error(t, err, "LastTaskOutcome() on a closed database")
	_, _, err = store.LastTaskSuccess(t.Context(), "sync", "")
	require.Error(t, err, "LastTaskSuccess() on a closed database")
}

// A task nobody has run yet is waiting rather than overdue, and the absence has
// to read as one rather than as a zero time.
func TestTaskHistoryLookupsReportNothingRecorded(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	_, found, err := store.LastTaskOutcome(t.Context(), "sync", "")
	require.NoError(t, err, "LastTaskOutcome()")
	assert.False(t, found, "an unrun task reported an outcome")

	_, found, err = store.LastTaskSuccess(t.Context(), "sync", "")
	require.NoError(t, err, "LastTaskSuccess()")
	assert.False(t, found, "an unrun task reported a success")
}

func TestTaskFaultStreakCountsBackToTheLastSuccess(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
	record := func(minute int, outcome string) {
		t.Helper()
		when := at.Add(time.Duration(minute) * time.Minute)
		require.NoError(t, store.RecordTaskRun(
			t.Context(), "sync", "", when, when, outcome, "", "reference", 50,
		), "RecordTaskRun("+outcome+")")
	}
	record(0, "failed")
	record(1, "succeeded")
	record(2, "failed")
	// Neither ends the streak nor counts towards it: the task was busy, not broken.
	record(3, "skipped")
	record(4, "blocked")

	faults, lastAt, err := store.TaskFaultStreak(t.Context(), "sync", "")
	require.NoError(t, err, "TaskFaultStreak()")
	assert.Equal(t, 2, faults, "faults since the last success")
	assert.Equal(t, at.Add(4*time.Minute), lastAt, "when the last fault finished")
}

func TestTaskFaultStreakIsPerArgumentAndReportsNoStreak(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-a", at, at, "failed", "", "reference", 50,
	), "RecordTaskRun(rider-a)")

	faults, _, err := store.TaskFaultStreak(t.Context(), "sync:target", "rider-b")
	require.NoError(t, err, "TaskFaultStreak(rider-b)")
	assert.Zero(t, faults, "one slot's faults held another back")

	faults, _, err = store.TaskFaultStreak(t.Context(), "sync:target", "rider-a")
	require.NoError(t, err, "TaskFaultStreak(rider-a)")
	assert.Equal(t, 1, faults, "faults")
}

func TestTaskFaultStreakRefusesAnIncompleteRequestAndReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	_, _, err := store.TaskFaultStreak(t.Context(), "", "")
	require.Error(t, err, "TaskFaultStreak() with no task")

	require.NoError(t, store.Close(), "Close()")
	_, _, err = store.TaskFaultStreak(t.Context(), "sync", "")
	require.Error(t, err, "TaskFaultStreak() on a closed database")
}
