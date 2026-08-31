package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskScheduleReportsOnlyWhatWasDecided(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))

	require.NoError(t, store.SetTaskSchedule(t.Context(), "sync:target", false), "SetTaskSchedule()")

	schedule, err := store.TaskSchedule(t.Context())
	require.NoError(t, err, "TaskSchedule()")
	assert.Equal(t, map[string]bool{
		// Seeded from the two switches this table replaced.
		"sync:source": true,
		"sync:target": false,
	}, schedule, "the stored schedule")
}

func TestSetTaskScheduleReplacesAnEarlierDecision(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))

	require.NoError(t, store.SetTaskSchedule(t.Context(), "surface:index", false), "SetTaskSchedule(off)")
	require.NoError(t, store.SetTaskSchedule(t.Context(), "surface:index", true), "SetTaskSchedule(on)")

	schedule, err := store.TaskSchedule(t.Context())
	require.NoError(t, err, "TaskSchedule()")
	assert.True(t, schedule["surface:index"], "the second decision did not replace the first")
}

func TestTaskScheduleRefusesAnIncompleteRequestAndReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.Error(t, store.SetTaskSchedule(t.Context(), "", true), "SetTaskSchedule() with no task")

	require.NoError(t, store.Close(), "Close()")
	_, err := store.TaskSchedule(t.Context())
	require.Error(t, err, "TaskSchedule() on a closed database")
	require.Error(t, store.SetTaskSchedule(t.Context(), "sync:source", true),
		"SetTaskSchedule() on a closed database")
}
