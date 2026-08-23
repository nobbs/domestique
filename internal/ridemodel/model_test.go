package ridemodel

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

func testCoefficients() Coefficients {
	return Coefficients{
		MassKG:                    90,
		PowerWatts:                200,
		DriveEfficiency:           0.97,
		CdAM2:                     0.4,
		AirDensityKGPerM3:         1.2,
		DescentCutoffPercent:      -1,
		DescentCapMetresPerSecond: 20,
		CrrBySurface: map[surface.Kind]float64{
			surface.KindAsphalt:   0.005,
			surface.KindPaving:    0.006,
			surface.KindCompacted: 0.007,
			surface.KindGravel:    0.009,
			surface.KindGround:    0.012,
		},
		Fingerprint: "test",
	}
}

// metresToLongitudeDegrees places a point at the given distance east of the
// origin, along the equator, where haversineMetres reduces to an exact great-
// circle arc — an exact, deterministic way to build geometry of a known length
// without depending on the distance function under test.
func metresToLongitudeDegrees(metres float64) float64 {
	return metres / earthRadiusMetres * 180 / math.Pi
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

func kindsAll(n int, kind surface.Kind) []surface.Kind {
	kinds := make([]surface.Kind, n)
	for index := range kinds {
		kinds[index] = kind
	}

	return kinds
}

// floatPtr is a plain pointer-to-literal helper rather than Go's newer
// new(expr) form: this repository's toolchain accepts it, but tooling that
// reviews this code may not recognise it yet, and a helper needs no reader to
// know which Go release added it. Mirrors internal/komoot's convert_test.go.
//
//nolint:modernize // deliberately not new(expr); see comment above.
func floatPtr(value float64) *float64 { return &value }

func TestPredictFlatAsphaltRoundTripsThroughItsOwnSolver(t *testing.T) {
	coefficients := testCoefficients()
	points := flatStage(10_000, 50, 0)

	result, ok := Predict(points, nil, coefficients)
	require.True(t, ok, "Predict() ok")

	sinTheta, cosTheta := gradientTrig(0)
	speed := poweredSpeedMetresPerSecond(coefficients.crr(surface.KindAsphalt), sinTheta, cosTheta, coefficients)
	assert.InDelta(t, 10_000/speed, result.MovingSeconds, 1e-6, "flat asphalt moving time")
}

func TestPredictSteadyClimbAgreesWithGravityPowerBalanceAndIsMonotonic(t *testing.T) {
	coefficients := testCoefficients()
	const ascentMetres = 500.0
	// A steady five percent climb over its whole length.
	distance := ascentMetres / 0.05
	points := sampledStage(distance, 200, func(d float64) float64 { return d * 0.05 })

	result, ok := Predict(points, nil, coefficients)
	require.True(t, ok, "Predict() ok")

	// Gravity alone, ignoring the drag and rolling correction: a lower bound
	// on the true time, since both resist the climb further.
	gravityOnlySeconds := ascentMetres * coefficients.MassKG * gravityMetresPerSecondSquared /
		(coefficients.PowerWatts * coefficients.DriveEfficiency)
	assert.Greater(t, result.MovingSeconds, gravityOnlySeconds, "moving time should exceed the gravity-only estimate")
	assert.InEpsilon(t, gravityOnlySeconds, result.MovingSeconds, 0.2,
		"moving time should stay within the drag and rolling correction")

	morePower := coefficients
	morePower.PowerWatts = coefficients.PowerWatts * 1.5
	fasterResult, ok := Predict(points, nil, morePower)
	require.True(t, ok, "Predict() ok with more power")
	assert.Less(t, fasterResult.MovingSeconds, result.MovingSeconds, "more power should climb faster")

	moreMass := coefficients
	moreMass.MassKG = coefficients.MassKG * 1.5
	slowerResult, ok := Predict(points, nil, moreMass)
	require.True(t, ok, "Predict() ok with more mass")
	assert.Greater(t, slowerResult.MovingSeconds, result.MovingSeconds, "more mass should climb slower")
}

func TestPredictPointDensityDoesNotMateriallyChangeTime(t *testing.T) {
	coefficients := testCoefficients()
	hill := func(d float64) float64 { return 80 * math.Sin(d/1500) }

	coarse, ok := Predict(sampledStage(20_000, 100, hill), nil, coefficients)
	require.True(t, ok, "Predict() ok for the coarse profile")
	dense, ok := Predict(sampledStage(20_000, 200, hill), nil, coefficients)
	require.True(t, ok, "Predict() ok for the doubled-density profile")

	assert.InEpsilon(t, coarse.MovingSeconds, dense.MovingSeconds, 0.01,
		"doubling point density should change the time only negligibly")
}

func TestPredictNeverExceedsTheDescentCap(t *testing.T) {
	coefficients := testCoefficients()
	// A steep, sustained descent: gravity alone would push well past the cap.
	points := sampledStage(2_000, 50, func(d float64) float64 { return 200 - d*0.1 })

	result, ok := Predict(points, nil, coefficients)
	require.True(t, ok, "Predict() ok")

	impliedSpeed := 2_000 / result.MovingSeconds
	assert.LessOrEqual(t, impliedSpeed, coefficients.DescentCapMetresPerSecond+1e-9,
		"a descent must never be credited a speed above the configured cap")
}

// A gradient just past the coasting cutoff, on rough enough ground, can leave
// rolling resistance stronger than the gravity pulling a freewheeling bike
// forward. Regression for a defect where that single borderline segment
// credited a near-stationary crawl and inflated a whole stage's time by an
// order of magnitude.
func TestPredictFallsBackToPedallingWhenCoastingWouldStall(t *testing.T) {
	coefficients := testCoefficients()
	coefficients.CrrBySurface = map[surface.Kind]float64{surface.KindAsphalt: 0.012}
	// Just past the -1% cutoff: rolling resistance at Crr 0.012 exceeds the
	// gravity component of a 1.2% descent, so coasting alone would stall.
	points := sampledStage(1_000, 20, func(d float64) float64 { return -0.012 * d })

	result, ok := Predict(points, nil, coefficients)
	require.True(t, ok, "Predict() ok")

	// A rider pedalling a shallow, rough downhill still covers it in minutes,
	// not the tens of minutes a stalled crawl would produce.
	assert.Less(t, result.MovingSeconds, 120.0, "a borderline segment must not dominate the stage's time")
}

func TestPredictTerminatesOnAVerticalWall(t *testing.T) {
	coefficients := testCoefficients()
	// One metre of run for five hundred metres of rise: as steep as geometry
	// gets without a zero-length segment.
	run, rise := 1.0, 500.0
	points := []route.Point{
		{Longitude: 0, Elevation: floatPtr(0)},
		{Longitude: metresToLongitudeDegrees(run), Elevation: floatPtr(rise)}, //nolint:modernize // see floatPtr's doc comment
	}

	result, ok := Predict(points, nil, coefficients)
	require.True(t, ok, "Predict() ok")
	assert.Positive(t, result.MovingSeconds, "a wall still gets a finite, positive time")
	assert.False(t, math.IsInf(result.MovingSeconds, 0) || math.IsNaN(result.MovingSeconds), "no infinity or NaN")
}

func TestPredictHandlesAZeroLengthSegment(t *testing.T) {
	coefficients := testCoefficients()
	point := route.Point{Longitude: 0, Elevation: floatPtr(10)}

	result, ok := Predict([]route.Point{point, point}, nil, coefficients)
	require.True(t, ok, "Predict() ok")
	assert.Zero(t, result.MovingSeconds, "a duplicate point contributes no time")
}

func TestPoweredSpeedBottomsOutRatherThanDivergingWhenNoBracketedSpeedSuffices(t *testing.T) {
	// Coefficients far outside anything Load would accept, chosen so that even
	// the fastest speed the solver brackets cannot absorb the climb: the
	// pathology the fixed bracket exists to survive rather than assume away.
	coefficients := Coefficients{
		MassKG:                    100_000,
		PowerWatts:                20,
		DriveEfficiency:           1,
		CdAM2:                     0.001,
		AirDensityKGPerM3:         1.2,
		DescentCutoffPercent:      -1,
		DescentCapMetresPerSecond: 20,
		CrrBySurface:              map[surface.Kind]float64{surface.KindAsphalt: 0.005},
	}
	sinTheta, cosTheta := gradientTrig(20)

	speed := poweredSpeedMetresPerSecond(coefficients.crr(surface.KindAsphalt), sinTheta, cosTheta, coefficients)
	assert.InDelta(t, minSolveSpeedMetresPerSecond, speed, 1e-12)
}

func TestPredictSurfaceSelectsRollingResistance(t *testing.T) {
	coefficients := testCoefficients()
	points := flatStage(10_000, 50, 0)

	asphalt, ok := Predict(points, kindsAll(len(points), surface.KindAsphalt), coefficients)
	require.True(t, ok, "Predict() ok for asphalt")
	gravel, ok := Predict(points, kindsAll(len(points), surface.KindGravel), coefficients)
	require.True(t, ok, "Predict() ok for gravel")
	unknown, ok := Predict(points, kindsAll(len(points), surface.KindUnknown), coefficients)
	require.True(t, ok, "Predict() ok for unknown")
	uncached, ok := Predict(points, nil, coefficients)
	require.True(t, ok, "Predict() ok with no classification")

	assert.Less(t, asphalt.MovingSeconds, gravel.MovingSeconds, "asphalt's lower Crr should be faster")
	assert.InDelta(t, asphalt.MovingSeconds, unknown.MovingSeconds, 1e-9, "unknown should match asphalt")
	assert.InDelta(t, asphalt.MovingSeconds, uncached.MovingSeconds, 1e-9, "no classification should match asphalt")
}

func TestPredictReportsNoUsableElevation(t *testing.T) {
	coefficients := testCoefficients()
	points := []route.Point{{Longitude: 0}, {Longitude: metresToLongitudeDegrees(1_000)}}

	result, ok := Predict(points, nil, coefficients)
	assert.False(t, ok, "Predict() ok without elevation")
	assert.Zero(t, result, "Predict() result without elevation")
}

func TestPredictReportsNoUsableElevationWithFewerThanTwoPoints(t *testing.T) {
	coefficients := testCoefficients()

	_, ok := Predict([]route.Point{{Longitude: 0, Elevation: floatPtr(0)}}, nil, coefficients)
	assert.False(t, ok, "Predict() ok with a single point")
}
