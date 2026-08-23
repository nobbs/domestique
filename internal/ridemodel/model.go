package ridemodel

import (
	"math"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// earthRadiusMetres is the spherical Earth model shared with the route and
// surface packages, so a length measured here agrees with the distance shown
// beside it. It is kept as its own copy rather than imported: route's is
// unexported, and surface already keeps its own copy for the same reason.
const earthRadiusMetres = 6_371_000.0

// gradientWindowMetres is the shortest span a gradient is measured over,
// matching internal/route's MaxGradientPercent and lib/profile.ts. Fed
// per-segment gradients, the model produces absurd speeds along ordinary
// roads, and two recordings of the same road would differ purely by point
// density.
const gradientWindowMetres = 100.0

const gravityMetresPerSecondSquared = 9.80665

const (
	// minSolveSpeedMetresPerSecond bounds the bisection below zero so the
	// solver never divides by a stationary speed, and is what a segment gets
	// when even the slowest bracketed speed cannot absorb the configured
	// power — a vertical-wall pathology rather than a normal climb.
	minSolveSpeedMetresPerSecond = 0.01
	// maxSolveSpeedMetresPerSecond bounds the bisection above any speed a
	// pedalling rider reaches, so the bracket always contains the true root.
	maxSolveSpeedMetresPerSecond = 30.0
	// bisectionIterations is fixed rather than loop-until-converged, which is
	// what guarantees the solver terminates on every segment, pathological
	// gradients included.
	bisectionIterations = 60
	// minMeaningfulCoastingSpeedMetresPerSecond is the floor below which
	// coasting is treated as not applicable rather than as a valid, if slow,
	// answer. Right at the descent cutoff, the gravity component pulling a
	// freewheeling bike forward can be a floating-point hair away from the
	// rolling resistance opposing it; the driving force this produces is
	// technically positive but arbitrarily small, and the coasting equation
	// has no lower bound of its own the way the powered bisection's bracket
	// does. A rider would simply pedal through ground that shallow rather
	// than freewheel at a near-standstill, so this sends the segment to the
	// powered branch instead of crediting a below-walking-pace crawl.
	minMeaningfulCoastingSpeedMetresPerSecond = 1.0
)

// Result is one stage's predicted moving time: the total, and the running time
// at every point of the geometry it was computed from. The cumulative series is
// stored alongside the total because two downstream consumers need a time at an
// arbitrary point along the stage and neither can reconstruct one from a
// scalar.
type Result struct {
	CumulativeSeconds []float64
	MovingSeconds     float64
}

// Predict runs the forward model over one stage's geometry. kinds is the
// surface class of each point, aligned with points; a nil or short kinds is
// read as asphalt throughout, which is how a stage with no cached
// classification is timed. The second return is false when the geometry has no
// usable elevation, in which case Result is the zero value and carries no
// prediction.
//
//nolint:gocritic // value param: Coefficients is immutable once loaded, and a pointer would let a caller mutate the shared instance mid-prediction.
func Predict(points []route.Point, kinds []surface.Kind, coefficients Coefficients) (Result, bool) {
	if !hasCompleteElevation(points) {
		return Result{}, false
	}

	distances := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		distances[index] = distances[index-1] + haversineMetres(points[index-1], points[index])
	}
	gradients := windowedGradientPercent(distances, points)

	cumulative := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		span := distances[index] - distances[index-1]
		if span <= 0 {
			cumulative[index] = cumulative[index-1]

			continue
		}

		kind := surface.KindAsphalt
		if index-1 < len(kinds) {
			kind = kinds[index-1]
		}

		speed := segmentSpeedMetresPerSecond(gradients[index], coefficients.crr(kind), coefficients)
		cumulative[index] = cumulative[index-1] + span/speed
	}

	return Result{
		MovingSeconds:     cumulative[len(cumulative)-1],
		CumulativeSeconds: cumulative,
	}, true
}

func hasCompleteElevation(points []route.Point) bool {
	if len(points) < 2 {
		return false
	}
	for _, point := range points {
		if point.Elevation == nil {
			return false
		}
	}

	return true
}

// windowedGradientPercent returns, for every point but the first, the signed
// gradient over the trailing window ending at it — the same window
// route.Stage.MaxGradientPercent measures, kept signed here because climbing
// and descending segments are solved differently.
//
// ponytail: near the start of a stage, fewer than gradientWindowMetres of
// history exist yet. Rather than skip those points as MaxGradientPercent does
// — which has nothing to report and still needs a speed for every segment —
// this uses whatever span is available, degrading toward the raw point-to-point
// gradient for the first segment. Upgrade path: extend the window forward past
// the first full window's worth of points if a smoother early gradient is ever
// worth the complexity.
func windowedGradientPercent(distances []float64, points []route.Point) []float64 {
	gradients := make([]float64, len(points))
	trailing := 0
	for leading := 1; leading < len(points); leading++ {
		for distances[leading]-distances[trailing+1] >= gradientWindowMetres {
			trailing++
		}
		span := distances[leading] - distances[trailing]
		if span <= 0 {
			continue
		}
		rise := *points[leading].Elevation - *points[trailing].Elevation
		gradients[leading] = rise / span * 100
	}

	return gradients
}

// segmentSpeedMetresPerSecond picks the branch: below the cutoff the rider
// coasts, at or above it the rider holds the configured power.
//
// Coasting is offered, never forced. On rough ground with a shallow cutoff, a
// gradient just past it can still leave rolling resistance stronger than the
// gravity component pulling the coasting bike forward — gravity alone would
// have the bike stall rather than freewheel. A real rider would simply keep
// pedalling through a stretch that mild, so this falls back to the powered
// branch rather than crediting that one segment a near-zero crawl, which
// would otherwise let a single borderline segment dominate a whole stage's
// time.
//
//nolint:gocritic // value param: same reasoning as Predict above.
func segmentSpeedMetresPerSecond(gradientPercent, crr float64, coefficients Coefficients) float64 {
	sinTheta, cosTheta := gradientTrig(gradientPercent)
	if gradientPercent <= coefficients.DescentCutoffPercent {
		if speed, coasting := coastingSpeedMetresPerSecond(crr, sinTheta, cosTheta, coefficients); coasting {
			return speed
		}
	}

	return poweredSpeedMetresPerSecond(crr, sinTheta, cosTheta, coefficients)
}

// gradientTrig converts a percent gradient (rise over run, as a percentage) to
// sin and cos of the slope angle exactly, from its tangent. This stays
// well-behaved at any gradient, including the near-vertical and reversed
// pathologies a noisy elevation window can produce: cosTheta approaches zero
// and sinTheta approaches ±1 rather than either diverging.
func gradientTrig(gradientPercent float64) (sinTheta, cosTheta float64) {
	tanTheta := gradientPercent / 100
	cosTheta = 1 / math.Sqrt(1+tanTheta*tanTheta)
	sinTheta = tanTheta * cosTheta

	return sinTheta, cosTheta
}

// poweredSpeedMetresPerSecond solves P·η = v·(Crr·m·g·cosθ + m·g·sinθ +
// ½·ρ·CdA·v²) for v by bisection on a bounded interval. The right-hand side is
// zero at v=0 and grows without bound as v→∞ regardless of gradient, so a sign
// change always exists in the bracket; bisecting a fixed number of iterations
// rather than looping to convergence is what guarantees termination on every
// segment.
//
//nolint:gocritic // value param: same reasoning as Predict above.
func poweredSpeedMetresPerSecond(crr, sinTheta, cosTheta float64, coefficients Coefficients) float64 {
	target := coefficients.PowerWatts * coefficients.DriveEfficiency
	gravityTerm := coefficients.MassKG * gravityMetresPerSecondSquared * (crr*cosTheta + sinTheta)
	residual := func(speed float64) float64 {
		return speed*(gravityTerm+0.5*coefficients.AirDensityKGPerM3*coefficients.CdAM2*speed*speed) - target
	}

	low, high := minSolveSpeedMetresPerSecond, maxSolveSpeedMetresPerSecond
	if residual(high) <= 0 || residual(low) >= 0 {
		// Either extreme means the bracket holds no sign change: a climb so
		// steep, relative to the configured power, that no speed the solver
		// considers balances it. Report the crawl speed the solver bottoms out
		// at rather than extrapolating past the bracket.
		return low
	}
	for range bisectionIterations {
		mid := (low + high) / 2
		if residual(mid) > 0 {
			high = mid
		} else {
			low = mid
		}
	}

	return (low + high) / 2
}

// coastingSpeedMetresPerSecond solves 0 = Crr·m·g·cosθ + m·g·sinθ +
// ½·ρ·CdA·v² for the speed at which gravity balances drag and rolling
// resistance with no pedalling power, then bounds it. The upper bound has no
// physical justification of its own — it stands in for a rider braking into a
// descent rather than free-wheeling to whatever speed drag alone would allow.
//
// ok is false when coasting does not clearly apply, in which case the caller
// falls back to the powered branch: either gravity alone cannot overcome
// rolling resistance — right at the cutoff boundary on rough ground, coasting
// would stall rather than freewheel — or it can, but only by a floating-point
// hair, which the coasting equation has no lower bound to protect against the
// way the powered bisection's bracket does. A rider would pedal through
// ground that shallow rather than freewheel at a near-standstill, so both
// cases defer to the powered branch instead of crediting a below-walking-pace
// crawl.
//
//nolint:gocritic // value param: same reasoning as Predict above.
func coastingSpeedMetresPerSecond(crr, sinTheta, cosTheta float64, coefficients Coefficients) (speed float64, ok bool) {
	drivingForce := coefficients.MassKG * gravityMetresPerSecondSquared * (-sinTheta - crr*cosTheta)
	if drivingForce <= 0 {
		return 0, false
	}

	speed = math.Sqrt(2 * drivingForce / (coefficients.AirDensityKGPerM3 * coefficients.CdAM2))
	if speed < minMeaningfulCoastingSpeedMetresPerSecond {
		return 0, false
	}

	return min(speed, coefficients.DescentCapMetresPerSecond), true
}

// haversineMetres returns the great-circle distance between two points. It
// matches the spherical model internal/route and internal/surface use, each
// keeping its own copy for the reason earthRadiusMetres above states.
func haversineMetres(left, right route.Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	chord := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(chord), math.Sqrt(1-chord))
}
