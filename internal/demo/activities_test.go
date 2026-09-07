package demo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/route"
)

// rideByID is one synthetic ride, which every test here names rather than
// takes by position: the order of the fixtures is not part of the contract.
func rideByID(t *testing.T, rides []demo.Ride, id int64) *demo.Ride {
	t.Helper()

	for index := range rides {
		if rides[index].Listing.ID == id {
			return &rides[index]
		}
	}
	require.FailNowf(t, "no such ride", "ride %d is not among the fixtures", id)

	return nil
}

func TestRidesCoverTheShapesTheActivityViewsHaveToDraw(t *testing.T) {
	t.Parallel()

	rides, err := demo.Rides(seededAt())
	require.NoError(t, err)
	require.Len(t, rides, 4)

	full := rideByID(t, rides, 90_101)
	assert.NotEmpty(t, full.FIT.Records)
	for _, record := range full.FIT.Records {
		require.True(t, record.HasPosition, "the fully instrumented ride is positioned throughout")
		require.True(t, record.HasAltitude, "and carries a height throughout")
		require.True(t, record.HasHeartRate && record.HasCadence && record.HasPower,
			"and every sensor a ride page has a panel for")
	}

	warmUp := rideByID(t, rides, 90_102)
	assert.True(t, warmUp.FIT.Records[0].HasPosition, "the receiver had a fix from the start")
	assert.False(t, warmUp.FIT.Records[0].HasAltitude, "a position without a height is the shape being fixtured")
	assert.True(t, warmUp.FIT.Records[len(warmUp.FIT.Records)-1].HasAltitude,
		"and the height arrives once the barometer settles")

	noMeter := rideByID(t, rides, 90_103)
	for _, record := range noMeter.FIT.Records {
		require.False(t, record.HasPower, "the ride an estimate stands in for carries no meter at all")
		require.True(t, record.HasHeartRate && record.HasCadence, "but does carry the sensors it has")
	}

	trainer := rideByID(t, rides, 90_104)
	for _, record := range trainer.FIT.Records {
		require.False(t, record.HasPosition, "an indoor ride has no ground to record")
		require.False(t, record.HasAltitude, "and nothing to be at the height of")
	}
	assert.True(t, trainer.FIT.Records[0].HasPower, "it is ridden against a meter all the same")
}

// The totals a ride is listed by have to agree with the samples underneath it,
// or the ride page contradicts the list that opened it.
func TestEachRidesTotalsAgreeWithItsOwnSamples(t *testing.T) {
	t.Parallel()

	rides, err := demo.Rides(seededAt())
	require.NoError(t, err)

	for index := range rides {
		ride := &rides[index]
		records := ride.FIT.Records
		last := &records[len(records)-1]
		assert.InDelta(t, last.DistanceMetres, ride.Summary.DistanceMetres, 0.001,
			"the listed distance is the last sample's")
		assert.InDelta(t, last.Time.Sub(records[0].Time).Seconds(), ride.Summary.MovingSeconds, 0.001,
			"and the listed moving time is what the samples span")
		assert.GreaterOrEqual(t, ride.Summary.ElapsedSeconds, ride.Summary.MovingSeconds,
			"a ride cannot have been out for less time than it was moving")
		assert.False(t, ride.Listing.Starts.After(seededAt()), "no ride is dated in the future")
	}
}

// An outdoor ride is recorded over a library stage rather than over ground of
// its own, so the demo's map draws a track the rider can recognise from the
// route beside it. Rides() already fails outright for a stage the library has
// dropped; this pins that the samples really lie on the stage it names.
func TestAPositionedRideIsRecordedOverTheStageItFollowed(t *testing.T) {
	t.Parallel()

	stages, err := demo.Routes()
	require.NoError(t, err)
	rides, err := demo.Rides(seededAt())
	require.NoError(t, err)

	followed := map[int64]struct {
		routeID    int64
		stageOrder int
	}{
		90_101: {4102, 1},
		90_102: {4101, 1},
		90_103: {4101, 2},
	}
	for id, stageKey := range followed {
		ride := rideByID(t, rides, id)
		var geometry []route.Point
		for index := range stages {
			key := stages[index].Key()
			if key.SourceRouteID() == stageKey.routeID && key.StageOrder() == stageKey.stageOrder {
				geometry = stages[index].Geometry()

				break
			}
		}
		require.NotEmpty(t, geometry, "ride %d names a stage the library holds", id)

		records := ride.FIT.Records
		require.Len(t, records, len(geometry), "one sample per point of the stage")
		assert.InDelta(t, geometry[0].Latitude, records[0].Latitude, 1e-9,
			"ride %d sets off where the stage does", id)
		assert.InDelta(t, geometry[len(geometry)-1].Longitude, records[len(records)-1].Longitude, 1e-9,
			"ride %d finishes where the stage does", id)
		assert.Positive(t, records[len(records)-1].DistanceMetres, "and covers ground on the way")
	}
}

func TestSeedGivesAnOnboardedSlotItsRidesAndLeavesAnUnauthorizedOneEmpty(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{
		{ID: "rider-a", State: demo.SlotCurrent},
		{ID: "rider-b", State: demo.SlotUnauthorized},
	})
	window := seededAt().AddDate(0, 0, -30)

	onboarded, err := store.ActivitiesBetween(t.Context(), "rider-a", window, seededAt(), 50)
	require.NoError(t, err)
	assert.Len(t, onboarded, 4, "every fixture ride reached the slot that has been onboarded")

	unauthorized, err := store.ActivitiesBetween(t.Context(), "rider-b", window, seededAt(), 50)
	require.NoError(t, err)
	assert.Empty(t, unauthorized, "nothing has ever read the account behind an unauthorized slot")
}

// The samples have to arrive as stored, not pending: a ride whose records the
// store still considers outstanding is one no derivation will ever pick up.
func TestSeededRidesArriveWithTheirSamplesStored(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})

	for _, id := range []int64{90_101, 90_102, 90_103, 90_104} {
		state, found, err := store.ActivityRecordsState(t.Context(), "rider-a", id)
		require.NoError(t, err)
		require.True(t, found, "ride %d is stored", id)
		assert.Equal(t, activity.RecordsStored, state, "ride %d has its samples", id)
	}

	track, err := store.ActivityTrack(t.Context(), "rider-a", 90_101)
	require.NoError(t, err)
	assert.NotEmpty(t, track, "a positioned ride has a track for the map to draw")

	indoors, err := store.ActivityTrack(t.Context(), "rider-a", 90_104)
	require.NoError(t, err)
	assert.Empty(t, indoors, "an indoor ride has none")
}

// The whole point of seeding a profile beside the rides: the demo's derivation
// is the shipped one, and it must find something to work out. Without this the
// training-load panel and the fitness timeline stay empty however many rides
// the fixtures carry.
func TestSeededRidesDeriveTrainingNumbersAgainstTheSeededProfile(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})
	deriver, err := activity.NewDeriver(store, nil, nil, seededAt)
	require.NoError(t, err)

	result := deriver.Derive(t.Context(), "rider-a")
	require.Equal(t, activity.Polled, result.Outcome, "the seeded profile is enough to derive against")

	metrics, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err)
	require.Len(t, metrics, 4, "every ride yields something")

	measured := metrics[90_101]
	assert.True(t, measured.Load.HasTRIMP, "a ride with heart rate has a load")
	assert.True(t, measured.Load.HasZones, "and time in zones for the bar to draw")
	assert.True(t, measured.Averages.HasPower, "a ride with a meter reports its average power")
	assert.False(t, measured.Load.HasEstimatedPower, "and is never given an estimate beside it")

	estimated := metrics[90_103]
	assert.False(t, estimated.Averages.HasPower, "the ride with no meter measures none")
	assert.True(t, estimated.Load.HasEstimatedPower,
		"and is the one the estimate is drawn for; its track is what makes that possible")

	// The average alone would leave the track's own estimate series empty,
	// which is half of what these fixtures exist to make visible.
	track, err := store.ActivityTrack(t.Context(), "rider-a", 90_103)
	require.NoError(t, err)
	estimates := 0
	for index := range track {
		if track[index].HasEstimatedPower {
			estimates++
		}
	}
	assert.Positive(t, estimates, "the estimate is written back beside the samples it describes")
}
