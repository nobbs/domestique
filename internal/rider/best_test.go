package rider_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// series is a one-per-second recording starting at a fixed moment.
func series(values ...float64) (times []time.Time, samples []float64) {
	start := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	times = make([]time.Time, len(values))
	for index := range values {
		times[index] = start.Add(time.Duration(index) * time.Second)
	}

	return times, values
}

func constant(value float64, count int) []float64 {
	values := make([]float64, count)
	for index := range values {
		values[index] = value
	}

	return values
}

func TestBestAverageFindsTheHighestWindowRatherThanTheHighestSample(t *testing.T) {
	t.Parallel()
	// A single spike over a minute of rest is not a minute held at the spike.
	values := append(constant(100, 90), 200)
	values = append(values, constant(100, 90)...)
	times, samples := series(values...)

	best, ok := rider.BestAverage(times, samples, time.Minute)
	require.True(t, ok, "a series longer than the window holds one")
	assert.InDelta(t, 101.64, best, 0.5, "the spike is averaged into the minute around it")
}

func TestBestAverageReportsNothingForASeriesShorterThanTheWindow(t *testing.T) {
	t.Parallel()
	times, values := series(constant(300, 30)...)

	_, ok := rider.BestAverage(times, values, time.Minute)
	assert.False(t, ok, "half a minute cannot answer a minute")
}

// A recorder that paused leaves its last sample standing for the whole pause.
// Counting that as an effort held is how a coffee stop becomes an FTP.
func TestBestAverageWillNotSpanARecordingGap(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	times := []time.Time{start, start.Add(time.Second), start.Add(30 * time.Minute)}
	values := []float64{400, 400, 100}

	_, ok := rider.BestAverage(times, values, time.Minute)
	assert.False(t, ok, "the two seconds before the pause are all there is")
}

// A recorder that samples unevenly would otherwise have one second weigh the
// same as the nine that follow it.
func TestBestAverageWeightsEachSampleByHowLongItStands(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	times := make([]time.Time, 8)
	for index := range times {
		times[index] = start.Add(time.Duration(index) * 9 * time.Second)
	}
	values := append([]float64{100}, constant(300, 7)...)

	best, ok := rider.BestAverage(times, values, time.Minute)
	require.True(t, ok, "the series covers the window")
	assert.InDelta(t, 271.43, best, 0.01, "nine seconds at 100 and fifty-four at 300")
}

func TestBestAverageRefusesMismatchedSeries(t *testing.T) {
	t.Parallel()
	times, _ := series(constant(1, 120)...)

	_, ok := rider.BestAverage(times, []float64{1}, time.Minute)
	assert.False(t, ok, "a value per sample or nothing")
}

func TestThresholdPowerTakesTheConventionalShare(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 285.0, rider.ThresholdPower(300), 0.001)
}

func TestValueRoundTripsThroughItsPointerForm(t *testing.T) {
	t.Parallel()
	assert.Nil(t, rider.Value{}.Pointer(), "an unset value has no pointer")
	assert.Equal(t, rider.Set(52.5), rider.FromPointer(rider.Set(52.5).Pointer()))
	assert.Equal(t, rider.Value{}, rider.FromPointer(nil))
}
