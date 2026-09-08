package activity_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A synthetic route running due east, flat for a kilometre, then climbing at
// six percent for six hundred metres, then flat again. One climb, in the
// middle, which is what every case below is about.
const (
	routeStep    = 20.0
	flatMetres   = 1000.0
	climbMetres  = 600.0
	climbPercent = 6.0
)

func routeLine() (line []measure.Coordinate, elevations []float64) {
	total := int((2*flatMetres + climbMetres) / routeStep)
	line = make([]measure.Coordinate, 0, total+1)
	elevations = make([]float64, 0, total+1)
	// A degree of longitude at the equator is close enough to 111_320 m; the
	// exact figure does not matter, only that the points are evenly spaced.
	const metresPerDegree = 111_320.0
	altitude := 100.0
	for index := 0; index <= total; index++ {
		metres := float64(index) * routeStep
		line = append(line, measure.Coordinate{Latitude: 0, Longitude: metres / metresPerDegree})
		if metres > flatMetres && metres <= flatMetres+climbMetres {
			altitude += routeStep * climbPercent / 100
		}
		elevations = append(elevations, altitude)
	}

	return line, elevations
}

func rideStart() time.Time { return time.Date(2026, 7, 4, 7, 0, 0, 0, time.UTC) }

// ride walks the route from fromMetres to toMetres at the given pace, one
// sample every routeStep of ground.
func ride(fromMetres, toMetres, secondsPerStep, heartRate, power float64) (
	track []activity.TrackPoint, series []activity.SampleRow,
) {
	line, elevations := routeLine()
	track = []activity.TrackPoint{}
	series = []activity.SampleRow{}
	at := rideStart()
	for index := range line {
		metres := float64(index) * routeStep
		if metres < fromMetres || metres > toMetres {
			continue
		}
		track = append(track, activity.TrackPoint{
			Time: at, Latitude: line[index].Latitude, Longitude: line[index].Longitude,
			AltitudeMetres: elevations[index], HasAltitude: true,
		})
		series = append(series, activity.SampleRow{
			Time:         at,
			HeartRateBPM: activity.Reading{Value: heartRate, Known: heartRate > 0},
			PowerWatts:   activity.Reading{Value: power, Known: power > 0},
		})
		at = at.Add(time.Duration(secondsPerStep * float64(time.Second)))
	}

	return track, series
}

func routeClimbs(t *testing.T) (climbs []measure.Climb, line []measure.Coordinate) {
	t.Helper()
	routeCoordinates, elevations := routeLine()
	climbs = activity.RouteClimbs(routeCoordinates, elevations)
	require.Len(t, climbs, 1, "the synthetic route holds exactly one climb")

	return climbs, routeCoordinates
}

func TestClimbAttemptsTimeTheRideOverTheRoutesClimb(t *testing.T) {
	t.Parallel()
	climbs, line := routeClimbs(t)
	track, series := ride(0, 2600, 4, 150, 240)

	attempts := activity.ClimbAttempts(climbs, line, track, series, activity.DirectionForward)

	require.Len(t, attempts, 1)
	assert.Equal(t, 0, attempts[0].ClimbIndex)
	// The climb spans thirty steps of twenty metres at four seconds each.
	assert.InDelta(t, climbs[0].DistanceMetres/routeStep*4, attempts[0].Seconds, 8)
	require.True(t, attempts[0].HasHeartRate)
	assert.InDelta(t, 150.0, attempts[0].HeartRateBPM, 0.001)
	require.True(t, attempts[0].HasPower)
	assert.InDelta(t, 240.0, attempts[0].PowerWatts, 0.001)
	assert.False(t, attempts[0].HasEstimatedPower, "a ride with a meter carries no estimate")
}

// A faster ride over the same ground takes less time; nothing else moves.
func TestClimbAttemptsTimeADifferentPaceDifferently(t *testing.T) {
	t.Parallel()
	climbs, line := routeClimbs(t)
	slowTrack, slowSeries := ride(0, 2600, 6, 150, 200)
	fastTrack, fastSeries := ride(0, 2600, 3, 168, 300)

	slow := activity.ClimbAttempts(climbs, line, slowTrack, slowSeries, activity.DirectionForward)
	fast := activity.ClimbAttempts(climbs, line, fastTrack, fastSeries, activity.DirectionForward)

	require.Len(t, slow, 1)
	require.Len(t, fast, 1)
	assert.Greater(t, slow[0].Seconds, fast[0].Seconds, "the slower ride took longer")
}

// The acceptance criterion: a climb the ride did not cover records nothing.
func TestClimbAttemptsRecordNothingForAClimbTheRideDidNotFinish(t *testing.T) {
	t.Parallel()
	climbs, line := routeClimbs(t)
	// Turned back half way up.
	track, series := ride(0, flatMetres+climbMetres/2, 4, 150, 240)

	attempts := activity.ClimbAttempts(climbs, line, track, series, activity.DirectionForward)

	assert.Empty(t, attempts, "half a climb is not an attempt at it")
}

func TestClimbAttemptsRecordNothingForARideThatNeverReachedTheClimb(t *testing.T) {
	t.Parallel()
	climbs, line := routeClimbs(t)
	track, series := ride(0, flatMetres-100, 4, 150, 240)

	attempts := activity.ClimbAttempts(climbs, line, track, series, activity.DirectionForward)

	assert.Empty(t, attempts)
}

// A route ridden the other way round has its climbs as its descents, and a
// descent timed as a climb is a different thing rather than a fast attempt.
func TestClimbAttemptsRecordNothingForARideTheOtherWayRound(t *testing.T) {
	t.Parallel()
	climbs, line := routeClimbs(t)
	track, series := ride(0, 2600, 4, 150, 240)

	assert.Empty(t, activity.ClimbAttempts(climbs, line, track, series, activity.DirectionReverse))
	assert.Empty(t, activity.ClimbAttempts(climbs, line, track, series, activity.DirectionUnknown))
}

// A bicycle with no meter carries the estimate instead, and the two are never
// the same figure.
func TestClimbAttemptsKeepMeasuredAndEstimatedPowerApart(t *testing.T) {
	t.Parallel()
	climbs, line := routeClimbs(t)
	track, series := ride(0, 2600, 4, 150, 0)
	for index := range track {
		track[index].EstimatedPowerWatts, track[index].HasEstimatedPower = 210, true
	}

	attempts := activity.ClimbAttempts(climbs, line, track, series, activity.DirectionForward)

	require.Len(t, attempts, 1)
	assert.False(t, attempts[0].HasPower, "no meter, no measured power")
	require.True(t, attempts[0].HasEstimatedPower)
	assert.InDelta(t, 210.0, attempts[0].EstimatedPowerWatts, 0.001)
}

func TestRouteClimbsAreEmptyWithoutElevation(t *testing.T) {
	t.Parallel()
	line, _ := routeLine()

	assert.Empty(t, activity.RouteClimbs(line, nil), "a route with no height has no climb")
}

// twoClimbRoute runs flat, climbs, runs flat, climbs again, and ends flat.
func twoClimbRoute() (line []measure.Coordinate, elevations []float64) {
	const step, metresPerDegree = 20.0, 111_320.0
	line = []measure.Coordinate{}
	elevations = []float64{}
	altitude := 100.0
	for metres := 0.0; metres <= 4000; metres += step {
		line = append(line, measure.Coordinate{Latitude: 0, Longitude: metres / metresPerDegree})
		climbing := (metres > 1000 && metres <= 1600) || (metres > 2600 && metres <= 3200)
		if climbing {
			altitude += step * climbPercent / 100
		}
		elevations = append(elevations, altitude)
	}

	return line, elevations
}

// The acceptance criterion in full: a route of two climbs, and a ride that
// rode the first and turned for home before the second.
func TestClimbAttemptsRecordOnlyTheClimbTheRideCovered(t *testing.T) {
	t.Parallel()
	line, elevations := twoClimbRoute()
	climbs := activity.RouteClimbs(line, elevations)
	require.Len(t, climbs, 2, "the route holds two sustained climbs")

	track := []activity.TrackPoint{}
	series := []activity.SampleRow{}
	at := rideStart()
	for index := range line {
		if float64(index)*routeStep > 2000 {
			break
		}
		track = append(track, activity.TrackPoint{
			Time: at, Latitude: line[index].Latitude, Longitude: line[index].Longitude,
		})
		series = append(series, activity.SampleRow{Time: at})
		at = at.Add(4 * time.Second)
	}

	attempts := activity.ClimbAttempts(climbs, line, track, series, activity.DirectionForward)

	require.Len(t, attempts, 1, "one climb ridden, one not reached")
	assert.Equal(t, 0, attempts[0].ClimbIndex, "the first climb, which it did ride")
	assert.Positive(t, attempts[0].Seconds)
}

// shortClimbRoute holds the shortest climb the detector reports: one window
// long, which is where a fixed hundred-metre tolerance would have admitted a
// ride that merely crossed it.
func shortClimbRoute() (line []measure.Coordinate, elevations []float64) {
	const step, metresPerDegree = 10.0, 111_320.0
	altitude := 100.0
	for metres := 0.0; metres <= 800; metres += step {
		line = append(line, measure.Coordinate{Latitude: 0, Longitude: metres / metresPerDegree})
		if metres > 300 && metres <= 420 {
			altitude += step * 8 / 100
		}
		elevations = append(elevations, altitude)
	}

	return line, elevations
}

func TestClimbAttemptsHoldAShortClimbToItsOwnLength(t *testing.T) {
	t.Parallel()
	line, elevations := shortClimbRoute()
	climbs := activity.RouteClimbs(line, elevations)
	require.Len(t, climbs, 1, "the route holds one short climb")
	require.Less(t, climbs[0].DistanceMetres, 200.0, "and it is shorter than a fixed tolerance")

	// Stops half way up: less than a tenth short, and it would have counted.
	half := climbs[0].StartMetres + climbs[0].DistanceMetres/2
	track := []activity.TrackPoint{}
	series := []activity.SampleRow{}
	at := rideStart()
	for index := range line {
		if float64(index)*10 > half {
			break
		}
		track = append(track, activity.TrackPoint{
			Time: at, Latitude: line[index].Latitude, Longitude: line[index].Longitude,
		})
		series = append(series, activity.SampleRow{Time: at})
		at = at.Add(2 * time.Second)
	}

	attempts := activity.ClimbAttempts(climbs, line, track, series, activity.DirectionForward)

	assert.Empty(t, attempts, "half of a short climb is not an attempt at it")
}
