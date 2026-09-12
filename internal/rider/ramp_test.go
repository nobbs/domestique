package rider_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hold is one stretch of a ride at one power, recorded once a second.
func hold(seconds int, watts float64) []float64 {
	stretch := make([]float64, seconds)
	for index := range stretch {
		stretch[index] = watts
	}

	return stretch
}

// rampSteps climbs ten watts a minute from the given start, the shape a Zwift
// ramp test drives.
func rampSteps(minutes int, firstWatts float64) []float64 {
	watts := make([]float64, 0, minutes*60)
	for step := range minutes {
		watts = append(watts, hold(60, firstWatts+10*float64(step))...)
	}

	return watts
}

// recorded times the watts one a second, the rate a trainer records at.
func recorded(watts []float64) []time.Time {
	start := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC)
	times := make([]time.Time, len(watts))
	for index := range watts {
		times[index] = start.Add(time.Duration(index) * time.Second)
	}

	return times
}

func TestRampThresholdPowerAcceptsARideShapedLikeARampTest(t *testing.T) {
	t.Parallel()
	series := rampSteps(28, 100)
	times := recorded(series)
	bestMinute, found := rider.BestAverage(times, series, time.Minute)
	require.True(t, found, "a 28-minute series holds a minute")

	watts, ok := rider.RampThresholdPower(times, series)

	require.True(t, ok, "28 minutes climbing to failure is a ramp test")
	assert.InDelta(t, bestMinute*0.75, watts, 1e-9, "75% of the best minute")
}

// The regression this shape exists for: a ramp is ridden to failure, but the
// file carries the cooldown that follows, so a real one's hardest minute ends
// eleven to sixteen minutes before its last sample. A test that the peak minute
// is the ride's last rejected every genuine ramp test in the corpus.
func TestRampThresholdPowerAcceptsARampFollowedByItsCooldown(t *testing.T) {
	t.Parallel()
	series := rampSteps(18, 100)
	series = append(series, hold(12*60, 90)...) // Twelve minutes spinning it out.
	times := recorded(series)
	bestMinute, found := rider.BestAverage(times, series, time.Minute)
	require.True(t, found, "a 30-minute series holds a minute")

	watts, ok := rider.RampThresholdPower(times, series)

	require.True(t, ok, "a cooldown after the failure does not stop it being a ramp test")
	assert.InDelta(t, bestMinute*0.75, watts, 1e-9, "still 75% of the best minute")
}

// The bound the loose shape rests on, pinned: an admitted ride can never offer
// more than 94% of the five minutes it was read over, whatever else it is. This
// easy ride is admitted, and the estimate it offers is one only a rider with no
// harder riding at all would ever be shown.
func TestRampThresholdPowerHoldsUnderAShareOfTheBestFiveMinutes(t *testing.T) {
	t.Parallel()
	series := hold(960, 50)
	series = append(series, hold(240, 100)...)
	series = append(series, hold(60, 125)...) // Around 105 W over five and 125 over one: a ratio of 1.19.
	times := recorded(series)
	bestFive, found := rider.BestAverage(times, series, 5*time.Minute)
	require.True(t, found)

	watts, ok := rider.RampThresholdPower(times, series)

	require.True(t, ok, "inside the ratio band, so the shape admits it")
	assert.LessOrEqual(t, watts, bestFive*0.9375,
		"0.75 of a minute at most 1.25 of the best five cannot exceed 0.9375 of it")
}

// A pause ends a stretch of recording, so a few valid minutes either side of one
// must not present themselves as a ride of the right length.
func TestRampThresholdPowerRejectsASeriesBrokenByAPause(t *testing.T) {
	t.Parallel()
	series := rampSteps(21, 100)
	times := recorded(series)
	// One step of eleven seconds, past the ten the recording gap allows. The
	// span still reads inside the bound and every window either side is still
	// whole, so nothing but the step itself gives the pause away.
	for index := len(times) / 2; index < len(times); index++ {
		times[index] = times[index].Add(11 * time.Second)
	}

	_, ok := rider.RampThresholdPower(times, series)

	assert.False(t, ok, "an eleven-second step is a pause, and a ramp is one stretch")
}

func TestRampThresholdPowerRejectsAnHourLongRide(t *testing.T) {
	t.Parallel()
	series := rampSteps(60, 100)

	_, ok := rider.RampThresholdPower(recorded(series), series)

	assert.False(t, ok, "an hour is well outside a ramp test's own length")
}

func TestRampThresholdPowerRejectsAHardMinuteEndingAnEasyRide(t *testing.T) {
	t.Parallel()
	series := hold(1200, 150)
	series = append(series, hold(60, 380)...) // A best five of 196 W under a best minute of 380: a ratio of 1.9.

	_, ok := rider.RampThresholdPower(recorded(series), series)

	assert.False(t, ok, "a minute nearly twice the best five is a sprint, not a ramp")
}

func TestRampThresholdPowerRejectsASeriesItCannotRead(t *testing.T) {
	t.Parallel()
	series := rampSteps(28, 100)

	_, ok := rider.RampThresholdPower(recorded(series), series[:len(series)-1])

	assert.False(t, ok, "times and watts that do not pair describe no ride")
}
