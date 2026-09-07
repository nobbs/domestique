package activity_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
)

// splitStart is the moment every sample below is timed from.
func splitStart() time.Time {
	return time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC)
}

// sample is one recorded row: how far in, how long in, and what it read there.
func sample(second int, metres float64, options ...func(*activity.SampleRow)) activity.SampleRow {
	row := activity.SampleRow{
		Time:           splitStart().Add(time.Duration(second) * time.Second),
		DistanceMetres: activity.Reading{Value: metres, Known: true},
	}
	for _, option := range options {
		option(&row)
	}

	return row
}

func altitude(metres float64) func(*activity.SampleRow) {
	return func(row *activity.SampleRow) {
		row.AltitudeMetres = activity.Reading{Value: metres, Known: true}
	}
}

func heartRate(bpm float64) func(*activity.SampleRow) {
	return func(row *activity.SampleRow) {
		row.HeartRateBPM = activity.Reading{Value: bpm, Known: true}
	}
}

// A kilometre ridden in even ten-metre steps, one sample a second.
func evenKilometre(fromSecond int, fromMetres float64) []activity.SampleRow {
	rows := make([]activity.SampleRow, 0, 100)
	for step := 1; step <= 100; step++ {
		rows = append(rows, sample(fromSecond+step, fromMetres+float64(step)*10))
	}

	return rows
}

func TestSplitsCutsAStretchAtTheOdometersOwnDistance(t *testing.T) {
	rows := append([]activity.SampleRow{sample(0, 0)}, evenKilometre(0, 0)...)
	rows = append(rows, evenKilometre(100, 1000)...)

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 2)
	for _, split := range splits {
		assert.InDelta(t, 1000.0, split.DistanceMetres, 0.001)
		assert.InDelta(t, 100.0, split.MovingSeconds, 0.001)
	}
}

// The odometer, not the clock: a ride that started its count part way along is
// still cut into stretches from where its samples begin.
func TestSplitsMeasuresFromTheFirstSampleRatherThanFromNought(t *testing.T) {
	rows := append([]activity.SampleRow{sample(0, 4200)}, evenKilometre(0, 4200)...)

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 1)
	assert.InDelta(t, 1000.0, splits[0].DistanceMetres, 0.001)
}

func TestSplitsReportsWhatIsLeftOverAsAShorterLastStretch(t *testing.T) {
	rows := append([]activity.SampleRow{sample(0, 0)}, evenKilometre(0, 0)...)
	rows = append(rows, sample(130, 1300))

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 2)
	assert.InDelta(t, 1000.0, splits[0].DistanceMetres, 0.001)
	assert.InDelta(t, 300.0, splits[1].DistanceMetres, 0.001)
}

// A rider at a café is not riding slowly.
func TestSplitsLeavesOutTheSecondsTheOdometerDidNotAdvanceOver(t *testing.T) {
	rows := []activity.SampleRow{
		sample(0, 0),
		sample(60, 500),
		// Ten minutes at a standstill.
		sample(660, 500),
		sample(720, 1000),
	}

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 1)
	assert.InDelta(t, 120.0, splits[0].MovingSeconds, 0.001)
}

// Two records in the same second are ordinary at one hertz, and a device
// correcting its clock mid-ride puts a pair behind. Neither times anything,
// rather than timing nought or less.
func TestSplitsIgnoresAPairWhoseClockDidNotAdvance(t *testing.T) {
	rows := []activity.SampleRow{
		sample(0, 0),
		sample(60, 400),
		// The same second, with the odometer still advancing.
		sample(60, 700),
		// And a correction that puts the clock behind the sample before it.
		sample(30, 1000),
	}

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 1)
	assert.InDelta(t, 60.0, splits[0].MovingSeconds, 0.001)
}

func TestSplitsCountsOnlyTheClimbingPartsOfAStretch(t *testing.T) {
	rows := []activity.SampleRow{
		sample(0, 0, altitude(100)),
		sample(30, 500, altitude(150)),
		sample(60, 800, altitude(120)),
		sample(90, 1000, altitude(170)),
	}

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 1)
	assert.InDelta(t, 100.0, splits[0].AscentMetres, 0.001)
}

func TestSplitsAveragesOnlyTheSamplesThatCarriedAReading(t *testing.T) {
	rows := []activity.SampleRow{
		sample(0, 0),
		sample(30, 400, heartRate(140)),
		// The strap dropped out here rather than reading nought.
		sample(60, 700),
		sample(90, 1000, heartRate(160)),
	}

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 1)
	require.True(t, splits[0].HeartRateBPM.Known)
	assert.InDelta(t, 150.0, splits[0].HeartRateBPM.Value, 0.001)
	assert.False(t, splits[0].PowerWatts.Known, "a bicycle with no meter has no average power")
}

// A recording that stopped for a while must not hand one stretch the ground of
// several: the stretches nothing was recorded over are there and empty.
func TestSplitsKeepsAStretchesPlaceAcrossAGapInTheRecording(t *testing.T) {
	rows := []activity.SampleRow{
		sample(0, 0),
		sample(60, 900),
		sample(120, 3500),
	}

	splits := activity.Splits(rows, 1000)

	require.Len(t, splits, 4)
	assert.InDelta(t, 120.0, splits[0].MovingSeconds, 0.001)
	assert.Zero(t, splits[1].MovingSeconds)
	assert.Zero(t, splits[2].MovingSeconds)
	assert.InDelta(t, 500.0, splits[3].DistanceMetres, 0.001)
}

func TestSplitsReportsNothingForARideItCannotCut(t *testing.T) {
	rows := append([]activity.SampleRow{sample(0, 0)}, evenKilometre(0, 0)...)

	assert.Nil(t, activity.Splits(rows, 0), "a stretch of no length cuts nothing")
	assert.Nil(t, activity.Splits(nil, 1000))
	assert.Nil(t, activity.Splits(rows[:1], 1000), "one sample is no stretch")
	assert.Nil(
		t,
		activity.Splits([]activity.SampleRow{{Time: splitStart()}, {Time: splitStart()}}, 1000),
		"a ride whose samples carried no distance cannot be cut by distance",
	)
}
