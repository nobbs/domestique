package sqlite

import (
	"database/sql"
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

// A replacement is the whole reading: a listing the account no longer holds is
// gone from it, and one it still holds keeps its listing fields.
func TestReplaceActivityListingsMakesThemWhatTheAccountHolds(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	require.NoError(t, store.ReplaceActivityListings(t.Context(), "rider-a", []activity.Listing{
		{ID: 2, Starts: activityNow().Add(time.Hour), TypeID: 15, LocationID: 1},
		{ID: 1, Starts: activityNow(), TypeID: 40, LocationID: 0},
	}, activityNow()), "ReplaceActivityListings()")

	listings, readAt, err := store.ActivityListings(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityListings()")
	assert.Equal(t, activityNow(), readAt, "the reading's time was not kept with it")
	require.Len(t, listings, 2)
	assert.Equal(t, activity.Listing{ID: 1, Starts: activityNow(), TypeID: 40, LocationID: 0}, listings[0],
		"the listings are not oldest first, or lost a listing field")

	require.NoError(t, store.ReplaceActivityListings(t.Context(), "rider-a", []activity.Listing{
		{ID: 2, Starts: activityNow().Add(time.Hour), TypeID: 15, LocationID: 1},
	}, activityNow()), "ReplaceActivityListings()")

	listings, _, err = store.ActivityListings(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityListings()")
	require.Len(t, listings, 1)
	assert.Equal(t, int64(2), listings[0].ID)
}

// The kept listings mirror the account rather than what is left to read, so
// storing an activity leaves its listing in place; only a fresh reading of the
// account takes one away.
func TestStoreActivityLeavesTheListingInPlace(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	pending := []activity.Listing{{ID: 1, Starts: activityNow()}, {ID: 2, Starts: activityNow()}}
	require.NoError(t, store.ReplaceActivityListings(t.Context(), "rider-a", pending, activityNow()), "ReplaceActivityListings()")
	require.NoError(t, store.ReplaceActivityListings(t.Context(), "rider-b", pending, activityNow()), "ReplaceActivityListings()")

	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")

	listings, readAt, err := store.ActivityListings(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityListings()")
	assert.Equal(t, activityNow(), readAt, "the reading's time was not kept with it")
	assert.Len(t, listings, 2, "a stored activity was dropped from the account's listings")

	others, _, err := store.ActivityListings(t.Context(), "rider-b")
	require.NoError(t, err, "ActivityListings()")
	assert.Len(t, others, 2, "another target's listings were changed")
}

func TestReplaceActivityListingsRefusesWhatItCannotAddress(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	require.ErrorContains(t, store.ReplaceActivityListings(t.Context(), "", nil, activityNow()), "is required")
	require.ErrorContains(t,
		store.ReplaceActivityListings(t.Context(), "rider-a", []activity.Listing{{ID: 0}}, activityNow()), "is required")
	require.Error(t,
		store.ReplaceActivityListings(t.Context(), "rider-b", []activity.Listing{{ID: 1}}, activityNow()),
		"an unknown target was accepted")
}

func TestActivityListingsReportsAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, _, err := store.ActivityListings(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading activity listings")
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

// A stored activity owes its records until they are written; the sensors a ride
// did not carry are stored as NULL rather than as a zero reading.
func TestStoreActivityRecordsWritesSamplesAndSettlesTheActivity(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")

	pending, err := store.ActivitiesAwaitingRecords(t.Context(), "rider-a", 10)
	require.NoError(t, err, "ActivitiesAwaitingRecords()")
	require.Len(t, pending, 1)
	assert.Equal(t, int64(1), pending[0].ID)
	assert.JSONEq(t, `{"distance_accum":"100"}`, string(pending[0].Summary.Raw))

	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		ChecksumFailed: true,
		Records: []activity.Record{
			{Time: activityNow(), PowerWatts: 240, HasPower: true},
			{Time: activityNow().Add(time.Second)},
		},
	}), "StoreActivityRecords()")

	var index int64
	var power, cadence sql.NullFloat64
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT record_index, power_watts, cadence_rpm FROM activity_records
		 WHERE target_slot = 'rider-a' AND workout_id = 1 ORDER BY record_index`).Scan(&index, &power, &cadence))
	assert.Zero(t, index)
	assert.True(t, power.Valid)
	assert.InDelta(t, 240.0, power.Float64, 1e-9)
	assert.False(t, cadence.Valid, "an absent sensor must be stored as NULL")

	var state string
	var checksumFailed int
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT records_state, fit_checksum_failed FROM activities WHERE target_slot = 'rider-a' AND workout_id = 1`).
		Scan(&state, &checksumFailed))
	assert.Equal(t, "stored", state)
	assert.Equal(t, 1, checksumFailed)

	pending, err = store.ActivitiesAwaitingRecords(t.Context(), "rider-a", 10)
	require.NoError(t, err, "ActivitiesAwaitingRecords() again")
	assert.Empty(t, pending, "a settled activity must no longer be awaiting records")
}

// A re-download replaces that activity's samples rather than adding to them.
func TestStoreActivityRecordsReplacesWhatWasThere(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")

	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow()}, {Time: activityNow().Add(time.Second)}},
	}), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow()}},
	}), "StoreActivityRecords() again")

	var rows int
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM activity_records WHERE target_slot = 'rider-a' AND workout_id = 1`).Scan(&rows))
	assert.Equal(t, 1, rows)
}

// Records are the activity's: removing it removes them, and no other target's.
func TestActivityRecordsCascadeAndStayWithTheirTarget(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-b", 1, 200), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow()}},
	}), "StoreActivityRecords()")

	pending, err := store.ActivitiesAwaitingRecords(t.Context(), "rider-b", 10)
	require.NoError(t, err, "ActivitiesAwaitingRecords()")
	require.Len(t, pending, 1, "another rider's activity was settled")

	_, err = store.database.ExecContext(t.Context(),
		`DELETE FROM activities WHERE target_slot = 'rider-a' AND workout_id = 1`)
	require.NoError(t, err)

	var rows int
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM activity_records`).Scan(&rows))
	assert.Zero(t, rows, "records outlived the activity they belong to")
}

// The oldest rides come first and only limit of them, so a long history fills
// in chronologically over successive polls.
func TestActivitiesAwaitingRecordsIsOldestFirstAndLimited(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	for index, starts := range []time.Time{
		activityNow(), activityNow().Add(-2 * time.Hour), activityNow().Add(-time.Hour),
	} {
		require.NoError(t, store.StoreActivity(t.Context(), "rider-a",
			activity.Listing{ID: int64(index + 1), TypeID: 15, LocationID: 1, Starts: starts},
			activity.Summary{Raw: []byte(`{}`)}, activityNow()), "StoreActivity()")
	}

	pending, err := store.ActivitiesAwaitingRecords(t.Context(), "rider-a", 2)
	require.NoError(t, err, "ActivitiesAwaitingRecords()")
	require.Len(t, pending, 2)
	assert.Equal(t, []int64{2, 3}, []int64{pending[0].ID, pending[1].ID}, "oldest first")
}

// An undecodable file is recorded as such so no later poll downloads it again.
func TestMarkActivityUnreadableTakesItOutOfThePendingSet(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")

	require.NoError(t, store.MarkActivityUnreadable(t.Context(), "rider-a", 1), "MarkActivityUnreadable()")

	pending, err := store.ActivitiesAwaitingRecords(t.Context(), "rider-a", 10)
	require.NoError(t, err, "ActivitiesAwaitingRecords()")
	assert.Empty(t, pending)

	var state string
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT records_state FROM activities WHERE target_slot = 'rider-a' AND workout_id = 1`).Scan(&state))
	assert.Equal(t, "unreadable", state)
}

func TestActivityRecordWritesReportAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivitiesAwaitingRecords(t.Context(), "rider-a", 10)
	require.ErrorContains(t, err, "reading activities awaiting records")
	require.Error(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{}))
	require.ErrorContains(t, store.MarkActivityUnreadable(t.Context(), "rider-a", 1), "marking an activity unreadable")
}

func TestActivitiesBetweenReportsAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivitiesBetween(t.Context(), "rider-a", activityNow(), activityNow().Add(time.Hour), 10)
	require.ErrorContains(t, err, "reading stored activities")
}

func TestActivityRecordWritesRefuseInvalidInputs(t *testing.T) {
	store := openTestStore(t, testKey(1))

	_, err := store.ActivitiesAwaitingRecords(t.Context(), "", 10)
	require.ErrorContains(t, err, "target and a positive limit")
	_, err = store.ActivitiesAwaitingRecords(t.Context(), "rider-a", 0)
	require.ErrorContains(t, err, "target and a positive limit")
	_, err = store.ActivitiesAwaitingRecords(t.Context(), "rider-a", -1)
	require.ErrorContains(t, err, "target and a positive limit")

	require.ErrorContains(t, store.StoreActivityRecords(t.Context(), "", 1, activity.FIT{}), "target and an activity id")
	require.ErrorContains(t, store.StoreActivityRecords(t.Context(), "rider-a", 0, activity.FIT{}), "target and an activity id")

	require.ErrorContains(t, store.MarkActivityUnreadable(t.Context(), "", 1), "target and an activity id")
	require.ErrorContains(t, store.MarkActivityUnreadable(t.Context(), "rider-a", 0), "target and an activity id")
}

// A calibration pools every target's rides, in the order they were ridden.
func TestActivityRidesReadsEveryTargetsRidesOldestFirst(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")

	for index, starts := range []time.Time{
		activityNow().Add(time.Hour), activityNow(), activityNow().Add(2 * time.Hour),
	} {
		require.NoError(t, store.StoreActivity(t.Context(), "rider-a",
			activity.Listing{ID: int64(index + 1), TypeID: 15, LocationID: 1, Starts: starts},
			activity.Summary{
				DistanceMetres: 30000, MovingSeconds: 4500, ElapsedSeconds: 4800, AscentMetres: 300,
				Raw: []byte(`{}`),
			},
			activityNow()), "StoreActivity()")
	}
	require.NoError(t, store.StoreActivity(t.Context(), "rider-b",
		activity.Listing{ID: 9, TypeID: 15, LocationID: 1, Starts: activityNow().Add(-time.Hour)},
		activity.Summary{DistanceMetres: 20000, MovingSeconds: 3600, AscentMetres: 100, Raw: []byte(`{}`)},
		activityNow()), "StoreActivity() for another rider")

	rides, err := store.ActivityRides(t.Context())
	require.NoError(t, err, "ActivityRides()")
	require.Len(t, rides, 4)
	assert.Equal(t, activityNow().Add(-time.Hour), rides[0].StartedAt, "oldest first")
	assert.InDelta(t, 20000.0, rides[0].DistanceMetres, 1e-9, "the other rider's ride is pooled in")
	assert.InDelta(t, 4500.0, rides[3].MovingSeconds, 1e-9)
	assert.InDelta(t, 300.0, rides[3].AscentMetres, 1e-9)
}

func TestActivityRidesReportsAnUnreadableStore(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivityRides(t.Context())
	require.ErrorContains(t, err, "reading activities for calibration")
}
