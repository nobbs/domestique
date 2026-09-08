package rider_test

import (
	"math"
	"testing"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopped is a ride that moved for an hour over twenty kilometres and stood
// still for the given seconds, so its rate per moving hour is that figure.
func stopped(seconds float64) rider.StoppingRide {
	return rider.StoppingRide{
		DistanceMetres: 20_000,
		MovingSeconds:  3600,
		ElapsedSeconds: 3600 + seconds,
	}
}

func TestMeasureStoppingReadsTheMedianAndQuartilesOfTheRates(t *testing.T) {
	t.Parallel()

	measured := rider.MeasureStopping([]rider.StoppingRide{
		stopped(300), stopped(100), stopped(500), stopped(400), stopped(200),
	})

	require.True(t, measured.Set, "five rides are enough to read a habit from")
	assert.InDelta(t, 300.0, measured.MedianSecondsPerHour, 0.001)
	assert.InDelta(t, 200.0, measured.LowerQuartileSecondsPerHour, 0.001)
	assert.InDelta(t, 400.0, measured.UpperQuartileSecondsPerHour, 0.001)
	assert.Equal(t, 5, measured.Rides)
}

// An even count has no sample sitting on the median, so the two either side of
// it are interpolated between.
func TestMeasureStoppingInterpolatesBetweenSamples(t *testing.T) {
	t.Parallel()

	measured := rider.MeasureStopping([]rider.StoppingRide{
		stopped(0), stopped(100), stopped(200), stopped(300), stopped(400), stopped(500),
	})

	require.True(t, measured.Set, "MeasureStopping()")
	assert.InDelta(t, 250.0, measured.MedianSecondsPerHour, 0.001)
	assert.InDelta(t, 125.0, measured.LowerQuartileSecondsPerHour, 0.001)
	assert.InDelta(t, 375.0, measured.UpperQuartileSecondsPerHour, 0.001)
}

// A rate is per moving hour, so a half-hour ride's stopping counts double.
func TestMeasureStoppingScalesAStopByTheMovingTimeItInterrupted(t *testing.T) {
	t.Parallel()
	half := rider.StoppingRide{DistanceMetres: 12_000, MovingSeconds: 1800, ElapsedSeconds: 1800 + 150}

	measured := rider.MeasureStopping([]rider.StoppingRide{half, half, half, half, half})

	require.True(t, measured.Set, "MeasureStopping()")
	assert.InDelta(t, 300.0, measured.MedianSecondsPerHour, 0.001)
}

func TestMeasureStoppingIsAbsentBelowTheMinimumRides(t *testing.T) {
	t.Parallel()
	rides := make([]rider.StoppingRide, rider.StoppingMinimumRides-1)
	for index := range rides {
		rides[index] = stopped(300)
	}

	measured := rider.MeasureStopping(rides)

	assert.False(t, measured.Set, "too few rides describe those rides, not a habit")
	assert.Equal(t, rider.Stopping{}, measured, "an absent habit carries no figures")
}

// An errand, a ride too brief to be one, a device reporting more moving than
// elapsed, and a summary that does not reduce to a finite rate are each left
// out rather than read as one.
func TestMeasureStoppingLeavesOutWhatIsNotRiding(t *testing.T) {
	t.Parallel()
	rides := []rider.StoppingRide{
		stopped(300), stopped(300), stopped(300), stopped(300), stopped(300),
		{DistanceMetres: 400, MovingSeconds: 3600, ElapsedSeconds: 7200},
		{DistanceMetres: 20_000, MovingSeconds: 30, ElapsedSeconds: 3600},
		{DistanceMetres: 20_000, MovingSeconds: 3600, ElapsedSeconds: 1800},
		{DistanceMetres: 20_000, MovingSeconds: 3600, ElapsedSeconds: math.Inf(1)},
		{DistanceMetres: 20_000, MovingSeconds: math.NaN(), ElapsedSeconds: 7200},
	}

	measured := rider.MeasureStopping(rides)

	require.True(t, measured.Set, "MeasureStopping()")
	assert.Equal(t, 5, measured.Rides, "only the rides that are riding are counted")
	assert.InDelta(t, 300.0, measured.MedianSecondsPerHour, 0.001)
}

// One forgotten stop bends the median rather than setting it, which is why no
// upper bound is placed on a single ride.
func TestMeasureStoppingSurvivesARecordingLeftRunning(t *testing.T) {
	t.Parallel()

	measured := rider.MeasureStopping([]rider.StoppingRide{
		stopped(100), stopped(200), stopped(300), stopped(400), stopped(50_000),
	})

	require.True(t, measured.Set, "MeasureStopping()")
	assert.InDelta(t, 300.0, measured.MedianSecondsPerHour, 0.001)
}

func TestMeasureStoppingIsAbsentWithoutRides(t *testing.T) {
	t.Parallel()

	assert.False(t, rider.MeasureStopping(nil).Set, "a rider with no rides keeps the seeded figures")
}
