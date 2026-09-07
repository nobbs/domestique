package measure_test

import (
	"slices"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedForm is the model written out by hand, which is what the acceptance
// criterion measures the implementation against.
func closedForm(speedMS, grade, mass float64) float64 {
	weight := mass * 9.80665

	return speedMS * (weight*grade + weight*0.005 + 0.5*1.225*0.32*speedMS*speedMS)
}

// The acceptance criterion: on a constant grade at a constant speed the
// estimate matches the closed form within a few per cent.
func TestEstimateSeriesMatchesTheClosedFormOnASteadyClimb(t *testing.T) {
	t.Parallel()
	for name, grade := range map[string]float64{
		"flat":           0,
		"a gentle rise":  0.02,
		"a proper climb": 0.08,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			estimates, _, ok := measure.EstimateSeries(ramp(300, 7.5, grade), 82)
			require.True(t, ok, "a steady ride yields an estimate")

			mean, hasMean := measure.MeanEstimate(estimates)
			require.True(t, hasMean)
			want := closedForm(7.5, grade, 82)
			assert.InEpsilon(t, want, mean, 0.03, "within a few per cent of the closed form")
		})
	}
}

// A rider freewheeling downhill puts nothing in, and the model has no way to
// say they are taking something out.
func TestEstimateSeriesClampsADescentToZeroRatherThanNegativeWatts(t *testing.T) {
	t.Parallel()
	estimates, _, ok := measure.EstimateSeries(ramp(300, 12, -0.08), 82)
	require.True(t, ok)

	for index, estimate := range estimates {
		assert.GreaterOrEqual(t, estimate.Watts, 0.0, "sample %d", index)
	}
	mean, hasMean := measure.MeanEstimate(estimates)
	require.True(t, hasMean)
	assert.Zero(t, mean, "a steep enough descent costs nothing at all")
}

// The first sample has no step behind it to measure a speed over, so it carries
// no estimate rather than a zero the chart would draw as a moment of rest.
func TestEstimateSeriesLeavesTheFirstSampleWithoutAnEstimate(t *testing.T) {
	t.Parallel()
	estimates, _, ok := measure.EstimateSeries(ramp(10, 7.5, 0.02), 82)
	require.True(t, ok)

	require.Len(t, estimates, 10)
	assert.False(t, estimates[0].Known, "nothing precedes the first sample")
	assert.True(t, estimates[1].Known)
}

// A recorder that paused leaves a step no speed can be worked out across, and
// the stretch after it starts again with no acceleration behind it.
func TestEstimateSeriesWillNotCrossARecordingGap(t *testing.T) {
	t.Parallel()
	samples := []measure.Sample{
		{At: start(), DistanceMetres: 0, AltitudeMetres: 100},
		{At: start().Add(time.Second), DistanceMetres: 7.5, AltitudeMetres: 100},
		{At: start().Add(time.Hour), DistanceMetres: 15, AltitudeMetres: 100},
		{At: start().Add(time.Hour + time.Second), DistanceMetres: 22.5, AltitudeMetres: 100},
	}

	estimates, _, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)
	assert.True(t, estimates[1].Known, "before the pause")
	assert.False(t, estimates[2].Known, "the step across the pause is not a speed")
	assert.True(t, estimates[3].Known, "and the stretch after it starts again")
}

// The regression this model was rebuilt for: a real recorder's distance jitters
// by a metre or so a second and its barometer steps in fifths of a metre.
// Measured sample to sample that noise reaches the zero clamp, which keeps its
// positive half and throws the negative half away, and the mean comes out
// several times the truth. Measured over the window it is a rounding error.
//
// The live ride that found this averaged 441 W against Strava's 184 W.
func TestEstimateSeriesIsNotInflatedByRecorderNoise(t *testing.T) {
	t.Parallel()
	const speedMS, mass = 7.0, 82.0
	clean := ramp(1200, speedMS, 0)
	rough := noisy(clean)

	quiet, _, ok := measure.EstimateSeries(clean, mass)
	require.True(t, ok)
	roughEstimates, _, ok := measure.EstimateSeries(rough, mass)
	require.True(t, ok)
	quietMean, ok := measure.MeanEstimate(quiet)
	require.True(t, ok)
	roughMean, ok := measure.MeanEstimate(roughEstimates)
	require.True(t, ok)

	assert.InDelta(t, closedForm(speedMS, 0, mass), quietMean, 1,
		"the clean track is the closed form")
	assert.InDelta(t, quietMean, roughMean, 0.1*quietMean,
		"and noise the rider never rode moves the mean by less than a tenth")
}

// The altitude either side of a pause is minutes of barometric drift apart, and
// the distance between them is not a slope the rider ever rode.
func TestGradeIsNotMeasuredAcrossARecordingGapForEstimatedPower(t *testing.T) {
	t.Parallel()
	// Flat before the pause, flat after it, and forty metres higher afterwards —
	// a coffee stop at the top of a hill the recorder never saw.
	before := ramp(120, 7.5, 0)
	after := ramp(120, 7.5, 0)
	for index := range after {
		after[index].At = before[len(before)-1].At.Add(time.Hour + time.Duration(index)*time.Second)
		after[index].DistanceMetres += before[len(before)-1].DistanceMetres
		after[index].AltitudeMetres += 40
	}
	samples := slices.Concat(before, after)

	estimates, _, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)

	flat := closedForm(7.5, 0, 82)
	for index := len(before) + 1; index < len(samples); index++ {
		require.True(t, estimates[index].Known, "sample %d", index)
		assert.InEpsilon(t, flat, estimates[index].Watts, 0.05,
			"sample %d rides flat ground, whatever happened during the stop", index)
	}
}

// A rider stopped at the lights still records. The stretch covers no distance,
// so there is no gradient to name and nothing to charge them for — and the
// window is measured end to end rather than walked outward from every one of
// those samples, which would cost the square of the stop's length.
func TestEstimateSeriesHandlesAStretchShorterThanTheGradeWindow(t *testing.T) {
	t.Parallel()
	samples := stationary(600)
	for index := range samples {
		samples[index].AltitudeMetres = 100
	}

	estimates, _, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok, "the samples are still samples")
	for index, estimate := range estimates {
		assert.Zero(t, estimate.Watts, "sample %d: standing still costs nothing", index)
	}
}

// Every step a recorder took longer than a stop yields nothing to estimate at
// all, not merely nothing at the samples either side of it.
func TestEstimateSeriesReportsNoEstimateWhenEveryStepIsAGap(t *testing.T) {
	t.Parallel()
	samples := []measure.Sample{
		{At: start(), DistanceMetres: 0, AltitudeMetres: 100},
		{At: start().Add(time.Hour), DistanceMetres: 7.5, AltitudeMetres: 100},
	}

	estimates, quality, ok := measure.EstimateSeries(samples, 82)
	assert.False(t, ok)
	assert.False(t, estimates[1].Known)
	assert.Equal(t, measure.Quality{}, quality)
}

// A GPS glitch that reports distance running backward is not a speed this
// model can name, any more than a step across a recording gap is.
func TestEstimateSeriesSkipsAStepWhereDistanceWentBackward(t *testing.T) {
	t.Parallel()
	samples := []measure.Sample{
		{At: start(), DistanceMetres: 10, AltitudeMetres: 100},
		{At: start().Add(time.Second), DistanceMetres: 0, AltitudeMetres: 100},
	}

	estimates, _, ok := measure.EstimateSeries(samples, 82)
	assert.False(t, ok)
	assert.False(t, estimates[1].Known)
}

func TestEstimateSeriesNeedsAMassAndMoreThanOneSample(t *testing.T) {
	t.Parallel()
	_, _, ok := measure.EstimateSeries(ramp(300, 7.5, 0), 0)
	assert.False(t, ok, "without a mass there is nothing to accelerate or lift")

	_, _, ok = measure.EstimateSeries(ramp(1, 7.5, 0), 82)
	assert.False(t, ok, "one sample is not a track")
}

func TestMeanEstimateIsAbsentWhenNothingWasEstimated(t *testing.T) {
	t.Parallel()
	_, ok := measure.MeanEstimate([]measure.Estimate{{}, {}})
	assert.False(t, ok)
}

// The smoothing is what keeps barometric noise from reading as a wall: a flat
// ride recorded with a jittery altimeter still estimates as a flat ride.
func TestGradeIsSmoothedAcrossAltitudeNoiseForEstimatedPower(t *testing.T) {
	t.Parallel()
	jittery := ramp(300, 7.5, 0)
	for index := range jittery {
		if index%2 == 1 {
			jittery[index].AltitudeMetres += 0.5
		}
	}

	estimates, _, ok := measure.EstimateSeries(jittery, 82)
	require.True(t, ok)
	mean, hasMean := measure.MeanEstimate(estimates)
	require.True(t, hasMean)
	assert.InEpsilon(t, closedForm(7.5, 0, 82), mean, 0.05,
		"half a metre of jitter every second is not a one-in-fifteen ramp")
}

// A real ride's power is strongly autocorrelated sample to sample and moves by
// a few watts a second; a steady climb out and the equal descent back down is
// the cleanest case of both, its one direction change aside.
func TestEstimateSeriesQualityOnASteadyClimbReadsAsTrustworthy(t *testing.T) {
	t.Parallel()
	_, quality, ok := measure.EstimateSeries(outAndBack(500, 7.5, 0.04), 82)
	require.True(t, ok)

	assert.Greater(t, quality.Autocorrelation1, 0.95)
	assert.Less(t, quality.MeanAbsDeltaWattsPerSecond, 1.0)
}

// A slower ride's rolling resistance and drag are smaller relative to its
// weight, so the same recorder noise that never dips below zero at 7 m/s
// (see TestEstimateSeriesIsNotInflatedByRecorderNoise) pushes the windowed
// grade estimate past the point a slower rider's momentum could explain.
func TestEstimateSeriesQualityOnTheNoisyFixtureHasAPositiveClipBias(t *testing.T) {
	t.Parallel()
	_, quality, ok := measure.EstimateSeries(noisy(flat(600, 3.0)), 82)
	require.True(t, ok)

	assert.Greater(t, quality.ClipBiasWatts, 0.0)
	assert.Less(t, quality.ClipBiasWatts, 20.0)
}

// Too few known estimates to say anything statistical yields the zero value
// rather than a diagnostic computed over one or two points.
func TestEstimateSeriesQualityIsZeroWithFewerThanThreeKnownEstimates(t *testing.T) {
	t.Parallel()
	_, quality, ok := measure.EstimateSeries(ramp(2, 7.5, 0), 82)
	require.True(t, ok)

	assert.Equal(t, measure.Quality{}, quality)
}

// A pause breaks the run of adjacent known estimates a pair needs: the sample
// just after the gap has no known predecessor to pair with, so the pause
// contributes no pair to either diagnostic.
func TestEstimateSeriesQualityPairsOnlyWithinAStretch(t *testing.T) {
	t.Parallel()
	samples := []measure.Sample{
		{At: start(), DistanceMetres: 0, AltitudeMetres: 100},
		{At: start().Add(time.Second), DistanceMetres: 7.5, AltitudeMetres: 100},
		{At: start().Add(2 * time.Second), DistanceMetres: 15, AltitudeMetres: 100},
		{At: start().Add(time.Hour), DistanceMetres: 22.5, AltitudeMetres: 100},
		{At: start().Add(time.Hour + time.Second), DistanceMetres: 30, AltitudeMetres: 100},
	}

	_, quality, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)

	// Only samples 1 and 2 are an adjacent known pair; sample 4 has no known
	// predecessor across the gap. One pair alone cannot correlate.
	assert.Zero(t, quality.Autocorrelation1)
}
