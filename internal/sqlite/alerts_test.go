package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertTogglesReadBackWhatWasDecided(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetAlertToggles(t.Context(), []AlertToggle{
		{Task: "sync", Alert: "destination", Enabled: false},
		{Task: "sync", Alert: "source", Enabled: true},
	}), "SetAlertToggles()")

	toggles, err := store.AlertToggles(t.Context())
	require.NoError(t, err, "AlertToggles()")
	assert.Equal(t, []AlertToggle{
		{Task: "sync", Alert: "destination", Enabled: false},
		{Task: "sync", Alert: "source", Enabled: true},
	}, toggles, "stored decisions")
}

// Deciding again replaces the decision rather than adding to it.
func TestASecondDecisionReplacesTheFirst(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetAlertToggles(t.Context(),
		[]AlertToggle{{Task: "sync", Alert: "destination", Enabled: false}}), "first decision")
	require.NoError(t, store.SetAlertToggles(t.Context(),
		[]AlertToggle{{Task: "sync", Alert: "destination", Enabled: true}}), "second decision")

	toggles, err := store.AlertToggles(t.Context())
	require.NoError(t, err, "AlertToggles()")
	assert.Equal(t, []AlertToggle{{Task: "sync", Alert: "destination", Enabled: true}}, toggles, "stored decisions")
}

// A slot an operator removed takes its decisions with it, so re-adding one
// starts from the task's defaults rather than from what was said about the last
// slot to carry that name.
func TestForgettingAScopeLeavesTheOtherScopesAlone(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetAlertToggles(t.Context(), []AlertToggle{
		{Task: "sync:target", Scope: "rider-a", Alert: "destination", Enabled: false},
		{Task: "sync:target", Scope: "rider-b", Alert: "destination", Enabled: false},
	}), "SetAlertToggles()")

	require.NoError(t, store.ForgetAlertScope(t.Context(), "sync:target", "rider-a"), "ForgetAlertScope()")

	toggles, err := store.AlertToggles(t.Context())
	require.NoError(t, err, "AlertToggles()")
	assert.Equal(t, []AlertToggle{
		{Task: "sync:target", Scope: "rider-b", Alert: "destination", Enabled: false},
	}, toggles, "stored decisions")
}

func TestAlertTogglesRefuseIncompleteDecisions(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))

	require.Error(t, store.SetAlertToggles(t.Context(),
		[]AlertToggle{{Alert: "destination"}}), "a decision with no task")
	require.Error(t, store.SetAlertToggles(t.Context(),
		[]AlertToggle{{Task: "sync"}}), "a decision with no alert")
	require.Error(t, store.ForgetAlertScope(t.Context(), "", "rider-a"), "forgetting with no task")
	require.Error(t, store.ForgetAlertScope(t.Context(), "sync:target", ""), "forgetting with no scope")
}

func TestAlertTogglesReportAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.AlertToggles(t.Context())
	require.Error(t, err, "AlertToggles() on a closed database")
	require.Error(t, store.SetAlertToggles(t.Context(),
		[]AlertToggle{{Task: "sync", Alert: "destination"}}), "SetAlertToggles() on a closed database")
	require.Error(t, store.ForgetAlertScope(t.Context(), "sync:target", "rider-a"),
		"ForgetAlertScope() on a closed database")
}
