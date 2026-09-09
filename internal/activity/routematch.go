package activity

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

const (
	// corridorMetres is how far a recorded position may sit from a route's line
	// and still be riding it. Wider than the surface matcher's snap radius: that
	// one places a planned line on a mapped centreline, this one places a
	// consumer GPS on a planned line, through tree cover and beside buildings.
	corridorMetres = 40.0

	// maximumCoverageRatio is the route length one metre of riding may be
	// credited with: 15 per cent more than itself, and no further. A route that
	// doubles back along its own road lies wholly inside the corridor of a ride
	// that only went out, and would otherwise be recorded as ridden whole; a
	// recorded track is if anything longer than the line it followed, so riding
	// that did happen stays well under this.
	maximumCoverageRatio = 1.15

	// minimumRouteCoverage is the share of a route the ride must have covered,
	// and minimumRideCoverage the share of the ride that must have lain on it.
	// Both are needed: the first says the route was ridden, the second that the
	// ride was that route rather than a day out taking it in on the way, which
	// would put a six-hour ride in a short route's history.
	//
	// Fitted to the operator's own judgement over 89 decided rides, and swept
	// against every share from 0.85 to 0.95: this is the last one at which no
	// ride clears the gate against two routes at once, so the order below never
	// has to settle anything. Loosening it admits rides that merely followed
	// most of a route and brings contested rides with them; tightening it starts
	// refusing rides the operator called their own. The margin below one is what
	// a closure detour and a commute either side cost.
	minimumRouteCoverage = 0.92
	minimumRideCoverage  = 0.92

	// minimumDirectionShare is how much of a route's length a ride must have
	// advanced along, one way or the other, before it is called a direction. A
	// route ridden out and back nets nothing however far it went, and is left
	// without one rather than given whichever sign the noise came to.
	minimumDirectionShare = 0.5

	// maximumDirectionStepShare is how far around a closed route two positions
	// in a row may be and still say which way the ride went between them. A
	// wrapped step beyond this could as well have gone the other way about, and
	// wrapping picks a sign rather than admitting it cannot tell.
	maximumDirectionStepShare = 0.25
)

// RouteCandidate is one library route a ride may be attributed to.
type RouteCandidate struct {
	Geometry []measure.Coordinate
	// Elevations is the height at each of those coordinates, or nil for a route
	// whose stored geometry carries none. It is what the route's climbs are
	// found from; a route without it is one no ride can be timed over.
	Elevations []float64
	Key        route.Key
}

// Direction is which way round its route a ride went. It does not decide a
// match — a loop ridden anticlockwise is the same loop — but a route ridden the
// other way is not the same ride: its climbs are its descents, so a consumer
// comparing rides over a route reads this before pooling them.
type Direction int

const (
	// DirectionUnknown is a ride whose direction could not be told, which an
	// out-and-back never can: it advances as far one way as the other.
	DirectionUnknown Direction = iota
	// DirectionForward is a ride that ran the way the route is stored.
	DirectionForward
	// DirectionReverse is a ride that ran the other way round it.
	DirectionReverse
)

// String returns the stable name a direction is stored and served under.
func (d Direction) String() string {
	switch d {
	case DirectionForward:
		return "forward"
	case DirectionReverse:
		return "reverse"
	case DirectionUnknown:
	}

	return "unknown"
}

// ParseDirection is String's inverse. An unrecognised name — from a row written
// by a later version of this package — reads as unknown rather than failing.
func ParseDirection(name string) Direction {
	switch name {
	case "forward":
		return DirectionForward
	case "reverse":
		return DirectionReverse
	}

	return DirectionUnknown
}

// RouteMatch is the library route one ride was ridden on, with how much of each
// the two had in common. Coverage is a share in [0, 1] of length, not of points,
// so a densely sampled stretch does not outweigh a sparse one.
type RouteMatch struct {
	Key route.Key
	// RouteCoverage is the share of the route's length the ride covered. It is
	// what decides the match: a ride is on a route when it rode the route.
	RouteCoverage float64
	// RideCoverage is the share of the ride's length that lay on the route. A
	// long ride around a short route covers all of it and little of itself.
	RideCoverage float64
	// Direction is which way round the route the ride went.
	Direction Direction
}

// RouteMatcher attributes rides to one fixed library. The index over that
// library costs some forty times what asking it about one ride does, so a pass
// over many rides builds it once and asks it repeatedly.
type RouteMatcher struct {
	index      *measure.SnapIndex
	candidates []RouteCandidate
}

// NewRouteMatcher indexes a library to attribute rides to.
func NewRouteMatcher(candidates []RouteCandidate) *RouteMatcher {
	lines := make([][]measure.Coordinate, 0, len(candidates))
	for _, candidate := range candidates {
		lines = append(lines, candidate.Geometry)
	}

	return &RouteMatcher{
		index:      measure.NewSnapIndex(lines, corridorMetres),
		candidates: candidates,
	}
}

// MatchRoute attributes one track to one library, indexing it for the one
// question. A caller with more than one ride to ask about builds a RouteMatcher
// instead and keeps the index.
func MatchRoute(track []measure.Coordinate, candidates []RouteCandidate) (RouteMatch, bool) {
	return NewRouteMatcher(candidates).Match(track)
}

// Match attributes one recorded track to at most one library route: the
// route the ride was, rather than any route it merely met. The two must account
// for each other — the ride covering the route, and the route the ride — so a
// day out that takes in a short stage on its way is not a ride of that stage,
// and a ride of one route is not claimed by another over the same roads that
// runs on further.
//
// Which way round the route was ridden is not considered — a loop ridden
// anticlockwise is the same loop — but a route ridden only half way is not
// covered by it: an out-and-back's return leg is the road its outward leg is,
// and riding that road once buys credit for it once.
//
// A library holding two routes over the same roads will have both clear the
// gate, and the ride cannot tell them apart; the order below settles which is
// recorded, and records the same one every time.
func (m *RouteMatcher) Match(track []measure.Coordinate) (RouteMatch, bool) {
	candidates := m.candidates
	if len(track) < 2 || len(candidates) == 0 {
		return RouteMatch{}, false
	}

	onRoute, rideMetres := coveredMetres(track, m.index, len(candidates))
	if rideMetres == 0 {
		return RouteMatch{}, false
	}

	rideIndex := measure.NewSnapIndex([][]measure.Coordinate{track}, corridorMetres)
	best, found, bestIndex := RouteMatch{}, false, 0
	for index, candidate := range candidates {
		if onRoute[index] == 0 {
			continue
		}
		covered, routeMetres := coveredMetres(candidate.Geometry, rideIndex, 1)
		if routeMetres == 0 {
			continue
		}
		// Length the ride never rode is not length it covered.
		if covered[0] > onRoute[index]*maximumCoverageRatio {
			continue
		}
		match := RouteMatch{
			Key:           candidate.Key,
			RouteCoverage: covered[0] / routeMetres,
			RideCoverage:  onRoute[index] / rideMetres,
		}
		if match.RouteCoverage < minimumRouteCoverage || match.RideCoverage < minimumRideCoverage {
			continue
		}
		if !found || match.beats(best) {
			best, found, bestIndex = match, true, index
		}
	}
	if found {
		best.Direction = directionOf(track, candidates[bestIndex].Geometry)
	}

	return best, found
}

// directionOf reports which way round its route a ride went, by following how
// far along the route each position fell and totting up the advance. A stretch
// spent off the route breaks the run rather than counting as a leap along it.
//
// A route whose ends meet is measured around its length, so one joined part way
// round reads like one started at its beginning. An open route has no such
// wrap, and measuring one would read a ride of its whole length, recorded
// sparsely enough, as having gone nowhere.
func directionOf(track, geometry []measure.Coordinate) Direction {
	index := measure.NewSnapIndex([][]measure.Coordinate{geometry}, corridorMetres)
	length := index.LineMetres(0)
	if length == 0 {
		return DirectionUnknown
	}
	closed := measure.HaversineMetres(geometry[0], geometry[len(geometry)-1]) <= corridorMetres

	advance, previous, following := 0.0, 0.0, false
	for _, sample := range track {
		hit, near := index.Nearest(sample)
		if !near {
			following = false

			continue
		}
		if following {
			step := hit.AlongMetres - previous
			if closed {
				step = math.Remainder(step, length)
			}
			// An open route cannot wrap, so any step along it is the step taken.
			if !closed || math.Abs(step) <= maximumDirectionStepShare*length {
				advance += step
			}
		}
		previous, following = hit.AlongMetres, true
	}

	if math.Abs(advance) < minimumDirectionShare*length {
		return DirectionUnknown
	}
	if advance < 0 {
		return DirectionReverse
	}

	return DirectionForward
}

// beats orders two routes a ride rode enough of to have ridden. The one that
// accounts for more of the ride wins, so a long ride is attributed to the route
// it followed rather than to a short stage inside it that it happens to cover
// entirely. Coverage of the route itself only separates a tie, and the identity
// separates what that leaves, so the same corpus always yields the same match.
func (m RouteMatch) beats(other RouteMatch) bool {
	switch {
	case m.RideCoverage != other.RideCoverage:
		return m.RideCoverage > other.RideCoverage
	case m.RouteCoverage != other.RouteCoverage:
		return m.RouteCoverage > other.RouteCoverage
	case m.Key.Provider() != other.Key.Provider():
		return m.Key.Provider() < other.Key.Provider()
	case m.Key.SourceRouteID() != other.Key.SourceRouteID():
		return m.Key.SourceRouteID() < other.Key.SourceRouteID()
	}

	return m.Key.StageOrder() < other.Key.StageOrder()
}

// coveredMetres walks a line and returns, for each of the index's lines, how
// much of the walked length lay inside the corridor of it, along with that
// line's own total length. A step counts for an indexed line when either of its
// ends is in range, so no length is dropped where the two part company.
func coveredMetres(line []measure.Coordinate, index *measure.SnapIndex, indexed int) (covered []float64, total float64) {
	covered = make([]float64, indexed)
	previous := make([]bool, indexed)
	current := make([]bool, indexed)
	for position := range line {
		clear(current)
		for _, hit := range index.Near(line[position]) {
			current[hit.Line] = true
		}
		if position > 0 {
			step := measure.HaversineMetres(line[position-1], line[position])
			total += step
			for candidate := range covered {
				if previous[candidate] || current[candidate] {
					covered[candidate] += step
				}
			}
		}
		previous, current = current, previous
	}

	return covered, total
}

// RouteRide is one ride a route was ridden on, as the route's own page reads it.
type RouteRide struct {
	ID            int64
	RouteCoverage float64
	RideCoverage  float64
	Direction     Direction
}

// climbAttempts times one ride over the climbs of the route it was matched to.
// A route whose stored geometry carries no height has no climbs to be timed
// over, which is not a failure: it is a route this service cannot yet say
// anything about, and the ride keeps its match either way.
func (d *Deriver) climbAttempts(
	ctx context.Context, targetID string, id int64,
	candidates []RouteCandidate, match *RouteMatch, track []TrackPoint,
) ([]ClimbAttempt, error) {
	candidate := candidateFor(candidates, match.Key)
	if candidate == nil || len(candidate.Elevations) == 0 {
		return nil, nil
	}
	climbs := RouteClimbs(candidate.Geometry, candidate.Elevations)
	if len(climbs) == 0 {
		return nil, nil
	}
	series, err := d.store.ActivitySeries(ctx, targetID, id)
	if err != nil {
		return nil, fmt.Errorf("reading a ride's samples: %w", err)
	}

	return ClimbAttempts(climbs, candidate.Geometry, track, series, match.Direction), nil
}

// candidateFor is the library route a match names, or nil where the library has
// moved on since the match was made.
func candidateFor(candidates []RouteCandidate, key route.Key) *RouteCandidate {
	for index := range candidates {
		if candidates[index].Key == key {
			return &candidates[index]
		}
	}

	return nil
}

// RouteMatchStore is what attributing rides to routes needs of stored state.
// The library is read whole because a match is decided against all of it at
// once, and no upstream is involved: a match follows the geometry already
// stored, so it is worked out again on a library edit rather than on a poll.
type RouteMatchStore interface {
	// ActivitySeries is the non-positional part of one ride's positioned
	// samples, indexed 1:1 with what ActivityTrack returns for the same ride.
	ActivitySeries(ctx context.Context, targetID string, id int64) ([]SampleRow, error)
	// LibraryRoutes returns every route a ride may be attributed to, and a hash
	// over the geometry of the library as a whole. A match stored against a
	// different hash was measured against a library that has since changed.
	LibraryRoutes(ctx context.Context) (routes []RouteCandidate, libraryHash string, err error)
	// ActivitiesAwaitingRouteMatch lists the rides owed a match against this
	// library: those never matched, and those matched against another. A ride
	// of one of indoorTypeIDs was ridden over no ground to match against.
	ActivitiesAwaitingRouteMatch(ctx context.Context, targetID, libraryHash string,
		indoorTypeIDs []int) ([]int64, error)
	// ClearActivityRouteMatches removes every match one target holds and reports
	// how many went, for a library that no longer holds any route to have
	// ridden.
	ClearActivityRouteMatches(ctx context.Context, targetID string) (int, error)
	ActivityTrack(ctx context.Context, targetID string, id int64) ([]TrackPoint, error)
	// StoreActivityRouteMatch records which route a ride was ridden on and its
	// attempts at that route's climbs, together. A nil match records that it was
	// ridden on none, which is what stops the ride being matched again against
	// the same library; an empty set of attempts clears whatever was there.
	//
	// The two are one write because an attempt names no route of its own: it is
	// read through the match, so a new match stored beside another match's
	// attempts would serve one route's times under another's climbs.
	StoreActivityRouteMatch(
		ctx context.Context, targetID string, id int64,
		match *RouteMatch, attempts []ClimbAttempt, libraryHash string, now time.Time,
	) error
}

// matchRoutes attributes every ride of one target that is owed a match against
// the library as it stands now.
//
// A library with nothing in it is not a failure: there is no route to have
// ridden yet, and the rides wait for one rather than being recorded as having
// matched nothing against a library that was never asked.
func (d *Deriver) matchRoutes(ctx context.Context, targetID string) Result {
	candidates, libraryHash, err := d.store.LibraryRoutes(ctx)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	// A library with nothing in it is not a failure: there is no route to have
	// ridden yet, and rides that have never been matched wait for one rather
	// than being recorded as having ridden none of nothing. A library emptied
	// after the fact is a different thing — its matches name routes that are
	// gone — so those go.
	if len(candidates) == 0 {
		removed, clearErr := d.store.ClearActivityRouteMatches(ctx, targetID)
		if clearErr != nil {
			return Result{Outcome: Failed, Failure: FailureState}
		}
		if removed == 0 {
			return Result{Outcome: NotReady}
		}

		return Result{Outcome: Polled, Matched: removed}
	}
	ids, err := d.store.ActivitiesAwaitingRouteMatch(ctx, targetID, libraryHash, d.indoorTypes)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	matcher := NewRouteMatcher(candidates)
	matched := 0
	for _, id := range ids {
		// A read or write that fails part way keeps what it already stored: the
		// rides left are still owed a match, and the next attempt finds them
		// exactly as this one did.
		track, trackErr := d.store.ActivityTrack(ctx, targetID, id)
		if trackErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Matched: matched}
		}
		var stored *RouteMatch
		var attempts []ClimbAttempt
		if match, found := matcher.Match(trackCoordinates(track)); found {
			stored = &match
			attempts, err = d.climbAttempts(ctx, targetID, id, candidates, &match, track)
			if err != nil {
				return Result{Outcome: Failed, Failure: FailureState, Matched: matched}
			}
		}
		if storeErr := d.store.StoreActivityRouteMatch(
			ctx, targetID, id, stored, attempts, libraryHash, d.now(),
		); storeErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Matched: matched}
		}
		matched++
	}
	if matched == 0 {
		return Result{Outcome: Unchanged}
	}

	return Result{Outcome: Polled, Matched: matched}
}

// trackCoordinates is a stored track as the matcher reads it: position alone,
// in the order recorded.
func trackCoordinates(track []TrackPoint) []measure.Coordinate {
	coordinates := make([]measure.Coordinate, 0, len(track))
	for _, point := range track {
		coordinates = append(coordinates, measure.Coordinate{Latitude: point.Latitude, Longitude: point.Longitude})
	}

	return coordinates
}
