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

// A skip counts its attempts and keeps only the latest observation; a second
// poll sees one row, not two.
func TestRecordActivitySkipCountsAttempts(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	require.NoError(t, store.RecordActivitySkip(t.Context(), "rider-a", 7, "HTTP 404", activityNow()), "RecordActivitySkip()")
	later := activityNow().Add(25 * time.Hour)
	require.NoError(t, store.RecordActivitySkip(t.Context(), "rider-a", 7, "HTTP 401", later), "RecordActivitySkip() again")

	skips, err := store.ActivitySkips(t.Context(), "rider-a")
	require.NoError(t, err, "ActivitySkips()")
	assert.Equal(t, []activity.Skip{{ID: 7, Attempts: 2, LastAttempt: later}}, skips)

	var observed string
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		"SELECT observed FROM activity_skips WHERE target_slot = ? AND workout_id = ?", "rider-a", 7).Scan(&observed))
	assert.Equal(t, "HTTP 401", observed)
}

// A skipped activity is not a stored one: it must not reach the read model.
func TestASkippedActivityIsNotAStoredOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.RecordActivitySkip(t.Context(), "rider-a", 7, "HTTP 404", activityNow()), "RecordActivitySkip()")

	ids, err := store.KnownActivityIDs(t.Context(), "rider-a")
	require.NoError(t, err, "KnownActivityIDs()")
	assert.Empty(t, ids, "a skip was reported as a known activity")
	stored, err := store.ActivitiesBetween(t.Context(), "rider-a", activityNow().Add(-time.Hour), activityNow().Add(time.Hour), 10)
	require.NoError(t, err, "ActivitiesBetween()")
	assert.Empty(t, stored, "a skip was served as an activity")
}

// A read that succeeds after a skip leaves no trace that would hold it back.
func TestStoreActivityForgetsASkip(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.RecordActivitySkip(t.Context(), "rider-a", 7, "HTTP 404", activityNow()), "RecordActivitySkip()")

	require.NoError(t, storeTestActivity(t, store, "rider-a", 7, 100), "StoreActivity()")

	skips, err := store.ActivitySkips(t.Context(), "rider-a")
	require.NoError(t, err, "ActivitySkips()")
	assert.Empty(t, skips)
}

func TestRecordActivitySkipRefusesWhatItCannotAddress(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	require.ErrorContains(t, store.RecordActivitySkip(t.Context(), "", 7, "", activityNow()), "are required")
	require.ErrorContains(t, store.RecordActivitySkip(t.Context(), "rider-a", 0, "", activityNow()), "are required")
	require.Error(t, store.RecordActivitySkip(t.Context(), "rider-b", 7, "", activityNow()), "an unknown target was accepted")
}

func TestActivitySkipsReportsAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivitySkips(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading activity skips")
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

// The window is half-open: an activity starting exactly at from is served, one
// starting exactly at to is not.
func TestActivitiesBetweenServesAHalfOpenWindowNewestFirst(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")

	from := activityNow()
	for index, starts := range []time.Time{
		from.Add(-time.Hour), from, from.Add(time.Hour), from.Add(2 * time.Hour),
	} {
		require.NoError(t, store.StoreActivity(t.Context(), "rider-a",
			activity.Listing{ID: int64(index + 1), TypeID: 15, LocationID: 1, Starts: starts},
			activity.Summary{
				DistanceMetres: 1000, MovingSeconds: 60, ElapsedSeconds: 90, AscentMetres: 10,
				Raw: []byte(`{}`),
			},
			activityNow(),
		), "StoreActivity()")
	}
	require.NoError(t, storeTestActivity(t, store, "rider-b", 9, 500), "StoreActivity() for another rider")

	stored, err := store.ActivitiesBetween(t.Context(), "rider-a", from, from.Add(2*time.Hour), 5000)
	require.NoError(t, err, "ActivitiesBetween()")
	require.Len(t, stored, 2)
	assert.Equal(t, []int64{3, 2}, []int64{stored[0].ID, stored[1].ID}, "newest first")
	assert.Equal(t, from.Add(time.Hour), stored[0].StartedAt)
	assert.InDelta(t, 1000.0, stored[0].DistanceMetres, 1e-9)
	assert.Equal(t, 15, stored[0].TypeID)
	assert.Equal(t, 1, stored[0].LocationID)
}

func TestActivitiesBetweenHonoursTheLimit(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")

	stored, err := store.ActivitiesBetween(
		t.Context(), "rider-a", activityNow().Add(-time.Hour), activityNow().Add(time.Hour), 0)
	require.NoError(t, err, "ActivitiesBetween()")
	assert.Empty(t, stored)
}

func TestActivitiesBetweenReportsAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivitiesBetween(t.Context(), "rider-a", activityNow(), activityNow().Add(time.Hour), 10)
	require.ErrorContains(t, err, "reading stored activities")
}
