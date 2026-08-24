package main

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

// testGravityMPS2 is standard gravity, kept local to this file's own
// synthetic fixtures rather than shared with anything solver-adjacent —
// this test only needs a physically realistic ground truth to recover.
const testGravityMPS2 = 9.80665

// syntheticObservations generates coastingObservation rows consistent with
// known crr and cda at a range of speeds and grades, so solve2x2/irlsFit have
// a ground truth to be checked against. The coasting-window framing is a
// holdover from the physical fitter #241 deleted; this package's only
// remaining regression (fitRouteCoefficients' distance/ascent fit) is linear
// in exactly the same two-regressor shape, so recovering crr/cda here still
// exercises the solver correctly.
func syntheticObservations(rng *rand.Rand, n int, crr, cda, noise float64) []coastingObservation {
	const massKG = 90.0
	const airDensity = 1.2
	const duration = 10.0

	observations := make([]coastingObservation, n)
	for i := range n {
		speed := 5.0 + rng.Float64()*20.0 // 5-25 m/s
		grade := -8.0 + rng.Float64()*8.0 // -8%..0%, coasting is never uphill
		sinTheta := (grade / 100) / math.Sqrt(1+(grade/100)*(grade/100))
		cosTheta := 1 / math.Sqrt(1+(grade/100)*(grade/100))
		dissipative := crr*massKG*testGravityMPS2*cosTheta + cda*0.5*airDensity*speed*speed
		deltaV := (-dissipative/massKG - testGravityMPS2*sinTheta) * duration
		if noise > 0 {
			deltaV += rng.NormFloat64() * noise
		}
		observations[i] = coastingObservation{
			Y:      -massKG*(deltaV/duration) - massKG*testGravityMPS2*sinTheta,
			X1:     massKG * testGravityMPS2 * cosTheta,
			X2:     0.5 * airDensity * speed * speed,
			Weight: duration,
		}
	}

	return observations
}

func TestSolve2x2RecoversKnownCoefficients(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	observations := syntheticObservations(rng, 500, 0.006, 0.45, 0)

	crr, cda, ratio := solve2x2(observations)
	assert.InDelta(t, 0.006, crr, 0.0005)
	assert.InDelta(t, 0.45, cda, 0.02)
	assert.Less(t, ratio, maxAcceptableConditionRatio)
}

func TestIrlsFitIsRobustToAFewCorruptedWindows(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	observations := syntheticObservations(rng, 500, 0.006, 0.45, 0.01)
	// Corrupt 5% of rows with an implausible Y, as a light brake tap the
	// upstream physical-plausibility filter did not catch would.
	for i := 0; i < len(observations); i += 20 {
		observations[i].Y += 5.0 * 90.0 / 10.0
	}

	crr, cda, _ := irlsFit(observations)
	assert.InDelta(t, 0.006, crr, 0.001)
	assert.InDelta(t, 0.45, cda, 0.03)
}

func TestSolve2x2ReportsIllConditioningWhenSpeedBarelyVaries(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // deterministic seed for a reproducible synthetic fixture, not a security context
	observations := make([]coastingObservation, 200)
	for i := range observations {
		speed := 14.0 + rng.Float64()*0.2 // nearly one speed
		grade := -1.0 + rng.Float64()*0.2 // nearly one grade
		sinTheta := (grade / 100) / math.Sqrt(1+(grade/100)*(grade/100))
		cosTheta := 1 / math.Sqrt(1+(grade/100)*(grade/100))
		observations[i] = coastingObservation{
			Y:      -90.0*(-0.005+rng.Float64()*0.001) - 90.0*testGravityMPS2*sinTheta,
			X1:     90.0 * testGravityMPS2 * cosTheta,
			X2:     0.5 * 1.2 * speed * speed,
			Weight: 10.0,
		}
	}

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

func TestPercentileOfHandlesEmptyAndSingleton(t *testing.T) {
	assert.InDelta(t, 0, percentileOf(nil, 0.5), 1e-9)
	assert.InDelta(t, 7, percentileOf([]float64{7}, 0.5), 1e-9)
}
