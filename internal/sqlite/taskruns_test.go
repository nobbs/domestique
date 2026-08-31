package sqlite

import (
	"errors"
	"strconv"
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
		t.Context(), "sync", "source", "", startedAt, startedAt.Add(time.Minute), "succeeded", "", "abc123def456", 10,
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
				t.Context(), test.task, "", "", test.startedAt, test.finishedAt,
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
		require.NoError(t, store.RecordTaskRun(t.Context(), "chatty", "", "", at.Add(time.Duration(run)*time.Minute),
			at.Add(time.Duration(run)*time.Minute), "succeeded", "", "reference", 3,
		), "RecordTaskRun(chatty)")
	}
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "quiet", "", "", at, at, "succeeded", "", "reference", 3,
	), "RecordTaskRun(quiet)")

	assert.Len(t, readTaskRuns(t, store, "chatty"), 3, "the chatty task kept more than its bound")
	assert.Len(t, readTaskRuns(t, store, "quiet"), 1, "a chatty task evicted a quiet one's history")
}

func TestRecordTaskRunKeepsTheLatestAttemptOverEachArgument(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-a", "", at, at, "failed", "authorization", "reference", 1,
	), "RecordTaskRun(rider-a)")
	for run := range 4 {
		require.NoError(t, store.RecordTaskRun(
			t.Context(), "sync:target", "rider-b", "", at.Add(time.Duration(run)*time.Minute),
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

// The row kept for an argument outside the retain window is the one with the
// latest finished_at, not the one with the highest id: a refusal recorded off
// the caller's goroutine can commit, and so be inserted, after a later attempt.
func TestRecordTaskRunKeepsTheLatestByFinishedAtNotByInsertOrder(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-a", "", at, at.Add(time.Minute), "succeeded", "", "reference", 1,
	), "RecordTaskRun(later, inserted first)")
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-a", "", at.Add(-time.Hour), at.Add(-time.Minute), "skipped", "held", "reference", 1,
	), "RecordTaskRun(earlier, inserted second)")
	// Past the retain window of 1, so only the argument's single kept row survives.
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync:target", "rider-b", "", at, at, "succeeded", "", "reference", 1,
	), "RecordTaskRun(rider-b)")

	runs := readTaskRuns(t, store, "sync:target")
	require.Len(t, runs, 2, "runs")
	for _, run := range runs {
		if run.argument == "rider-a" {
			assert.Equal(t, "succeeded", run.outcome, "the row kept for rider-a was the earlier one, inserted later")
		}
	}
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
		t.Context(), "sync", "", "", at, at, "succeeded", "", "reference", 1,
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
		require.NoError(t, store.RecordTaskRun(t.Context(), "sync", "", "", at.Add(time.Duration(run)*time.Minute),
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
		require.NoError(t, store.RecordTaskRun(t.Context(), "sync", "rider-a", "",
			at.Add(time.Duration(run)*time.Minute), at.Add(time.Duration(run)*time.Minute),
			outcome, "", "reference", 5,
		), "RecordTaskRun(rider-a)")
	}
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "rider-b", "", at, at, "blocked", "", "reference", 5,
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

// A refusal recorded off the caller's goroutine can commit after a later
// attempt's row, so recency has to follow finished_at rather than insertion
// order: recording the later event first must not make the earlier one win.
func TestLastTaskOutcomeFollowsFinishedAtRatherThanInsertOrder(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "rider-a", "", at, at.Add(time.Minute), "succeeded", "", "reference", 5,
	), "RecordTaskRun(later, inserted first)")
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "rider-a", "", at.Add(-time.Hour), at.Add(-time.Minute), "skipped", "held", "reference", 5,
	), "RecordTaskRun(earlier, inserted second)")

	outcome, found, err := store.LastTaskOutcome(t.Context(), "sync", "rider-a")
	require.NoError(t, err, "LastTaskOutcome()")
	assert.True(t, found, "found")
	assert.Equal(t, "succeeded", outcome, "an earlier attempt inserted later was read as the most recent")
}

func TestLastTaskSuccessSkipsWhatDidNotSucceed(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "", "", at, at, "succeeded", "", "reference", 5,
	), "RecordTaskRun(succeeded)")
	require.NoError(t, store.RecordTaskRun(
		t.Context(), "sync", "", "", at.Add(time.Hour), at.Add(time.Hour), "failed", "", "reference", 5,
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
			t.Context(), "sync", "", "", when, when, outcome, "", "reference", 50,
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
		t.Context(), "sync:target", "rider-a", "", at, at, "failed", "", "reference", 50,
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

// pagedRun is one recorded attempt as the history feed serves it.
type pagedRun struct {
	task      string
	argument  string
	trigger   string
	outcome   string
	reference string
}

func readTaskRunPage(t *testing.T, store *Store, task, after string, limit int) (runs []pagedRun, next string) {
	t.Helper()

	next, usable, err := store.ForEachTaskRunPage(t.Context(), task, after, limit, func(
		name, argument, trigger string, _, _ time.Time, outcome, _, reference string,
	) error {
		runs = append(runs, pagedRun{
			task: name, argument: argument, trigger: trigger, outcome: outcome, reference: reference,
		})

		return nil
	})
	require.NoError(t, err, "ForEachTaskRunPage()")
	require.True(t, usable, "ForEachTaskRunPage() rejected a cursor it issued")

	return runs, next
}

// recordTaskRunAt writes one attempt that finished at a given instant, under a
// reference a test can recognise it by.
func recordTaskRunAt(t *testing.T, store *Store, task, argument, trigger string, finishedAt time.Time, reference string) {
	t.Helper()

	require.NoError(t, store.RecordTaskRun(
		t.Context(), task, argument, trigger, finishedAt.Add(-time.Second), finishedAt,
		"succeeded", "", reference, 100,
	), "RecordTaskRun(%s)", reference)
}

func TestForEachTaskRunPageWalksEveryTaskNewestFirst(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	for run := range 5 {
		task := "sync:source"
		if run%2 == 1 {
			task = "surface:index"
		}
		recordTaskRunAt(t, store, task, "", "schedule", at.Add(time.Duration(run)*time.Minute), "run"+strconv.Itoa(run))
	}

	page, next := readTaskRunPage(t, store, "", "", 2)
	assert.Equal(t, []string{"run4", "run3"}, referencesOf(page), "the newest page, newest first")
	require.NotEmpty(t, next, "a cursor for the attempts before that page")

	page, next = readTaskRunPage(t, store, "", next, 2)
	assert.Equal(t, []string{"run2", "run1"}, referencesOf(page), "the page after the cursor")
	require.NotEmpty(t, next, "a cursor for the attempts before that page")

	page, next = readTaskRunPage(t, store, "", next, 2)
	assert.Equal(t, []string{"run0"}, referencesOf(page), "the oldest page")
	assert.Empty(t, next, "a cursor past the oldest recorded attempt")
}

// A filter narrows the feed to one task without changing how it pages.
func TestForEachTaskRunPageNarrowsToOneTask(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	recordTaskRunAt(t, store, "sync:source", "", "schedule", at, "source-old")
	recordTaskRunAt(t, store, "surface:index", "", "schedule", at.Add(time.Minute), "index")
	recordTaskRunAt(t, store, "sync:source", "", "manual", at.Add(2*time.Minute), "source-new")

	page, next := readTaskRunPage(t, store, "sync:source", "", 1)
	assert.Equal(t, []string{"source-new"}, referencesOf(page), "the newest attempt of the named task")
	require.NotEmpty(t, next, "a cursor for the attempts before that page")

	page, next = readTaskRunPage(t, store, "sync:source", next, 1)
	assert.Equal(t, []string{"source-old"}, referencesOf(page), "the page after the cursor")
	assert.Empty(t, next, "a cursor past the named task's oldest attempt")

	page, _ = readTaskRunPage(t, store, "invented", "", 10)
	assert.Empty(t, page, "a task nothing recorded was served attempts")
}

// The cursor carries the whole recency tuple, so an attempt whose row commits
// after a later one's is still reached. An id alone would page straight past it.
func TestForEachTaskRunPagePagesPastARowCommittedOutOfOrder(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	recordTaskRunAt(t, store, "sync:target", "rider-a", "schedule", at.Add(2*time.Minute), "latest")
	recordTaskRunAt(t, store, "sync:target", "rider-b", "manual", at, "earliest")
	recordTaskRunAt(t, store, "sync:target", "rider-c", "chain", at.Add(time.Minute), "middle")

	walked := make([]string, 0, 3)
	cursor := ""
	for range 3 {
		page, next := readTaskRunPage(t, store, "", cursor, 1)
		require.Len(t, page, 1, "a page of one")
		walked = append(walked, page[0].reference)
		cursor = next
	}
	assert.Equal(t, []string{"latest", "middle", "earliest"}, walked, "the walk, newest first")
	assert.Empty(t, cursor, "a cursor past the oldest recorded attempt")
}

// A cursor this store did not issue is the caller's mistake rather than an empty
// history, and answering it with the newest page would restart the walk.
func TestForEachTaskRunPageRefusesACursorItDidNotIssue(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	recordTaskRunAt(t, store, "sync:source", "", "schedule", at, "only")

	// The last one pairs an id in range with an instant ahead of the history: a
	// position past either half is one this store never handed out.
	for _, cursor := range []string{
		"the-newest-one", "12345", "notanumber:1", "12345:notanumber", "12345:0", "12345:999", "9999999999:1",
	} {
		visited := 0
		next, usable, err := store.ForEachTaskRunPage(t.Context(), "", cursor, 10, func(
			string, string, string, time.Time, time.Time, string, string, string,
		) error {
			visited++

			return nil
		})
		require.NoError(t, err, "ForEachTaskRunPage(%q)", cursor)
		assert.False(t, usable, "ForEachTaskRunPage(%q) accepted a cursor it did not issue", cursor)
		assert.Empty(t, next, "a cursor served under %q", cursor)
		assert.Zero(t, visited, "attempts visited under %q", cursor)
	}
}

// What started an attempt is written down with it, which is what tells a run an
// operator asked for from one the schedule started.
func TestForEachTaskRunPageReadsBackWhatStartedTheAttempt(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	recordTaskRunAt(t, store, "sync:source", "veloplanner", "manual", at, "asked-for")

	page, _ := readTaskRunPage(t, store, "", "", 10)
	assert.Equal(t, []pagedRun{{
		task: "sync:source", argument: "veloplanner", trigger: "manual",
		outcome: "succeeded", reference: "asked-for",
	}}, page, "the recorded attempt")
}

// The visitor is this method's entire output and the page size decides how much
// of the table it reads, so a caller supplying neither is answered rather than
// served an empty history. A visitor that fails partway stops the page.
func TestForEachTaskRunPageStopsOnVisitorFailure(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	recordTaskRunAt(t, store, "sync:source", "", "schedule", at, "only")

	_, _, err := store.ForEachTaskRunPage(t.Context(), "", "", 10, nil)
	require.Error(t, err, "ForEachTaskRunPage() without a visitor")

	visit := func(string, string, string, time.Time, time.Time, string, string, string) error { return nil }
	_, _, err = store.ForEachTaskRunPage(t.Context(), "", "", 0, visit)
	require.Error(t, err, "ForEachTaskRunPage() without a page size")

	visitErr := errors.New("visiting task run")
	_, _, err = store.ForEachTaskRunPage(t.Context(), "", "", 10, func(
		string, string, string, time.Time, time.Time, string, string, string,
	) error {
		return visitErr
	})
	assert.ErrorIs(t, err, visitErr, "ForEachTaskRunPage() with a failing visitor")
}

func referencesOf(runs []pagedRun) []string {
	references := make([]string, 0, len(runs))
	for _, run := range runs {
		references = append(references, run.reference)
	}

	return references
}

func TestForEachTaskRunPageReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	recordTaskRunAt(t, store, "sync:source", "", "schedule", at, "only")
	require.NoError(t, store.Close(), "Close()")

	visit := func(string, string, string, time.Time, time.Time, string, string, string) error { return nil }
	for name, after := range map[string]string{"reading the page": "", "resolving the cursor": "1:1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := store.ForEachTaskRunPage(t.Context(), "", after, 10, visit)
			assert.Error(t, err, "ForEachTaskRunPage() on a closed database")
		})
	}
}
