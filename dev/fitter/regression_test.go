package main

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/surface"
)

// syntheticWindows generates coasting windows consistent with known crr and
// cda at a range of speeds and grades, so a fit over them has a ground truth
// to be checked against — the acceptance criterion's own wording.
func syntheticWindows(rng *rand.Rand, n int, crr, cda, noise float64) []coastingWindow {
	const massKG = 90.0
	const airDensity = 1.2
	const duration = 10.0

	windows := make([]coastingWindow, n)
	for i := range n {
		speed := 5.0 + rng.Float64()*20.0 // 5-25 m/s
		grade := -8.0 + rng.Float64()*8.0 // -8%..0%, coasting is never uphill
		sinTheta := (grade / 100) / math.Sqrt(1+(grade/100)*(grade/100))
		cosTheta := 1 / math.Sqrt(1+(grade/100)*(grade/100))
		dissipative := crr*massKG*gravityMetresPerSecondSquared*cosTheta + cda*0.5*airDensity*speed*speed
		deltaV := (-dissipative/massKG - gravityMetresPerSecondSquared*sinTheta) * duration
		if noise > 0 {
			deltaV += rng.NormFloat64() * noise
		}
		windows[i] = coastingWindow{
			DeltaSpeedMPS:   deltaV,
			MeanSpeedMPS:    speed,
			DurationSeconds: duration,
			GradePercent:    grade,
			AirDensity:      airDensity,
		}
	}

	return windows
}

func TestSolve2x2RecoversKnownCoefficients(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	windows := syntheticWindows(rng, 500, 0.006, 0.45, 0)
	observations := observationsFor(windows, 90.0)

	crr, cda, ratio := solve2x2(observations)
	assert.InDelta(t, 0.006, crr, 0.0005)
	assert.InDelta(t, 0.45, cda, 0.02)
	assert.Less(t, ratio, maxAcceptableConditionRatio)
}

func TestIrlsFitIsRobustToAFewCorruptedWindows(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	windows := syntheticWindows(rng, 500, 0.006, 0.45, 0.01)
	// Corrupt 5% of windows with implausible deceleration, as a light brake
	// tap the upstream physical-plausibility filter did not catch would.
	for i := 0; i < len(windows); i += 20 {
		windows[i].DeltaSpeedMPS -= 5.0
	}
	observations := observationsFor(windows, 90.0)

	crr, cda, _ := irlsFit(observations)
	assert.InDelta(t, 0.006, crr, 0.001)
	assert.InDelta(t, 0.45, cda, 0.03)
}

func TestSolve2x2ReportsIllConditioningWhenSpeedBarelyVaries(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	windows := make([]coastingWindow, 200)
	for i := range windows {
		windows[i] = coastingWindow{
			DeltaSpeedMPS:   -0.05 + rng.Float64()*0.01,
			MeanSpeedMPS:    14.0 + rng.Float64()*0.2, // nearly one speed
			DurationSeconds: 10.0,
			GradePercent:    -1.0 + rng.Float64()*0.2, // nearly one grade
			AirDensity:      1.2,
		}
	}
	observations := observationsFor(windows, 90.0)

	_, _, ratio := solve2x2(observations)
	assert.Greater(t, ratio, maxAcceptableConditionRatio)
}

func TestRobustScaleMatchesStandardDeviationOnCleanNormalData(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	residuals := make([]float64, 2000)
	for i := range residuals {
		residuals[i] = rng.NormFloat64() * 2.0
	}
	scale := robustScale(residuals)
	assert.InDelta(t, 2.0, scale, 0.15)
}

func TestPercentileOfInterpolatesBetweenRanks(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	assert.InDelta(t, 3, percentileOf(sorted, 0.5), 1e-9)
	assert.InDelta(t, 1, percentileOf(sorted, 0), 1e-9)
	assert.InDelta(t, 5, percentileOf(sorted, 1), 1e-9)
}

func TestCrrPerSurfaceRefitsOnlyClassesWithEnoughWindows(t *testing.T) {
	rng := rand.New(rand.NewPCG(9, 10)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	asphalt := syntheticWindows(rng, 200, 0.005, 0.45, 0)
	for i := range asphalt {
		asphalt[i].Surface = surface.KindAsphalt
	}
	gravel := syntheticWindows(rng, 200, 0.012, 0.45, 0)
	for i := range gravel {
		gravel[i].Surface = surface.KindGravel
	}
	thin := syntheticWindows(rng, 5, 0.02, 0.45, 0)
	for i := range thin {
		thin[i].Surface = surface.KindGround
	}

	all := append(append(asphalt, gravel...), thin...)
	perSurface := crrPerSurface(all, 90.0, 0.45)

	require.Contains(t, perSurface, surface.KindAsphalt)
	require.Contains(t, perSurface, surface.KindGravel)
	assert.NotContains(t, perSurface, surface.KindGround, "too few windows to trust a refit")
	assert.InDelta(t, 0.005, perSurface[surface.KindAsphalt], 0.001)
	assert.InDelta(t, 0.012, perSurface[surface.KindGravel], 0.001)
}

func TestQuarterlyInterceptsGroupsByCalendarQuarterAndSkipsThinRides(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	const massKG, cda = 90.0, 0.45

	q1 := syntheticWindows(rng, 10, 0.006, cda, 0)
	for i := range q1 {
		q1[i].RideID = "q1ride"
	}
	q2 := syntheticWindows(rng, 10, 0.010, cda, 0)
	for i := range q2 {
		q2[i].RideID = "q2ride"
	}
	thin := syntheticWindows(rng, 2, 0.02, cda, 0) // under minWindowsPerRideIntercept
	for i := range thin {
		thin[i].RideID = "thinride"
	}

	rideDates := map[string]time.Time{
		"q1ride":   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		"q2ride":   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		"thinride": time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
	}

	all := append(append(q1, q2...), thin...)
	byQuarter := quarterlyIntercepts(all, rideDates, massKG, cda)
	quarters := medianByQuarter(byQuarter)

	require.Len(t, quarters, 2)
	assert.Equal(t, "2026-Q1", quarters[0].Quarter)
	assert.Equal(t, 1, quarters[0].Rides)
	assert.InDelta(t, 0.006, quarters[0].Median, 0.001)
	assert.Equal(t, "2026-Q2", quarters[1].Quarter)
	assert.Equal(t, 1, quarters[1].Rides, "the thin ride must not count toward Q2")
	assert.InDelta(t, 0.010, quarters[1].Median, 0.001)
}

func TestPercentileOfHandlesEmptyAndSingleton(t *testing.T) {
	require.InDelta(t, 0, percentileOf(nil, 0.5), 1e-9)
	require.InDelta(t, 7, percentileOf([]float64{7}, 0.5), 1e-9)
}
