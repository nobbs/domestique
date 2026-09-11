package measure

import "time"

// DefaultMaxGap is the longest a single sample stands for, and the longest
// step any window may cross. Beyond it the recorder had stopped.
const DefaultMaxGap = 10 * time.Second

// Stretch is the half-open range of samples recorded without a pause in them.
type Stretch struct {
	First, Past int
}

// Stretches marks, for each sample, the stretch of recording it belongs to: a
// step of at most maxGap continues a stretch, a longer one starts another. A
// clock that did not advance is skipped where it stands by the per-step
// rules, not treated as a pause, so it never ends a stretch.
//
// See docs/specs/measurement.md §Recording gaps.
func Stretches(times []time.Time, maxGap time.Duration) []Stretch {
	stretches := make([]Stretch, len(times))
	for first := 0; first < len(times); {
		past := first + 1
		for past < len(times) && times[past].Sub(times[past-1]) <= maxGap {
			past++
		}
		for index := first; index < past; index++ {
			stretches[index] = Stretch{First: first, Past: past}
		}
		first = past
	}

	return stretches
}

// ForEachHeld visits each reading with how long it stood, the gap to the next
// one; a step that is not positive or exceeds maxGap is skipped. The last
// reading stands for nothing.
//
// See docs/specs/measurement.md §Recording gaps.
func ForEachHeld(readings []Reading, maxGap time.Duration, visit func(value, seconds float64)) {
	for index := range len(readings) - 1 {
		held := readings[index+1].At.Sub(readings[index].At)
		if held <= 0 || held > maxGap {
			continue
		}
		visit(readings[index].Value, held.Seconds())
	}
}

// MeanHeld is the time-weighted mean of the readings and how long they stood.
//
// See docs/specs/measurement.md §Recording gaps.
func MeanHeld(readings []Reading, maxGap time.Duration) (mean, seconds float64) {
	total := 0.0
	ForEachHeld(readings, maxGap, func(value, held float64) {
		total += value * held
		seconds += held
	})
	if seconds <= 0 {
		return 0, 0
	}

	return total / seconds, seconds
}

// Interval is a half-open time range, [Start, End).
type Interval struct {
	Start, End time.Time
}

// MovingIntervals is the time ranges a track was moving through: the odometer
// advancing between two samples in a row, the same rule a splits table cuts a
// stop's seconds out of its moving time by. Adjacent moving steps merge into
// one stretch.
func MovingIntervals(track []Sample) []Interval {
	var intervals []Interval
	for index := 1; index < len(track); index++ {
		previous, current := &track[index-1], &track[index]
		if !current.At.After(previous.At) || current.DistanceMetres <= previous.DistanceMetres {
			continue
		}
		if last := len(intervals) - 1; last >= 0 && !intervals[last].End.Before(previous.At) {
			intervals[last].End = current.At

			continue
		}
		intervals = append(intervals, Interval{Start: previous.At, End: current.At})
	}

	return intervals
}

// HeldWithinIntervals is the held time ForEachHeld would report, restricted to
// the portion of each step that falls within one of the given intervals:
// sorted ascending and non-overlapping. A sensor live only outside them --
// through a stop, say -- contributes nothing.
func HeldWithinIntervals(readings []Reading, maxGap time.Duration, intervals []Interval) (seconds float64) {
	cursor := 0
	for index := range len(readings) - 1 {
		step := Interval{Start: readings[index].At, End: readings[index+1].At}
		if held := step.End.Sub(step.Start); held <= 0 || held > maxGap {
			continue
		}
		for cursor < len(intervals) && !intervals[cursor].End.After(step.Start) {
			cursor++
		}
		for search := cursor; search < len(intervals) && intervals[search].Start.Before(step.End); search++ {
			seconds += overlapSeconds(step, intervals[search])
		}
	}

	return seconds
}

// overlapSeconds is how long two intervals share.
func overlapSeconds(a, b Interval) float64 {
	start, end := a.Start, a.End
	if b.Start.After(start) {
		start = b.Start
	}
	if b.End.Before(end) {
		end = b.End
	}
	if end.Before(start) {
		return 0
	}

	return end.Sub(start).Seconds()
}

// RollingMean visits each reading that has a full window behind it with the
// mean over the shortest window that still covers the length asked for and
// how long the reading stood. A step that is not positive or exceeds maxGap
// breaks the series: what follows starts a new window. It reports the
// seconds every held step accounted for, window or no window, and whether
// any window was visited.
//
// See docs/specs/measurement.md §Rolling mean.
func RollingMean(
	readings []Reading, window, maxGap time.Duration, visit func(mean, heldSeconds float64),
) (heldSeconds float64, visited bool) {
	weight := 0.0
	start := 0
	// integral[k] is the value integrated from the first reading to
	// readings[k]; a window's mean is one subtraction of it, and a gap moves
	// the window's start rather than the integral.
	integral := make([]float64, len(readings))
	for index := 1; index < len(readings); index++ {
		held := readings[index].At.Sub(readings[index-1].At)
		if held <= 0 || held > maxGap {
			integral[index] = integral[index-1]
			start = index

			continue
		}
		heldSeconds += held.Seconds()
		integral[index] = integral[index-1] + readings[index-1].Value*held.Seconds()
		for start+1 < index && readings[index].At.Sub(readings[start+1].At) >= window {
			start++
		}
		span := readings[index].At.Sub(readings[start].At)
		if span < window {
			continue
		}
		visit((integral[index]-integral[start])/span.Seconds(), held.Seconds())
		weight += held.Seconds()
	}

	return heldSeconds, weight > 0
}
