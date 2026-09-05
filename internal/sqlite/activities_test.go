package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activityNow() time.Time {
	return time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
}

func TestKnownActivityIDsIsEmptyForATargetWithNoActivities(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	ids, err := store.KnownActivityIDs(t.Context(), "rider-a")
	require.NoError(t, err, "KnownActivityIDs()")
	assert.Empty(t, ids)
}

// One target's activities are its own: a poll never sees another rider's rides
// as already stored.
func TestStoreActivityKeepsEachTargetsActivitiesApart(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")

	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-b", 2, 200), "StoreActivity()")

	ids, err := store.KnownActivityIDs(t.Context(), "rider-a")
	require.NoError(t, err, "KnownActivityIDs()")
	assert.Equal(t, []int64{1}, ids)
}

// A poll that reads the same workout twice overwrites its row rather than
// failing on the primary key or leaving stale totals behind.
func TestStoreActivityOverwritesAPriorSummary(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 250), "StoreActivity() again")

	ids, err := store.KnownActivityIDs(t.Context(), "rider-a")
	require.NoError(t, err, "KnownActivityIDs()")
	assert.Equal(t, []int64{1}, ids, "the second read added a row instead of replacing one")

	var distance float64
	var raw []byte
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		"SELECT distance_metres, raw_summary_json FROM activities WHERE target_slot = ? AND workout_id = ?",
		"rider-a", 1).Scan(&distance, &raw), "reading the stored activity")
	assert.InDelta(t, 250.0, distance, 1e-9)
	assert.JSONEq(t, `{"distance_accum":"250"}`, string(raw))
}

func TestStoreActivityRefusesAnActivityItCannotAddress(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.ErrorContains(t, storeTestActivity(t, store, "", 1, 1), "are required")
	require.ErrorContains(t, storeTestActivity(t, store, "rider-a", 0, 1), "are required")
}

// A store that is gone is reported rather than read as a target with no rides.
func TestKnownActivityIDsReportsAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.KnownActivityIDs(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading stored activity ids")
}

// An activity belongs to a target that exists; the foreign key says so.
func TestStoreActivityRefusesAnUnknownTarget(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.Error(t, storeTestActivity(t, store, "rider-a", 1, 1))
}

func storeTestActivity(t *testing.T, store *Store, targetID string, id int64, distance float64) error {
	t.Helper()

	return store.StoreActivity(t.Context(), targetID,
		activity.Listing{ID: id, TypeID: 15, LocationID: 1, Starts: activityNow()},
		activity.Summary{
			DistanceMetres: distance, MovingSeconds: 3600, ElapsedSeconds: 3900, AscentMetres: 120,
			Raw: fmt.Appendf(nil, `{"distance_accum":"%g"}`, distance),
		},
		activityNow(),
	)
}
