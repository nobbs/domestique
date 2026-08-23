package main

import (
	"math"
)

// Physical plausibility bounds for a single coasting window's implied
// dissipation, mirrored from internal/ridemodel/coefficients.go's own
// startup-validation bounds (PR #232) rather than invented here: a window
// this fit could not explain at any point in the range the forward model
// itself would accept is not coasting cleanly, whichever direction it
// misses by. Kept as this package's own constants rather than imported,
// because internal/ridemodel does not exist on main yet — #216 is still
// open — the same reason dev/ridemodel mirrors internal/route's
// gradientWindowMetres instead of importing internal/route for one
// constant.
const (
	plausibleMinCrr = 0.0
	plausibleMaxCrr = 0.05
	plausibleMinCdA = 0.15
	plausibleMaxCdA = 2.0
)

// windowDurationSeconds is one coasting regression observation's span. Long
// enough that an energy balance over it is not dominated by 1 Hz GPS and
// altitude noise, short enough that grade and air density stay close to
// constant across it.
const windowDurationSeconds = 10.0

// minCoastingRunSeconds is the shortest unbroken coasting stretch this
// package will window at all. The issue calls for "a sustained window rather
// than isolated records"; one full window's worth is the smallest stretch
// that can produce one.
const minCoastingRunSeconds = windowDurationSeconds

// maxRunGapSeconds is the largest gap between consecutive coasting-eligible
// samples that still counts as the same run. A larger gap means the rider
// stopped coasting in between — cadence briefly not read, say — and chaining
// across it would compute a window's speed delta over time the rider was not
// actually coasting throughout.
const maxRunGapSeconds = 3.0

// corneringChordSeconds is the half-span each bearing chord covers either
// side of the sample under test, matching the issue's own settled finding:
// summing raw bearing change between consecutive 1 Hz records rejects the
// great majority of windows on GPS jitter alone, while chords this long
// resolve an actual turn from noise.
const corneringChordSeconds = 3.0

// maxLateralAccelMPS2 is the implied lateral acceleration above which a
// sample is cornering rather than coasting in a straight line.
const maxLateralAccelMPS2 = 1.2

// coastingFilterCounts tags why each candidate sample or window was rejected,
// so the report can name every filter's own toll rather than one merged
// exclusion count.
type coastingFilterCounts struct {
	Cornering        int
	Plausibility     int
	SurvivingWindows int
}

// coastingWindowsFor builds this ride's coasting windows and reports how many
// candidate windows the cornering and plausibility filters each rejected.
// massKG is needed here, not just by the regression later: the plausibility
// bounds are on rolling resistance's force contribution (Crr * m * g * cosθ),
// which depends on mass the way drag's does not.
func coastingWindowsFor(samples []sampleRow, counts *coastingFilterCounts, massKG float64) []coastingWindow {
	var windows []coastingWindow

	start := 0
	for start < len(samples) {
		if !coastingEligible(samples, start) {
			start++

			continue
		}
		end := start + 1
		for end < len(samples) && coastingEligible(samples, end) &&
			samples[end].DeltaSeconds <= maxRunGapSeconds {
			end++
		}

		windows = append(windows, windowsInRun(samples[start:end], counts, massKG)...)
		start = end
	}

	return windows
}

// coastingEligible is a candidate sample's own qualification, independent of
// its neighbours: zero cadence from a working sensor, moving, and carrying
// the position and altitude channels every downstream computation needs.
// Cornering is tested separately, since it needs neighbouring samples rather
// than the sample alone.
func coastingEligible(samples []sampleRow, i int) bool {
	s := samples[i]

	return s.HasCadence && s.CadenceRPM == 0 && s.Moving && s.HasPosition && s.HasAltitude
}

// windowsInRun splits one unbroken coasting run at every cornering sample —
// a cornering sample breaks the run the same way a cadence change or a
// timing gap does, rather than being spliced out of it. Deleting a
// cornering sample and windowing over what remains would join intervals
// from before and after the turn into one "window" whose duration and
// distance come from one set of records while its start and end speeds
// come from another, non-contiguous, pair — breaking the energy-balance
// assumption every window here depends on.
func windowsInRun(run []sampleRow, counts *coastingFilterCounts, massKG float64) []coastingWindow {
	var windows []coastingWindow

	segmentStart := 0
	for i := 0; i <= len(run); i++ {
		cornering := i < len(run) && !corneringPass(run, i)
		if cornering {
			counts.Cornering++
		}
		if !cornering && i < len(run) {
			continue
		}

		windows = append(windows, windowsInStraightSegment(run[segmentStart:i], counts, massKG)...)
		segmentStart = i + 1
	}

	return windows
}

// windowsInStraightSegment tiles one contiguous, cornering-free stretch into
// non-overlapping windowDurationSeconds windows, testing each one's implied
// dissipation against the shared physical plausibility bounds.
func windowsInStraightSegment(segment []sampleRow, counts *coastingFilterCounts, massKG float64) []coastingWindow {
	var windows []coastingWindow

	segmentStart := 0
	for segmentStart < len(segment) {
		segmentEnd, duration := segmentAtLeast(segment, segmentStart, windowDurationSeconds)
		if segmentEnd == segmentStart {
			break
		}
		if duration < minCoastingRunSeconds {
			break
		}

		window := buildWindow(segment[segmentStart:segmentEnd])
		if plausible(window, massKG) {
			windows = append(windows, window)
			counts.SurvivingWindows++
		} else {
			counts.Plausibility++
		}
		segmentStart = segmentEnd
	}

	return windows
}

// segmentAtLeast returns the exclusive end index of the shortest run of
// consecutive samples starting at start whose summed DeltaSeconds reaches at
// least seconds, and that sum. It reports start itself, with a zero sum, when
// the segment runs out of samples first.
func segmentAtLeast(samples []sampleRow, start int, seconds float64) (end int, duration float64) {
	i := start
	for i < len(samples) {
		duration += samples[i].DeltaSeconds
		i++
		if duration >= seconds {
			return i, duration
		}
	}

	return start, 0
}

// buildWindow reduces a contiguous slice of coasting samples to one
// regression observation: total duration and distance, the speed delta
// across the whole window, and a distance-weighted mean of the per-sample
// gradient dev/ridemodel already computed over its own trailing window.
func buildWindow(segment []sampleRow) coastingWindow {
	var duration, distance, weightedGrade, weightedDensity float64
	for i := range segment {
		duration += segment[i].DeltaSeconds
		distance += segment[i].IntervalDistance
		weightedGrade += segment[i].GradientPercent * segment[i].IntervalDistance
		weightedDensity += airDensityFor(&segment[i]) * segment[i].IntervalDistance
	}

	grade := 0.0
	density := standardAirDensity
	if distance > 0 {
		grade = weightedGrade / distance
		density = weightedDensity / distance
	}

	first, last := &segment[0], &segment[len(segment)-1]

	return coastingWindow{
		RideID:          first.RideID,
		Surface:         first.Surface,
		DeltaSpeedMPS:   last.SpeedMPS - first.SpeedMPS,
		MeanSpeedMPS:    distance / duration,
		DurationSeconds: duration,
		GradePercent:    grade,
		AirDensity:      density,
	}
}

// plausible tests a window's implied dissipative force against every point
// in the shared (Crr, CdA) plausibility box, two-sided: too high is braking
// or a similar unmodelled deceleration, too low is not really coasting —
// pushed, drafted, or caught by a gust — and both fail the window rather
// than only the high side, the single largest lever the issue names.
func plausible(w coastingWindow, massKG float64) bool {
	grade := w.GradePercent / 100
	sinTheta := grade / math.Sqrt(1+grade*grade)
	cosTheta := 1 / math.Sqrt(1+grade*grade)
	x1 := massKG * gravityMetresPerSecondSquared * cosTheta
	x2 := 0.5 * w.AirDensity * w.MeanSpeedMPS * w.MeanSpeedMPS
	y := -massKG*(w.DeltaSpeedMPS/w.DurationSeconds) - massKG*gravityMetresPerSecondSquared*sinTheta

	minY := plausibleMinCrr*x1 + plausibleMinCdA*x2
	maxY := plausibleMaxCrr*x1 + plausibleMaxCdA*x2

	return y >= minY && y <= maxY
}

// corneringPass reports whether sample i in a coasting run looks like
// straight-line travel: bearing measured over a chord on each side, rather
// than between consecutive 1 Hz records, so GPS jitter does not itself read
// as cornering. A sample too near either end of the run to build a full
// chord is treated as untestable and passes — the plausibility filter, not
// this one, is what a run's opening or closing seconds still has to clear.
func corneringPass(run []sampleRow, i int) bool {
	before, ok1 := chordIndex(run, i, -1)
	after, ok2 := chordIndex(run, i, 1)
	if !ok1 || !ok2 {
		return true
	}

	bearingBefore := bearingBetween(&run[before], &run[i])
	bearingAfter := bearingBetween(&run[i], &run[after])
	deltaBearing := angularDifference(bearingAfter, bearingBefore)

	elapsed := run[after].Time.Sub(run[before].Time).Seconds()
	if elapsed <= 0 {
		return true
	}

	lateralAccel := run[i].SpeedMPS * deltaBearing / elapsed

	return math.Abs(lateralAccel) <= maxLateralAccelMPS2
}

// chordIndex walks from i in the given direction (-1 or 1) until it has
// accumulated at least corneringChordSeconds, and reports that index. It
// reports false when the run ends first.
func chordIndex(run []sampleRow, i, direction int) (int, bool) {
	elapsed := 0.0
	j := i
	for {
		next := j + direction
		if next < 0 || next >= len(run) {
			return 0, false
		}
		if direction > 0 {
			elapsed += run[next].DeltaSeconds
		} else {
			elapsed += run[j].DeltaSeconds
		}
		j = next
		if elapsed >= corneringChordSeconds {
			return j, true
		}
	}
}

// bearingBetween is the initial compass bearing from one point to the next,
// in radians, using an equirectangular approximation: accurate enough over
// the few metres a coasting chord covers, and not worth a full great-circle
// formula for that distance.
func bearingBetween(from, to *sampleRow) float64 {
	latRad := from.Latitude * math.Pi / 180
	dLon := (to.Longitude - from.Longitude) * math.Pi / 180 * math.Cos(latRad)
	dLat := (to.Latitude - from.Latitude) * math.Pi / 180

	return math.Atan2(dLon, dLat)
}

// angularDifference wraps b-a into (-π, π], so a bearing crossing due north
// reads as a small turn rather than a near-360° one.
func angularDifference(b, a float64) float64 {
	d := b - a
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}

	return d
}
