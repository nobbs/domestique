package trainingload_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeAtHeartRate(t *testing.T) {
	t.Parallel()
	at := func(second int, value float64) trainingload.Sample {
		return trainingload.Sample{At: rideStart().Add(time.Duration(second) * time.Second), Value: value}
	}

	tests := []struct {
		name        string
		samples     []trainingload.Sample
		wantSeconds []float64
		wantFromBPM int
	}{
		{name: "empty input", samples: nil, wantSeconds: nil},
		{name: "one sample holds nothing", samples: []trainingload.Sample{at(0, 130)}, wantSeconds: nil},
		{
			name:        "fractional readings floor",
			samples:     []trainingload.Sample{at(0, 130.9), at(5, 131.4)},
			wantFromBPM: 130,
			wantSeconds: []float64{5},
		},
		{
			name:        "contiguous noughts between bins",
			samples:     []trainingload.Sample{at(0, 120), at(3, 125), at(6, 125)},
			wantFromBPM: 120,
			wantSeconds: []float64{3, 0, 0, 0, 0, 3},
		},
		{
			name:        "a gap over DefaultMaxGap is not counted",
			samples:     []trainingload.Sample{at(0, 120), at(20, 130)},
			wantSeconds: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			distribution := trainingload.TimeAtHeartRate(test.samples)
			assert.Equal(t, test.wantFromBPM, distribution.FromBPM)
			if test.wantSeconds == nil {
				assert.Nil(t, distribution.Seconds)

				return
			}
			require.Len(t, distribution.Seconds, len(test.wantSeconds))
			for index, want := range test.wantSeconds {
				assert.InDelta(t, want, distribution.Seconds[index], 1e-9)
			}
		})
	}
}

// The distribution and TimeInZones fold the same held rule over the same
// samples, so the ground they account for must agree even though one cuts by
// zone and the other by whole beat.
func TestTimeAtHeartRateTotalMatchesTimeInZones(t *testing.T) {
	t.Parallel()
	bounds, ok := trainingload.BoundsFrom(0, 200)
	require.True(t, ok)
	samples := blocks(600, 37, 110, 165)

	distribution := trainingload.TimeAtHeartRate(samples)
	total := 0.0
	for _, seconds := range distribution.Seconds {
		total += seconds
	}
	assert.InDelta(t, trainingload.TimeInZones(samples, bounds).Total(), total, 1e-9)
}
