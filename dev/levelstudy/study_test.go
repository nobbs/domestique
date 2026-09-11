package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
