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
		t.Context(), "sync", "source", startedAt, startedAt.Add(time.Minute), "succeeded", "", 10,
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
		retain     int
	}{
		"no task":         {outcome: "succeeded", startedAt: at, finishedAt: at, retain: 1},
		"no outcome":      {task: "sync", startedAt: at, finishedAt: at, retain: 1},
		"no start":        {task: "sync", outcome: "succeeded", finishedAt: at, retain: 1},
		"no finish":       {task: "sync", outcome: "succeeded", startedAt: at, retain: 1},
		"finished first":  {task: "sync", outcome: "succeeded", startedAt: at, finishedAt: at.Add(-time.Hour), retain: 1},
		"retains nothing": {task: "sync", outcome: "succeeded", startedAt: at, finishedAt: at, retain: 0},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Error(t, store.RecordTaskRun(
				t.Context(), test.task, "", test.startedAt, test.finishedAt, test.outcome, "", test.retain,
			), "RecordTaskRun()")
		})
	}
}

// A task's history is bounded on its own terms, so one that runs every few
// minutes cannot evict the history of one that runs weekly.
func TestRecordTaskRunBoundsEachTaskSeparately(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	for run := range 8 {
		require.NoError(t, store.RecordTaskRun(
			t.Context(), "chatty", "", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", 3,
		), "RecordTaskRun(chatty)")
	}
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "quiet", "", at, at, "succeeded", "", 3,
	), "RecordTaskRun(quiet)")

	assert.Len(t, readTaskRuns(t, store, "chatty"), 3, "the chatty task kept more than its bound")
	assert.Len(t, readTaskRuns(t, store, "quiet"), 1, "a chatty task evicted a quiet one's history")
}

// The last attempt over one argument is what that argument came to, and a
// status page reads it whatever its age.
func TestRecordTaskRunKeepsTheLatestAttemptOverEachArgument(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-a", at, at, "failed", "authorization", 1,
	), "RecordTaskRun(rider-a)")
	for run := range 4 {
		require.NoError(t, store.RecordTaskRun(
			t.Context(), "sync:target", "rider-b", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", 1,
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
		t.Context(), "sync", "", at, at, "succeeded", "", 1,
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
		require.NoError(t, store.RecordTaskRun(
			t.Context(), "sync", "", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", 5,
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
