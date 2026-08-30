package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/task"
)

func TestAlertDecisionsReportWhatWasDecidedAndWhatWasNot(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	require.NoError(t, store.SetAlertToggles(t.Context(), []sqlite.AlertToggle{
		{Task: "sync", Alert: "destination", Enabled: false},
		{Task: "sync", Alert: "source", Enabled: true},
	}), "SetAlertToggles()")

	decisions, err := newAlertDecisions(t.Context(), store)
	require.NoError(t, err, "newAlertDecisions()")

	tests := map[string]struct {
		alert   task.Detail
		enabled bool
		decided bool
	}{
		"switched off":   {alert: "destination", enabled: false, decided: true},
		"switched on":    {alert: "source", enabled: true, decided: true},
		"nobody decided": {alert: "course"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			enabled, decided := decisions.Wanted(t.Context(), "sync", test.alert)
			assert.Equal(t, test.decided, decided, "decided")
			assert.Equal(t, test.enabled, enabled, "enabled")
		})
	}
}

// A decision has to reach the running service, not just the database, or an
// operator switching an alert off would keep being woken by it.
func TestSettingADecisionTakesEffectAtOnce(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	decisions, err := newAlertDecisions(t.Context(), store)
	require.NoError(t, err, "newAlertDecisions()")

	_, decided := decisions.Wanted(t.Context(), "sync", "destination")
	require.False(t, decided, "something was decided before anybody decided it")

	require.NoError(t, decisions.Set(t.Context(), []sqlite.AlertToggle{
		{Task: "sync", Alert: "destination", Enabled: false},
	}), "Set()")

	enabled, decided := decisions.Wanted(t.Context(), "sync", "destination")
	assert.True(t, decided, "a decision did not reach the running service")
	assert.False(t, enabled, "the decision that reached it was the wrong one")
}

func TestAlertDecisionsReportAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	require.NoError(t, store.Close(), "Close()")

	_, err := newAlertDecisions(t.Context(), store)
	require.Error(t, err, "newAlertDecisions() on a closed database")
}

func TestSettingADecisionReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	decisions, err := newAlertDecisions(t.Context(), store)
	require.NoError(t, err, "newAlertDecisions()")
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, decisions.Set(t.Context(), []sqlite.AlertToggle{
		{Task: "sync", Alert: "destination", Enabled: false},
	}), "Set() on a closed database")
}
