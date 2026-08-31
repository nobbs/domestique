package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskSwitchesRunWhatNobodyHasRuledOn(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	switches, err := newTaskSwitches(t.Context(), store)
	require.NoError(t, err, "newTaskSwitches()")

	assert.True(t, switches.enabledFor("surface:annotate")(), "a task nobody ruled on was held back")
	// The two switches the schedule table replaced are carried across, so an
	// operator's earlier choice survives the deploy.
	assert.True(t, switches.enabledFor("sync:source")(), "the carried-over read switch")
}

func TestSettingASwitchTakesEffectAtOnce(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	switches, err := newTaskSwitches(t.Context(), store)
	require.NoError(t, err, "newTaskSwitches()")
	enabled := switches.enabledFor("sync:target")
	require.True(t, enabled(), "the switch started off")

	require.NoError(t, switches.Set(t.Context(), "sync:target", false), "Set()")
	assert.False(t, enabled(), "switching a task off did not reach the running service")

	require.NoError(t, switches.Set(t.Context(), "sync:target", true), "Set()")
	assert.True(t, enabled(), "switching a task back on did not reach the running service")
}

func TestTaskSwitchesReportWhatWasDecided(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	switches, err := newTaskSwitches(t.Context(), store)
	require.NoError(t, err, "newTaskSwitches()")
	require.NoError(t, switches.Set(t.Context(), "surface:index", false), "Set()")

	assert.False(t, switches.snapshot()["surface:index"], "the snapshot")
	_, ruled := switches.snapshot()["surface:annotate"]
	assert.False(t, ruled, "a task nobody ruled on appeared in the snapshot")
}

func TestTaskSwitchesReportAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	require.NoError(t, store.Close(), "Close()")

	_, err := newTaskSwitches(t.Context(), store)
	require.Error(t, err, "newTaskSwitches() on a closed database")
}

func TestSettingASwitchReportsAnUnwritableDatabase(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	switches, err := newTaskSwitches(t.Context(), store)
	require.NoError(t, err, "newTaskSwitches()")
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, switches.Set(t.Context(), "sync:source", false), "Set() on a closed database")
}
