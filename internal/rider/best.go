package rider

import (
	"time"

	"github.com/nobbs/domestique/internal/measure"
)

// BestAverage reports the highest mean the series holds over a window of the
// given length, and whether it held one at all. Samples are one ride's, in
// recorded order; a window never spans two rides.
//
// One window is weighed per end sample: the shortest that still covers the
// length asked for, since the length asked for is what the figure means — a
// best twenty-minute power is over twenty minutes, not over the best stretch
// of any length above it. Where the sampling straddles the boundary the window
// is a little longer than asked for, never shorter, so the answer understates
// rather than flatters.
//
// Each sample counts for as long as it stands, so a series recorded at an
// irregular rate is averaged over time rather than over sample count.
func BestAverage(times []time.Time, values []float64, window time.Duration) (float64, bool) {
	if len(times) != len(values) {
		return 0, false
	}
	readings := make([]measure.Reading, len(times))
	for index := range times {
		readings[index] = measure.Reading{At: times[index], Value: values[index]}
	}
	best, found := 0.0, false
	measure.RollingMean(readings, window, measure.DefaultMaxGap, func(mean, _ float64) {
		if !found || mean > best {
			best, found = mean, true
		}
	})

	return best, found
}
