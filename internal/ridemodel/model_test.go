package ridemodel

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func testCoefficients() Coefficients {
	return Coefficients{
		SecondsPerKM:      140,
		SecondsPerAscentM: 4,
		Fingerprint:       "test",
	}
}

// metresToLongitudeDegrees places a point at the given distance east of the
// origin, along the equator, where HaversineMetres reduces to an exact great-
// circle arc — an exact, deterministic way to build geometry of a known length
// without depending on the distance function under test.
func metresToLongitudeDegrees(metres float64) float64 {
	return metres / route.EarthRadiusMetres * 180 / math.Pi
}

// sampledStage builds n points evenly spaced over totalDistanceMetres along the
// equator, with elevation read from elevationAt at each point's distance along
// the line.
func sampledStage(totalDistanceMetres float64, n int, elevationAt func(distanceMetres float64) float64) []route.Point {
	points := make([]route.Point, n)
	for index := range n {
		distance := totalDistanceMetres * float64(index) / float64(n-1)
		elevation := elevationAt(distance)
		points[index] = route.Point{Longitude: metresToLongitudeDegrees(distance), Elevation: &elevation}
	}

	return points
}

func flatStage(totalDistanceMetres float64, n int, elevationMetres float64) []route.Point {
	return sampledStage(totalDistanceMetres, n, func(float64) float64 { return elevationMetres })
}

func TestPredictFlatStageIsDistanceAlone(t *testing.T) {
	coefficients := testCoefficients()

	result, ok := Predict(flatStage(10_000, 50, 120), coefficients)

	require.True(t, ok, "Predict() ok")
	assert.InDelta(t, 10*coefficients.SecondsPerKM, result.MovingSeconds, 1e-6)
}

func TestPredictClimbAddsTheAscentTerm(t *testing.T) {
	coefficients := testCoefficients()
	climb := sampledStage(10_000, 50, func(distance float64) float64 { return distance * 0.05 })

	result, ok := Predict(climb, coefficients)

	require.True(t, ok, "Predict() ok")
	assert.InDelta(t, 10*coefficients.SecondsPerKM+500*coefficients.SecondsPerAscentM, result.MovingSeconds, 1e-6)
}

// The ascent term prices only what a rider climbs, matching
// route.Route.ElevationGainMetres(): a descent must not buy time back.
func TestPredictCreditsOnlyPositiveAscent(t *testing.T) {
	coefficients := testCoefficients()
	// Up 200 m over the first half, back down over the second.
	rolling := sampledStage(10_000, 51, func(distance float64) float64 {
		if distance <= 5_000 {
			return distance * 0.04
		}

		return (10_000 - distance) * 0.04
	})

	result, ok := Predict(rolling, coefficients)

	require.True(t, ok, "Predict() ok")
	assert.InDelta(t, 10*coefficients.SecondsPerKM+200*coefficients.SecondsPerAscentM, result.MovingSeconds, 1e-6)
}

// The cumulative series is what a stage page reads a time at an arbitrary point
// from, so it must carry one entry per geometry point, start at zero, never
// step backwards, and end at the total.
func TestPredictCumulativeSeriesAlignsWithTheGeometry(t *testing.T) {
	coefficients := testCoefficients()
	points := sampledStage(10_000, 40, func(distance float64) float64 { return math.Sin(distance/500) * 30 })

	result, ok := Predict(points, coefficients)

	require.True(t, ok, "Predict() ok")
	require.Len(t, result.CumulativeSeconds, len(points))
	assert.Zero(t, result.CumulativeSeconds[0], "the series starts at the first point")
	for index := 1; index < len(result.CumulativeSeconds); index++ {
		assert.GreaterOrEqual(t, result.CumulativeSeconds[index], result.CumulativeSeconds[index-1],
			"the running total must never step backwards at point %d", index)
	}
	assert.InDelta(t, result.MovingSeconds, result.CumulativeSeconds[len(points)-1], 1e-9)
}

// Point density is a property of the recording, not the route: two samplings of
// the same line must not be timed differently.
func TestPredictIsInsensitiveToPointDensity(t *testing.T) {
	coefficients := testCoefficients()
	elevation := func(distance float64) float64 { return distance * 0.03 }

	sparse, ok := Predict(sampledStage(10_000, 11, elevation), coefficients)
	require.True(t, ok, "Predict() ok for the sparse sampling")
	dense, ok := Predict(sampledStage(10_000, 501, elevation), coefficients)
	require.True(t, ok, "Predict() ok for the dense sampling")

	assert.InDelta(t, sparse.MovingSeconds, dense.MovingSeconds, 1e-6)
}

func TestPredictHandlesAZeroLengthSegment(t *testing.T) {
	coefficients := testCoefficients()
	elevation := 10.0
	point := route.Point{Longitude: 0, Elevation: &elevation}

	result, ok := Predict([]route.Point{point, point}, coefficients)
	require.True(t, ok, "Predict() ok")
	assert.Zero(t, result.MovingSeconds, "a duplicate point contributes no time")

	higher := 25.0
	noisy := route.Point{Longitude: 0, Elevation: &higher}
	result, ok = Predict([]route.Point{point, noisy}, coefficients)
	require.True(t, ok, "Predict() ok")
	assert.Zero(t, result.MovingSeconds, "elevation noise on a repeated point is not a climb")
}

func TestPredictReportsNoUsableElevation(t *testing.T) {
	coefficients := testCoefficients()
	points := []route.Point{{Longitude: 0}, {Longitude: metresToLongitudeDegrees(1_000)}}

	result, ok := Predict(points, coefficients)
	assert.False(t, ok, "Predict() ok without elevation")
	assert.Zero(t, result, "Predict() result without elevation")
}

func TestPredictReportsNoUsableElevationWithFewerThanTwoPoints(t *testing.T) {
	coefficients := testCoefficients()

	elevation := 0.0
	_, ok := Predict([]route.Point{{Longitude: 0, Elevation: &elevation}}, coefficients)
	assert.False(t, ok, "Predict() ok with a single point")
}
