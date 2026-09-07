package powerestimate_test

import (
	"slices"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/powerestimate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func start() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

// ride records one sample a second at a constant speed up a constant grade.
func ride(seconds int, speedMS, grade float64) []powerestimate.Sample {
	samples := make([]powerestimate.Sample, seconds)
	for index := range samples {
		distance := speedMS * float64(index)
		samples[index] = powerestimate.Sample{
			At:             start().Add(time.Duration(index) * time.Second),
			DistanceMetres: distance,
			AltitudeMetres: 100 + distance*grade,
		}
	}

	return samples
}

// closedForm is the model written out by hand, which is what the acceptance
// criterion measures the implementation against.
func closedForm(speedMS, grade, mass float64) float64 {
	weight := mass * 9.80665

	return speedMS * (weight*grade + weight*0.005 + 0.5*1.225*0.32*speedMS*speedMS)
}

// The acceptance criterion: on a constant grade at a constant speed the
// estimate matches the closed form within a few per cent.
func TestSeriesMatchesTheClosedFormOnASteadyClimb(t *testing.T) {
	t.Parallel()
	for name, grade := range map[string]float64{
		"flat":           0,
		"a gentle rise":  0.02,
		"a proper climb": 0.08,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			estimates, ok := powerestimate.Series(ride(300, 7.5, grade), 82)
			require.True(t, ok, "a steady ride yields an estimate")

			mean, hasMean := powerestimate.Average(estimates)
			require.True(t, hasMean)
			want := closedForm(7.5, grade, 82)
			assert.InEpsilon(t, want, mean, 0.03, "within a few per cent of the closed form")
		})
	}
}

// A rider freewheeling downhill puts nothing in, and the model has no way to
// say they are taking something out.
func TestSeriesClampsADescentToZeroRatherThanNegativeWatts(t *testing.T) {
	t.Parallel()
	estimates, ok := powerestimate.Series(ride(300, 12, -0.08), 82)
	require.True(t, ok)

	for index, estimate := range estimates {
		assert.GreaterOrEqual(t, estimate.Watts, 0.0, "sample %d", index)
	}
	mean, hasMean := powerestimate.Average(estimates)
	require.True(t, hasMean)
	assert.Zero(t, mean, "a steep enough descent costs nothing at all")
}

// The first sample has no step behind it to measure a speed over, so it carries
// no estimate rather than a zero the chart would draw as a moment of rest.
func TestSeriesLeavesTheFirstSampleWithoutAnEstimate(t *testing.T) {
	t.Parallel()
	estimates, ok := powerestimate.Series(ride(10, 7.5, 0.02), 82)
	require.True(t, ok)

	require.Len(t, estimates, 10)
	assert.False(t, estimates[0].Known, "nothing precedes the first sample")
	assert.True(t, estimates[1].Known)
}

// A recorder that paused leaves a step no speed can be worked out across, and
// the stretch after it starts again with no acceleration behind it.
func TestSeriesWillNotCrossARecordingGap(t *testing.T) {
	t.Parallel()
	samples := []powerestimate.Sample{
		{At: start(), DistanceMetres: 0, AltitudeMetres: 100},
		{At: start().Add(time.Second), DistanceMetres: 7.5, AltitudeMetres: 100},
		{At: start().Add(time.Hour), DistanceMetres: 15, AltitudeMetres: 100},
		{At: start().Add(time.Hour + time.Second), DistanceMetres: 22.5, AltitudeMetres: 100},
	}

	estimates, ok := powerestimate.Series(samples, 82)
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
func TestSeriesIsNotInflatedByRecorderNoise(t *testing.T) {
	t.Parallel()
	const speedMS, mass = 7.0, 82.0
	clean := ride(1200, speedMS, 0)
	noisy := slices.Clone(clean)
	// A deterministic wobble: a metre of distance error either way and the
	// barometer's own 0.2 m quantisation, neither of which the rider did.
	for index := range noisy {
		noisy[index].DistanceMetres += []float64{0, 1, -1, 0.5, -0.5, 1, -1}[index%7]
		noisy[index].AltitudeMetres += []float64{0, 0.2, -0.2, 0.2, 0, -0.2, 0.4}[index%7]
	}

	quiet, ok := powerestimate.Series(clean, mass)
	require.True(t, ok)
	rough, ok := powerestimate.Series(noisy, mass)
	require.True(t, ok)
	quietMean, ok := powerestimate.Average(quiet)
	require.True(t, ok)
	roughMean, ok := powerestimate.Average(rough)
	require.True(t, ok)

	assert.InDelta(t, closedForm(speedMS, 0, mass), quietMean, 1,
		"the clean track is the closed form")
	assert.InDelta(t, quietMean, roughMean, 0.1*quietMean,
		"and noise the rider never rode moves the mean by less than a tenth")
}

// The altitude either side of a pause is minutes of barometric drift apart, and
// the distance between them is not a slope the rider ever rode.
func TestGradeIsNotMeasuredAcrossARecordingGap(t *testing.T) {
	t.Parallel()
	// Flat before the pause, flat after it, and forty metres higher afterwards —
	// a coffee stop at the top of a hill the recorder never saw.
	before := ride(120, 7.5, 0)
	after := ride(120, 7.5, 0)
	for index := range after {
		after[index].At = before[len(before)-1].At.Add(time.Hour + time.Duration(index)*time.Second)
		after[index].DistanceMetres += before[len(before)-1].DistanceMetres
		after[index].AltitudeMetres += 40
	}
	samples := slices.Concat(before, after)

	estimates, ok := powerestimate.Series(samples, 82)
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
func TestSeriesHandlesAStretchShorterThanTheGradeWindow(t *testing.T) {
	t.Parallel()
	stationary := make([]powerestimate.Sample, 600)
	for index := range stationary {
		stationary[index] = powerestimate.Sample{
			At:             start().Add(time.Duration(index) * time.Second),
			DistanceMetres: 0,
			AltitudeMetres: 100,
		}
	}

	estimates, ok := powerestimate.Series(stationary, 82)
	require.True(t, ok, "the samples are still samples")
	for index, estimate := range estimates {
		assert.Zero(t, estimate.Watts, "sample %d: standing still costs nothing", index)
	}
}

func TestSeriesNeedsAMassAndMoreThanOneSample(t *testing.T) {
	t.Parallel()
	_, ok := powerestimate.Series(ride(300, 7.5, 0), 0)
	assert.False(t, ok, "without a mass there is nothing to accelerate or lift")

	_, ok = powerestimate.Series(ride(1, 7.5, 0), 82)
	assert.False(t, ok, "one sample is not a track")
}

func TestAverageIsAbsentWhenNothingWasEstimated(t *testing.T) {
	t.Parallel()
	_, ok := powerestimate.Average([]powerestimate.Estimate{{}, {}})
	assert.False(t, ok)
}

// The smoothing is what keeps barometric noise from reading as a wall: a flat
// ride recorded with a jittery altimeter still estimates as a flat ride.
func TestGradeIsSmoothedAcrossAltitudeNoise(t *testing.T) {
	t.Parallel()
	jittery := ride(300, 7.5, 0)
	for index := range jittery {
		if index%2 == 1 {
			jittery[index].AltitudeMetres += 0.5
		}
	}

	estimates, ok := powerestimate.Series(jittery, 82)
	require.True(t, ok)
	mean, hasMean := powerestimate.Average(estimates)
	require.True(t, hasMean)
	assert.InEpsilon(t, closedForm(7.5, 0, 82), mean, 0.05,
		"half a metre of jitter every second is not a one-in-fifteen ramp")
}
