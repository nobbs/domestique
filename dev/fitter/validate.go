package main

import (
	"math"
	"sort"
)

// heldOutFraction is how much of a group's rides, by date, are withheld from
// the fit and reserved for validation. The issue does not name a ratio, only
// that the split must be chronological — a random split would leak a
// fitness trend across two years and flatter the result.
const heldOutFraction = 0.2

// maxSolveSpeedMetresPerSecond bounds the bisection search predictedSpeed
// runs for a segment's speed. Wide enough for any plausible ride, narrow
// enough that a solve failure (no root in range) is itself informative
// rather than silently returning a nonsense speed.
const maxSolveSpeedMetresPerSecond = 30.0

// speedSolveIterations is enough bisection steps to resolve speed to a
// fraction of a millimetre per second — far finer than the physics model
// itself is accurate to.
const speedSolveIterations = 60

// splitByDate orders a group's rides chronologically and returns the earlier
// rides to fit against and the later ones to validate against — never a
// random split, per the issue's own stated reason.
func splitByDate(rides []rideRow) (train, heldOut []rideRow) {
	sorted := make([]rideRow, len(rides))
	copy(sorted, rides)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	heldOutCount := int(float64(len(sorted)) * heldOutFraction)
	if heldOutCount == 0 && len(sorted) > 1 {
		heldOutCount = 1
	}
	splitAt := len(sorted) - heldOutCount

	return sorted[:splitAt], sorted[splitAt:]
}

// predictedSpeed solves the power equation for the one positive speed that
// consumes the configured power at this grade and air density, by bisection
// rather than a closed form: the drag term makes it cubic in v, and a
// numerical root is a few lines against a division-heavy closed-form cubic
// solver that buys nothing a reader would trust more. It returns 0 when no
// speed in [0, maxSolveSpeedMetresPerSecond] reaches the configured power —
// the true root lies outside the bracket — so the caller can skip the
// segment rather than silently taking the bracket's edge as an answer.
//
// Below descentCutoffPercent the rider is assumed to coast rather than push
// power into a descent: predictedSpeed instead solves the zero-power
// equilibrium speed (where gravity exactly balances rolling resistance and
// drag) and caps it at descentCapMPS — the same two constants
// internal/ridemodel's own forward model will read from this package's
// output file, so a segment predicts the same way here as it will there. A
// grade only just past the cutoff can have no positive equilibrium speed at
// all (gravity does not yet overcome rolling resistance while coasting),
// in which case this also returns 0.
func predictedSpeed(grade, airDensity, crr, cda, massKG, driveEfficiency, powerWatts, descentCutoffPercent, descentCapMPS float64) float64 {
	if grade <= descentCutoffPercent {
		return coastEquilibriumSpeed(grade, airDensity, crr, cda, massKG, descentCapMPS)
	}

	maxPower := climbPowerWatts(
		climbSample{MeanSpeedMPS: maxSolveSpeedMetresPerSecond, GradePercent: grade, AirDensity: airDensity},
		crr, cda, massKG, driveEfficiency,
	)
	if maxPower < powerWatts {
		return 0
	}

	lo, hi := 0.0, maxSolveSpeedMetresPerSecond
	for range speedSolveIterations {
		mid := (lo + hi) / 2
		power := climbPowerWatts(climbSample{MeanSpeedMPS: mid, GradePercent: grade, AirDensity: airDensity}, crr, cda, massKG, driveEfficiency)
		if power < powerWatts {
			lo = mid
		} else {
			hi = mid
		}
	}

	return (lo + hi) / 2
}

// coastEquilibriumSpeed solves 0 = Crr·m·g·cosθ + m·g·sinθ + ½·ρ·CdA·v² for
// v — the speed at which drag and rolling resistance exactly balance
// gravity's forward push on a descent, the same equation climbPowerWatts
// evaluates with its power term set to zero — and caps it at descentCapMPS.
func coastEquilibriumSpeed(grade, airDensity, crr, cda, massKG, descentCapMPS float64) float64 {
	rad := grade / 100
	denom := math.Sqrt(1 + rad*rad)
	sinTheta, cosTheta := rad/denom, 1/denom

	drivingForce := -massKG*gravityMetresPerSecondSquared*sinTheta - crr*massKG*gravityMetresPerSecondSquared*cosTheta
	if drivingForce <= 0 || cda <= 0 || airDensity <= 0 {
		return 0
	}

	speed := math.Sqrt(2 * drivingForce / (airDensity * cda))
	if speed > descentCapMPS {
		return descentCapMPS
	}

	return speed
}

// predictedMovingSeconds sums each sample interval's predicted duration at
// the configured coefficients, giving a predicted moving time for the whole
// ride — the same quantity rides.csv's own moving_seconds reports, so the
// two are directly comparable.
func predictedMovingSeconds(samples []sampleRow, result *fitResult, config coefficientsConfig, driveEfficiency, powerWatts float64) float64 {
	var seconds float64
	for i := range samples {
		s := &samples[i]
		if !s.Moving || !s.HasAltitude {
			continue
		}
		crr := crrForSurface(result, s.Surface)
		speed := predictedSpeed(
			s.GradientPercent, airDensityFor(s), crr, result.CdA, result.MassKG, driveEfficiency, powerWatts,
			config.DescentCutoffPercent, config.DescentCapMetresPerSecond,
		)
		if speed <= 0 {
			continue
		}
		seconds += s.IntervalDistance / speed
	}

	return seconds
}

// baselineMovingSeconds is the trivial predictor the issue asks the model to
// beat: distance at a flat-riding mean speed, plus ascent at a mean climbing
// rate (VAM), both estimated from the same training rides the physics model
// was fitted against — never from the held-out rides being scored.
func baselineMovingSeconds(samples []sampleRow, flatSpeedMPS, vamMetresPerHour, climbThresholdPercent float64) float64 {
	var flatDistance, ascent float64
	for i := range samples {
		if !samples[i].Moving {
			continue
		}
		if samples[i].GradientPercent < climbThresholdPercent {
			flatDistance += samples[i].IntervalDistance
		} else if samples[i].HasAltitude {
			ascent += climbRise(samples, i)
		}
	}

	var seconds float64
	if flatSpeedMPS > 0 {
		seconds += flatDistance / flatSpeedMPS
	}
	if vamMetresPerHour > 0 {
		seconds += (ascent / vamMetresPerHour) * 3600
	}

	return seconds
}

// climbRise is one sample's own rise, backed out of its grade and distance
// rather than a raw altitude delta, so it agrees with the same
// windowed-gradient the rest of this package already uses.
func climbRise(samples []sampleRow, i int) float64 {
	return samples[i].IntervalDistance * samples[i].GradientPercent / 100
}

// baselineCoefficients estimates the trivial baseline's own two constants
// from a set of training rides: the mean speed of flat riding, and the mean
// vertical ascent rate of sustained climbing.
func baselineCoefficients(
	samplesByRide map[string][]sampleRow, rideIDs map[string]bool, climbThresholdPercent float64,
) (flatSpeedMPS, vamMetresPerHour float64) {
	var flatDistance, flatSeconds, ascent, climbSeconds float64
	for rideID, samples := range samplesByRide {
		if !rideIDs[rideID] {
			continue
		}
		for i := range samples {
			if !samples[i].Moving {
				continue
			}
			if samples[i].GradientPercent < climbThresholdPercent {
				flatDistance += samples[i].IntervalDistance
				flatSeconds += samples[i].DeltaSeconds
			} else if samples[i].HasAltitude {
				ascent += climbRise(samples, i)
				climbSeconds += samples[i].DeltaSeconds
			}
		}
	}

	if flatSeconds > 0 {
		flatSpeedMPS = flatDistance / flatSeconds
	}
	if climbSeconds > 0 {
		vamMetresPerHour = ascent / (climbSeconds / 3600)
	}

	return flatSpeedMPS, vamMetresPerHour
}

// heldOutValidation is the report's own held-out figures: mean absolute
// percentage error of the physics model and of the trivial baseline, over
// the same held-out rides, so the model's added complexity is visibly
// earning its place or not.
type heldOutValidation struct {
	Rides       int
	ModelMAE    float64
	BaselineMAE float64
}

func validateHeldOut(
	heldOutRideIDs map[string]bool, samplesByRide map[string][]sampleRow,
	actualMovingSeconds map[string]float64, result *fitResult, config coefficientsConfig,
	flatSpeedMPS, vamMetresPerHour, climbThresholdPercent float64,
) heldOutValidation {
	var modelErrors, baselineErrors []float64
	for rideID := range heldOutRideIDs {
		samples, ok := samplesByRide[rideID]
		actual, hasActual := actualMovingSeconds[rideID]
		if !ok || !hasActual || actual <= 0 {
			continue
		}

		// Scored only when both predictors produce a value for this ride: the
		// two MAEs are meant to be directly comparable, over the same rides,
		// not each over whatever subset happened to have a prediction.
		predicted := predictedMovingSeconds(samples, result, config, config.DriveEfficiency, result.PowerWatts)
		baseline := baselineMovingSeconds(samples, flatSpeedMPS, vamMetresPerHour, climbThresholdPercent)
		if predicted <= 0 || baseline <= 0 {
			continue
		}
		modelErrors = append(modelErrors, absPercentError(predicted, actual))
		baselineErrors = append(baselineErrors, absPercentError(baseline, actual))
	}

	return heldOutValidation{
		Rides:       len(modelErrors),
		ModelMAE:    meanOf(modelErrors),
		BaselineMAE: meanOf(baselineErrors),
	}
}

func absPercentError(predicted, actual float64) float64 {
	diff := predicted - actual
	if diff < 0 {
		diff = -diff
	}

	return 100 * diff / actual
}

func meanOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}
