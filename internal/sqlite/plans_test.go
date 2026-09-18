package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPlanRecord(id int64, name string, published bool, version int64) *PlanRecord {
	now := time.Unix(1_700_000_000, 0).UTC()

	return &PlanRecord{
		ID: id, Name: name, Profile: "gravel", Waypoints: [][2]float64{{8.4, 49.0}, {8.5, 49.1}},
		Geometry:       []route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.5, Latitude: 49.1}},
		DistanceMetres: 1234.5, AscentMetres: 67.8, Published: published, Version: version,
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestStoreInsertsAndReadsBackAPlan(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	record := testPlanRecord(42, "Sunday loop", false, 1)
	require.NoError(t, store.InsertPlan(t.Context(), record), "InsertPlan()")

	got, found, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	require.True(t, found, "GetPlan() found")
	assert.Equal(t, *record, got, "GetPlan()")
}

func TestStoreKeepsWhereAPlanIsWalked(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	record := testPlanRecord(42, "Old town", false, 1)
	record.Pushing = [][2]float64{{120, 180}, {900, 950.5}}
	require.NoError(t, store.InsertPlan(t.Context(), record), "InsertPlan()")

	inserted, _, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	assert.Equal(t, record.Pushing, inserted.Pushing, "after insert")

	replaced := *record
	replaced.Version = 2
	replaced.Pushing = [][2]float64{{10, 20}}
	ok, err := store.ReplacePlan(t.Context(), &replaced, 1)
	require.NoError(t, err, "ReplacePlan()")
	require.True(t, ok, "ReplacePlan()")

	got, _, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	assert.Equal(t, replaced.Pushing, got.Pushing, "after replace")
}

func TestStoreGetPlanReportsNotFound(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	_, found, err := store.GetPlan(t.Context(), 999)
	require.NoError(t, err, "GetPlan()")
	assert.False(t, found, "GetPlan() found")
}

func TestStoreListsPlansOrderedByID(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.InsertPlan(t.Context(), testPlanRecord(9, "B", false, 1)), "InsertPlan()")
	require.NoError(t, store.InsertPlan(t.Context(), testPlanRecord(3, "A", true, 1)), "InsertPlan()")

	all, err := store.ListPlans(t.Context())
	require.NoError(t, err, "ListPlans()")
	require.Len(t, all, 2, "ListPlans()")
	assert.Equal(t, []int64{3, 9}, []int64{all[0].ID, all[1].ID}, "ListPlans() order")

	published, err := store.ListPublishedPlans(t.Context())
	require.NoError(t, err, "ListPublishedPlans()")
	require.Len(t, published, 1, "ListPublishedPlans()")
	assert.Equal(t, int64(3), published[0].ID, "ListPublishedPlans()")
}

func TestStoreReplacesPlanWhenVersionMatches(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	record := testPlanRecord(7, "Original", false, 1)
	require.NoError(t, store.InsertPlan(t.Context(), record), "InsertPlan()")

	replacement := testPlanRecord(7, "Renamed", true, 2)
	ok, err := store.ReplacePlan(t.Context(), replacement, 1)
	require.NoError(t, err, "ReplacePlan()")
	assert.True(t, ok, "ReplacePlan() applied")

	got, found, err := store.GetPlan(t.Context(), 7)
	require.NoError(t, err, "GetPlan()")
	require.True(t, found, "GetPlan() found")
	assert.Equal(t, "Renamed", got.Name, "Name")
	assert.True(t, got.Published, "Published")
	assert.Equal(t, int64(2), got.Version, "Version")
}

func TestStoreReplacePlanRejectsStaleVersion(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	record := testPlanRecord(7, "Original", false, 1)
	require.NoError(t, store.InsertPlan(t.Context(), record), "InsertPlan()")

	ok, err := store.ReplacePlan(t.Context(), testPlanRecord(7, "Renamed", true, 2), 99)
	require.NoError(t, err, "ReplacePlan()")
	assert.False(t, ok, "ReplacePlan() must reject a stale version")

	got, _, err := store.GetPlan(t.Context(), 7)
	require.NoError(t, err, "GetPlan()")
	assert.Equal(t, "Original", got.Name, "Name must be unchanged")
}

func TestStoreDeletesPlanWhenVersionMatches(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.InsertPlan(t.Context(), testPlanRecord(5, "Doomed", false, 1)), "InsertPlan()")

	ok, err := store.DeletePlan(t.Context(), 5, 1)
	require.NoError(t, err, "DeletePlan()")
	assert.True(t, ok, "DeletePlan() applied")

	_, found, err := store.GetPlan(t.Context(), 5)
	require.NoError(t, err, "GetPlan()")
	assert.False(t, found, "GetPlan() found")
}

func TestStoreReportsAnUnreadablePlanQueryOnEachMethod(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.InsertPlan(t.Context(), testPlanRecord(1, "Plan", false, 1)), "InsertPlan()")
	_, _, err := store.GetPlan(t.Context(), 1)
	require.Error(t, err, "GetPlan()")
	_, err = store.ListPlans(t.Context())
	require.Error(t, err, "ListPlans()")
	_, err = store.ListPublishedPlans(t.Context())
	require.Error(t, err, "ListPublishedPlans()")
	_, err = store.ReplacePlan(t.Context(), testPlanRecord(1, "Plan", false, 1), 1)
	require.Error(t, err, "ReplacePlan()")
	_, err = store.DeletePlan(t.Context(), 1, 1)
	require.Error(t, err, "DeletePlan()")
}

// A row whose stored waypoints or coordinates are not the JSON encodeWaypoints
// and encodeCoordinates write is a corruption this store never produces
// itself; GetPlan and ListPlans must still report it rather than panic.
func TestStoreReportsUndecodablePlanGeometry(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	_, err := store.database.ExecContext(t.Context(), `
		INSERT INTO plans (id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
			published, version, created_at_unix_nano, updated_at_unix_nano)
		VALUES (1, 'Corrupt', 'gravel', 'not json', x'6e6f74206a736f6e',
			0, 0, 0, 1, 1700000000000000000, 1700000000000000000)`)
	require.NoError(t, err, "seeding a corrupt plan row")

	_, _, err = store.GetPlan(t.Context(), 1)
	require.Error(t, err, "GetPlan() over undecodable waypoints")

	_, err = store.ListPlans(t.Context())
	require.Error(t, err, "ListPlans() over undecodable waypoints")
}

func TestStoreReportsUndecodablePlanCoordinates(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	_, err := store.database.ExecContext(t.Context(), `
		INSERT INTO plans (id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
			published, version, created_at_unix_nano, updated_at_unix_nano)
		VALUES (1, 'Corrupt', 'gravel', '[[8.4,49.0]]', x'6e6f74206a736f6e',
			0, 0, 0, 1, 1700000000000000000, 1700000000000000000)`)
	require.NoError(t, err, "seeding a corrupt plan row")

	_, _, err = store.GetPlan(t.Context(), 1)
	require.Error(t, err, "GetPlan() over undecodable coordinates")
}

func TestStoreDeletePlanRejectsStaleVersion(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.InsertPlan(t.Context(), testPlanRecord(5, "Kept", false, 1)), "InsertPlan()")

	ok, err := store.DeletePlan(t.Context(), 5, 99)
	require.NoError(t, err, "DeletePlan()")
	assert.False(t, ok, "DeletePlan() must reject a stale version")

	_, found, err := store.GetPlan(t.Context(), 5)
	require.NoError(t, err, "GetPlan()")
	assert.True(t, found, "GetPlan() found")
}

func TestStoreKeepsAPlansTurnsAndWhetherItsCourseCarriesThem(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	record := testPlanRecord(42, "Cued", false, 1)
	record.Turns = []route.Cue{{Turn: route.TurnLeft, Metres: 120.5}, {Turn: route.TurnRoundabout, Metres: 900, Exit: 3}}
	record.Cues = true
	require.NoError(t, store.InsertPlan(t.Context(), record), "InsertPlan()")

	inserted, _, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	assert.Equal(t, record.Turns, inserted.Turns, "turns after insert")
	assert.True(t, inserted.Cues, "cues after insert")

	replaced := *record
	replaced.Version, replaced.Turns, replaced.Cues = 2, nil, false
	ok, err := store.ReplacePlan(t.Context(), &replaced, 1)
	require.NoError(t, err, "ReplacePlan()")
	require.True(t, ok, "ReplacePlan()")

	got, _, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	assert.Nil(t, got.Turns, "turns after replace")
	assert.False(t, got.Cues, "cues after replace")
}

func TestStoreReportsUndecodablePlanTurns(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.InsertPlan(t.Context(), testPlanRecord(1, "Plan", false, 1)), "InsertPlan()")
	_, err := store.database.ExecContext(t.Context(), `UPDATE plans SET turns = 'not json' WHERE id = 1`)
	require.NoError(t, err, "corrupting the turns")

	_, _, err = store.GetPlan(t.Context(), 1)

	assert.ErrorContains(t, err, "turns", "GetPlan()")
}

func TestStoreKeepsAPlansStraightLegsAndAvoidedAreas(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	record := testPlanRecord(42, "Around town", false, 1)
	record.Straight = []int{1}
	record.Avoid = [][3]float64{{8.45, 49.05, 250}}
	require.NoError(t, store.InsertPlan(t.Context(), record), "InsertPlan()")

	inserted, _, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	assert.Equal(t, record.Straight, inserted.Straight, "straight legs after insert")
	assert.Equal(t, record.Avoid, inserted.Avoid, "avoided areas after insert")

	replaced := *record
	replaced.Version, replaced.Straight, replaced.Avoid = 2, nil, nil
	ok, err := store.ReplacePlan(t.Context(), &replaced, 1)
	require.NoError(t, err, "ReplacePlan()")
	require.True(t, ok, "ReplacePlan()")

	got, _, err := store.GetPlan(t.Context(), 42)
	require.NoError(t, err, "GetPlan()")
	assert.Nil(t, got.Straight, "straight legs after replace")
	assert.Nil(t, got.Avoid, "avoided areas after replace")
}

func TestStoreReportsUndecodableRoutingOptions(t *testing.T) {
	t.Parallel()
	corruptions := map[string]string{
		`UPDATE plans SET straight = 'not json' WHERE id = 1`: "straight legs",
		`UPDATE plans SET avoid = 'not json' WHERE id = 1`:    "avoided areas",
	}
	for corruption, want := range corruptions {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			store := openTestStore(t, testKey(1))
			require.NoError(t, store.InsertPlan(t.Context(), testPlanRecord(1, "Plan", false, 1)), "InsertPlan()")
			_, err := store.database.ExecContext(t.Context(), corruption)
			require.NoError(t, err, "corrupting the column")

			_, _, err = store.GetPlan(t.Context(), 1)

			assert.ErrorContains(t, err, want, "GetPlan()")
		})
	}
}
