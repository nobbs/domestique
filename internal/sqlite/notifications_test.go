package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A zero sentAt clears the record instead of recording one: recording and
// clearing are complements of the same row.
func TestStoreRecordFailureNotificationClearsOnAZeroTime(t *testing.T) {
	store := openTestStore(t, testKey(1))
	sentAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordFailureNotification(t.Context(), "source:stale", sentAt), "RecordFailureNotification()")
	_, found, err := store.LastFailureNotification(t.Context(), "source:stale")
	require.NoError(t, err, "LastFailureNotification()")
	require.True(t, found, "the notification that was recorded is not readable")

	require.NoError(t, store.RecordFailureNotification(t.Context(), "source:stale", time.Time{}), "RecordFailureNotification() clear")
	_, found, err = store.LastFailureNotification(t.Context(), "source:stale")
	require.NoError(t, err, "LastFailureNotification()")
	assert.False(t, found, "the cleared notification is still readable")
}

func TestStoreRecordFailureNotificationRequiresACategory(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.Error(t, store.RecordFailureNotification(t.Context(), "", time.Time{}), "RecordFailureNotification() accepted an empty category")
}

func TestStoreRecordFailureNotificationReportsAnUnreadableDatabaseWhenClearing(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.RecordFailureNotification(t.Context(), "source:stale", time.Time{}), "RecordFailureNotification() clear on a closed database")
}

func TestStoreRecordsDigestNotificationState(t *testing.T) {
	store := openTestStore(t, testKey(1))
	sentAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)

	_, _, found, err := store.LastDigestNotification(t.Context())
	require.NoError(t, err, "LastDigestNotification()")
	assert.False(t, found, "a digest was recorded before one was sent")

	require.NoError(t, store.RecordDigestNotification(t.Context(), sentAt, 7), "RecordDigestNotification()")
	readBack, runID, found, err := store.LastDigestNotification(t.Context())
	require.NoError(t, err, "LastDigestNotification()")
	require.True(t, found, "the digest that was recorded is not readable")
	assert.WithinDuration(t, sentAt, readBack, 0, "LastDigestNotification()")
	assert.Equal(t, int64(7), runID, "the run the digest covered up to")

	// The window moves forward in place: a digest keeps one row, not one per send.
	later := sentAt.Add(24 * time.Hour)
	require.NoError(t, store.RecordDigestNotification(t.Context(), later, 19), "second RecordDigestNotification()")
	readBack, runID, _, err = store.LastDigestNotification(t.Context())
	require.NoError(t, err, "LastDigestNotification()")
	assert.WithinDuration(t, later, readBack, 0, "LastDigestNotification() after the second send")
	assert.Equal(t, int64(19), runID, "the run boundary after the second send")
}

// The digest reads are argument-guarded like every other store method, and the
// visitor's own error stops the walk rather than being counted as the end of it.
func TestStoreRejectsUnusableDigestArguments(t *testing.T) {
	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err := store.RecordSyncRun(t.Context(), "targets", at, at, "succeeded", "", 0, 1, 0, 0)
	require.NoError(t, err, "RecordSyncRun()")

	_, _, err = store.LastPhaseOutcome(t.Context(), "")
	require.Error(t, err, "LastPhaseOutcome() accepted no phase")

	require.Error(t, store.RecordDigestNotification(t.Context(), time.Time{}, 0),
		"RecordDigestNotification() accepted no time")

	require.Error(t, store.ForEachSuccessfulRunAfter(t.Context(), 0, nil),
		"ForEachSuccessfulRunAfter() accepted no visitor")

	stop := errors.New("stop")
	require.ErrorIs(t, store.ForEachSuccessfulRunAfter(t.Context(), 0,
		func(int64, string, int, int, int) error { return stop }), stop,
		"the visitor's error did not stop the walk")
}
