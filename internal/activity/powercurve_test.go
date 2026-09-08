package activity_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func curveStart() time.Time { return time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC) }

// watts is one ride's power recorded once a second.
func watts(seconds int, value func(second int) float64) []trainingload.Sample {
	samples := make([]trainingload.Sample, seconds)
	for index := range samples {
		samples[index] = trainingload.Sample{
			At:    curveStart().Add(time.Duration(index) * time.Second),
			Value: value(index),
		}
	}

	return samples
}

// The curve's durations are the contract a stored row is keyed by, and the
// threshold point is what lets the FTP suggestion and the curve agree.
func TestPowerCurveDurationsHoldTheThresholdWindowAtItsNamedPoint(t *testing.T) {
	t.Parallel()

	durations := rider.PowerCurveDurations()

	assert.Equal(t, rider.ThresholdPowerWindow, durations[rider.ThresholdPowerPoint])
	assert.Len(t, durations, rider.PowerCurvePoints)
	for point := 1; point < rider.PowerCurvePoints; point++ {
		assert.Greater(t, durations[point], durations[point-1], "shortest first")
	}
}

// A flat ride holds the same mean over every duration it is long enough for,
// and nothing at all over the ones it is not.
func TestPowerBestsHoldEveryDurationTheRideReached(t *testing.T) {
	t.Parallel()
	samples := activity.RideSamples{Power: watts(30*60, func(int) float64 { return 200 })}

	curve := samples.PowerBests()

	for point, window := range rider.PowerCurveDurations() {
		if window <= 30*time.Minute {
			require.True(t, curve.Held[point], "a half-hour ride holds %s", window)
			assert.InDelta(t, 200.0, curve.Watts[point], 0.5)

			continue
		}
		assert.False(t, curve.Held[point], "and holds nothing over %s", window)
	}
}

// The best over a duration is the best window anywhere in the ride, not the
// ride's own average: a five-second sprint sits at the sprint's watts.
func TestPowerBestsFindTheBestWindowRatherThanTheAverage(t *testing.T) {
	t.Parallel()
	samples := activity.RideSamples{
		Power: watts(600, func(second int) float64 {
			if second >= 300 && second < 305 {
				return 900
			}

			return 150
		}),
	}

	curve := samples.PowerBests()

	require.True(t, curve.Held[0], "the five-second point")
	assert.InDelta(t, 900.0, curve.Watts[0], 1, "the sprint, not the ride")
	require.True(t, curve.Held[2], "the one-minute point")
	assert.Less(t, curve.Watts[2], 250.0, "over a minute the sprint is diluted")
}

// The acceptance criterion: the estimate never enters the curve, so a ride with
// no meter holds no point at all.
func TestPowerBestsAreEmptyWithoutMeasuredPower(t *testing.T) {
	t.Parallel()
	samples := activity.RideSamples{}

	curve := samples.PowerBests()

	assert.False(t, curve.Any(), "no meter, no curve")
}
