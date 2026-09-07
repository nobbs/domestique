package activity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seriesStart() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

// rows is a ride sampled every minute, carrying whatever the fields say.
func rows(readings ...SampleRow) []SampleRow {
	for index := range readings {
		readings[index].Time = seriesStart().Add(time.Duration(index) * time.Minute)
	}

	return readings
}

// A recorded series comes back one reading per sample, in the sample order, so
// a caller can lay it over the coordinates the same rows produced.
func TestSeriesReadsARecordedSeriesInSampleOrder(t *testing.T) {
	series, present := Series(rows(
		SampleRow{HeartRateBPM: Reading{Value: 121, Known: true}},
		SampleRow{HeartRateBPM: Reading{Value: 138, Known: true}},
	), SeriesHeartRate)

	require.True(t, present)
	require.Len(t, series, 2)
	assert.InDelta(t, 121.0, series[0].Value, 1e-9)
	assert.InDelta(t, 138.0, series[1].Value, 1e-9)
}

// A reading of nought is a reading — a stopped rider's cadence — and must not
// be confused with a sample the sensor missed.
func TestSeriesKeepsAZeroReadingApartFromAMissingOne(t *testing.T) {
	series, present := Series(rows(
		SampleRow{CadenceRPM: Reading{Value: 0, Known: true}},
		SampleRow{},
	), SeriesCadence)

	require.True(t, present)
	require.Len(t, series, 2)
	assert.True(t, series[0].Known, "a stopped rider still recorded a cadence")
	assert.False(t, series[1].Known, "the sensor recorded nothing here")
}

// Nothing recorded at all is reported as absent, which is what tells a bicycle
// with no meter from a meter that dropped out.
func TestSeriesReportsASeriesNothingRecorded(t *testing.T) {
	series, present := Series(rows(SampleRow{}, SampleRow{}), SeriesPower)

	assert.False(t, present)
	assert.Len(t, series, 2, "still one reading per sample")
}

func TestSeriesReadsEveryRecordedName(t *testing.T) {
	sample := rows(SampleRow{
		HeartRateBPM:       Reading{Value: 140, Known: true},
		CadenceRPM:         Reading{Value: 88, Known: true},
		PowerWatts:         Reading{Value: 210, Known: true},
		TemperatureCelsius: Reading{Value: 21, Known: true},
	})
	for name, expected := range map[SeriesName]float64{
		SeriesHeartRate:   140,
		SeriesCadence:     88,
		SeriesPower:       210,
		SeriesTemperature: 21,
	} {
		series, present := Series(sample, name)
		require.True(t, present, string(name))
		require.Len(t, series, 1, string(name))
		assert.InDelta(t, expected, series[0].Value, 1e-9, string(name))
	}
}

// Speed is worked out from the distance covered since the sample before, in
// kilometres per hour. The first sample has nothing behind it to measure.
func TestSeriesDerivesSpeedFromDistanceAgainstTime(t *testing.T) {
	series, present := Series(rows(
		SampleRow{DistanceMetres: Reading{Value: 0, Known: true}},
		SampleRow{DistanceMetres: Reading{Value: 500, Known: true}},
		SampleRow{DistanceMetres: Reading{Value: 1100, Known: true}},
	), SeriesSpeed)

	require.True(t, present)
	require.Len(t, series, 3)
	assert.False(t, series[0].Known, "nothing behind the first sample")
	assert.InDelta(t, 30.0, series[1].Value, 1e-9, "500 m in a minute")
	assert.InDelta(t, 36.0, series[2].Value, 1e-9, "600 m in a minute")
}

// An odometer that went backwards over a reset, and a pair whose clock did not
// advance, have no speed to report rather than a negative or infinite one.
func TestSeriesReportsNoSpeedOverAnImpossibleInterval(t *testing.T) {
	backwards := rows(
		SampleRow{DistanceMetres: Reading{Value: 900, Known: true}},
		SampleRow{DistanceMetres: Reading{Value: 100, Known: true}},
	)
	series, present := Series(backwards, SeriesSpeed)
	assert.False(t, present)
	assert.False(t, series[1].Known, "the odometer went backwards")

	stopped := []SampleRow{
		{Time: seriesStart(), DistanceMetres: Reading{Value: 0, Known: true}},
		{Time: seriesStart(), DistanceMetres: Reading{Value: 40, Known: true}},
	}
	series, present = Series(stopped, SeriesSpeed)
	assert.False(t, present)
	assert.False(t, series[1].Known, "the clock did not advance")
}

// A ride that recorded no distance has no speed either, and says so rather
// than reporting a ride that never moved.
func TestSeriesReportsNoSpeedWithoutADistance(t *testing.T) {
	series, present := Series(rows(SampleRow{}, SampleRow{}), SeriesSpeed)

	assert.False(t, present)
	assert.Len(t, series, 2)
}

// A name the served surface does not carry reads as nothing recorded rather
// than as another series' values.
func TestSeriesReportsAnUnknownNameAsAbsent(t *testing.T) {
	series, present := Series(rows(SampleRow{HeartRateBPM: Reading{Value: 140, Known: true}}), SeriesName("altitude"))

	assert.False(t, present)
	require.Len(t, series, 1)
	assert.False(t, series[0].Known)
}
