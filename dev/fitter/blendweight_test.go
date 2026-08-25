package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhysicsOnlyScaleFactorInvertsTheBlendWeight pins this package's
// physics-only diagnostic to internal/ridemodel's blend weight, which is
// unexported and so cannot simply be compared against. The weight is
// recovered from Predict itself instead: zeroing the route coefficients
// leaves weight x physics, and restoring them adds (1 - weight) x a linear
// term this test computes independently, so the difference between the two
// predictions reveals the weight without either side naming it.
//
// Without this, reweighting the blend would silently turn the physics-only
// column of every benchmark into a scaled fiction, and no other test would
// notice: the hybrid and route-only columns stay correct.
func TestPhysicsOnlyScaleFactorInvertsTheBlendWeight(t *testing.T) {
	_, samplesByRide := syntheticCorpus(1)
	var samples []sampleRow
	for _, s := range samplesByRide {
		samples = s
	}
	coefficients := testCoefficients()

	physicsOnly := coefficients
	physicsOnly.SecondsPerKM, physicsOnly.SecondsPerAscentM = 0, 0
	weightedPhysics := predictHybrid(samples, &physicsOnly)
	require.Positive(t, weightedPhysics)

	blended := predictHybrid(samples, &coefficients)
	distanceKM, ascentM := distanceAndAscent(samples)
	routeTerm := coefficients.SecondsPerKM*distanceKM + coefficients.SecondsPerAscentM*ascentM
	require.Positive(t, routeTerm)

	// blended - weightedPhysics is the route half's contribution, which is
	// (1 - weight) of the full linear term.
	weight := 1 - (blended-weightedPhysics)/routeTerm

	assert.InDelta(t, physicsOnlyScaleFactor, 1/weight, 1e-6,
		"physicsOnlyScaleFactor must be the reciprocal of internal/ridemodel's blend weight")
}
