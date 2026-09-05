package ridemodel

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fitted() Coefficients {
	return Coefficients{
		CalibrationCutoff: "2025-08-01",
		SecondsPerKM:      145.3578,
		SecondsPerAscentM: 3.2190,
	}
}

// The built-in pair is what every deployment predicts with before its first
// calibration, so it has to be usable and has to claim no measured error.
func TestDefaultIsAUsablePairWithNoValidation(t *testing.T) {
	coefficients := Default()

	require.NoError(t, coefficients.Validate(), "Validate() on the built-in pair")
	assert.InDelta(t, 145.3578, coefficients.SecondsPerKM, 0, "seconds_per_km")
	assert.InDelta(t, 3.2190, coefficients.SecondsPerAscentM, 0, "seconds_per_ascent_m")
	assert.NotEmpty(t, coefficients.Fingerprint, "fingerprint")
	assert.False(t, coefficients.HasValidation(), "the built-in pair has never been measured")
	assert.Zero(t, coefficients.TrainingWindowMonths, "the built-in pair was fitted over no stated window")
}

// A negative bias is a legitimate reading — the model can run fast as easily
// as slow — so it must pass rather than be rejected as out of range.
func TestValidateAcceptsAMeasuredPairIncludingANegativeBias(t *testing.T) {
	coefficients := fitted()
	coefficients.EvaluatedRides, coefficients.BiasPercent = 42, -1.20
	coefficients.MAEPercent, coefficients.P90Percent = 6.80, 14.10

	require.NoError(t, coefficients.Validate(), "Validate()")
	assert.True(t, coefficients.HasValidation(), "HasValidation() on a measured pair")
}

func TestValidateRejectsImplausibleValues(t *testing.T) {
	testCases := map[string]func(*Coefficients){
		"seconds per km zero":            func(c *Coefficients) { c.SecondsPerKM = 0 },
		"seconds per km negative":        func(c *Coefficients) { c.SecondsPerKM = -1 },
		"seconds per ascent metre zero":  func(c *Coefficients) { c.SecondsPerAscentM = 0 },
		"calibration cutoff not a date":  func(c *Coefficients) { c.CalibrationCutoff = "not-a-date" },
		"negative evaluated rides":       func(c *Coefficients) { c.EvaluatedRides = -1 },
		"negative training window":       func(c *Coefficients) { c.TrainingWindowMonths = -1 },
		"negative mean absolute error":   func(c *Coefficients) { c.MAEPercent = -0.1 },
		"negative ninetieth percentile":  func(c *Coefficients) { c.P90Percent = -0.1 },
		"bias that is not a real number": func(c *Coefficients) { c.BiasPercent = math.NaN() },
		"error that is not finite":       func(c *Coefficients) { c.MAEPercent = math.Inf(1) },
	}
	for name, invalidate := range testCases {
		t.Run(name, func(t *testing.T) {
			coefficients := fitted()
			invalidate(&coefficients)
			require.Error(t, coefficients.Validate(), "Validate() should reject %s", name)
		})
	}
}

// A pair with no cutoff is the built-in one, which was never calibrated.
func TestValidateAcceptsAnAbsentCalibrationCutoff(t *testing.T) {
	coefficients := fitted()
	coefficients.CalibrationCutoff = ""

	require.NoError(t, coefficients.Validate(), "Validate() with no cutoff")
}

func TestFingerprintFollowsTheTermsAndNothingElse(t *testing.T) {
	base := fitted().WithFingerprint()

	faster := fitted()
	faster.SecondsPerKM = 146.0
	assert.NotEqual(t, base.Fingerprint, faster.WithFingerprint().Fingerprint,
		"a changed term must invalidate every cached duration")

	steeper := fitted()
	steeper.SecondsPerAscentM = 3.5
	assert.NotEqual(t, base.Fingerprint, steeper.WithFingerprint().Fingerprint,
		"a changed ascent term must invalidate every cached duration")

	// Re-measuring a fit changes what the page reports, not what it predicts.
	remeasured := fitted()
	remeasured.EvaluatedRides, remeasured.MAEPercent = 42, 6.8
	remeasured.CalibrationCutoff, remeasured.TrainingWindowMonths = "2026-01-01", 12
	assert.Equal(t, base.Fingerprint, remeasured.WithFingerprint().Fingerprint,
		"metadata does not change a prediction, so it must not change the fingerprint")
}

// The exact case plain concatenation would get wrong: "ab"+"cdef" and
// "abcd"+"ef" concatenate to the same bytes, so a naive hash of version+data
// would give the same fingerprint to two different (version, data) pairs.
func TestFingerprintOfDoesNotCollideAcrossAVersionDataBoundaryShift(t *testing.T) {
	left := fingerprintOf("ab", []byte("cdef"))
	right := fingerprintOf("abcd", []byte("ef"))

	assert.NotEqual(t, left, right, "different (version, data) pairs must not share a fingerprint")
}

func TestValidateRejectsANonFiniteTerm(t *testing.T) {
	for name, term := range map[string]float64{"nan": math.NaN(), "inf": math.Inf(1)} {
		t.Run(name, func(t *testing.T) {
			coefficients := Default()
			coefficients.SecondsPerKM = term
			require.ErrorContains(t, coefficients.Validate(), "finite")
		})
	}
}
