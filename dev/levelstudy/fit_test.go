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
	// Three seconds coasting, seven pedalling: comfortably over the block's
	// own five-second held-duration threshold on the pedalling side alone.
	for index := range power {
		at := start().Add(time.Duration(index) * time.Second)
		heartRate[index] = trainingload.Sample{At: at, Value: 140}
		if index < 3 {
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

// The regression: the heart-rate target must apply the same two exclusions
// the power side does -- coasting samples, and any reading outside the
// powered ride's own span -- or the bridge learns a heart rate the pedalling
// watts beside it never produced.
func TestMeteredBlocksOfExcludesCoastingAndOutOfSpanHeartRateFromTheTarget(t *testing.T) {
	t.Parallel()
	const block = 10 * time.Second
	power := make([]trainingload.Sample, 10)
	heartRate := make([]trainingload.Sample, 10)
	cadence := make([]trainingload.Sample, 10)
	// Three seconds coasting, seven pedalling: comfortably over the block's
	// own five-second held-duration threshold on the pedalling side alone.
	for index := range power {
		at := start().Add(time.Duration(index) * time.Second)
		if index < 3 {
			// Coasting: the power side excludes these already; the heart
			// rate here is a spike that must not enter the target either.
			power[index] = trainingload.Sample{At: at, Value: 0}
			cadence[index] = trainingload.Sample{At: at, Value: 0}
			heartRate[index] = trainingload.Sample{At: at, Value: 200}
		} else {
			power[index] = trainingload.Sample{At: at, Value: 200}
			cadence[index] = trainingload.Sample{At: at, Value: 80}
			heartRate[index] = trainingload.Sample{At: at, Value: 140}
		}
	}
	// A reading from before the powered ride started: must not shift the
	// block mean by time the model never scores.
	heartRate = append([]trainingload.Sample{{At: start().Add(-time.Hour), Value: 220}}, heartRate...)

	blocks := meteredBlocksOf(power, heartRate, cadence, block)

	require.Len(t, blocks, 1)
	assert.InDelta(t, 140.0, blocks[0].HeartRateBPM, 1e-9,
		"only the pedalling heart rate within the powered span, not the coast or the reading before it")
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

// The regression: a sensor sampling once every five seconds covers a
// five-minute block in full with a sixth of the readings a one-second sensor
// would need. Judging coverage by reading count rather than held duration
// silently dropped every ride a coarser sensor recorded.
func TestMeteredBlocksOfAdmitsABlockASparselySampledSensorFullyCovers(t *testing.T) {
	t.Parallel()
	const block = 5 * time.Minute
	const interval = 5 * time.Second
	count := int(block / interval)
	power := make([]trainingload.Sample, count)
	heartRate := make([]trainingload.Sample, count)
	for index := range power {
		at := start().Add(time.Duration(index) * interval)
		power[index] = trainingload.Sample{At: at, Value: 200}
		heartRate[index] = trainingload.Sample{At: at, Value: 140}
	}

	blocks := meteredBlocksOf(power, heartRate, nil, block)

	require.Len(t, blocks, 1, "a 5s-interval sensor still covers the whole block")
	assert.InDelta(t, 200.0, blocks[0].WattsMeasured, 1e-9)
}

// The regression: an isolated coast between two pedalling stretches must
// break held time there. Bridging across it -- pairing the readings either
// side of the dropped coast because the gap between them, once it is gone,
// sits within the ordinary gap tolerance -- can credit a block with more held
// time than either pedalling stretch holds on its own, admitting a block
// whose true pedalling coverage sits below the half-block threshold.
func TestMeteredBlocksOfDoesNotBridgeHeldDurationAcrossAnIsolatedCoast(t *testing.T) {
	t.Parallel()
	const block = 10 * time.Second
	power := make([]trainingload.Sample, 10)
	heartRate := make([]trainingload.Sample, 10)
	cadence := make([]trainingload.Sample, 10)
	// Pedalling at 0,1,2 (2 held seconds) and 4,5 (1 held second): 3 true held
	// seconds, well under the 5-second half-block threshold. Coasting at 3
	// leaves only a 2-second gap between the two pedalling stretches once it
	// is dropped -- inside the ordinary 10-second gap tolerance, so bridging
	// across it would read as 5 held seconds and wrongly admit the block.
	coasting := map[int]bool{3: true, 6: true, 7: true, 8: true, 9: true}
	for index := range power {
		at := start().Add(time.Duration(index) * time.Second)
		heartRate[index] = trainingload.Sample{At: at, Value: 140}
		if coasting[index] {
			power[index] = trainingload.Sample{At: at, Value: 0}
			cadence[index] = trainingload.Sample{At: at, Value: 0}
		} else {
			power[index] = trainingload.Sample{At: at, Value: 200}
			cadence[index] = trainingload.Sample{At: at, Value: 80}
		}
	}

	blocks := meteredBlocksOf(power, heartRate, cadence, block)

	assert.Empty(t, blocks, "true pedalling coverage sits under half the block, coast or no")
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

// The regression: the handover's own recovery test found the HR objective
// can have no interior minimum at all, collapsing CdA to the scan range's
// edge whatever the rider's true value is. A target below what even the
// least-draggy candidate bicycle would produce leaves the search with
// nowhere to turn but that edge, and the fit must refuse rather than report it.
func TestFitDragAreaRefusesAFitThatCollapsesToTheSearchBound(t *testing.T) {
	t.Parallel()
	const crr = 0.005
	ride := rideAtSpeeds([]float64{6, 10, 14}, 300, 92)
	floorEstimates, ok := measure.EstimateSeries(ride.Samples, ride.TotalMassKG,
		measure.Coefficients{DragArea: minDragArea, RollingResistance: crr})
	require.True(t, ok)
	for blockIndex, block := range ride.Blocks {
		total, count := 0.0, 0
		for _, index := range block.Indices {
			if floorEstimates[index].Known {
				total += floorEstimates[index].Watts
				count++
			}
		}
		require.Positive(t, count)
		// Below even the floor bicycle's own output: RMS only grows as drag
		// area rises, so the search has no interior minimum to find.
		ride.Blocks[blockIndex].TargetWatts = total/float64(count) - 50
	}

	_, _, fitOK := FitDragArea(crr, []Ride{ride})

	assert.False(t, fitOK, "a fit that lands on the search bound is the failure itself, not a measurement")
}

// smoothedTimes is a helper for one-second-apart timestamps, from as many
// seconds after start() as each offset names.
func smoothedTimes(offsetsSeconds ...int) []time.Time {
	times := make([]time.Time, len(offsetsSeconds))
	for index, offset := range offsetsSeconds {
		times[index] = start().Add(time.Duration(offset) * time.Second)
	}

	return times
}

// The acceptance criterion: an evenly one-second-apart series smoothed over
// a thirty-second window averages every sample within fifteen seconds either
// side, the count a fixed-sample window would also have reached.
func TestSmoothedMatchesAFixedCountOnAnEvenlySpacedSeries(t *testing.T) {
	t.Parallel()
	times := smoothedTimes(0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	values := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	out := smoothed(times, values, smoothingWindow)

	// Every sample sits within fifteen seconds of every other in a ten-second
	// span, so the window reaches the whole series and every mean is 4.5.
	for index, value := range out {
		assert.InDelta(t, 4.5, value, 1e-9, "index %d", index)
	}
}

// The regression: a gap left by coasting removed from between two clusters
// of samples must not let the window blend one cluster's values into the
// other's mean just because a fixed sample count would still reach across it.
func TestSmoothedDoesNotBlendAcrossAGapLeftByRemovedSamples(t *testing.T) {
	t.Parallel()
	// Five samples a second apart, then a 96-second gap (coasting removed),
	// then five more a second apart -- ten samples total, the same count a
	// fixed thirty-sample window would have reached from either cluster.
	times := smoothedTimes(0, 1, 2, 3, 4, 100, 101, 102, 103, 104)
	values := []float64{10, 10, 10, 10, 10, 20, 20, 20, 20, 20}

	out := smoothed(times, values, smoothingWindow)

	for index := range 5 {
		assert.InDelta(t, 10, out[index], 1e-9, "index %d: the first cluster alone", index)
	}
	for index := 5; index < 10; index++ {
		assert.InDelta(t, 20, out[index], 1e-9, "index %d: the second cluster alone", index)
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
// minimise1D must never wander outside the bounds it started from, even when
// pulled hard toward one edge: those are what a bicycle can be, not a hint.
// This is the search primitive's own contract; FitDragArea layers a further,
// stricter one on top of it -- see
// TestFitDragAreaRefusesAFitThatCollapsesToTheSearchBound -- refusing a fit
// that lands on the edge rather than reporting it.
func TestMinimise1DStaysInsideTheBoundsItStartedFrom(t *testing.T) {
	t.Parallel()
	// Targets built at a drag area far under anything rideable, so the search
	// is pulled hard at its own floor.
	base := rideAtSpeeds([]float64{4, 8, 12}, 300, 92)
	rides := []Ride{targetedAt(t, base, measure.Coefficients{DragArea: 0.05, RollingResistance: 0.010})}

	best, ok := minimise1D(minDragArea, maxDragArea, func(dragArea float64) (float64, bool) {
		result, evalOK := Evaluate(rides, measure.Coefficients{DragArea: dragArea, RollingResistance: 0.010})
		return result.RMSWatts, evalOK
	})

	require.True(t, ok)
	assert.GreaterOrEqual(t, best, minDragArea)
	assert.LessOrEqual(t, best, maxDragArea)
}

// minimise1D had no direct test of its own: only FitDragArea's coarser
// end-to-end tolerance ever exercised it. A minimum sitting between two
// coarse grid points ((0.75-0.15)/16 = 0.0375 apart, so the coarse grid alone
// can do no better than half that step, 0.01875) proves the later rounds
// narrow the answer well past the first grid's own resolution.
func TestMinimise1DRefinesPastTheCoarseGridsOwnStep(t *testing.T) {
	t.Parallel()
	const trueMinimum = 0.43125
	best, ok := minimise1D(0.15, 0.75, func(at float64) (float64, bool) {
		return (at - trueMinimum) * (at - trueMinimum), true
	})

	require.True(t, ok)
	assert.InDelta(t, trueMinimum, best, 0.001, "refinement must resolve well past the coarse grid's own step")
}
