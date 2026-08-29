package ridemodel

import (
	"math"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// earthRadiusMetres is the spherical model shared with the route and surface
// packages. Its own copy: route's is unexported.
const earthRadiusMetres = 6_371_000.0

// gradientWindowMetres is the shortest span a gradient is measured over,
// matching internal/route and lib/profile.ts. Per-segment gradients produce
// absurd speeds and make two recordings differ by point density alone.
const gradientWindowMetres = 100.0

const gravityMetresPerSecondSquared = 9.80665

const (
	// minSolveSpeedMetresPerSecond bounds the bisection below zero so the solver
	// never divides by a stationary speed.
	minSolveSpeedMetresPerSecond = 0.01
	// maxSolveSpeedMetresPerSecond bounds the bisection above any speed a
	// pedalling rider reaches, so the bracket always contains the true root.
	maxSolveSpeedMetresPerSecond = 30.0
	// bisectionIterations is fixed rather than loop-until-converged, which
	// guarantees termination on every segment.
	bisectionIterations = 60
)

// modelVersion identifies the exact set of constants below, which do not live in
// a coefficient file. Load mixes it into a loaded file's fingerprint: bump it
// whenever any constant in this block changes, or a stale cached duration would
// look as current as it did before the upgrade.
const modelVersion = "hybrid-v2"

// The non-varying half of the hybrid model: independently defensible physical
// constants, not values this service's ride corpus can reliably identify.
const (
	// hybridPhysicsWeight is the physics half's share of the blended time; the
	// linear half takes the remainder. Not zero because the linear half is blind
	// to how ascent is distributed — 200 m up one wall and 200 m of rolling ground
	// are the same input to it — and a quarter leaves the blend near-unbiased.
	hybridPhysicsWeight    = 0.25
	fixedDriveEfficiency   = 0.975
	fixedAirDensityKGPerM3 = 1.225
	// fixedSpeedCapMetresPerSecond is the fastest this model will time any segment,
	// standing in for a rider braking into a descent. It is not gated on a negative
	// gradient: Load admits profiles that solve past it on level ground, and capping
	// only below zero would put a discontinuity at exactly zero gradient.
	fixedSpeedCapMetresPerSecond = 60.0 / 3.6 // 60 km/h
)

// Result is one stage's predicted moving time: the total, and the running time at
// every point of the geometry. Two consumers need a time at an arbitrary point
// and neither can reconstruct one from a scalar.
type Result struct {
	CumulativeSeconds []float64
	MovingSeconds     float64
}

// Predict runs the forward model over one stage's geometry: a weighted average,
// per segment, of the fixed-physics time and the route-calibrated linear time,
// computed per segment so the running total stays aligned 1:1 with the geometry.
// kinds is the surface class of each point; nil or short reads as asphalt
// throughout. False when the geometry has no usable elevation, which both halves
// need.
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

		physicsSpeed := segmentSpeedMetresPerSecond(gradients[index], coefficients.crr(kind), coefficients)
		physicsSeconds := span / physicsSpeed

		// The raw, unwindowed rise — not gradients[index] — because the linear half's
		// ascent term matches route.Route.ElevationGainMetres(), which is what
		// seconds_per_ascent_m was calibrated against.
		rise := *points[index].Elevation - *points[index-1].Elevation
		linearSeconds := coefficients.SecondsPerKM*(span/1000) + coefficients.SecondsPerAscentM*math.Max(0, rise)

		cumulative[index] = cumulative[index-1] +
			hybridPhysicsWeight*physicsSeconds + (1-hybridPhysicsWeight)*linearSeconds
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
// gradient over the trailing window ending at it. Signed because climbing and
// descending segments are solved differently.
//
// ponytail: near the start of a stage fewer than gradientWindowMetres of history
// exist, so this uses whatever span is available. Upgrade path: extend the window
// forward if a smoother early gradient is ever worth the complexity.
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

// segmentSpeedMetresPerSecond solves one segment at the configured power,
// whatever its gradient, and brakes the answer to the descent cap. There is no
// freewheeling branch: adding power to a coasting bike can only make it faster,
// and the cutoff that produced one put a cliff just past the threshold.
//
// The cap applies at every gradient; see fixedSpeedCapMetresPerSecond.
//
//nolint:gocritic // value param: same reasoning as Predict above.
func segmentSpeedMetresPerSecond(gradientPercent, crr float64, coefficients Coefficients) float64 {
	sinTheta, cosTheta := gradientTrig(gradientPercent)

	return min(poweredSpeedMetresPerSecond(crr, sinTheta, cosTheta, coefficients), fixedSpeedCapMetresPerSecond)
}

// gradientTrig converts a percent gradient to sin and cos of the slope angle
// exactly, from its tangent. It stays well-behaved at any gradient, including the
// pathologies a noisy elevation window produces: cosTheta approaches zero and
// sinTheta ±1 rather than either diverging.
func gradientTrig(gradientPercent float64) (sinTheta, cosTheta float64) {
	tanTheta := gradientPercent / 100
	cosTheta = 1 / math.Sqrt(1+tanTheta*tanTheta)
	sinTheta = tanTheta * cosTheta

	return sinTheta, cosTheta
}

// poweredSpeedMetresPerSecond solves P·η = v·(Crr·m·g·cosθ + m·g·sinθ +
// ½·ρ·CdA·v²) for v by bisection on a bounded interval. The right-hand side is
// zero at v=0 and grows without bound, so a sign change always exists in the
// bracket; a fixed iteration count guarantees termination.
//
//nolint:gocritic // value param: same reasoning as Predict above.
func poweredSpeedMetresPerSecond(crr, sinTheta, cosTheta float64, coefficients Coefficients) float64 {
	target := coefficients.PowerWatts * fixedDriveEfficiency
	gravityTerm := coefficients.MassKG * gravityMetresPerSecondSquared * (crr*cosTheta + sinTheta)
	residual := func(speed float64) float64 {
		return speed*(gravityTerm+0.5*fixedAirDensityKGPerM3*coefficients.CdAM2*speed*speed) - target
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
