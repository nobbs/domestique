package surface

import (
	"math"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

const (
	// snapRadiusMetres is the furthest a stage point may sit from a way before the
	// two are unrelated. The query is sized from it, so widening it widens the ask.
	snapRadiusMetres = 25.0

	// headingWeightMetres is what a full right-angle heading disagreement costs as a
	// distance penalty. It separates the road ridden from the ways joining it.
	headingWeightMetres = 20.0

	// switchPenaltyMetres is charged for leaving the previous point's way. Small
	// enough for legitimate splits, large enough to reject a parallel footway.
	switchPenaltyMetres = 10.0

	// minimumRunPoints is the shortest run of one class kept. Snapping decides each
	// point alone, so a parallel cycleway produces flickers that are artefacts of
	// the match rather than changes of surface.
	minimumRunPoints = 3
)

// Way is one candidate OpenStreetMap way, already classified. Its line carries
// no elevation, which is why it is deliberately not route.Point geometry.
type Way struct {
	Line []measure.Coordinate
	ID   int64
	Kind Kind
}

// Range is a contiguous run of stage points sharing one class. Both indices are
// inclusive and address the stage geometry as stored.
type Range struct {
	StartIndex int
	EndIndex   int
	Kind       Kind
}

// Match assigns a class to every stage point by snapping it to the nearest
// plausible candidate way. Not full map matching: a planned route already sits
// on the centrelines, and no topologically connected path is guaranteed. Points
// with no candidate within snapRadiusMetres are KindUnknown.
func Match(points []route.Point, ways []Way) []Kind {
	kinds := make([]Kind, len(points))
	if len(points) == 0 || len(ways) == 0 {
		return kinds
	}

	index := measure.NewSnapIndex(wayLines(ways), snapRadiusMetres)

	previousWay := int64(-1)
	for pointIndex := range points {
		headingEast, headingNorth := heading(index, points, pointIndex)

		bestCost := math.Inf(1)
		bestWay := int64(-1)
		bestKind := KindUnknown
		for _, hit := range index.Near(points[pointIndex].Coordinate()) {
			way := &ways[hit.Line]
			cost := hit.DistanceMetres + headingWeightMetres*(1-hit.Alignment(headingEast, headingNorth))
			if previousWay >= 0 && way.ID != previousWay {
				cost += switchPenaltyMetres
			}
			if cost < bestCost {
				bestCost, bestWay, bestKind = cost, way.ID, way.Kind
			}
		}
		kinds[pointIndex] = bestKind
		if bestWay >= 0 {
			previousWay = bestWay
		}
	}

	return despeckle(kinds)
}

// wayLines is the candidate ways' geometry alone, in their order, so a hit
// addresses the way it came from.
func wayLines(ways []Way) [][]measure.Coordinate {
	lines := make([][]measure.Coordinate, 0, len(ways))
	for index := range ways {
		lines = append(lines, ways[index].Line)
	}

	return lines
}

// Compress folds per-point classes into contiguous ranges. A route changes
// surface tens of times, not thousands.
func Compress(kinds []Kind) []Range {
	ranges := make([]Range, 0)
	for index, kind := range kinds {
		if index > 0 && kind == kinds[index-1] { //nolint:gosec // index > 0 guards kinds[index-1]; G602 misreads this as unbounded.
			ranges[len(ranges)-1].EndIndex = index

			continue
		}
		ranges = append(ranges, Range{StartIndex: index, EndIndex: index, Kind: kind})
	}

	return ranges
}

// Expand is Compress's inverse, restoring one class per point. Positions outside
// every range are KindUnknown.
func Expand(ranges []Range, pointCount int) []Kind {
	kinds := make([]Kind, pointCount)
	for _, band := range ranges {
		// A negative StartIndex cannot come from Compress, but Expand reads whatever
		// a caller decoded and must degrade rather than panic.
		for index := max(band.StartIndex, 0); index <= band.EndIndex && index < pointCount; index++ {
			kinds[index] = band.Kind
		}
	}

	return kinds
}

// MatchedMetres returns the stage length that snapped to a classified way, the
// denominator for any share a caller reports. A segment counts when either end
// is classified, so no length is dropped at a boundary.
func MatchedMetres(points []route.Point, kinds []Kind) float64 {
	if len(points) != len(kinds) {
		return 0
	}

	total := 0.0
	for index := 1; index < len(points); index++ {
		if kinds[index-1] == KindUnknown && kinds[index] == KindUnknown {
			continue
		}
		total += measure.HaversineMetres(points[index-1].Coordinate(), points[index].Coordinate())
	}

	return total
}

// despeckle replaces runs shorter than minimumRunPoints with the longer
// neighbour's class. End runs take their only neighbour; a stage too short for
// one full run is left as matched.
func despeckle(kinds []Kind) []Kind {
	if len(kinds) < minimumRunPoints {
		return kinds
	}

	result := make([]Kind, len(kinds))
	copy(result, kinds)
	for _, run := range Compress(kinds) {
		length := run.EndIndex - run.StartIndex + 1
		if length >= minimumRunPoints {
			continue
		}
		replacement, ok := dominantNeighbour(kinds, run)
		if !ok {
			continue
		}
		for index := run.StartIndex; index <= run.EndIndex; index++ {
			result[index] = replacement
		}
	}

	return result
}

// dominantNeighbour returns the class of the longer run adjoining the given one.
func dominantNeighbour(kinds []Kind, run Range) (Kind, bool) {
	beforeLength, afterLength := 0, 0
	for index := run.StartIndex - 1; index >= 0 && kinds[index] == kinds[run.StartIndex-1]; index-- {
		beforeLength++
	}
	for index := run.EndIndex + 1; index < len(kinds) && kinds[index] == kinds[run.EndIndex+1]; index++ {
		afterLength++
	}

	switch {
	case beforeLength == 0 && afterLength == 0:
		return KindUnknown, false
	case afterLength > beforeLength:
		return kinds[run.EndIndex+1], true
	default:
		return kinds[run.StartIndex-1], true
	}
}

// heading returns the route's local direction at one point in the index's own
// projected frame, taken across the neighbouring points so a closely spaced
// pair does not decide it.
func heading(index *measure.SnapIndex, points []route.Point, at int) (east, north float64) {
	before := max(at-1, 0)
	after := min(at+1, len(points)-1)
	if before == after {
		return 0, 0
	}

	return index.Offset(points[before].Coordinate(), points[after].Coordinate())
}
