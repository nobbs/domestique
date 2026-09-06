package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A zero sentAt clears the record instead of recording one: recording and
// clearing are complements of the same row.
func TestStoreRecordFailureNotificationClearsOnAZeroTime(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	store := openTestStore(t, testKey(1))

	require.Error(t, store.RecordFailureNotification(t.Context(), "", time.Time{}), "RecordFailureNotification() accepted an empty category")
}

func TestStoreRecordFailureNotificationReportsAnUnreadableDatabaseWhenClearing(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.RecordFailureNotification(t.Context(), "source:stale", time.Time{}), "RecordFailureNotification() clear on a closed database")
}
