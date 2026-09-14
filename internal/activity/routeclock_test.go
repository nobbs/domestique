package activity_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readingsOverRoute is how many readings a ride over the whole of routeLine
// takes: the last hundred-metre mark lies a hair past its projected end.
const readingsOverRoute = int((2*flatMetres + climbMetres) / 100)

// clockRide walks routeLine from its start to its end, one sample every step,
// taking secondsPerStep over each and standing still for stopSeconds at
// stopAtMetres. The odometer advances with the ground covered.
func clockRide(secondsPerStep, stopAtMetres, stopSeconds float64) (
	line []measure.Coordinate, track []activity.TrackPoint, series []activity.SampleRow,
) {
	line, _ = routeLine()
	at := rideStart()
	for index := range line {
		metres := float64(index) * routeStep
		sample := func(odometer float64) {
			track = append(track, activity.TrackPoint{
				Time: at, Latitude: line[index].Latitude, Longitude: line[index].Longitude,
			})
			series = append(series, activity.SampleRow{
				Time: at, DistanceMetres: activity.Reading{Value: odometer, Known: true},
			})
		}
		sample(metres)
		if stopSeconds > 0 && metres == stopAtMetres {
			at = at.Add(time.Duration(stopSeconds * float64(time.Second)))
			// An auto-paused head unit's first record after the stop carries the
			// few metres it rolled away on.
			sample(metres + 2)
		}
		at = at.Add(time.Duration(secondsPerStep * float64(time.Second)))
	}

	return line, track, series
}

// uniformPrediction predicts secondsPerMetre over every metre of line.
func uniformPrediction(line []measure.Coordinate, secondsPerMetre float64) []float64 {
	cumulative := make([]float64, len(line))
	for index := 1; index < len(line); index++ {
		cumulative[index] = cumulative[index-1] +
			measure.HaversineMetres(line[index-1], line[index])*secondsPerMetre
	}

	return cumulative
}

func lastPoint(t *testing.T, clock *activity.RouteClock) activity.RouteClockPoint {
	t.Helper()
	require.NotNil(t, clock)
	require.NotEmpty(t, clock.Readings)

	return clock.Readings[len(clock.Readings)-1]
}

func TestRouteClockReadsMovingTimeEveryHundredMetres(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)

	clock := activity.ReadRouteClock(line, track, series, activity.DirectionForward)

	require.NotNil(t, clock)
	require.Len(t, clock.Readings, readingsOverRoute)
	for index, point := range clock.Readings {
		assert.InDelta(t, float64(index)*100, point.AlongMetres, routeStep, "reading %d", index)
		assert.InDelta(t, point.AlongMetres/routeStep*4, point.MovingSeconds, 1, "reading %d", index)
	}
}

// The acceptance criterion: a long stop is not time behind the prediction, even
// where the odometer crept across the pause.
func TestAheadOfPredictionReadsAStopAsOnModel(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 1200, 900)
	clock := activity.ReadRouteClock(line, track, series, activity.DirectionForward)
	last := lastPoint(t, clock)

	readings, ok := activity.AheadOfPrediction(line, uniformPrediction(line, 4/routeStep), clock, len(track))

	require.True(t, ok)
	require.Len(t, readings, len(track))
	for index := clock.Readings[0].Sample; index <= last.Sample; index++ {
		require.True(t, readings[index].Known, "sample %d lies between the first reading and the last", index)
		assert.InDelta(t, 0, readings[index].Value, 2, "sample %d", index)
	}
	assert.False(t, readings[len(readings)-1].Known, "past the last reading there is nothing to compare")
}

func TestAheadOfPredictionIsPositiveForARideFasterThanPredicted(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(2, 0, 0)
	clock := activity.ReadRouteClock(line, track, series, activity.DirectionForward)
	last := lastPoint(t, clock)

	readings, ok := activity.AheadOfPrediction(line, uniformPrediction(line, 4/routeStep), clock, len(track))

	require.True(t, ok)
	require.True(t, readings[last.Sample].Known)
	// Half of four seconds per twenty metres over the stretch read.
	assert.InDelta(t, last.AlongMetres/routeStep*2, readings[last.Sample].Value, 4)
	middle := readings[last.Sample/2]
	require.True(t, middle.Known)
	assert.InDelta(t, readings[last.Sample].Value/2, middle.Value, 4, "the lead builds evenly along the route")
}

func TestRouteClockRefusesARideTheOtherWayRound(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)

	assert.Nil(t, activity.ReadRouteClock(line, track, series, activity.DirectionReverse))
	assert.Nil(t, activity.ReadRouteClock(line, track, series, activity.DirectionUnknown))
}

func TestAheadOfPredictionIsAbsentWithoutAPrediction(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)
	clock := activity.ReadRouteClock(line, track, series, activity.DirectionForward)

	_, ok := activity.AheadOfPrediction(line, nil, clock, len(track))
	assert.False(t, ok, "no prediction")
	_, ok = activity.AheadOfPrediction(line, uniformPrediction(line[1:], 1), clock, len(track))
	assert.False(t, ok, "a prediction measured against other geometry")
	_, ok = activity.AheadOfPrediction(line, uniformPrediction(line, 1), nil, len(track))
	assert.False(t, ok, "no clock")
}

// A clock placed along a line that has since moved is in another frame, and
// reading it against the new line would compare two different places.
func TestAheadOfPredictionRefusesAClockReadAlongAnotherLine(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)
	clock := activity.ReadRouteClock(line, track, series, activity.DirectionForward)
	moved := append([]measure.Coordinate{}, line...)
	moved[0].Latitude += 0.0001

	_, ok := activity.AheadOfPrediction(moved, uniformPrediction(moved, 1), clock, len(track))

	assert.False(t, ok)
}

// A closed loop's finish lies beside its start, so the ride's samples snap to
// either leg; the clock must follow the ride out, not jump to the way back.
func TestRouteClockFollowsALoopFromItsStart(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)
	loop := append([]measure.Coordinate{}, line...)
	for index := len(line) - 2; index >= 0; index-- {
		loop = append(loop, line[index])
	}

	clock := activity.ReadRouteClock(loop, track, series, activity.DirectionForward)

	require.NotNil(t, clock)
	assert.Less(t, clock.Readings[0].AlongMetres, 100.0)
	assert.Len(t, clock.Readings, readingsOverRoute)
	assert.Less(t, lastPoint(t, clock).AlongMetres, 2*flatMetres+climbMetres)
}

// A rider who joins the route part way along is compared from where they joined.
func TestRouteClockStartsWhereTheRideJoinedTheRoute(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)
	joined := int(800 / routeStep)

	clock := activity.ReadRouteClock(line, track[joined:], series[joined:], activity.DirectionForward)

	require.NotNil(t, clock)
	assert.InDelta(t, 800, clock.Readings[0].AlongMetres, 2)
	assert.Greater(t, lastPoint(t, clock).AlongMetres, 2400.0)
}

// A stretch the GPS lost is a jump along the route, and readings carry on past it.
func TestRouteClockCarriesOnPastAGapInTheTrack(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)
	from, to := int(600/routeStep), int(1400/routeStep)
	track = append(track[:from:from], track[to:]...)
	series = append(series[:from:from], series[to:]...)

	clock := activity.ReadRouteClock(line, track, series, activity.DirectionForward)

	assert.Greater(t, lastPoint(t, clock).AlongMetres, 2400.0)
}

func TestRouteClockIsAbsentWithoutAnOdometer(t *testing.T) {
	t.Parallel()
	line, track, series := clockRide(4, 0, 0)
	for index := range series {
		series[index].DistanceMetres = activity.Reading{}
	}

	assert.Nil(t, activity.ReadRouteClock(line, track, series, activity.DirectionForward),
		"a ride with no odometer cannot tell moving from standing still")
}
