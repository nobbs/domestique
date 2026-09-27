package main

import (
	"math"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
)

// These mirror internal/activity/routematch.go, whose own copies are
// unexported; the report's agreement line is what catches the two drifting.
const (
	corridorMetres            = 40.0
	maximumCoverageRatio      = 1.15
	productionBand            = 0.92
	minimumDirectionShare     = 0.5
	maximumDirectionStepShare = 0.25
)

// A coverage is how much one ride and one library route had in common, before
// any acceptance band: the share of the route the ride covered and the share
// of the ride that lay on the route.
type coverage struct {
	candidate   int
	route, ride float64
}

// joint is the lower of the two shares: the band a ride clears against this
// route when both shares are held to it.
func (c coverage) joint() float64 { return math.Min(c.route, c.ride) }

// coveragesOf is every library route the track came within the corridor of,
// with both shares, the way activity.RouteMatcher.Match measures them before
// it applies its band.
func coveragesOf(track []measure.Coordinate, library *measure.SnapIndex, candidates []activity.RouteCandidate) []coverage {
	if len(track) < 2 || len(candidates) == 0 {
		return nil
	}
	onRoute, rideMetres := coveredMetres(track, library, len(candidates))
	if rideMetres == 0 {
		return nil
	}
	rideIndex := measure.NewSnapIndex([][]measure.Coordinate{track}, corridorMetres)
	var found []coverage
	for index := range candidates {
		if onRoute[index] == 0 {
			continue
		}
		covered, routeMetres := coveredMetres(candidates[index].Geometry, rideIndex, 1)
		if routeMetres == 0 || covered[0] > onRoute[index]*maximumCoverageRatio {
			continue
		}
		found = append(found, coverage{candidate: index, route: covered[0] / routeMetres, ride: onRoute[index] / rideMetres})
	}

	return found
}

// winner is the route a band would record for a ride, ordered the way
// activity.RouteMatch.beats orders them, and how many routes cleared it.
func winner(coverages []coverage, candidates []activity.RouteCandidate, routeBand, rideBand float64) (best coverage, cleared int) {
	for _, held := range coverages {
		if held.route < routeBand || held.ride < rideBand {
			continue
		}
		if cleared == 0 || beats(held, best, candidates) {
			best = held
		}
		cleared++
	}

	return best, cleared
}

func beats(a, b coverage, candidates []activity.RouteCandidate) bool {
	keyA, keyB := candidates[a.candidate].Key, candidates[b.candidate].Key
	switch {
	case a.ride != b.ride:
		return a.ride > b.ride
	case a.route != b.route:
		return a.route > b.route
	case keyA.Provider() != keyB.Provider():
		return keyA.Provider() < keyB.Provider()
	case keyA.SourceRouteID() != keyB.SourceRouteID():
		return keyA.SourceRouteID() < keyB.SourceRouteID()
	}

	return keyA.StageOrder() < keyB.StageOrder()
}

// coveredMetres is activity's own: for each indexed line, how much of the
// walked length lay inside its corridor, and the walked line's total length.
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

// directionOf is activity's own: the net advance along the route, around its
// length where its ends meet, called a direction past half of it.
func directionOf(track, geometry []measure.Coordinate) activity.Direction {
	index := measure.NewSnapIndex([][]measure.Coordinate{geometry}, corridorMetres)
	length := index.LineMetres(0)
	if length == 0 {
		return activity.DirectionUnknown
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
			if !closed || math.Abs(step) <= maximumDirectionStepShare*length {
				advance += step
			}
		}
		previous, following = hit.AlongMetres, true
	}

	switch {
	case math.Abs(advance) < minimumDirectionShare*length:
		return activity.DirectionUnknown
	case advance < 0:
		return activity.DirectionReverse
	}

	return activity.DirectionForward
}
