package activity

import (
	"math"
	"slices"
	"testing"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// matchOrigin is the arbitrary point every case here is laid out around.
func matchOrigin() measure.Coordinate {
	return measure.Coordinate{Latitude: 49.9, Longitude: 8.2}
}

// pointAt returns the coordinate the given metres east and north of the origin, so a
// case reads as the shape it is rather than as degrees.
func pointAt(eastMetres, northMetres float64) measure.Coordinate {
	metresPerDegree := measure.EarthRadiusMetres * math.Pi / 180
	origin := matchOrigin()

	return measure.Coordinate{
		Latitude:  origin.Latitude + northMetres/metresPerDegree,
		Longitude: origin.Longitude + eastMetres/(metresPerDegree*math.Cos(origin.Latitude*math.Pi/180)),
	}
}

// line walks the corners given, sampling every 20 m so a corridor test is not
// decided by where the corners happen to fall.
func line(corners ...[2]float64) []measure.Coordinate {
	const stepMetres = 20.0

	points := []measure.Coordinate{}
	for index := 1; index < len(corners); index++ {
		start, end := corners[index-1], corners[index]
		length := math.Hypot(end[0]-start[0], end[1]-start[1])
		steps := max(int(length/stepMetres), 1)
		for step := range steps {
			ratio := float64(step) / float64(steps)
			points = append(points, pointAt(start[0]+ratio*(end[0]-start[0]), start[1]+ratio*(end[1]-start[1])))
		}
	}
	if len(corners) > 0 {
		last := corners[len(corners)-1]
		points = append(points, pointAt(last[0], last[1]))
	}

	return points
}

// loop is a 2 km square, the route most of these cases are ridden on.
func loop() []measure.Coordinate {
	return line([2]float64{0, 0}, [2]float64{500, 0}, [2]float64{500, 500}, [2]float64{0, 500}, [2]float64{0, 0})
}

func candidate(routeID int64, geometry []measure.Coordinate) RouteCandidate {
	return RouteCandidate{Key: route.NewKey(route.ProviderVeloPlanner, routeID, 1), Geometry: geometry}
}

// reversed returns the same geometry ridden the other way round.
func reversed(points []measure.Coordinate) []measure.Coordinate {
	flipped := make([]measure.Coordinate, 0, len(points))
	for _, point := range slices.Backward(points) {
		flipped = append(flipped, point)
	}

	return flipped
}

// scattered offsets every other point sideways, standing in for the reading a
// consumer GPS gives of a line it is following exactly.
func scattered(points []measure.Coordinate, metres float64) []measure.Coordinate {
	metresPerDegree := measure.EarthRadiusMetres * math.Pi / 180
	noisy := make([]measure.Coordinate, 0, len(points))
	for index, point := range points {
		offset := metres
		if index%2 == 0 {
			offset = -metres
		}
		noisy = append(noisy, measure.Coordinate{
			Latitude:  point.Latitude + offset/metresPerDegree,
			Longitude: point.Longitude,
		})
	}

	return noisy
}

func TestMatchRouteClaimsTheLoopItWasRiddenRound(t *testing.T) {
	match, found := MatchRoute(loop(), []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.Equal(t, route.NewKey(route.ProviderVeloPlanner, 1, 1), match.Key)
	assert.InDelta(t, 1, match.RouteCoverage, 0.01)
	assert.InDelta(t, 1, match.RideCoverage, 0.01)
}

// The corridor exists for the reading, not for the plan: a track that wanders
// either side of the line is still the same ride.
func TestMatchRouteClaimsALoopReadThroughGPSScatter(t *testing.T) {
	match, found := MatchRoute(scattered(loop(), 15), []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.InDelta(t, 1, match.RouteCoverage, 0.02)
}

func TestMatchRouteRefusesALoopRiddenOnlyHalfway(t *testing.T) {
	half := loop()[:len(loop())/2]

	_, found := MatchRoute(half, []RouteCandidate{candidate(1, loop())})

	assert.False(t, found)
}

// Just over the gate the ride is still the route's: a rider who cut the last
// corner rode it.
func TestMatchRouteClaimsALoopRiddenAllButItsLastStretch(t *testing.T) {
	almost := loop()[:int(float64(len(loop()))*0.96)]

	match, found := MatchRoute(almost, []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.Greater(t, match.RouteCoverage, minimumRouteCoverage)
	assert.InDelta(t, 1, match.RideCoverage, 0.01)
}

func TestMatchRouteClaimsARouteRiddenTheOtherWayRound(t *testing.T) {
	match, found := MatchRoute(reversed(loop()), []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.InDelta(t, 1, match.RouteCoverage, 0.01)
}

// Two routes sharing a long approach: the ride rode one of them to the end, and
// the shared stretch must not hand it to the other.
func TestMatchRoutePrefersTheRouteTheRideActuallyFinished(t *testing.T) {
	approach := [][2]float64{{0, 0}, {1000, 0}}
	ridden := line(append(approach, [2]float64{1000, 800})...)
	other := line(append(approach, [2]float64{1000, -800})...)

	match, found := MatchRoute(ridden, []RouteCandidate{candidate(1, other), candidate(2, ridden)})

	require.True(t, found)
	assert.Equal(t, int64(2), match.Key.SourceRouteID())
}

// A ride that covers a short route entirely on its way round a long one belongs
// to the long one: the short route explains a fraction of the ride.
func TestMatchRouteAttributesALongRideToTheRouteItFollowed(t *testing.T) {
	long := loop()
	short := line([2]float64{0, 0}, [2]float64{200, 0})

	match, found := MatchRoute(long, []RouteCandidate{candidate(1, short), candidate(2, long)})

	require.True(t, found)
	assert.Equal(t, int64(2), match.Key.SourceRouteID())
	assert.InDelta(t, 1, match.RideCoverage, 0.01)
}

// A day out that takes a short stage in on its way did not ride that stage.
// Without this the stage's history would fill with rides that were not it.
func TestMatchRouteRefusesARouteTheRideMerelyTookIn(t *testing.T) {
	short := line([2]float64{0, 0}, [2]float64{200, 0})

	_, found := MatchRoute(loop(), []RouteCandidate{candidate(1, short)})

	assert.False(t, found)
}

// A commute either side of the route is still a ride of the route.
func TestMatchRouteClaimsARouteRiddenWithAShortApproachEitherSide(t *testing.T) {
	approach := line([2]float64{-60, 0}, [2]float64{0, 0})
	ride := append(append([]measure.Coordinate{}, approach...), loop()...)
	ride = append(ride, reversed(approach)...)

	match, found := MatchRoute(ride, []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.InDelta(t, 1, match.RouteCoverage, 0.02)
	assert.Greater(t, match.RideCoverage, minimumRideCoverage)
}

func TestMatchRouteFindsNothingForARideNowhereNearTheLibrary(t *testing.T) {
	elsewhere := line([2]float64{50000, 50000}, [2]float64{50500, 50000}, [2]float64{50500, 50500})

	_, found := MatchRoute(elsewhere, []RouteCandidate{candidate(1, loop())})

	assert.False(t, found)
}

// A ride whose route is covered by two routes over the same roads is recorded
// against the same one every time, whatever order the library arrives in.
func TestMatchRouteSettlesTwinRoutesTheSameWayWhicheverOrderTheyArriveIn(t *testing.T) {
	first := candidate(7, loop())
	second := candidate(3, loop())

	forwards, foundForwards := MatchRoute(loop(), []RouteCandidate{first, second})
	backwards, foundBackwards := MatchRoute(loop(), []RouteCandidate{second, first})

	require.True(t, foundForwards)
	require.True(t, foundBackwards)
	assert.Equal(t, backwards.Key, forwards.Key)
	assert.Equal(t, int64(3), forwards.Key.SourceRouteID())
}

func TestMatchRouteNeedsATrackAndALibrary(t *testing.T) {
	_, withoutLibrary := MatchRoute(loop(), nil)
	_, withoutTrack := MatchRoute(nil, []RouteCandidate{candidate(1, loop())})
	_, withOnePoint := MatchRoute(loop()[:1], []RouteCandidate{candidate(1, loop())})

	assert.False(t, withoutLibrary)
	assert.False(t, withoutTrack)
	assert.False(t, withOnePoint)
}

// A stored route with no length cannot be covered, and must not divide by zero
// or claim a ride that passed its point.
func TestMatchRouteIgnoresARouteWithNoLength(t *testing.T) {
	degenerate := []measure.Coordinate{pointAt(0, 0), pointAt(0, 0)}

	_, found := MatchRoute(loop(), []RouteCandidate{candidate(1, degenerate)})

	assert.False(t, found)
}

// An out-and-back's return leg is the same road as its outward leg, so a ride
// that only went out lies within the corridor of all of it. Riding a road once
// covers it once: the rider who turned back early did not ride the route.
func TestMatchRouteRefusesAnOutAndBackRiddenOnlyOutward(t *testing.T) {
	outward := line([2]float64{0, 0}, [2]float64{2000, 0})
	outAndBack := append(append([]measure.Coordinate{}, outward...), reversed(outward)...)

	_, found := MatchRoute(outward, []RouteCandidate{candidate(1, outAndBack)})

	assert.False(t, found)
}

func TestMatchRouteClaimsAnOutAndBackRiddenBothWays(t *testing.T) {
	outward := line([2]float64{0, 0}, [2]float64{2000, 0})
	outAndBack := append(append([]measure.Coordinate{}, outward...), reversed(outward)...)

	match, found := MatchRoute(outAndBack, []RouteCandidate{candidate(1, outAndBack)})

	require.True(t, found)
	assert.InDelta(t, 1, match.RouteCoverage, 0.01)
}

// A loop ridden twice over rode all of it, and the second lap is not a reason
// to doubt the first.
func TestMatchRouteClaimsALoopRiddenTwice(t *testing.T) {
	twice := append(append([]measure.Coordinate{}, loop()...), loop()...)

	match, found := MatchRoute(twice, []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.InDelta(t, 1, match.RouteCoverage, 0.01)
}

func TestMatchRouteReadsALoopRiddenTheWayItWasPlanned(t *testing.T) {
	match, found := MatchRoute(loop(), []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.Equal(t, DirectionForward, match.Direction)
}

// The same loop the other way round is the same route and a different ride.
func TestMatchRouteReadsALoopRiddenTheOtherWayRound(t *testing.T) {
	match, found := MatchRoute(reversed(loop()), []RouteCandidate{candidate(1, loop())})

	require.True(t, found)
	assert.Equal(t, DirectionReverse, match.Direction)
}

// A loop joined part way round is still ridden the way it was planned: the
// advance is measured around the route, not from its stored first point.
func TestMatchRouteReadsALoopJoinedPartWayRound(t *testing.T) {
	whole := loop()
	joined := append(append([]measure.Coordinate{}, whole[len(whole)/3:]...), whole[:len(whole)/3]...)

	match, found := MatchRoute(joined, []RouteCandidate{candidate(1, whole)})

	require.True(t, found)
	assert.Equal(t, DirectionForward, match.Direction)
}

// An out-and-back advances as far one way as the other, so it has no direction
// rather than whichever sign the noise came to.
func TestMatchRouteGivesAnOutAndBackNoDirection(t *testing.T) {
	outward := line([2]float64{0, 0}, [2]float64{2000, 0})
	outAndBack := append(append([]measure.Coordinate{}, outward...), reversed(outward)...)

	match, found := MatchRoute(outAndBack, []RouteCandidate{candidate(1, outAndBack)})

	require.True(t, found)
	assert.Equal(t, DirectionUnknown, match.Direction)
}

func TestDirectionNamesRoundTrip(t *testing.T) {
	for _, direction := range []Direction{DirectionUnknown, DirectionForward, DirectionReverse} {
		assert.Equal(t, direction, ParseDirection(direction.String()), "%v", direction)
	}
	assert.Equal(t, DirectionUnknown, ParseDirection("sideways"))
}

// An open route has no wrap: a ride of its whole length recorded sparsely was
// read as having gone nowhere while every delta was taken around the route.
func TestMatchRouteReadsASparselyRecordedOpenRoute(t *testing.T) {
	open := line([2]float64{0, 0}, [2]float64{3000, 0})
	sparse := []measure.Coordinate{open[0], open[len(open)-1]}

	match, found := MatchRoute(sparse, []RouteCandidate{candidate(1, open)})

	require.True(t, found)
	assert.Equal(t, DirectionForward, match.Direction)
}

func TestMatchRouteReadsAnOpenRouteRiddenEachWay(t *testing.T) {
	open := line([2]float64{0, 0}, [2]float64{3000, 0}, [2]float64{3000, 2000})

	forward, found := MatchRoute(open, []RouteCandidate{candidate(1, open)})
	require.True(t, found)
	assert.Equal(t, DirectionForward, forward.Direction)

	backward, found := MatchRoute(reversed(open), []RouteCandidate{candidate(1, open)})
	require.True(t, found)
	assert.Equal(t, DirectionReverse, backward.Direction)
}
