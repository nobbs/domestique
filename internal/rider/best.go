package rider

import "time"

// maxSampleGap is the longest silence a window may contain. A recorder that
// paused, or dropped out, leaves the last sample standing for as long as the
// pause lasted, which would read as an effort held for that whole time.
const maxSampleGap = 10 * time.Second

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
	best, found := 0.0, false
	for begin := 0; begin < len(times); {
		end := begin + 1
		for end < len(times) && times[end].Sub(times[end-1]) <= maxSampleGap {
			end++
		}
		if mean, ok := bestUnbroken(times[begin:end], values[begin:end], window); ok && (!found || mean > best) {
			best, found = mean, true
		}
		begin = end
	}

	return best, found
}

// bestUnbroken slides the window over samples with no silence in them.
func bestUnbroken(times []time.Time, values []float64, window time.Duration) (float64, bool) {
	if len(times) < 2 {
		return 0, false
	}
	// integral[k] is the value integrated from the first sample to times[k], in
	// value-seconds, which any window's mean is then one subtraction away.
	integral := make([]float64, len(times))
	for index := 1; index < len(times); index++ {
		integral[index] = integral[index-1] + values[index-1]*times[index].Sub(times[index-1]).Seconds()
	}

	best, found := 0.0, false
	start := 0
	for end := 1; end < len(times); end++ {
		// The latest start that still covers the length asked for, which is the
		// window closest to that length ending here.
		for start+1 < end && times[end].Sub(times[start+1]) >= window {
			start++
		}
		span := times[end].Sub(times[start])
		if span < window || span <= 0 {
			continue
		}
		if mean := (integral[end] - integral[start]) / span.Seconds(); !found || mean > best {
			best, found = mean, true
		}
	}

	return best, found
}
