package sqlite

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func libraryGeometry() []route.Point {
	return []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	}
}

// storeTestLibrary seeds one stage and returns its key.
func storeTestLibrary(t *testing.T, store *Store, routeID int64, contentHash string) route.Key {
	t.Helper()
	stage := storeTestStageWithGeometry(t, routeID, 1, "revision", contentHash, "Alpine loop", "Descent", libraryGeometry())
	require.NoError(t,
		store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}),
		"StoreTrustedInventory()")

	return stage.Key()
}

// matchStore is a store with the targets these cases record rides against.
func matchStore(t *testing.T, targets ...string) *Store {
	t.Helper()
	store := openTestStore(t, testKey(1))
	for _, target := range targets {
		require.NoError(t, store.EnsureTargetOwner(t.Context(), target), "EnsureTargetOwner()")
	}

	return store
}

// matchOf is a match as the matcher would have produced one: both shares clear
// the gate it applies, so no case here stores a row that could not exist.
func matchOf(key route.Key) *activity.RouteMatch {
	return &activity.RouteMatch{
		Key: key, RouteCoverage: 0.97, RideCoverage: 0.94, Direction: activity.DirectionReverse,
	}
}

func TestStoreRoundTripsARouteMatch(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	matches, err := store.ActivityRouteMatches(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityRouteMatches()")
	require.Contains(t, matches, int64(11))
	assert.Equal(t, key, matches[11].Key)
	assert.InDelta(t, 0.97, matches[11].RouteCoverage, 1e-9)
	assert.InDelta(t, 0.94, matches[11].RideCoverage, 1e-9)
	assert.Equal(t, activity.DirectionReverse, matches[11].Direction,
		"which way round the ride went is stored with it")
}

// A ride recorded as being on no route is not served as a match, but is still
// a stored answer: the ride is not offered for matching again.
func TestStoreRecordsANoMatchWithoutServingOne(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, nil, "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	matches, err := store.ActivityRouteMatches(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityRouteMatches()")
	assert.Empty(t, matches)

	owed, err := store.ActivitiesAwaitingRouteMatch(t.Context(), "rider-a", "library-1")
	require.NoError(t, err, "ActivitiesAwaitingRouteMatch()")
	assert.Empty(t, owed, "a recorded no-match is an answer, not an omission")
}

func TestStoreOwesAMatchForARideMeasuredAgainstAnotherLibrary(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, storeTestRecords(t, store, "rider-a", 11), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	unchanged, err := store.ActivitiesAwaitingRouteMatch(t.Context(), "rider-a", "library-1")
	require.NoError(t, err, "ActivitiesAwaitingRouteMatch()")
	assert.Empty(t, unchanged)

	edited, err := store.ActivitiesAwaitingRouteMatch(t.Context(), "rider-a", "library-2")
	require.NoError(t, err, "ActivitiesAwaitingRouteMatch()")
	assert.Equal(t, []int64{11}, edited, "a library that has changed owes every ride a fresh match")
}

// Only a ride whose samples are stored can be matched: there is no track to
// match one whose FIT never arrived.
func TestStoreOwesNoMatchForARideWithoutSamples(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	owed, err := store.ActivitiesAwaitingRouteMatch(t.Context(), "rider-a", "library-1")
	require.NoError(t, err, "ActivitiesAwaitingRouteMatch()")
	assert.Empty(t, owed)
}

// The match describes the samples it was worked out from. Replacing them takes
// it with them, so the next derivation works it out again from the new track.
func TestStoreDropsARouteMatchWhenTheSamplesAreReplaced(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, storeTestRecords(t, store, "rider-a", 11), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	require.NoError(t, storeTestRecords(t, store, "rider-a", 11), "StoreActivityRecords() again")

	matches, err := store.ActivityRouteMatches(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityRouteMatches()")
	assert.Empty(t, matches)

	owed, err := store.ActivitiesAwaitingRouteMatch(t.Context(), "rider-a", "library-1")
	require.NoError(t, err, "ActivitiesAwaitingRouteMatch()")
	assert.Equal(t, []int64{11}, owed)
}

func TestStoreServesARoutesRidesToTheTargetThatRodeThem(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-b", 12, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-b", 12, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	rides, err := store.RouteActivities(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteActivities()")
	require.Len(t, rides, 1, "one rider's rides are not another's")
	assert.Equal(t, int64(11), rides[0].ID)
	assert.InDelta(t, 0.97, rides[0].RouteCoverage, 1e-9)
	assert.Equal(t, activity.DirectionReverse, rides[0].Direction)
}

func TestStoreServesNoRidesForARouteNobodyRode(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	storeTestLibrary(t, store, 7, "hash-a")

	rides, err := store.RouteActivities(
		t.Context(), "rider-a", route.NewKey(route.ProviderVeloPlanner, 99, 1),
	)
	require.NoError(t, err, "RouteActivities()")
	assert.Empty(t, rides)
}

// The hash covers where the library's routes run, so moving a line owes every
// stored match a fresh reading — and nothing else does.
func TestStoreLibraryHashFollowsTheGeometryItCovers(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	storeTestLibrary(t, store, 7, "hash-a")

	routes, first, err := store.LibraryRoutes(t.Context())
	require.NoError(t, err, "LibraryRoutes()")
	require.Len(t, routes, 1)
	assert.Equal(t, route.NewKey(route.ProviderVeloPlanner, 7, 1), routes[0].Key)
	assert.Len(t, routes[0].Geometry, 2, "the stored line is what a ride is matched against")

	moved := []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.6, Latitude: 49.3},
	}
	// A real geometry edit changes the stage's content hash too, that hash
	// covering the line among other things; what this pins is that the digest
	// followed the line, which the rename case shows it did not follow the hash.
	stage := storeTestStageWithGeometry(t, 7, 1, "revision", "hash-moved", "Alpine loop", "Descent", moved)
	require.NoError(t,
		store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}),
		"StoreTrustedInventory()")
	_, edited, err := store.LibraryRoutes(t.Context())
	require.NoError(t, err, "LibraryRoutes()")
	assert.NotEqual(t, first, edited, "a route whose line moved")

	storeTestLibrary(t, store, 8, "hash-c")
	_, added, err := store.LibraryRoutes(t.Context())
	require.NoError(t, err, "LibraryRoutes()")
	assert.NotEqual(t, edited, added, "a route joining the library changes it too")
}

// Renaming a route, or a revision carrying the same line, is not a reason to
// match every ride again: the stage's own content hash covers its title and
// revision, which is why the digest does not read it.
func TestStoreLibraryHashIgnoresARenameAndARevision(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	storeTestLibrary(t, store, 7, "hash-a")

	_, before, err := store.LibraryRoutes(t.Context())
	require.NoError(t, err, "LibraryRoutes()")

	renamed := storeTestStageWithGeometry(
		t, 7, 1, "a-later-revision", "a-different-content-hash", "Renamed loop", "Renamed stage", libraryGeometry(),
	)
	require.NoError(t,
		store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{renamed}),
		"StoreTrustedInventory()")

	_, after, err := store.LibraryRoutes(t.Context())
	require.NoError(t, err, "LibraryRoutes()")
	assert.Equal(t, before, after, "the ground the route covers has not moved")
}

func TestStoreReportsAnUnreadableRouteMatchStore(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, _, libraryErr := store.LibraryRoutes(t.Context())
	require.ErrorContains(t, libraryErr, "reading the library geometry")

	_, owedErr := store.ActivitiesAwaitingRouteMatch(t.Context(), "rider-a", "library-1")
	require.ErrorContains(t, owedErr, "listing activities awaiting a route match")

	_, matchesErr := store.ActivityRouteMatches(t.Context(), "rider-a")
	require.ErrorContains(t, matchesErr, "reading activity route matches")

	_, ridesErr := store.RouteActivities(t.Context(), "rider-a", route.NewKey(route.ProviderVeloPlanner, 7, 1))
	require.ErrorContains(t, ridesErr, "reading a route's activities")

	writeErr := store.StoreActivityRouteMatch(t.Context(), "rider-a", 11, nil, "library-1", activityNow())
	require.ErrorContains(t, writeErr, "storing an activity's route match")
}

// storeTestRecords gives a ride the stored samples that make it matchable.
func storeTestRecords(t *testing.T, store *Store, targetID string, id int64) error {
	t.Helper()

	return store.StoreActivityRecords(t.Context(), targetID, id, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), Latitude: 49.0, Longitude: 8.4, HasPosition: true},
			{Time: activityNow().Add(time.Second), Latitude: 49.2, Longitude: 8.5, HasPosition: true},
		},
	})
}

// A ride whose direction could not be told stores that, rather than a direction
// nothing measured.
func TestStoreRoundTripsAMatchWithNoDirection(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	match := matchOf(key)
	match.Direction = activity.DirectionUnknown
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, match, "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	matches, err := store.ActivityRouteMatches(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityRouteMatches()")
	assert.Equal(t, activity.DirectionUnknown, matches[11].Direction)

	var direction sql.NullString
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT direction FROM activity_route_match WHERE target_slot = ? AND workout_id = ?`,
		"rider-a", 11,
	).Scan(&direction), "reading the stored direction")
	assert.False(t, direction.Valid, "a direction that could not be told is absent, not the word for it")
}

// The read paths take a named route's coverage as given. That holds because the
// table refuses a row that names a route without it, rather than because every
// writer remembers to.
func TestStoreRefusesAMatchNamingARouteWithoutItsCoverage(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	_, err := store.database.ExecContext(t.Context(),
		`INSERT INTO activity_route_match
		   (target_slot, workout_id, provider, route_id, stage_order,
		    route_coverage, ride_coverage, library_hash, matched_at_unix)
		 VALUES (?, ?, 'veloplanner', 7, 1, NULL, NULL, 'library-1', 0)`,
		"rider-a", 11)

	require.ErrorContains(t, err, "CHECK constraint failed")
}

// The other half of the same rule: a row that names no route carries no
// coverage either, so "unmatched" cannot be confused with "matched nothing well".
func TestStoreRefusesCoverageWithoutARoute(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	_, err := store.database.ExecContext(t.Context(),
		`INSERT INTO activity_route_match
		   (target_slot, workout_id, provider, route_id, stage_order,
		    route_coverage, ride_coverage, library_hash, matched_at_unix)
		 VALUES (?, ?, NULL, NULL, NULL, 0.9, 0.9, 'library-1', 0)`,
		"rider-a", 11)

	require.ErrorContains(t, err, "CHECK constraint failed")
}

// A ride recorded as being on no route went no way round it. Without this the
// column could carry a direction for a ride the service says rode nothing.
func TestStoreRefusesADirectionOnANoMatch(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	_, err := store.database.ExecContext(t.Context(),
		`INSERT INTO activity_route_match
		   (target_slot, workout_id, provider, route_id, stage_order,
		    route_coverage, ride_coverage, direction, library_hash, matched_at_unix)
		 VALUES (?, ?, NULL, NULL, NULL, NULL, NULL, 'forward', 'library-1', 0)`,
		"rider-a", 11)

	require.ErrorContains(t, err, "CHECK constraint failed")
}

// Coverage is a share of a length, and nothing covers more of a route than all
// of it. A writer that computed one wrongly is refused rather than served.
func TestStoreRefusesCoverageBeyondAWholeRoute(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	for name, coverage := range map[string][2]float64{
		"more route than there is": {1.5, 0.95},
		"more ride than there is":  {0.95, 1.5},
		"a negative share":         {-0.1, 0.95},
	} {
		_, err := store.database.ExecContext(t.Context(),
			`INSERT OR REPLACE INTO activity_route_match
			   (target_slot, workout_id, provider, route_id, stage_order,
			    route_coverage, ride_coverage, direction, library_hash, matched_at_unix)
			 VALUES (?, ?, 'veloplanner', 7, 1, ?, ?, NULL, 'library-1', 0)`,
			"rider-a", 11, coverage[0], coverage[1])

		require.ErrorContains(t, err, "CHECK constraint failed", name)
	}
}

// A direction is one of two, or absent. A word meaning absence would say the
// same as the column being empty, leaving two ways to record one thing.
func TestStoreRefusesADirectionItDoesNotKnow(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")

	for _, direction := range []string{"unknown", "sideways", ""} {
		_, err := store.database.ExecContext(t.Context(),
			`INSERT OR REPLACE INTO activity_route_match
			   (target_slot, workout_id, provider, route_id, stage_order,
			    route_coverage, ride_coverage, direction, library_hash, matched_at_unix)
			 VALUES (?, ?, 'veloplanner', 7, 1, 0.97, 0.94, ?, 'library-1', 0)`,
			"rider-a", 11, direction)

		require.ErrorContains(t, err, "CHECK constraint failed", direction)
	}
}

// attemptOf is one ride's attempt at one climb, as the derivation would produce.
func attemptOf(climbIndex int, seconds float64) activity.ClimbAttempt {
	return activity.ClimbAttempt{
		ClimbIndex: climbIndex, Seconds: seconds,
		HeartRateBPM: 162, HasHeartRate: true,
		PowerWatts: 268, HasPower: true,
	}
}

func TestStoreRoundTripsClimbAttempts(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	require.NoError(t, store.StoreActivityClimbAttempts(t.Context(), "rider-a", 11,
		[]activity.ClimbAttempt{attemptOf(0, 780), attemptOf(2, 420)}), "StoreActivityClimbAttempts()")

	attempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteClimbAttempts()")
	require.Len(t, attempts, 2)
	assert.Equal(t, int64(11), attempts[0].WorkoutID)
	assert.Equal(t, 0, attempts[0].ClimbIndex)
	assert.InDelta(t, 780.0, attempts[0].Seconds, 0.001)
	assert.Equal(t, 2, attempts[1].ClimbIndex)
	assert.True(t, attempts[0].HasPower)
}

// A ride derived again replaces its attempts whole, so a climb the route no
// longer holds leaves no row behind.
func TestStoreReplacesClimbAttemptsWhole(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")
	require.NoError(t, store.StoreActivityClimbAttempts(t.Context(), "rider-a", 11,
		[]activity.ClimbAttempt{attemptOf(0, 780), attemptOf(1, 300)}), "StoreActivityClimbAttempts()")

	require.NoError(t, store.StoreActivityClimbAttempts(t.Context(), "rider-a", 11,
		[]activity.ClimbAttempt{attemptOf(0, 760)}), "StoreActivityClimbAttempts() again")

	attempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteClimbAttempts()")
	require.Len(t, attempts, 1, "the climb that went took its attempt with it")
	assert.InDelta(t, 760.0, attempts[0].Seconds, 0.001)
}

// Attempts belong to the target whose ride made them, and are read only within it.
func TestRouteClimbAttemptsAreScopedToOneTarget(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a", "rider-b")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-b", 11, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-b", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")
	require.NoError(t, store.StoreActivityClimbAttempts(t.Context(), "rider-b", 11,
		[]activity.ClimbAttempt{attemptOf(0, 780)}), "StoreActivityClimbAttempts()")

	attempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteClimbAttempts()")
	assert.Empty(t, attempts, "another rider's attempts are not this rider's")
}

// Clearing a target's matches takes its attempts with them: an attempt names a
// climb of a route the match is what attributed the ride to.
func TestClearActivityRouteMatchesTakesTheClimbAttempts(t *testing.T) {
	t.Parallel()
	store := matchStore(t, "rider-a")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 11, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 11, matchOf(key), "library-1", activityNow(),
	), "StoreActivityRouteMatch()")
	require.NoError(t, store.StoreActivityClimbAttempts(t.Context(), "rider-a", 11,
		[]activity.ClimbAttempt{attemptOf(0, 780)}), "StoreActivityClimbAttempts()")

	_, err := store.ClearActivityRouteMatches(t.Context(), "rider-a")
	require.NoError(t, err, "ClearActivityRouteMatches()")

	attempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteClimbAttempts()")
	assert.Empty(t, attempts, "no match, no attempt at its climbs")
}
