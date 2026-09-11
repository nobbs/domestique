package measure_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readingsOf(samples []measure.Sample) []measure.Reading {
	readings := make([]measure.Reading, len(samples))
	for index, sample := range samples {
		readings[index] = measure.Reading{At: sample.At, Value: sample.AltitudeMetres}
	}

	return readings
}

func timesAndValues(readings []measure.Reading) (times []time.Time, values []float64) {
	times = make([]time.Time, len(readings))
	values = make([]float64, len(readings))
	for index, reading := range readings {
		times[index] = reading.At
		values[index] = reading.Value
	}

	return times, values
}

func TestStretchesMarksOneStretchForAnUnbrokenSeries(t *testing.T) {
	t.Parallel()
	times := []time.Time{start(), start().Add(time.Second), start().Add(2 * time.Second)}

	stretches := measure.Stretches(times, measure.DefaultMaxGap)

	require.Len(t, stretches, 3)
	assert.Equal(t, measure.Stretch{First: 0, Past: 3}, stretches[0])
	assert.Equal(t, stretches[0], stretches[2])
}

func TestStretchesSplitsAtAGapLongerThanMaxGap(t *testing.T) {
	t.Parallel()
	times := []time.Time{start(), start().Add(time.Second), start().Add(time.Hour)}

	stretches := measure.Stretches(times, measure.DefaultMaxGap)

	require.Len(t, stretches, 3)
	assert.Equal(t, measure.Stretch{First: 0, Past: 2}, stretches[0])
	assert.Equal(t, measure.Stretch{First: 2, Past: 3}, stretches[2])
}

func TestStretchesIsEmptyForNoSamples(t *testing.T) {
	t.Parallel()
	assert.Empty(t, measure.Stretches(nil, measure.DefaultMaxGap))
}

func TestStretchesMatchesThePowerEstimateReferenceOverTheRandomCorpus(t *testing.T) {
	t.Parallel()
	samples := randomCorpus()
	times := make([]time.Time, len(samples))
	for index, sample := range samples {
		times[index] = sample.At
	}

	stretches := measure.Stretches(times, measure.DefaultMaxGap)

	require.Len(t, stretches, len(times))
	for first := 0; first < len(times); {
		past := first + 1
		for past < len(times) && times[past].Sub(times[past-1]) <= measure.DefaultMaxGap {
			past++
		}
		for index := first; index < past; index++ {
			assert.Equal(t, measure.Stretch{First: first, Past: past}, stretches[index])
		}
		first = past
	}
}

func TestForEachHeldSkipsANonPositiveStep(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 1},
		{At: start(), Value: 2}, // zero step
		{At: start().Add(time.Second), Value: 3},
	}

	var visited int
	measure.ForEachHeld(readings, measure.DefaultMaxGap, func(float64, float64) { visited++ })

	assert.Equal(t, 1, visited)
}

func TestForEachHeldSkipsAStepBeyondMaxGap(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 1},
		{At: start().Add(time.Hour), Value: 2},
	}

	var visited int
	measure.ForEachHeld(readings, measure.DefaultMaxGap, func(float64, float64) { visited++ })

	assert.Equal(t, 0, visited)
}

func TestForEachHeldDoesNotVisitTheLastReading(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{{At: start(), Value: 1}}

	var visited int
	measure.ForEachHeld(readings, measure.DefaultMaxGap, func(float64, float64) { visited++ })

	assert.Equal(t, 0, visited)
}

func TestMeanHeldIsZeroWhenNoStepStands(t *testing.T) {
	t.Parallel()
	mean, seconds := measure.MeanHeld([]measure.Reading{{At: start(), Value: 5}}, measure.DefaultMaxGap)
	assert.InDelta(t, 0, mean, 1e-9)
	assert.InDelta(t, 0, seconds, 1e-9)
}

func TestMeanHeldWeightsByHowLongEachReadingStood(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 100},
		{At: start().Add(9 * time.Second), Value: 300},
		{At: start().Add(18 * time.Second), Value: 300},
	}

	mean, seconds := measure.MeanHeld(readings, measure.DefaultMaxGap)

	assert.InDelta(t, 18, seconds, 1e-9)
	assert.InDelta(t, (100*9+300*9)/18.0, mean, 1e-9)
}

// A stop the odometer does not advance through must not count as moving, and
// two moving steps in a row must merge into one stretch rather than two the
// caller has to know are adjacent.
func TestMovingIntervalsSkipsAStopWhereTheOdometerDidNotAdvance(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{
		{At: start(), DistanceMetres: 0},
		{At: start().Add(time.Second), DistanceMetres: 5},
		{At: start().Add(2 * time.Second), DistanceMetres: 5}, // stopped
		{At: start().Add(3 * time.Second), DistanceMetres: 5}, // still stopped
		{At: start().Add(4 * time.Second), DistanceMetres: 10},
		{At: start().Add(5 * time.Second), DistanceMetres: 15}, // merges with the step before it
	}

	intervals := measure.MovingIntervals(track)

	require.Len(t, intervals, 2)
	assert.Equal(t, measure.Interval{Start: start(), End: start().Add(time.Second)}, intervals[0])
	assert.Equal(t,
		measure.Interval{Start: start().Add(3 * time.Second), End: start().Add(5 * time.Second)}, intervals[1])
}

func TestMovingIntervalsIsEmptyForNoTrack(t *testing.T) {
	t.Parallel()
	assert.Empty(t, measure.MovingIntervals(nil))
}

func TestHeldWithinIntervalsCountsOnlyTheOverlap(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 100},
		{At: start().Add(10 * time.Second), Value: 100},
	}
	intervals := []measure.Interval{{Start: start().Add(2 * time.Second), End: start().Add(6 * time.Second)}}

	held := measure.HeldWithinIntervals(readings, measure.DefaultMaxGap, intervals)

	assert.InDelta(t, 4, held, 1e-9)
}

func TestHeldWithinIntervalsIsZeroOutsideEveryInterval(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 100},
		{At: start().Add(time.Second), Value: 100},
	}
	intervals := []measure.Interval{{Start: start().Add(time.Hour), End: start().Add(2 * time.Hour)}}

	held := measure.HeldWithinIntervals(readings, measure.DefaultMaxGap, intervals)

	assert.Zero(t, held)
}

func TestGapFunctionsMatchTheTrainingLoadReferenceOverEveryBuilder(t *testing.T) {
	t.Parallel()
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			readings := readingsOf(samples)
			times, values := timesAndValues(readings)

			wantMean, wantSeconds := referenceMeanHeld(times, values)
			gotMean, gotSeconds := measure.MeanHeld(readings, measure.DefaultMaxGap)

			assert.InDelta(t, wantMean, gotMean, 1e-9)
			assert.InDelta(t, wantSeconds, gotSeconds, 1e-9)
		})
	}
}

// internal/trainingload/zones.go meanHeld, copied verbatim over a plain
// (time, value) series in place of trainingload.Sample.
func referenceMeanHeld(times []time.Time, values []float64) (mean, seconds float64) {
	total := 0.0
	for index := range len(times) - 1 {
		held := times[index+1].Sub(times[index])
		if held <= 0 || held > measure.DefaultMaxGap {
			continue
		}
		total += values[index] * held.Seconds()
		seconds += held.Seconds()
	}
	if seconds <= 0 {
		return 0, 0
	}

	return total / seconds, seconds
}

func TestRollingMeanMatchesTheTrainingLoadReferenceOverEveryBuilder(t *testing.T) {
	t.Parallel()
	const window = 30 * time.Second
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			readings := readingsOf(samples)
			times, values := timesAndValues(readings)

			wantMean, wantSeconds, wantOK := referenceRollingFourthPowerMean(times, values, window, measure.DefaultMaxGap)

			var total, weight float64
			gotSeconds, gotAny := measure.RollingMean(readings, window, measure.DefaultMaxGap, func(mean, held float64) {
				total += mean * mean * mean * mean * held
				weight += held
			})

			assert.Equal(t, wantOK, gotAny)
			assert.InDelta(t, wantSeconds, gotSeconds, 1e-9)
			if wantOK {
				gotMean := total / weight
				assert.InDelta(t, wantMean, gotMean, 1e-6)
			}
		})
	}
}

func TestRollingMeanFindsTheHighestWindowRatherThanTheHighestSample(t *testing.T) {
	t.Parallel()
	values := append(append(constantValues(100, 90), 200), constantValues(100, 90)...)
	readings := seriesOf(values)

	best, ok := bestOverRollingMean(readings, time.Minute)

	require.True(t, ok)
	assert.InDelta(t, 101.64, best, 0.5, "the spike is averaged into the minute around it")
}

func TestRollingMeanWillNotSpanARecordingGap(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 400},
		{At: start().Add(time.Second), Value: 400},
		{At: start().Add(30 * time.Minute), Value: 100},
	}

	_, ok := bestOverRollingMean(readings, time.Minute)

	assert.False(t, ok, "the two seconds before the pause are all there is")
}

func TestRollingMeanWeightsEachReadingByHowLongItStands(t *testing.T) {
	t.Parallel()
	times := make([]time.Time, 8)
	for index := range times {
		times[index] = start().Add(time.Duration(index) * 9 * time.Second)
	}
	values := append([]float64{100}, constantValues(300, 7)...)
	readings := make([]measure.Reading, len(values))
	for index, value := range values {
		readings[index] = measure.Reading{At: times[index], Value: value}
	}

	best, ok := bestOverRollingMean(readings, time.Minute)

	require.True(t, ok, "the series covers the window")
	assert.InDelta(t, 271.43, best, 0.01, "nine seconds at 100 and fifty-four at 300")
}

func TestRollingMeanMatchesRiderBestAverageForAStrictlyIncreasingSeries(t *testing.T) {
	t.Parallel()
	const window = 20 * time.Second
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			readings := readingsOf(samples)
			times, values := timesAndValues(readings)

			want, wantOK := referenceBestAverage(times, values, window, measure.DefaultMaxGap)
			got, gotOK := bestOverRollingMean(readings, window)

			assert.Equal(t, wantOK, gotOK)
			if wantOK {
				assert.InDelta(t, want, got, 1e-6)
			}
		})
	}
}

func constantValues(value float64, count int) []float64 {
	values := make([]float64, count)
	for index := range values {
		values[index] = value
	}

	return values
}

func seriesOf(values []float64) []measure.Reading {
	readings := make([]measure.Reading, len(values))
	for index, value := range values {
		readings[index] = measure.Reading{At: start().Add(time.Duration(index) * time.Second), Value: value}
	}

	return readings
}

// bestOverRollingMean is the maximum mean RollingMean ever visits, which
// TestRollingMeanMatchesRiderBestAverageForAStrictlyIncreasingSeries proves
// equals rider.BestAverage's result for a strictly increasing series.
func bestOverRollingMean(readings []measure.Reading, window time.Duration) (float64, bool) {
	best, found := 0.0, false
	measure.RollingMean(readings, window, measure.DefaultMaxGap, func(mean, _ float64) {
		if !found || mean > best {
			best, found = mean, true
		}
	})

	return best, found
}
