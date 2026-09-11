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

	return speedMS * (weight*grade + weight*0.005 + 0.5*density*0.36*speedMS*speedMS) / 0.977
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
			samples := ramp(300, 7.5, grade)
			estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
			require.True(t, ok, "a steady ride yields an estimate")

			mean, _, hasMean := measure.PedallingMean(samples, estimates)
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
	samples := ramp(300, 12, -0.08)
	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	require.True(t, ok)

	for index, estimate := range estimates {
		assert.GreaterOrEqual(t, estimate.Watts, 0.0, "sample %d", index)
	}
	mean, _, hasMean := measure.PedallingMean(samples, estimates)
	require.True(t, hasMean)
	assert.Zero(t, mean, "a steep enough descent costs nothing at all")
}

// The cadence gate is physics, checked before the numerical clamp ever runs:
// a sample the rider was not pedalling through reads no power, whatever the
// track says about grade and speed at that moment.
func TestEstimateSeriesReadsZeroWattsWhereCadenceIsKnownAndZero(t *testing.T) {
	t.Parallel()
	samples := ramp(10, 7.5, 0.08)
	samples[5].HasCadence, samples[5].CadenceRPM = true, 0

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
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

	estimates, ok := measure.EstimateSeries(track, mass, measure.DefaultCoefficients())
	require.True(t, ok)
	mean, _, hasMean := measure.PedallingMean(track, estimates)
	require.True(t, hasMean)
	assert.InEpsilon(t, closedFormAtTemperature(speedMS, 0, mass, 100, 30), mean, 0.03)
}

// A ride with no cadence sensor at all is unaffected by the gate: it is the
// same series as a copy of itself with a cadence known and positive
// throughout, which never trips the gate either.
func TestEstimateSeriesIsUnchangedByARideWithNoCadenceSensor(t *testing.T) {
	t.Parallel()
	track := ramp(300, 7.5, 0.04)

	withoutSensor, withoutOK := measure.EstimateSeries(track, 82, measure.DefaultCoefficients())
	withSensor, withOK := measure.EstimateSeries(withCadence(track, 80), 82, measure.DefaultCoefficients())

	require.True(t, withoutOK)
	require.True(t, withOK)
	assert.Equal(t, withoutSensor, withSensor)
}

// The first sample has no step behind it to measure a speed over, so it carries
// no estimate rather than a zero the chart would draw as a moment of rest.
func TestEstimateSeriesLeavesTheFirstSampleWithoutAnEstimate(t *testing.T) {
	t.Parallel()
	estimates, ok := measure.EstimateSeries(ramp(10, 7.5, 0.02), 82, measure.DefaultCoefficients())
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

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
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

	quiet, ok := measure.EstimateSeries(clean, mass, measure.DefaultCoefficients())
	require.True(t, ok)
	roughEstimates, ok := measure.EstimateSeries(rough, mass, measure.DefaultCoefficients())
	require.True(t, ok)
	quietMean, _, ok := measure.PedallingMean(clean, quiet)
	require.True(t, ok)
	roughMean, _, ok := measure.PedallingMean(rough, roughEstimates)
	require.True(t, ok)

	assert.InDelta(t, closedForm(speedMS, 0, mass, 100), quietMean, 1,
		"the clean track is the closed form")
	assert.InDelta(t, quietMean, roughMean, 0.1*quietMean,
		"and noise the rider never rode moves the mean by less than a tenth")
	// The derived window for this fixture is 100 m (see gradeWindowMetres),
	// more than three times the old fixed 30 m, and the mean it produces sits
	// within two tenths of a per cent of the closed form rather than merely
	// within a tenth of it. The inertial term differentiates that window speed
	// over a baseline wide enough to keep it there.
	assert.InDelta(t, quietMean, roughMean, 0.002*quietMean,
		"a wider derived window brings the noisy mean closer still")
}

// The inertial term, against the closed form written out by hand: a rider
// gaining speed pays for the momentum as well as the air and the road, at the
// mass they carry plus the wheels' own rotational share.
// See docs/references/power-estimation-handover.md §2.
func TestEstimateSeriesChargesForTheAccelerationOnFlatGround(t *testing.T) {
	t.Parallel()
	const speedMS, accelerationMSS, mass = 7.0, 0.01, 82.0
	estimates, ok := measure.EstimateSeries(accelerating(300, speedMS, accelerationMSS), mass, measure.DefaultCoefficients())
	require.True(t, ok)

	// Read away from either end, where the window stops being centred on its
	// own sample and the window mean is no longer the speed at that instant.
	for _, index := range []int{100, 150, 200} {
		speed := speedMS + accelerationMSS*float64(index)
		want := closedForm(speed, 0, mass, 100) + speed*(mass+1.5)*accelerationMSS/0.977
		assert.InEpsilon(t, want, estimates[index].Watts, 0.01,
			"the estimate at sample %d carries the inertial term", index)
	}
}

// Over a surge and the deceleration undoing it the inertial term nets to
// nothing, but the clamp refunds none of the braking, so covering the same
// ground in the same time costs a surging rider more than a steady one --
// which is what a rider accelerating away from every junction actually pays.
// See docs/specs/measurement.md §Estimated power.
func TestEstimateSeriesChargesASurgingRideAboveASteadyOne(t *testing.T) {
	t.Parallel()
	const speedMS, mass = 7.0, 82.0
	flatTrack := flat(600, speedMS)
	surgingTrack := surges(600, speedMS, 1.5)
	steady, ok := measure.EstimateSeries(flatTrack, mass, measure.DefaultCoefficients())
	require.True(t, ok)
	surging, ok := measure.EstimateSeries(surgingTrack, mass, measure.DefaultCoefficients())
	require.True(t, ok)

	steadyMean, _, ok := measure.PedallingMean(flatTrack, steady)
	require.True(t, ok)
	surgingMean, _, ok := measure.PedallingMean(surgingTrack, surging)
	require.True(t, ok)

	assert.Greater(t, surgingMean, steadyMean)
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

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	require.True(t, ok)

	flatWatts := closedForm(7.5, 0, 82, 140)
	for index := len(before) + 1; index < len(samples); index++ {
		require.True(t, estimates[index].Known, "sample %d", index)
		assert.InEpsilon(t, flatWatts, estimates[index].Watts, 0.05,
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

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
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

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	assert.False(t, ok)
	assert.False(t, estimates[1].Known)
}

// A GPS glitch that reports distance running backward is not a speed this
// model can name, any more than a step across a recording gap is.
func TestEstimateSeriesSkipsAStepWhereDistanceWentBackward(t *testing.T) {
	t.Parallel()
	samples := []measure.Sample{
		{At: start(), DistanceMetres: 10, AltitudeMetres: 100},
		{At: start().Add(time.Second), DistanceMetres: 0, AltitudeMetres: 100},
	}

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	assert.False(t, ok)
	assert.False(t, estimates[1].Known)
}

// The regression: a backward distance step within the same recording
// stretch -- an odometer reset, not a pause -- must break the grade window
// there too, the same way a recording gap already does. Without that, a
// window several samples after the reset can still expand back across it,
// combining a stretch of distance from before the reset with one from after
// into a speed neither pace on its own produced.
func TestEstimateSeriesDoesNotWindowAcrossAnOdometerReset(t *testing.T) {
	t.Parallel()
	// A steady 10 m/s pace throughout, on flat ground, but the device's own
	// distance resets partway through (index 4) rather than pausing.
	distances := []float64{0, 10, 20, 30, 5, 15, 25, 35, 45, 55}
	samples := make([]measure.Sample, len(distances))
	for index, distance := range distances {
		samples[index] = measure.Sample{
			At: start().Add(time.Duration(index) * time.Second), DistanceMetres: distance, AltitudeMetres: 100,
		}
	}

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())

	require.True(t, ok)
	require.True(t, estimates[5].Known)
	want := closedForm(10, 0, 82, 100)
	assert.InEpsilon(t, want, estimates[5].Watts, 0.03,
		"the true 10 m/s pace on both sides of the reset, not a rate blended across it")
}

func TestEstimateSeriesNeedsAMassAndMoreThanOneSample(t *testing.T) {
	t.Parallel()
	_, ok := measure.EstimateSeries(ramp(300, 7.5, 0), 0, measure.DefaultCoefficients())
	assert.False(t, ok, "without a mass there is nothing to accelerate or lift")

	_, ok = measure.EstimateSeries(ramp(1, 7.5, 0), 82, measure.DefaultCoefficients())
	assert.False(t, ok, "one sample is not a track")
}

// Coefficients that could not have come from a real bicycle are refused
// rather than quietly estimated at.
func TestEstimateSeriesRefusesCoefficientsNoBicycleCouldHave(t *testing.T) {
	t.Parallel()
	_, ok := measure.EstimateSeries(ramp(300, 7.5, 0), 82, measure.Coefficients{DragArea: -1, RollingResistance: 0.005})
	assert.False(t, ok)
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

	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	require.True(t, ok)
	// The very first sample after the pause has no step behind it to measure,
	// same as after any other gap; only what follows it is checked here.
	for index := len(coarse) + 1; index < len(samples); index++ {
		require.True(t, estimates[index].Known, "sample %d", index)
		assert.Zero(t, estimates[index].Watts, "sample %d: no distance covered in the short stretch", index)
	}
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

	estimates, ok := measure.EstimateSeries(jittery, 82, measure.DefaultCoefficients())
	require.True(t, ok)
	mean, _, hasMean := measure.PedallingMean(jittery, estimates)
	require.True(t, hasMean)
	assert.InEpsilon(t, closedForm(7.5, 0, 82, 100), mean, 0.05,
		"half a metre of jitter every second is not a one-in-fifteen ramp")
}

// The Strava regression itself: the same noisy fixture, but with a cadence of
// zero throughout — nobody was pedalling, so the gate reads every sample as
// zero and the mean is zero rather than whatever the recorder's own noise
// would otherwise have suggested.
func TestEstimateSeriesReadsZeroThroughoutWhenCadenceIsZeroThroughoutDespiteRecorderNoise(t *testing.T) {
	t.Parallel()
	samples := withCadence(noisy(flat(600, 3.0)), 0)
	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	require.True(t, ok)

	mean, share, hasMean := measure.PedallingMean(samples, estimates)
	require.True(t, hasMean)
	assert.Zero(t, mean)
	assert.Zero(t, share, "nobody pedalled a stroke of it")
}

// A bicycle produces nothing while it coasts: a coasted sample is excluded
// from the mean and from the pedalling share's numerator, but still counts
// among the ride's known samples.
func TestPedallingMeanExcludesCoastedSamplesFromTheMeanButCountsThemInTheShare(t *testing.T) {
	t.Parallel()
	samples := withCadence(ramp(10, 7.5, 0.04), 80)
	samples[5].CadenceRPM = 0 // one coasted sample out of ten
	estimates, ok := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	require.True(t, ok)
	require.Zero(t, estimates[5].Watts, "the coasted sample reads zero watts from the cadence gate itself")

	pedallingOnly, ok := measure.EstimateSeries(withCadence(ramp(10, 7.5, 0.04), 80), 82, measure.DefaultCoefficients())
	require.True(t, ok)

	mean, share, hasMean := measure.PedallingMean(samples, estimates)
	require.True(t, hasMean)
	wantMean, _, wantOK := measure.PedallingMean(samples, pedallingOnly)
	require.True(t, wantOK)
	assert.InDelta(t, wantMean, mean, 1e-9, "the coasted sample does not drag the mean toward zero")
	assert.Less(t, share, 1.0, "one sample out of the known ones was not pedalled")
}

// ok is false only where nothing was estimated at all -- an empty series or
// one whose every sample precedes what the model could work anything from.
func TestPedallingMeanRefusesAnEstimatePerSampleMismatch(t *testing.T) {
	t.Parallel()
	_, _, ok := measure.PedallingMean(nil, []measure.Estimate{{Watts: 100, Known: true}})
	assert.False(t, ok)
}

func TestPedallingMeanIsAbsentWhenNothingWasEstimated(t *testing.T) {
	t.Parallel()
	_, _, ok := measure.PedallingMean(nil, nil)
	assert.False(t, ok)

	samples := []measure.Sample{
		{At: start(), DistanceMetres: 0, AltitudeMetres: 100},
		{At: start().Add(time.Hour), DistanceMetres: 7.5, AltitudeMetres: 100},
	}
	estimates, seriesOK := measure.EstimateSeries(samples, 82, measure.DefaultCoefficients())
	require.False(t, seriesOK, "the one step in this fixture is a recording gap")
	_, _, ok = measure.PedallingMean(samples, estimates)
	assert.False(t, ok)
}
