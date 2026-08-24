package main

import (
	"math"
	"sort"

	"github.com/nobbs/domestique/internal/surface"
)

// defaultClimbThresholdPercent is where stage B starts looking for sustained
// climbing: above it, air resistance is a small correction and gravity is
// nearly everything, so power falls out of the equation directly. The
// issue's own settled finding on a real corpus: 5% would be the cleaner
// physics, but it leaves too thin a sample (10.5 h in six years against 22 h
// at 4%) to trust a robust central estimate over. -climb-threshold-percent
// overrides it; the report always states the corpus's own hours above both,
// so the trade is visible rather than assumed.
const defaultClimbThresholdPercent = 4.0

// minClimbRunSeconds is the shortest unbroken above-threshold stretch stage
// B will window. Longer than coasting's minimum: a climb's grade and speed
// change more slowly, so noise averages out over a longer window without
// the run itself needing to be an unusual length.
const minClimbRunSeconds = 20.0

const climbWindowDurationSeconds = 20.0

// climbWindowsFor builds this ride's sustained above-threshold climbing
// windows. It needs no cadence and no coefficients: unlike coasting, nothing
// here is filtered against a fit that has not happened yet.
func climbWindowsFor(samples []sampleRow, thresholdPercent float64) []climbSample {
	var climbs []climbSample

	start := 0
	for start < len(samples) {
		if !climbEligible(samples, start, thresholdPercent) {
			start++

			continue
		}
		end := start + 1
		for end < len(samples) && climbEligible(samples, end, thresholdPercent) &&
			samples[end].DeltaSeconds <= maxRunGapSeconds {
			end++
		}

		climbs = append(climbs, climbWindowsInRun(samples[start:end])...)
		start = end
	}

	return climbs
}

func climbEligible(samples []sampleRow, i int, thresholdPercent float64) bool {
	s := samples[i]

	return s.Moving && s.HasAltitude && s.GradientPercent >= thresholdPercent
}

func climbWindowsInRun(run []sampleRow) []climbSample {
	var climbs []climbSample

	segmentStart := 0
	for segmentStart < len(run) {
		segmentEnd, duration := segmentAtLeast(run, segmentStart, climbWindowDurationSeconds)
		if segmentEnd == segmentStart {
			break
		}
		if duration < minClimbRunSeconds {
			break
		}

		climbs = append(climbs, buildClimbSample(run[segmentStart:segmentEnd]))
		segmentStart = segmentEnd
	}

	return climbs
}

func buildClimbSample(segment []sampleRow) climbSample {
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

	return climbSample{
		RideID:       segment[0].RideID,
		Date:         segment[0].Time,
		Surface:      segment[0].Surface,
		MeanSpeedMPS: distance / duration,
		GradePercent: grade,
		AirDensity:   density,
	}
}

// sustainedPowerWatts evaluates the explicit power equation over every climb
// sample — with CdA and Crr already fixed by stage A there is nothing to
// solve — and returns a trimmed mean, the same robustness posture stage A
// takes for the same reason: soft-pedalling invisible to the cadence filter,
// a wind gust, or a grade smoothed across a window that briefly dipped are
// each one bad sample, not a reason to distrust the whole run.
func sustainedPowerWatts(
	climbs []climbSample, crrBySurface map[surface.Kind]float64, crrOverall, cda, massKG, driveEfficiency float64,
) float64 {
	if len(climbs) == 0 {
		return 0
	}

	powers := make([]float64, len(climbs))
	for i, c := range climbs {
		crr := crrOverall
		if fitted, ok := crrBySurface[c.Surface]; ok && fitted > 0 {
			crr = fitted
		}
		powers[i] = climbPowerWatts(c, crr, cda, massKG, driveEfficiency)
	}

	return trimmedMean(powers, 0.1)
}

// climbPowerWatts is the issue's own equation:
// P = v·(Crr·m·g·cosθ + m·g·sinθ + ½·ρ·CdA·v²) / η
func climbPowerWatts(c climbSample, crr, cda, massKG, driveEfficiency float64) float64 {
	grade := c.GradePercent / 100
	denom := math.Sqrt(1 + grade*grade)
	sinTheta := grade / denom
	cosTheta := 1 / denom
	rolling := crr * massKG * gravityMetresPerSecondSquared * cosTheta
	gravityForce := massKG * gravityMetresPerSecondSquared * sinTheta
	drag := 0.5 * c.AirDensity * cda * c.MeanSpeedMPS * c.MeanSpeedMPS

	return c.MeanSpeedMPS * (rolling + gravityForce + drag) / driveEfficiency
}

// trimmedMean drops the fraction/2 lowest and highest values from each tail
// before averaging what remains — cheap, standard robustness against the
// contamination the issue names, without irlsFit's iterative machinery for a
// stage that (unlike stage A) has nothing else to solve for.
func trimmedMean(values []float64, fraction float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	trim := int(float64(len(sorted)) * fraction / 2)
	kept := sorted[trim : len(sorted)-trim]
	if len(kept) == 0 {
		kept = sorted
	}

	sum := 0.0
	for _, v := range kept {
		sum += v
	}

	return sum / float64(len(kept))
}
