package measure_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedFormAirDensity is the handover document's barometric-pressure and
// ideal-gas formula, reproduced here rather than reaching into estimate.go's
// own unexported airDensity, so the acceptance test does not merely echo the
// implementation back at itself.
func closedFormAirDensity(altitudeMetres, temperatureCelsius float64) float64 {
	pressure := 101325 * math.Pow(1-2.25577e-5*altitudeMetres, 5.25588)

	return pressure / (287.058 * (temperatureCelsius + 273.15))
}

// closedFormAtTemperature is the model written out by hand, which is what the
// acceptance criterion measures the implementation against, at a caller-chosen
// altitude and temperature.
func closedFormAtTemperature(speedMS, grade, mass, altitudeMetres, temperatureCelsius float64) float64 {
	weight := mass * 9.80665
	density := closedFormAirDensity(altitudeMetres, temperatureCelsius)

	return speedMS * (weight*grade + weight*0.005 + 0.5*density*0.32*speedMS*speedMS)
}

// closedForm is closedFormAtTemperature at the model's default of fifteen
// degrees, which is what a fixture with no thermometer is estimated at.
func closedForm(speedMS, grade, mass, altitudeMetres float64) float64 {
	return closedFormAtTemperature(speedMS, grade, mass, altitudeMetres, 15)
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
			want := closedForm(7.5, grade, 82, 100)
			assert.InEpsilon(t, want, mean, 0.03, "within a few per cent of the closed form")
		})
	}
}

// A rider freewheeling downhill puts nothing in, and the model has no way to
// say they are taking something out.
func TestEstimateSeriesClampsADescentToZeroRatherThanNegativeWatts(t *testing.T) {
	t.Parallel()
	estimates, quality, ok := measure.EstimateSeries(ramp(300, 12, -0.08), 82)
	require.True(t, ok)

	for index, estimate := range estimates {
		assert.GreaterOrEqual(t, estimate.Watts, 0.0, "sample %d", index)
	}
	mean, hasMean := measure.MeanEstimate(estimates)
	require.True(t, hasMean)
	assert.Zero(t, mean, "a steep enough descent costs nothing at all")
	assert.Positive(t, quality.ClipBiasWatts, "and every sample of it is what the clamp added")
}

// The cadence gate is physics, checked before the numerical clamp ever runs:
// a sample the rider was not pedalling through reads no power, whatever the
// track says about grade and speed at that moment.
func TestEstimateSeriesReadsZeroWattsWhereCadenceIsKnownAndZero(t *testing.T) {
	t.Parallel()
	samples := ramp(10, 7.5, 0.08)
	samples[5].HasCadence, samples[5].CadenceRPM = true, 0

	estimates, _, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)
	require.True(t, estimates[5].Known, "the gate still yields an estimate, just a zero one")
	assert.Zero(t, estimates[5].Watts)
}

// Where a sample carries a temperature reading, it drives the model's air
// density in place of the fifteen-degree default.
func TestEstimateSeriesUsesTheSamplesTemperatureWhereKnown(t *testing.T) {
	t.Parallel()
	const speedMS, mass = 7.5, 82.0
	track := flat(300, speedMS)
	for index := range track {
		track[index].HasTemperature, track[index].TemperatureCelsius = true, 30
	}

	estimates, _, ok := measure.EstimateSeries(track, mass)
	require.True(t, ok)
	mean, hasMean := measure.MeanEstimate(estimates)
	require.True(t, hasMean)
	assert.InEpsilon(t, closedFormAtTemperature(speedMS, 0, mass, 100, 30), mean, 0.03)
}

// A ride with no cadence sensor at all is unaffected by the gate: it is the
// same series as a copy of itself with a cadence known and positive
// throughout, which never trips the gate either.
func TestEstimateSeriesIsUnchangedByARideWithNoCadenceSensor(t *testing.T) {
	t.Parallel()
	track := ramp(300, 7.5, 0.04)

	withoutSensor, withoutQuality, withoutOK := measure.EstimateSeries(track, 82)
	withSensor, withQuality, withOK := measure.EstimateSeries(withCadence(track, 80), 82)

	require.True(t, withoutOK)
	require.True(t, withOK)
	assert.Equal(t, withoutSensor, withSensor)
	assert.Equal(t, withoutQuality, withQuality)
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

	assert.InDelta(t, closedForm(speedMS, 0, mass, 100), quietMean, 1,
		"the clean track is the closed form")
	assert.InDelta(t, quietMean, roughMean, 0.1*quietMean,
		"and noise the rider never rode moves the mean by less than a tenth")
	// The derived window for this fixture is 100 m (see gradeWindowMetres),
	// more than three times the old fixed 30 m, and the mean it produces sits
	// within two tenths of a per cent of the closed form rather than merely
	// within a tenth of it.
	assert.InDelta(t, quietMean, roughMean, 0.002*quietMean,
		"a wider derived window brings the noisy mean closer still")
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

	flat := closedForm(7.5, 0, 82, 140)
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
	// The window is still derived from the track even where every step is a
	// gap and nothing else in Quality ends up non-zero: this flat two-sample
	// track has no positive altitude step, so it takes the floor.
	assert.Equal(t, measure.Quality{WindowMetres: 30}, quality)
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

// The window is derived per ride from the altimeter's own resolution:
// window = clamp(quantum / 0.002, 30, 300). See docs/specs/measurement.md
// §Gradient.
func TestEstimateSeriesDerivesTheGradeWindowFromTheAltimetersResolution(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		samples []measure.Sample
		want    float64
	}{
		"a barometer that steps in fifths of a metre gets 100 m": {
			samples: noisy(flat(1200, 7.0)),
			want:    100,
		},
		"a ramp rising 0.35 m a second gets 175 m": {
			samples: ramp(10, 7, 0.05),
			want:    175,
		},
		"metre-scale steps clamp to the 300 m ceiling": {
			samples: ramp(10, 1, 1),
			want:    300,
		},
		"two-centimetre steps clamp to the 30 m floor": {
			samples: ramp(10, 1, 0.02),
			want:    30,
		},
		"a flat ride with no positive step takes the floor": {
			samples: flat(300, 7.5),
			want:    30,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, quality, ok := measure.EstimateSeries(testCase.samples, 82)
			require.True(t, ok)
			assert.InDelta(t, testCase.want, quality.WindowMetres, 0.001)
		})
	}
}

// A step across a pause is drift, not a reading of the altimeter's own
// resolution: the only positive altitude change in this fixture sits between
// two flat stretches either side of a stop longer than DefaultMaxGap, so it
// is ignored and the flat stretches leave the ride at the floor.
func TestEstimateSeriesIgnoresAnAltitudeStepAcrossAPauseWhenDerivingTheWindow(t *testing.T) {
	t.Parallel()
	before := flat(120, 7.5)
	after := flat(120, 7.5)
	for index := range after {
		after[index].At = before[len(before)-1].At.Add(time.Hour + time.Duration(index)*time.Second)
		after[index].DistanceMetres += before[len(before)-1].DistanceMetres
		after[index].AltitudeMetres += 40
	}
	samples := slices.Concat(before, after)

	_, quality, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)
	assert.InDelta(t, 30, quality.WindowMetres, 0.001)
}

// A stretch shorter than the derived window still takes the "measure it end
// to end" branch even where the window is far larger than the old fixed
// 30 m: a coarse, metre-stepping climb derives a 300 m window, and the short
// stationary stretch recorded after a stop is measured whole rather than
// walked outward sample by sample.
func TestCentredWindowHandlesAShortStretchAtALargeDerivedWindow(t *testing.T) {
	t.Parallel()
	coarse := ramp(400, 1, 1)
	last := coarse[len(coarse)-1]
	short := make([]measure.Sample, 5)
	for index := range short {
		short[index] = measure.Sample{
			At:             last.At.Add(time.Hour + time.Duration(index)*time.Second),
			DistanceMetres: last.DistanceMetres,
			AltitudeMetres: last.AltitudeMetres,
		}
	}
	samples := slices.Concat(coarse, short)

	estimates, quality, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)
	require.InDelta(t, 300, quality.WindowMetres, 0.001)
	// The very first sample after the pause has no step behind it to measure,
	// same as after any other gap; only what follows it is checked here.
	for index := len(coarse) + 1; index < len(samples); index++ {
		require.True(t, estimates[index].Known, "sample %d", index)
		assert.Zero(t, estimates[index].Watts, "sample %d: no distance covered in the short stretch", index)
	}
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
	assert.InEpsilon(t, closedForm(7.5, 0, 82, 100), mean, 0.05,
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

// At the fixed 30 m window this fixture used to demonstrate a slower rider's
// smaller rolling resistance and drag letting recorder noise push the
// windowed grade past what their momentum could explain. The window this
// fixture derives is 100 m (see gradeWindowMetres), more than three times as
// wide, and over it the same noise averages out to nothing: this is the
// deviation docs/specs/measurement.md §Gradient Status named as resolved.
func TestEstimateSeriesQualityOnTheNoisyFixtureHasNoClipBiasAtTheDerivedWindow(t *testing.T) {
	t.Parallel()
	_, quality, ok := measure.EstimateSeries(withCadence(noisy(flat(600, 3.0)), 80), 82)
	require.True(t, ok)

	assert.Zero(t, quality.ClipBiasWatts)
}

// The Strava regression itself: the same noisy fixture, but with a cadence of
// zero throughout — nobody was pedalling, so the gate reads every sample as
// zero before the clamp ever sees the noise, and the clamp's own bias
// diagnostic has nothing left to report.
func TestEstimateSeriesReadsZeroThroughoutWhenCadenceIsZeroThroughoutDespiteRecorderNoise(t *testing.T) {
	t.Parallel()
	estimates, quality, ok := measure.EstimateSeries(withCadence(noisy(flat(600, 3.0)), 0), 82)
	require.True(t, ok)

	mean, hasMean := measure.MeanEstimate(estimates)
	require.True(t, hasMean)
	assert.Zero(t, mean)
	assert.Zero(t, quality.ClipBiasWatts)
}

// Too few known estimates to say anything statistical yields the zero value
// rather than a diagnostic computed over one or two points.
func TestEstimateSeriesQualityIsZeroWithFewerThanThreeKnownEstimates(t *testing.T) {
	t.Parallel()
	_, quality, ok := measure.EstimateSeries(ramp(2, 7.5, 0), 82)
	require.True(t, ok)

	// A flat two-sample ramp has no positive altitude step either, so the
	// window still reads as the floor.
	assert.Equal(t, measure.Quality{WindowMetres: 30}, quality)
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

// A clock that did not advance is no step the series measures over, so an
// altitude change across one is no reading of the altimeter's resolution.
func TestEstimateSeriesIgnoresAnAltitudeStepAcrossANonAdvancingClockWhenDerivingTheWindow(t *testing.T) {
	t.Parallel()
	samples := noisy(flat(600, 7))
	last := samples[len(samples)-1]
	last.AltitudeMetres += 0.01
	samples = append(samples, last)

	_, quality, ok := measure.EstimateSeries(samples, 82)
	require.True(t, ok)
	assert.InDelta(t, 100, quality.WindowMetres, 1e-6)
}
