package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/nobbs/domestique/internal/surface"
)

// minWindowsPerRideIntercept is the fewest coasting windows one ride needs
// before its own rolling-resistance intercept is trusted enough to report.
// A ride below it still rode, it just cannot answer this diagnostic on its
// own.
const minWindowsPerRideIntercept = 5

// minWindowsPerSurfaceClass is the fewest coasting windows a surface class
// needs before this package fits its own Crr rather than leaving it to
// crrForSurface's pooled fallback. Aerodynamic drag does not depend on
// surface, so CdA is fit once, globally, over every surviving window; only
// Crr is refit per class, and only where there is enough of that class to
// trust a refit over the pooled figure.
const minWindowsPerSurfaceClass = 30

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
// matrix's eigenvalue ratio crosses it. Chosen as an order-of-magnitude
// heuristic, not a derived bound: a normal matrix this lopsided means one
// regressor (almost always Crr's, when coasting clusters at one speed) is
// barely determined and will absorb noise the other regressor should carry.
//
// Note: a fixed threshold, not calibrated per corpus; revisit if a real
// corpus reports a ratio just under it with a visibly bad fit.
const maxAcceptableConditionRatio = 5000.0

// coastingObservation is one regression row: the dissipative force a window
// implies (y), and the two regressors whose weighted sum should explain it —
// rolling resistance's coefficient of mass*g*cosθ (x1), and drag's
// coefficient of ½ρv² (x2). Built once per window so the solver and the
// conditioning diagnostic share exactly the same numbers.
type coastingObservation struct {
	Y, X1, X2, Weight float64
}

func observationsFor(windows []coastingWindow, massKG float64) []coastingObservation {
	observations := make([]coastingObservation, len(windows))
	for i, w := range windows {
		grade := w.GradePercent / 100
		sinTheta := grade / math.Sqrt(1+grade*grade)
		cosTheta := 1 / math.Sqrt(1+grade*grade)
		observations[i] = coastingObservation{
			Y:      -massKG*(w.DeltaSpeedMPS/w.DurationSeconds) - massKG*gravityMetresPerSecondSquared*sinTheta,
			X1:     massKG * gravityMetresPerSecondSquared * cosTheta,
			X2:     0.5 * w.AirDensity * w.MeanSpeedMPS * w.MeanSpeedMPS,
			Weight: w.DurationSeconds,
		}
	}

	return observations
}

// solve2x2 fits y = crr*x1 + cda*x2 by weighted least squares, and returns
// the normal matrix's eigenvalue ratio alongside the fit: the same solve the
// conditioning diagnostic needs, computed once.
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
// observations, starting from solve2x2's plain fit. The coasting samples are
// contaminated in ways no upstream filter fully removes — a dropped-out
// cadence reading, a light brake tap that evades the physical plausibility
// test, a gust — and ordinary least squares has no defence against any of it.
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

// linearRegression fits y = intercept + slope*x by weighted least squares.
// Used for the indoor heart-rate-to-power relation, where — unlike the
// coasting fit — there is exactly one regressor and no physical plausibility
// bound to defend, so this is the only extra machinery plain OLS needs: a
// weight per row, so an indoor row spanning several seconds (indoor.csv is
// not strictly 1 Hz) counts for as much as the seconds it actually covers,
// the same Δt-weighting invariant #214 established for the outdoor corpus.
func linearRegression(xs, ys, weights []float64) (slope, intercept float64) {
	var sumW, sumX, sumY, sumXY, sumXX float64
	for i := range xs {
		w := weights[i]
		sumW += w
		sumX += w * xs[i]
		sumY += w * ys[i]
		sumXY += w * xs[i] * ys[i]
		sumXX += w * xs[i] * xs[i]
	}
	if sumW == 0 {
		return 0, 0
	}

	denominator := sumW*sumXX - sumX*sumX
	if denominator == 0 {
		return 0, sumY / sumW
	}
	slope = (sumW*sumXY - sumX*sumY) / denominator
	intercept = (sumY - slope*sumX) / sumW

	return slope, intercept
}

// pearsonCorrelation is the standard linear correlation coefficient between
// two equal-length series.
func pearsonCorrelation(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n == 0 {
		return 0
	}

	var sumX, sumY, sumXY, sumXX, sumYY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumXX += xs[i] * xs[i]
		sumYY += ys[i] * ys[i]
	}

	numerator := n*sumXY - sumX*sumY
	denominator := math.Sqrt((n*sumXX - sumX*sumX) * (n*sumYY - sumY*sumY))
	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// crrGivenCdA solves the one remaining unknown once CdA is already fixed:
// y - cda*x2 = crr*x1, a one-parameter weighted least squares rather than
// the full 2×2 solve, since only Crr varies by surface.
func crrGivenCdA(observations []coastingObservation, cda float64) float64 {
	var weightedX1Y, weightedX1X1 float64
	for _, o := range observations {
		residual := o.Y - cda*o.X2
		weightedX1Y += o.Weight * o.X1 * residual
		weightedX1X1 += o.Weight * o.X1 * o.X1
	}
	if weightedX1X1 == 0 {
		return 0
	}

	return weightedX1Y / weightedX1X1
}

// crrPerSurface refits Crr for every surface class with enough surviving
// windows to trust a refit over the pooled figure, holding CdA at its
// already-fitted global value. A class this run never saw enough of is
// simply absent from the result — crrForSurface's own fallback to the
// pooled Crr is what a caller uses for it, rather than this function
// inventing a value it has no basis for.
func crrPerSurface(windows []coastingWindow, massKG, cda float64) map[surface.Kind]float64 {
	bySurface := make(map[surface.Kind][]coastingWindow)
	for _, w := range windows {
		bySurface[w.Surface] = append(bySurface[w.Surface], w)
	}

	result := make(map[surface.Kind]float64)
	for kind, classWindows := range bySurface {
		if len(classWindows) < minWindowsPerSurfaceClass {
			continue
		}
		result[kind] = crrGivenCdA(observationsFor(classWindows, massKG), cda)
	}

	return result
}

// quarterlyIntercepts reports the per-ride rolling-resistance intercept
// (CdA held at the group's own fitted value) grouped by calendar quarter,
// for a human to read for a step after new tyres or a wheel change — the
// issue is explicit that this fitter itself must never split a corpus on
// an inferred change point, however tempting the data looks; a step is
// obvious to a person reading five rows and invisible to any automatic
// test that would not also fire on ordinary honest ride-to-ride spread.
// Nothing here does that inference — it only groups and reports.
func quarterlyIntercepts(windows []coastingWindow, rideDates map[string]time.Time, massKG, cda float64) map[string][]float64 {
	byRide := make(map[string][]coastingWindow)
	for _, w := range windows {
		byRide[w.RideID] = append(byRide[w.RideID], w)
	}

	byQuarter := make(map[string][]float64)
	for rideID, rideWindows := range byRide {
		if len(rideWindows) < minWindowsPerRideIntercept {
			continue
		}
		date, ok := rideDates[rideID]
		if !ok {
			continue
		}
		crr := crrGivenCdA(observationsFor(rideWindows, massKG), cda)
		quarter := fmt.Sprintf("%d-Q%d", date.Year(), (int(date.Month())-1)/3+1)
		byQuarter[quarter] = append(byQuarter[quarter], crr)
	}

	return byQuarter
}

// quarterlyCrr is one calendar quarter's median per-ride rolling-resistance
// intercept, for the report's flat table.
type quarterlyCrr struct {
	Quarter string
	Median  float64
	Rides   int
}

// medianByQuarter reduces quarterlyIntercepts' raw values to one median per
// quarter, in chronological order, for the report to print as a flat table.
func medianByQuarter(byQuarter map[string][]float64) []quarterlyCrr {
	quarters := make([]string, 0, len(byQuarter))
	for q := range byQuarter {
		quarters = append(quarters, q)
	}
	sort.Strings(quarters)

	result := make([]quarterlyCrr, 0, len(quarters))
	for _, q := range quarters {
		values := append([]float64(nil), byQuarter[q]...)
		sort.Float64s(values)
		result = append(result, quarterlyCrr{Quarter: q, Median: percentileOf(values, 0.5), Rides: len(values)})
	}

	return result
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
