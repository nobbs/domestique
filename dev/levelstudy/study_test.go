package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/trainingload"
)

func day(n int) time.Time { return start().Add(time.Duration(n) * 24 * time.Hour) }

// The recovery test fitBridge itself is owed: a shared slope with no noise
// must come back exactly, whatever intercept each ride happened to sit at.
func TestFitBridgeRecoversTheSlopeFromExactBlocks(t *testing.T) {
	t.Parallel()
	const slope, intercept = 2.0, -100.0
	rides := []meteredRide{
		{at: day(0), blocks: []MeasuredBlock{{HeartRateBPM: 120, WattsMeasured: slope*120 + intercept}}},
		{at: day(1), blocks: []MeasuredBlock{{HeartRateBPM: 140, WattsMeasured: slope*140 + intercept}}},
		{at: day(2), blocks: []MeasuredBlock{{HeartRateBPM: 160, WattsMeasured: slope*160 + intercept}}},
	}

	fitted, ok := fitBridge(rides, 2)

	require.True(t, ok)
	assert.InDelta(t, slope, fitted.wattsPerBPM, 1e-9)
	assert.InDelta(t, 0, fitted.residualRMS, 1e-9, "an exact linear fit leaves no residual")
	assert.Len(t, fitted.levels, 3)
	assert.Equal(t, 3, fitted.blocks)
}

func TestFitBridgeRefusesWithNoMeteredRides(t *testing.T) {
	t.Parallel()
	_, ok := fitBridge(nil, 5)
	assert.False(t, ok)
}

// No heart-rate variance names no slope to fit: every block sat at the same
// rate, so covariance and variance are both zero.
func TestFitBridgeRefusesWhenEveryHeartRateIsTheSame(t *testing.T) {
	t.Parallel()
	rides := []meteredRide{
		{at: day(0), blocks: []MeasuredBlock{{HeartRateBPM: 140, WattsMeasured: 200}}},
		{at: day(1), blocks: []MeasuredBlock{{HeartRateBPM: 140, WattsMeasured: 210}}},
	}

	_, ok := fitBridge(rides, 5)

	assert.False(t, ok)
}

// The acceptance criterion wattsAt states in its own comment: the ride on the
// date being scored is never among the levels its median is read from, even
// when the window is wide enough to have reached every ride at once.
func TestWattsAtExcludesTheRideOnItsOwnDateInTheEarlyWindow(t *testing.T) {
	t.Parallel()
	b := bridge{
		window:      5,
		wattsPerBPM: 1.0,
		levels: []rideLevel{
			{at: day(0), heartRate: 100, watts: 110}, // intercept 10
			{at: day(1), heartRate: 100, watts: 999}, // the ride being scored
			{at: day(2), heartRate: 100, watts: 110}, // intercept 10
		},
	}

	got := b.wattsAt(day(1), 150)

	assert.InDelta(t, 160, got, 1e-9, "median intercept of 10 from the other two rides, plus the slope at 150 bpm")
}

// The same exclusion holds once the window is past the early-date branch: the
// rides strictly before at, never the one on it.
func TestWattsAtExcludesTheRideOnItsOwnDatePastTheEarlyWindow(t *testing.T) {
	t.Parallel()
	b := bridge{
		window:      2,
		wattsPerBPM: 1.0,
		levels: []rideLevel{
			{at: day(0), heartRate: 100, watts: 100}, // intercept 0
			{at: day(1), heartRate: 100, watts: 100}, // intercept 0
			{at: day(2), heartRate: 100, watts: 999}, // the ride being scored
		},
	}

	got := b.wattsAt(day(2), 150)

	assert.InDelta(t, 150, got, 1e-9, "intercept 0 from the two rides before it, plus the slope at 150 bpm")
}

// The regression: excluding the ride being scored must never be compensated
// for by reading one ride further than the configured window, even in the
// early-date branch where the whole corpus is otherwise in scope.
func TestWattsAtNeverReachesPastTheWindowToReplaceTheExcludedRide(t *testing.T) {
	t.Parallel()
	b := bridge{
		window:      4,
		wattsPerBPM: 1.0,
		levels: []rideLevel{
			{at: day(0), heartRate: 100, watts: 110}, // intercept 10
			{at: day(1), heartRate: 100, watts: 120}, // intercept 20
			{at: day(2), heartRate: 100, watts: 130}, // intercept 30
			{at: day(3), heartRate: 100, watts: 999}, // the ride being scored
			// Outside the first window; must never be read. Its intercept (15)
			// sits between two already-collected ones, so reading it would
			// change which value lands at the median rank rather than simply
			// appending past it -- the earlier draft of this test could not
			// tell the two behaviours apart.
			{at: day(4), heartRate: 100, watts: 115},
		},
	}

	got := b.wattsAt(day(3), 150)

	assert.InDelta(t, 170, got, 1e-9,
		"the median of the first window's other intercepts (20), plus the slope, never reaching day(4)")
}

// A very negative intercept -- a rider far off the fitted line on a handful
// of rides -- must not carry through as a negative power.
func TestWattsAtNeverGoesNegative(t *testing.T) {
	t.Parallel()
	b := bridge{
		window:      2,
		wattsPerBPM: 1.0,
		levels:      []rideLevel{{at: day(0), heartRate: 200, watts: 50}}, // intercept -150
	}

	got := b.wattsAt(day(1), 50)

	assert.Zero(t, got)
}

// With no levels to read a median from at all, wattsAt reports no power
// rather than the slope alone.
func TestWattsAtIsZeroWithNoLevels(t *testing.T) {
	t.Parallel()
	b := bridge{window: 2, wattsPerBPM: 1.0}

	assert.Zero(t, b.wattsAt(day(0), 150))
}

// The regression: a heart-rate reading from before or after the stretch that
// carried a position -- while GPS or altitude was unavailable, say -- must
// not shift the mean the whole-ride block is judged against.
func TestMeanHeartRateOverTrackExcludesReadingsOutsideTheTracksSpan(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{
		{At: start().Add(10 * time.Second)},
		{At: start().Add(20 * time.Second)},
	}
	heartRate := []trainingload.Sample{
		{At: start(), Value: 60},                        // before the track: excluded
		{At: start().Add(15 * time.Second), Value: 140}, // within the track
		{At: start().Add(30 * time.Second), Value: 200}, // after the track: excluded
	}

	mean, ok := meanHeartRateOverTrack(heartRate, track)

	require.True(t, ok)
	assert.InDelta(t, 140, mean, 1e-9)
}

// The regression: a heart-rate reading recorded while the track's own
// cadence says the rider was coasting must not enter the target the
// pedalling-only estimate is judged against, even though it falls within the
// track's span.
func TestMeanHeartRateOverTrackExcludesReadingsDuringACoast(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{
		{At: start(), HasCadence: true, CadenceRPM: 80},
		{At: start().Add(10 * time.Second), HasCadence: true, CadenceRPM: 0}, // coasting
		{At: start().Add(20 * time.Second), HasCadence: true, CadenceRPM: 80},
	}
	heartRate := []trainingload.Sample{
		{At: start(), Value: 140},
		{At: start().Add(10 * time.Second), Value: 200}, // during the coast: excluded
		{At: start().Add(20 * time.Second), Value: 140},
	}

	mean, ok := meanHeartRateOverTrack(heartRate, track)

	require.True(t, ok)
	assert.InDelta(t, 140, mean, 1e-9, "the coasting spike must not shift the pedalling target")
}

// nought is the invalid predicate a dropout's own value names: a strap that
// wrote nought took no reading there.
func nought(r trainingload.Sample) bool { return r.Value <= 0 }

// The regression: a dropout the strap wrote as a run of noughts must break
// held time there, not bridge across it within the ordinary ten-second
// recording-gap tolerance the way a reading it simply never took would.
func TestTotalHeldSecondsDoesNotBridgeADropoutWrittenAsNought(t *testing.T) {
	t.Parallel()
	values := []float64{150, 150, 0, 0, 0, 0, 0, 0, 150, 150}
	samples := make([]trainingload.Sample, len(values))
	for index, value := range values {
		samples[index] = trainingload.Sample{At: start().Add(time.Duration(index) * time.Second), Value: value}
	}

	held := totalHeldSeconds(samples, nought)

	// One held second either side of the six-second dropout: two, not the
	// whole nine-second span bridged across it.
	assert.InDelta(t, 2.0, held, 1e-9)
}

// The same regression, bucketed per block: a dropout must not inflate the
// held duration of whichever block it falls in.
func TestHeldSecondsAcrossRunsDoesNotBridgeADropout(t *testing.T) {
	t.Parallel()
	const block = 10 * time.Second
	values := []float64{150, 150, 0, 0, 0, 0, 0, 0, 150, 150}
	samples := make([]trainingload.Sample, len(values))
	for index, value := range values {
		samples[index] = trainingload.Sample{At: start().Add(time.Duration(index) * time.Second), Value: value}
	}

	held := heldSecondsAcrossRuns(samples, nought, start(), block)

	assert.InDelta(t, 2.0, held[0], 1e-9)
}

// The regression: an excluded reading in the middle of an otherwise-held run
// must break held time there too, even when it is not a nought -- a coast, or
// a reading outside the span being judged, breaks it exactly the same way.
func TestHeldSecondsAcrossRunsDoesNotBridgeAnyExcludedReading(t *testing.T) {
	t.Parallel()
	const block = 10 * time.Second
	samples := []trainingload.Sample{
		{At: start(), Value: 150},
		{At: start().Add(time.Second), Value: 150},
		{At: start().Add(2 * time.Second), Value: 999}, // excluded, not a nought
		{At: start().Add(3 * time.Second), Value: 150},
	}
	excludeSpike := func(r trainingload.Sample) bool { return r.Value == 999 }

	held := heldSecondsAcrossRuns(samples, excludeSpike, start(), block)

	assert.InDelta(t, 1.0, held[0], 1e-9, "only the one second before the excluded reading")
}

func TestMeanHeartRateOverTrackRefusesAnEmptyTrack(t *testing.T) {
	t.Parallel()
	_, ok := meanHeartRateOverTrack([]trainingload.Sample{{At: start(), Value: 140}}, nil)
	assert.False(t, ok)
}

func TestMeanHeartRateOverTrackRefusesNoHeartRateOverTheTrack(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{{At: start()}, {At: start().Add(time.Second)}}
	heartRate := []trainingload.Sample{{At: start().Add(time.Hour), Value: 140}}

	_, ok := meanHeartRateOverTrack(heartRate, track)

	assert.False(t, ok)
}

// The regression: a whole-corpus fit that came back unavailable must read as
// unavailable, not as a fitted CdA of 0.000 -- the report's own zero value
// for a coefficient the fit never actually produced.
func TestReportStringMarksAnUnavailableWholeCorpusFit(t *testing.T) {
	t.Parallel()
	r := report{
		fitted:    map[string][]measure.Coefficients{"cda": {{DragArea: 0.40, RollingResistance: 0.008}}},
		shippedOK: false,
	}

	out := r.String()

	assert.Contains(t, out, "unavailable")
	assert.NotContains(t, out, "CdA 0.000")
}
