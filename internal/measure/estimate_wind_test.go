package measure_test

import (
	"math"
	"testing"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedFormWithWind is closedFormAtTemperature with the aerodynamic term's
// speed replaced by the signed relative airspeed, as
// docs/references/power-estimation-handover.md §2 requires: |v+w|·(v+w), not
// (v+w)², so a tailwind faster than the rider pushes rather than drags.
func closedFormWithWind(speedMS, grade, mass, altitudeMetres, headwindMS float64) float64 {
	weight := mass * 9.80665
	density := closedFormAirDensity(altitudeMetres, 15)
	airspeed := speedMS + headwindMS

	return speedMS * (weight*grade + weight*0.005 + 0.5*density*0.32*math.Abs(airspeed)*airspeed)
}

// A zero headwind at every sample must reproduce EstimateSeries exactly: the
// wind term drops out to the same aero force either way.
func TestEstimateSeriesWithWindMatchesTheWindFreeEstimateAtZeroWind(t *testing.T) {
	t.Parallel()
	for name, samples := range map[string][]measure.Sample{
		"flat":                    flat(300, 7.5),
		"a climb":                 ramp(300, 7.5, 0.05),
		"a descent":               ramp(300, 12, -0.08),
		"an out and back":         outAndBack(500, 7.5, 0.04),
		"a noisy flat":            noisy(flat(600, 3.0)),
		"a cadence-gated fixture": withCadence(ramp(300, 7.5, 0.08), 0),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			headwind := make([]float64, len(samples))

			withoutWind, withoutQuality, withoutOK := measure.EstimateSeries(samples, 82)
			withWind, withQuality, withOK := measure.EstimateSeriesWithWind(samples, 82, headwind)

			require.Equal(t, withoutOK, withOK)
			assert.Equal(t, withoutWind, withWind)
			assert.Equal(t, withoutQuality, withQuality)
		})
	}
}

// A nil headwind series is the same wind-free case as a zero one, spelled the
// way a caller with no wind data at all would call it.
func TestEstimateSeriesWithWindTreatsNilAsNoWind(t *testing.T) {
	t.Parallel()
	samples := ramp(300, 7.5, 0.04)

	withoutWind, withoutQuality, withoutOK := measure.EstimateSeries(samples, 82)
	withNilWind, withQuality, withOK := measure.EstimateSeriesWithWind(samples, 82, nil)

	require.Equal(t, withoutOK, withOK)
	assert.Equal(t, withoutWind, withNilWind)
	assert.Equal(t, withoutQuality, withQuality)
}

// A headwind series whose length does not match the samples is refused rather
// than read against the wrong sample.
func TestEstimateSeriesWithWindRefusesAMismatchedHeadwindLength(t *testing.T) {
	t.Parallel()
	_, _, ok := measure.EstimateSeriesWithWind(ramp(300, 7.5, 0), 82, make([]float64, 10))
	assert.False(t, ok)
}

// A tailwind faster than the rider costs less than still air: its
// aerodynamic term is signed and pushes, and only the reported power is
// clamped at nought. The sign test from
// docs/references/power-estimation-handover.md §2.
func TestEstimateSeriesWithWindTailwindFasterThanTheRiderCostsLessThanStillAir(t *testing.T) {
	t.Parallel()
	const speedMS, mass = 7.5, 82.0
	samples := flat(300, speedMS)
	headwind := make([]float64, len(samples))
	for index := range headwind {
		headwind[index] = -10 // a 10 m/s tailwind, faster than the rider
	}

	stillAir, _, ok := measure.EstimateSeries(samples, mass)
	require.True(t, ok)
	stillMean, ok := measure.MeanEstimate(stillAir)
	require.True(t, ok)

	withTailwind, _, ok := measure.EstimateSeriesWithWind(samples, mass, headwind)
	require.True(t, ok)
	tailwindMean, ok := measure.MeanEstimate(withTailwind)
	require.True(t, ok)

	assert.Less(t, tailwindMean, stillMean, "a push from behind costs less than still air")
	for index, estimate := range withTailwind {
		assert.GreaterOrEqual(t, estimate.Watts, 0.0, "sample %d: the clamp never reports negative watts", index)
	}
	assert.InEpsilon(t, closedFormWithWind(speedMS, 0, mass, 100, -10), tailwindMean, 0.03)
}

// An out-and-back against a fixed wind vector along the road rides a headwind
// out and the same wind as a tailwind back: the two halves' mean power differ
// by exactly what the closed form predicts from the sign change alone.
func TestEstimateSeriesWithWindOutAndBackHalvesDifferByTheClosedFormAmount(t *testing.T) {
	t.Parallel()
	const speedMS, mass, windMS = 7.5, 82.0, 4.0
	samples := outAndBack(300, speedMS, 0) // flat, so wind is the only thing that can split the halves
	headwind := make([]float64, len(samples))
	for index := range headwind {
		if index < 300 {
			headwind[index] = windMS // heading out: into the wind
		} else {
			headwind[index] = -windMS // heading back: the same wind from behind
		}
	}

	estimates, _, ok := measure.EstimateSeriesWithWind(samples, mass, headwind)
	require.True(t, ok)

	outMean, ok := measure.MeanEstimate(estimates[1:300])
	require.True(t, ok)
	backMean, ok := measure.MeanEstimate(estimates[300:])
	require.True(t, ok)

	wantOut := closedFormWithWind(speedMS, 0, mass, 100, windMS)
	wantBack := closedFormWithWind(speedMS, 0, mass, 100, -windMS)
	assert.InEpsilon(t, wantOut, outMean, 0.03)
	assert.InEpsilon(t, wantBack, backMean, 0.03)
	assert.InEpsilon(t, wantOut-wantBack, outMean-backMean, 0.03)
}
