package main

import (
	"math"
	"sort"
)

// huberK is the standard Huber tuning constant: a residual beyond this many
// robust standard deviations from zero is downweighted rather than fit
// exactly. 1.345 gives 95% efficiency against a normal error distribution,
// the usual default and not tuned to this corpus.
const huberK = 1.345

// robustIterations is how many reweight-and-resolve passes irlsFit runs.
// Huber IRLS on a well-behaved 2-parameter fit converges in a handful of
// passes; more buys nothing measurable here.
const robustIterations = 8

// maxAcceptableConditionRatio flags a fit as ill-conditioned once the normal
// matrix's eigenvalue ratio crosses it. An order-of-magnitude heuristic, not a
// derived bound, and not calibrated per corpus.
const maxAcceptableConditionRatio = 5000.0

// coastingObservation is one regression row. Its only remaining use is
// fitRouteCoefficients' two-variable fit of seconds_per_km and
// seconds_per_ascent_m against a ride's distance and ascent; the name is a
// holdover.
type coastingObservation struct {
	Y, X1, X2, Weight float64
}

// solve2x2 fits y = a*x1 + b*x2 by weighted least squares, and returns the
// normal matrix's eigenvalue ratio alongside the fit: the same solve the
// conditioning diagnostic needs, computed once. Generic over what x1/x2/y
// represent — today that's a ride's distance/ascent against moving seconds.
func solve2x2(observations []coastingObservation) (crr, cda, conditionRatio float64) {
	var sxx1, sxx12, sxx2, sx1y, sx2y float64
	for _, o := range observations {
		sxx1 += o.Weight * o.X1 * o.X1
		sxx12 += o.Weight * o.X1 * o.X2
		sxx2 += o.Weight * o.X2 * o.X2
		sx1y += o.Weight * o.X1 * o.Y
		sx2y += o.Weight * o.X2 * o.Y
	}

	det := sxx1*sxx2 - sxx12*sxx12
	if det == 0 {
		return 0, 0, math.Inf(1)
	}
	crr = (sx1y*sxx2 - sx2y*sxx12) / det
	cda = (sxx1*sx2y - sxx12*sx1y) / det

	trace := sxx1 + sxx2
	discriminant := math.Sqrt(math.Max(0, trace*trace-4*det))
	lambdaMax := (trace + discriminant) / 2
	lambdaMin := (trace - discriminant) / 2
	if lambdaMin <= 0 {
		conditionRatio = math.Inf(1)
	} else {
		conditionRatio = lambdaMax / lambdaMin
	}

	return crr, cda, conditionRatio
}

// irlsFit runs Huber-weighted iteratively reweighted least squares over
// observations, starting from solve2x2's plain fit — robust against the odd
// contaminated ride ordinary least squares has no defence against.
func irlsFit(observations []coastingObservation) (crr, cda, conditionRatio float64) {
	if len(observations) == 0 {
		return 0, 0, math.Inf(1)
	}

	weighted := make([]coastingObservation, len(observations))
	copy(weighted, observations)

	for range robustIterations {
		crr, cda, conditionRatio = solve2x2(weighted)

		residuals := make([]float64, len(observations))
		for i, o := range observations {
			residuals[i] = o.Y - crr*o.X1 - cda*o.X2
		}
		scale := robustScale(residuals)
		if scale == 0 {
			break
		}
		for i, o := range observations {
			z := math.Abs(residuals[i] / scale)
			huber := 1.0
			if z > huberK {
				huber = huberK / z
			}
			weighted[i] = coastingObservation{Y: o.Y, X1: o.X1, X2: o.X2, Weight: o.Weight * huber}
		}
	}

	return crr, cda, conditionRatio
}

// robustScale is the median absolute deviation, scaled to estimate a normal
// distribution's standard deviation, so huberK's usual normal-distribution
// tuning applies unchanged regardless of the residuals' actual spread.
func robustScale(residuals []float64) float64 {
	if len(residuals) == 0 {
		return 0
	}
	sorted := make([]float64, len(residuals))
	copy(sorted, residuals)
	sort.Float64s(sorted)
	median := percentileOf(sorted, 0.5)

	deviations := make([]float64, len(sorted))
	for i, r := range sorted {
		deviations[i] = math.Abs(r - median)
	}
	sort.Float64s(deviations)

	return percentileOf(deviations, 0.5) * 1.4826
}

// percentileOf reads a fractional position from an already-sorted slice by
// linear interpolation between the two nearest ranks.
func percentileOf(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := fraction * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}

	weight := position - float64(lower)

	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
