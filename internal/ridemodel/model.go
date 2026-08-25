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
)

// modelVersion identifies the exact set of constants below. #213 accepted
// them as fixed physical priors rather than operator-configurable inputs —
// mass, power, CdA and Crr still vary per coefficient file, but these do not.
// Load mixes this into a loaded file's fingerprint precisely because they
// don't live in the file: bump it whenever any of the constants in this block
// changes, or a code upgrade that changes what a stage's prediction means
// would leave a stale cached duration looking exactly as current as it did
// before the upgrade.
const modelVersion = "hybrid-v2"

// The non-varying half of #213's accepted hybrid model: independently
// defensible physical constants, not values #239's benchmark found this
// service's ride corpus can reliably identify on its own.
const (
	// hybridPhysicsWeight is the physics half's share of the blended time; the
	// route-calibrated linear half takes the remainder. #213 settled on an
	// equal weighting.
	hybridPhysicsWeight    = 0.5
	fixedDriveEfficiency   = 0.975
	fixedAirDensityKGPerM3 = 1.225
	// fixedDescentCapMetresPerSecond stands in for a rider braking into a
	// descent rather than letting one run away, and is the only bound on a
	// descending segment: the rider holds the configured power there as
	// everywhere else. See segmentSpeedMetresPerSecond for why freewheeling
	// is not modelled separately.
	fixedDescentCapMetresPerSecond = 60.0 / 3.6 // 60 km/h
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

// Predict runs the forward model over one stage's geometry: an equal-weight
// average, per segment, of the fixed-physics time and the route-calibrated
// linear time — #213's accepted hybrid, computed once per segment so the
// running total stays aligned 1:1 with the geometry rather than only agreeing
// with the benchmark on the stage's grand total. kinds is the surface class of
// each point, aligned with points; a nil or short kinds is read as asphalt
// throughout, which is how a stage with no cached classification is timed,
// though every surface currently resolves to the same Crr regardless — see
// Coefficients.crr. The second return is false when the geometry has no
// usable elevation, in which case Result is the zero value and carries no
// prediction: the linear half needs elevation exactly as much as the physics
// half does, so one gate in front of both is enough.
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

		// The raw, unwindowed rise — not gradients[index] — because the linear
		// half's ascent term is #239's own definition of ascent: the same
		// positive-delta sum route.Stage.ElevationGainMetres() reports, which is
		// what seconds_per_ascent_m was calibrated against.
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

// segmentSpeedMetresPerSecond solves one segment at the configured power,
// whatever its gradient, and brakes the answer to the descent cap.
//
// An earlier model switched to freewheeling below a -1% gradient. Adding
// power to a coasting bike can only make it faster — both branches solve the
// same cubic, one with P·η on the right where the other has zero — so that
// branch could only ever credit a segment less speed than the rider was
// already producing, and just past the cutoff it credited far less: 11 km/h
// down a 1.5% grade a rider covers at better than 30. #239's corpus scores a
// steeper cutoff and no cutoff at all within 0.01 percentage points of each
// other, so nothing is lost by dropping the branch that produced the cliff.
//
//nolint:gocritic // value param: same reasoning as Predict above.
func segmentSpeedMetresPerSecond(gradientPercent, crr float64, coefficients Coefficients) float64 {
	sinTheta, cosTheta := gradientTrig(gradientPercent)

	return min(poweredSpeedMetresPerSecond(crr, sinTheta, cosTheta, coefficients), fixedDescentCapMetresPerSecond)
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
