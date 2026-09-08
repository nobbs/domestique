package activity_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func driftStart() time.Time { return time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC) }

// series is one sensor recorded once a second, its reading given per second by
// value.
func series(seconds int, value func(second int) float64) []trainingload.Sample {
	samples := make([]trainingload.Sample, seconds)
	for index := range samples {
		samples[index] = trainingload.Sample{
			At:    driftStart().Add(time.Duration(index) * time.Second),
			Value: value(index),
		}
	}

	return samples
}

func flat(value float64) func(int) float64 {
	return func(int) float64 { return value }
}

// Ninety minutes at a steady 200 W, with the heart rate stepping from 140 to
// 154 at the halfway mark. A tenth more beats for the same watts leaves the
// ratio at 1/1.1 of itself, which is a decoupling of 9.09%, not of 10%.
func TestDecouplingReportsTheRatioLostOverTheSecondHalf(t *testing.T) {
	t.Parallel()
	const seconds = 90 * 60
	samples := activity.RideSamples{
		Power: series(seconds, flat(200)),
		HeartRate: series(seconds, func(second int) float64 {
			if second < seconds/2 {
				return 140
			}

			return 154
		}),
	}

	decoupling := samples.Decoupling(samples.HeartRate)

	require.True(t, decoupling.Known, "a ninety-minute ride with both sensors")
	assert.InDelta(t, 100.0/11, decoupling.Percent, 0.05)
}

// A ride that held its ratio has no drift, which is a decoupling of nought
// rather than an absent one.
func TestDecouplingIsZeroForARideThatHeldItsRatio(t *testing.T) {
	t.Parallel()
	const seconds = 90 * 60
	samples := activity.RideSamples{
		Power:     series(seconds, flat(200)),
		HeartRate: series(seconds, flat(140)),
	}

	decoupling := samples.Decoupling(samples.HeartRate)

	require.True(t, decoupling.Known, "Decoupling()")
	assert.InDelta(t, 0.0, decoupling.Percent, 0.001)
}

// The acceptance criterion: an estimate never stands in for a meter here.
func TestDecouplingIsAbsentWithoutMeasuredPower(t *testing.T) {
	t.Parallel()
	const seconds = 90 * 60
	samples := activity.RideSamples{HeartRate: series(seconds, flat(140))}

	assert.False(t, samples.Decoupling(samples.HeartRate).Known, "no meter, no decoupling")
}

func TestDecouplingIsAbsentForARideTooShortToHaveDrifted(t *testing.T) {
	t.Parallel()
	const seconds = 40 * 60
	samples := activity.RideSamples{
		Power:     series(seconds, flat(200)),
		HeartRate: series(seconds, flat(140)),
	}

	assert.False(t, samples.Decoupling(samples.HeartRate).Known, "under an hour, no comparison worth making")
}

func TestDecouplingIsAbsentWithoutHeartRate(t *testing.T) {
	t.Parallel()
	samples := activity.RideSamples{Power: series(90*60, flat(200))}

	assert.False(t, samples.Decoupling(samples.HeartRate).Known, "no strap, no ratio")
}

// The band is 55% to 75% of threshold power, so at 250 W it runs 137.5 W to
// 187.5 W: the samples at 160 W count and those at 240 W do not.
func TestHeatDriftReadsTheHeartRateHeldInTheEnduranceBand(t *testing.T) {
	t.Parallel()
	const seconds = 3600
	inBand := func(second int) bool { return second < seconds/2 }
	samples := activity.RideSamples{
		Power: series(seconds, func(second int) float64 {
			if inBand(second) {
				return 160
			}

			return 240
		}),
		HeartRate: series(seconds, func(second int) float64 {
			if inBand(second) {
				return 138
			}

			return 172
		}),
		Temperature: series(seconds, func(second int) float64 {
			if inBand(second) {
				return 28
			}

			return 34
		}),
	}

	drift := samples.HeatDrift(samples.HeartRate, 250)

	require.True(t, drift.Known, "HeatDrift()")
	assert.InDelta(t, 138.0, drift.HeartRateBPM, 0.001, "only the band's beats")
	assert.InDelta(t, 28.0, drift.TemperatureCelsius, 0.001, "and only its degrees")
	assert.Equal(t, seconds/2, drift.Samples)
}

func TestHeatDriftIsAbsentWithoutAThresholdPowerToPlaceTheBand(t *testing.T) {
	t.Parallel()
	const seconds = 3600
	samples := activity.RideSamples{
		Power:       series(seconds, flat(160)),
		HeartRate:   series(seconds, flat(138)),
		Temperature: series(seconds, flat(28)),
	}

	assert.False(t, samples.HeatDrift(samples.HeartRate, 0).Known, "no threshold power, no band")
}

func TestHeatDriftIsAbsentWithoutMeasuredPowerOrTemperature(t *testing.T) {
	t.Parallel()
	const seconds = 3600
	withoutPower := activity.RideSamples{
		HeartRate:   series(seconds, flat(138)),
		Temperature: series(seconds, flat(28)),
	}
	withoutTemperature := activity.RideSamples{
		Power:     series(seconds, flat(160)),
		HeartRate: series(seconds, flat(138)),
	}

	assert.False(t, withoutPower.HeatDrift(withoutPower.HeartRate, 250).Known, "an estimate never places the band")
	assert.False(t, withoutTemperature.HeatDrift(withoutTemperature.HeartRate, 250).Known, "no thermometer, no reading")
}

// A ride that only passed through the band is not a reading of it.
func TestHeatDriftIsAbsentWhenTooFewSamplesFallInTheBand(t *testing.T) {
	t.Parallel()
	const seconds = 3600
	samples := activity.RideSamples{
		Power: series(seconds, func(second int) float64 {
			if second < 120 {
				return 160
			}

			return 240
		}),
		HeartRate:   series(seconds, flat(138)),
		Temperature: series(seconds, flat(28)),
	}

	assert.False(t, samples.HeatDrift(samples.HeartRate, 250).Known, "two minutes in the band is not a reading")
}

// The two sensors are paired by the second they were recorded at, so a
// thermometer that reported for only part of the ride contributes only there.
func TestHeatDriftPairsSensorsBySecondRecorded(t *testing.T) {
	t.Parallel()
	const seconds = 3600
	samples := activity.RideSamples{
		Power:       series(seconds, flat(160)),
		HeartRate:   series(seconds, flat(138)),
		Temperature: series(seconds, flat(28))[:seconds/2],
	}

	drift := samples.HeatDrift(samples.HeartRate, 250)

	require.True(t, drift.Known, "HeatDrift()")
	assert.Equal(t, seconds/2, drift.Samples, "only the seconds a temperature was recorded at")
}

// A strap that gave out at the halfway mark leaves the second half with no
// ratio, and half a comparison is not a decoupling.
func TestDecouplingIsAbsentWhereAHalfCarriesNoHeartRate(t *testing.T) {
	t.Parallel()
	const seconds = 90 * 60
	samples := activity.RideSamples{
		Power:     series(seconds, flat(200)),
		HeartRate: series(seconds, flat(140))[:seconds/2],
	}

	assert.False(t, samples.Decoupling(samples.HeartRate).Known, "no ratio for the second half")
}

// A strap reporting nought throughout is not a heart rate to divide by.
func TestDecouplingIsAbsentWhereTheHeartRateReadsNought(t *testing.T) {
	t.Parallel()
	const seconds = 90 * 60
	samples := activity.RideSamples{
		Power:     series(seconds, flat(200)),
		HeartRate: series(seconds, flat(0)),
	}

	assert.False(t, samples.Decoupling(samples.HeartRate).Known, "nought beats is not a ratio")
}

// The derive pass cleans the heart rate before anything reads it, and these two
// read the cleaned series. A spike above the rider's maximum lands in one half
// only, so read raw it would be drift that never happened.
func TestDriftReadingsFollowTheCleanedHeartRate(t *testing.T) {
	t.Parallel()
	const seconds = 90 * 60
	const maxBPM = 185.0
	// Five minutes of nonsense in the second half, which is where it reads as
	// the second half having lost its ratio.
	spiked := series(seconds, func(second int) float64 {
		if second >= seconds/2 && second < seconds/2+300 {
			return 240
		}

		return 140
	})
	samples := activity.RideSamples{
		// Inside the endurance band at a threshold of 250 W, so the same series
		// answers the band reading too.
		Power:       series(seconds, flat(160)),
		HeartRate:   spiked,
		Temperature: series(seconds, flat(28)),
	}
	cleaned := measure.CapHeartRate(spiked, maxBPM)

	raw := samples.Decoupling(samples.HeartRate)
	clean := samples.Decoupling(cleaned)

	require.True(t, raw.Known, "Decoupling() over the raw series")
	require.True(t, clean.Known, "Decoupling() over the cleaned series")
	assert.Greater(t, raw.Percent, 5.0, "read raw, the spike alone reads as drift")
	assert.InDelta(t, 0.0, clean.Percent, 0.001, "cleaned, the ride held its ratio")

	// The band reading follows the same series, so the spike leaves it alone.
	assert.InDelta(t, 140.0, samples.HeatDrift(cleaned, 250).HeartRateBPM, 0.001)
}
