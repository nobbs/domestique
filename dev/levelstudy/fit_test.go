package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/trainingload"
)

func start() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

// rideAtSpeeds records one sample a second on flat ground, holding each of
// speedsMS in turn for blockSeconds, and blocks the result to match. A fit
// needs blocks at different speeds to tell drag from rolling resistance:
// rolling grows with speed and drag with its cube.
func rideAtSpeeds(speedsMS []float64, blockSeconds int, massKG float64) Ride {
	ride := Ride{TotalMassKG: massKG}
	distance, at := 0.0, 0
	for _, speed := range speedsMS {
		block := Block{}
		for range blockSeconds {
			ride.Samples = append(ride.Samples, measure.Sample{
				At:             start().Add(time.Duration(at) * time.Second),
				DistanceMetres: distance,
				AltitudeMetres: 100,
			})
			block.Indices = append(block.Indices, at)
			distance += speed
			at++
		}
		ride.Blocks = append(ride.Blocks, block)
	}

	return ride
}

// targetedAt rewrites a ride's block targets to what the model itself
// produces at the given coefficients, so a fit run against it has a known
// answer to recover.
func targetedAt(t *testing.T, ride Ride, coefficients measure.Coefficients) Ride {
	t.Helper()
	estimates, ok := measure.EstimateSeries(ride.Samples, ride.TotalMassKG, coefficients)
	require.True(t, ok)
	for blockIndex, block := range ride.Blocks {
		total, count := 0.0, 0
		for _, index := range block.Indices {
			if estimates[index].Known {
				total += estimates[index].Watts
				count++
			}
		}
		require.Positive(t, count)
		ride.Blocks[blockIndex].TargetWatts = total / float64(count)
	}

	return ride
}

// The regression: a coasting half of the block must not lower the bridge
// target the model is scored against, which is built from the pedalling
// samples alone.
func TestMeteredBlocksOfExcludesCoastingSamplesFromTheBlockMean(t *testing.T) {
	t.Parallel()
	const block = 10 * time.Second
	power := make([]trainingload.Sample, 10)
	heartRate := make([]trainingload.Sample, 10)
	cadence := make([]trainingload.Sample, 10)
	for index := range power {
		at := start().Add(time.Duration(index) * time.Second)
		heartRate[index] = trainingload.Sample{At: at, Value: 140}
		if index < 5 {
			power[index] = trainingload.Sample{At: at, Value: 0}
			cadence[index] = trainingload.Sample{At: at, Value: 0}
		} else {
			power[index] = trainingload.Sample{At: at, Value: 200}
			cadence[index] = trainingload.Sample{At: at, Value: 80}
		}
	}

	blocks := meteredBlocksOf(power, heartRate, cadence, block)

	require.Len(t, blocks, 1)
	assert.InDelta(t, 200.0, blocks[0].WattsMeasured, 1e-9, "the coasting half must not lower the pedalling mean")
}

// A ride with no cadence recorded at all names no sample a coast, so every
// power reading is kept exactly as it was before this filter existed.
func TestMeteredBlocksOfKeepsEveryPowerSampleWithoutACadenceSeries(t *testing.T) {
	t.Parallel()
	const block = 10 * time.Second
	power := make([]trainingload.Sample, 10)
	heartRate := make([]trainingload.Sample, 10)
	for index := range power {
		at := start().Add(time.Duration(index) * time.Second)
		power[index] = trainingload.Sample{At: at, Value: 200}
		heartRate[index] = trainingload.Sample{At: at, Value: 140}
	}

	blocks := meteredBlocksOf(power, heartRate, nil, block)

	require.Len(t, blocks, 1)
	assert.InDelta(t, 200.0, blocks[0].WattsMeasured, 1e-9)
}

// The regression: cadence and power are two independent series and need not
// share exact timestamps, so a coast recorded a moment off the power reading
// it explains must still be matched to it.
func TestCoastingAtMatchesACadenceReadingOffsetFromThePowerSample(t *testing.T) {
	t.Parallel()
	cadence := []trainingload.Sample{
		{At: start().Add(4 * time.Second), Value: 80},
		{At: start().Add(6 * time.Second), Value: 0},
		{At: start().Add(8 * time.Second), Value: 80},
	}

	assert.True(t, coastingAt(start().Add(5500*time.Millisecond), cadence),
		"the nearest reading, 500ms away, names a coast")
	assert.False(t, coastingAt(start().Add(4200*time.Millisecond), cadence),
		"the nearest reading here is still pedalling")
}

// A cadence reading further away than the tolerance is not close enough to
// trust, whatever it says.
func TestCoastingAtRefusesACadenceReadingTooFarAway(t *testing.T) {
	t.Parallel()
	cadence := []trainingload.Sample{{At: start(), Value: 0}}

	assert.False(t, coastingAt(start().Add(10*time.Second), cadence))
}

func TestCoastingAtIsFalseWithNoCadenceSeries(t *testing.T) {
	t.Parallel()
	assert.False(t, coastingAt(start(), nil))
}

// The recovery test the handover asks of any fitter (§6): a target generated
// from a known answer must lead back to it.
func TestFitDragAreaRecoversTheDragAreaItsTargetsWereBuiltAt(t *testing.T) {
	t.Parallel()
	base := rideAtSpeeds([]float64{4, 6, 8, 10, 12, 14}, 300, 92)
	for _, want := range []measure.Coefficients{
		{DragArea: 0.30, RollingResistance: 0.005},
		{DragArea: 0.38, RollingResistance: 0.010},
		{DragArea: 0.60, RollingResistance: 0.010},
	} {
		rides := []Ride{targetedAt(t, base, want)}

		got, result, ok := FitDragArea(want.RollingResistance, rides)
		require.True(t, ok)

		assert.InEpsilon(t, want.DragArea, got.DragArea, 0.02, "want %v", want)
		assert.InDelta(t, want.RollingResistance, got.RollingResistance, 0, "want %v", want)
		assert.Less(t, result.RMSWatts, 1.0, "want %v", want)
	}
}

func TestEvaluateReportsTheSignedBiasOfACandidate(t *testing.T) {
	t.Parallel()
	base := rideAtSpeeds([]float64{6, 10, 14}, 300, 92)
	rides := []Ride{targetedAt(t, base, measure.Coefficients{DragArea: 0.50, RollingResistance: 0.008})}

	// A bicycle slipperier than the one the targets were built at must read
	// low, and say so with a negative bias rather than only a bare RMS.
	result, ok := Evaluate(rides, measure.Coefficients{DragArea: 0.30, RollingResistance: 0.004})
	require.True(t, ok)

	assert.Negative(t, result.BiasWatts)
	assert.Equal(t, 3, result.Blocks)
}

func TestEvaluateRefusesRidesWithNoBlockItCanEstimateAcross(t *testing.T) {
	t.Parallel()
	_, ok := Evaluate(nil, measure.DefaultCoefficients())
	assert.False(t, ok)

	empty := []Ride{{Samples: rideAtSpeeds([]float64{6}, 300, 92).Samples, TotalMassKG: 92}}
	_, ok = Evaluate(empty, measure.DefaultCoefficients())
	assert.False(t, ok)
}

func TestFitRefusesCoefficientsNoBicycleCouldHave(t *testing.T) {
	t.Parallel()
	rides := []Ride{targetedAt(t, rideAtSpeeds([]float64{6, 10}, 300, 92), measure.DefaultCoefficients())}

	_, ok := Evaluate(rides, measure.Coefficients{DragArea: -1, RollingResistance: 0.005})
	assert.False(t, ok)
}

// A search that refines around its best point must not refine its way out of
// the bounds it was given: those say what a bicycle can be, and a drag area
// beyond them is a fit that ran away rather than an answer.
func TestFitDragAreaStaysInsideTheBoundsABicycleCouldHave(t *testing.T) {
	t.Parallel()
	// Targets built at a drag area far under anything rideable, so the search
	// is pulled hard at its own floor.
	base := rideAtSpeeds([]float64{4, 8, 12}, 300, 92)
	rides := []Ride{targetedAt(t, base, measure.Coefficients{DragArea: 0.05, RollingResistance: 0.010})}

	got, _, ok := FitDragArea(0.010, rides)
	require.True(t, ok)

	assert.GreaterOrEqual(t, got.DragArea, 0.15)
}
