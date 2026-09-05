package ridemodel

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

// Ride is one recorded activity as a calibration reads it: the three totals the
// two-term model is fitted against, and when it was ridden.
type Ride struct {
	StartedAt      time.Time
	DistanceMetres float64
	MovingSeconds  float64
	AscentMetres   float64
}

// MinRides is the floor below which a fit is refused: fewer rides than this
// cannot separate the two terms from one rider's habits.
const MinRides = 10

// ErrTooFewRides and ErrDegenerate are the two ways a calibration declines,
// each leaving the pair in force untouched.
var (
	ErrTooFewRides = errors.New("ridemodel: too few usable rides to calibrate")
	ErrDegenerate  = errors.New("ridemodel: the rides cannot separate distance from ascent")
)

// A ride shorter or briefer than this is a fragment — a paused start, a trip to
// the shops — and prices distance against noise rather than against riding.
const (
	minRideMetres  = 1000
	minRideSeconds = 60
)

// The robust fit's tuning: huberK is the standard 1.345 in robust-scale units.
// The condition ratio is measured on unit-normalised columns, so it reads
// collinearity alone; 100 admits a correlation up to about 0.98 between
// kilometres and metres climbed before the terms are declared inseparable.
const (
	huberK                      = 1.345
	robustIterations            = 8
	maxAcceptableConditionRatio = 100.0
)

// Fit calibrates the coefficient pair from recorded rides by Huber-weighted
// iteratively reweighted least squares, so a single mis-recorded ride bends the
// result rather than setting it. The clock bounds the corpus against a clock
// skew that would otherwise admit a ride recorded in the future.
// The only errors it returns are ErrTooFewRides and ErrDegenerate: a fit whose
// own terms don't hold up is reported as the latter, never as anything else.
func Fit(rides []Ride, now time.Time) (Coefficients, error) {
	usable := usableRides(rides, now)
	if len(usable) < MinRides {
		return Coefficients{}, ErrTooFewRides
	}

	secondsPerKM, secondsPerAscentM, conditionRatio := irlsFit(usable)
	if conditionRatio > maxAcceptableConditionRatio ||
		secondsPerKM <= 0 || secondsPerAscentM <= 0 ||
		math.IsNaN(secondsPerKM) || math.IsNaN(secondsPerAscentM) {
		return Coefficients{}, ErrDegenerate
	}

	latest := usable[0].StartedAt
	for _, ride := range usable {
		if ride.StartedAt.After(latest) {
			latest = ride.StartedAt
		}
	}
	bias, mae, p90 := inSampleErrors(usable, secondsPerKM, secondsPerAscentM)

	coefficients := Coefficients{
		CalibrationCutoff: latest.UTC().Format(time.DateOnly),
		SecondsPerKM:      secondsPerKM,
		SecondsPerAscentM: secondsPerAscentM,
		EvaluatedRides:    len(usable),
		BiasPercent:       bias,
		MAEPercent:        mae,
		P90Percent:        p90,
	}.WithFingerprint()
	if err := coefficients.Validate(); err != nil {
		// A fit whose own terms don't hold up is exactly the same kind of
		// decline as one the condition ratio already refused: the wrapped
		// cause stays in the error text for a reader, not for the caller.
		return Coefficients{}, fmt.Errorf("%w: %w", ErrDegenerate, err)
	}

	return coefficients, nil
}

// usableRides keeps the rides that can price the model: real riding, finite
// totals, and nothing dated past the moment the fit runs.
func usableRides(rides []Ride, now time.Time) []Ride {
	usable := make([]Ride, 0, len(rides))
	for _, ride := range rides {
		finite := !math.IsNaN(ride.DistanceMetres) && !math.IsInf(ride.DistanceMetres, 0) &&
			!math.IsNaN(ride.MovingSeconds) && !math.IsInf(ride.MovingSeconds, 0) &&
			!math.IsNaN(ride.AscentMetres) && !math.IsInf(ride.AscentMetres, 0)
		if !finite || ride.DistanceMetres < minRideMetres || ride.MovingSeconds < minRideSeconds ||
			ride.AscentMetres < 0 || ride.StartedAt.After(now) {
			continue
		}
		usable = append(usable, ride)
	}

	return usable
}

// inSampleErrors measures the fitted pair against the rides it was fitted over,
// each ride's error as a percentage of its own moving time.
func inSampleErrors(rides []Ride, secondsPerKM, secondsPerAscentM float64) (bias, mae, p90 float64) {
	absolute := make([]float64, 0, len(rides))
	for _, ride := range rides {
		predicted := secondsPerKM*(ride.DistanceMetres/1000) + secondsPerAscentM*ride.AscentMetres
		percent := (predicted - ride.MovingSeconds) / ride.MovingSeconds * 100
		bias += percent / float64(len(rides))
		mae += math.Abs(percent) / float64(len(rides))
		absolute = append(absolute, math.Abs(percent))
	}
	slices.Sort(absolute)

	return bias, mae, percentileOf(absolute, 0.9)
}

// observation is one regression row: moving seconds against kilometres ridden
// and metres climbed.
type observation struct {
	y, x1, x2, weight float64
}

// irlsFit reweights and re-solves the two-term fit, downweighting each pass's
// outliers by the Huber loss, and returns the normal matrix's condition ratio.
func irlsFit(rides []Ride) (secondsPerKM, secondsPerAscentM, conditionRatio float64) {
	weighted := make([]observation, len(rides))
	for i, ride := range rides {
		weighted[i] = observation{
			y: ride.MovingSeconds, x1: ride.DistanceMetres / 1000, x2: ride.AscentMetres, weight: 1,
		}
	}

	residuals := make([]float64, len(weighted))
	for range robustIterations {
		secondsPerKM, secondsPerAscentM, conditionRatio = solve2x2(weighted)

		for i, o := range weighted {
			residuals[i] = o.y - secondsPerKM*o.x1 - secondsPerAscentM*o.x2
		}
		scale := robustScale(residuals)
		if scale == 0 {
			break
		}
		for i := range weighted {
			z := math.Abs(residuals[i] / scale)
			weighted[i].weight = 1
			if z > huberK {
				weighted[i].weight = huberK / z
			}
		}
	}

	return secondsPerKM, secondsPerAscentM, conditionRatio
}

// solve2x2 fits y = a*x1 + b*x2 by weighted least squares with no intercept,
// returning the normal matrix's eigenvalue ratio from the same solve.
func solve2x2(observations []observation) (a, b, conditionRatio float64) {
	var sxx1, sxx12, sxx2, sx1y, sx2y float64
	for _, o := range observations {
		sxx1 += o.weight * o.x1 * o.x1
		sxx12 += o.weight * o.x1 * o.x2
		sxx2 += o.weight * o.x2 * o.x2
		sx1y += o.weight * o.x1 * o.y
		sx2y += o.weight * o.x2 * o.y
	}

	det := sxx1*sxx2 - sxx12*sxx12
	// The weighted normal matrix is positive-semidefinite, so a negative
	// determinant is rounding on near-collinear columns rather than a real
	// solution; treated as degenerate the same as an exact zero.
	if det <= 0 {
		return 0, 0, math.Inf(1)
	}
	a = (sx1y*sxx2 - sx2y*sxx12) / det
	b = (sxx1*sx2y - sxx12*sx1y) / det

	// Normalised to unit columns the matrix is [[1 r] [r 1]], whose eigenvalues
	// are 1±|r|: the ratio then says how collinear the columns are, not how big.
	r := math.Abs(sxx12) / math.Sqrt(sxx1*sxx2)
	if r >= 1 {
		return a, b, math.Inf(1)
	}

	return a, b, (1 + r) / (1 - r)
}

// robustScale is the median absolute deviation scaled to a normal
// distribution's standard deviation, which is the unit huberK is tuned in.
func robustScale(residuals []float64) float64 {
	if len(residuals) == 0 {
		return 0
	}
	sorted := slices.Clone(residuals)
	slices.Sort(sorted)
	median := percentileOf(sorted, 0.5)

	deviations := make([]float64, len(sorted))
	for i, residual := range sorted {
		deviations[i] = math.Abs(residual - median)
	}
	slices.Sort(deviations)

	return percentileOf(deviations, 0.5) * 1.4826
}

// percentileOf reads a fractional position from an already-sorted slice by
// linear interpolation between the two nearest ranks.
func percentileOf(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
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
