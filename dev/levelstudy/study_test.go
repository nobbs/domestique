package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
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

// meanOf refuses an empty series, the same as any other series with nothing
// to hold.
func TestMeanOfRefusesAnEmptySeries(t *testing.T) {
	t.Parallel()
	_, ok := meanOf(nil)
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
