package surface

import (
	"math"

	"github.com/nobbs/domestique/internal/route"
)

// earthRadiusMetres is the spherical model shared with the route and elevation
// packages, so a length here agrees with the distance shown beside it.
const earthRadiusMetres = 6_371_000.0

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

// Coordinate is one point of candidate way geometry. Ways carry no elevation,
// which is why this is deliberately not a route.Point.
type Coordinate struct {
	Longitude float64
	Latitude  float64
}

// Way is one candidate OpenStreetMap way, already classified.
type Way struct {
	Line []Coordinate
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

	projection := newProjection(points[0].Longitude, points[0].Latitude)
	segments := buildSegments(projection, ways)
	if len(segments) == 0 {
		return kinds
	}
	index := newSegmentGrid(segments, snapRadiusMetres)

	previousWay := int64(-1)
	for pointIndex := range points {
		east, north := projection.project(points[pointIndex].Longitude, points[pointIndex].Latitude)
		headingEast, headingNorth := heading(projection, points, pointIndex)

		bestCost := math.Inf(1)
		bestWay := int64(-1)
		bestKind := KindUnknown
		for _, segmentIndex := range index.near(east, north) {
			candidate := segments[segmentIndex]
			distance := candidate.distanceTo(east, north)
			if distance > snapRadiusMetres {
				continue
			}
			way := &ways[candidate.wayIndex]
			cost := distance + candidate.headingPenalty(headingEast, headingNorth)
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
		total += haversineMetres(points[index-1], points[index])
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

// projection converts geographic coordinates to local metres about a reference
// point, equirectangular. The grid index needs square cells, and the error over
// the span where candidates compete is far below the tolerances involved.
// Lengths reported to callers still use haversine.
type projection struct {
	referenceLongitude float64
	referenceLatitude  float64
	longitudeScale     float64
}

func newProjection(longitude, latitude float64) projection {
	return projection{
		referenceLongitude: longitude,
		referenceLatitude:  latitude,
		longitudeScale:     math.Cos(latitude * math.Pi / 180),
	}
}

func (p projection) project(longitude, latitude float64) (east, north float64) {
	metresPerDegree := earthRadiusMetres * math.Pi / 180

	return (longitude - p.referenceLongitude) * metresPerDegree * p.longitudeScale,
		(latitude - p.referenceLatitude) * metresPerDegree
}

// segment is one straight piece of one candidate way, in projected metres.
type segment struct {
	startEast  float64
	startNorth float64
	endEast    float64
	endNorth   float64
	wayIndex   int
}

func buildSegments(projection projection, ways []Way) []segment {
	segments := make([]segment, 0, len(ways))
	for wayIndex := range ways {
		line := ways[wayIndex].Line
		for pointIndex := 1; pointIndex < len(line); pointIndex++ {
			startEast, startNorth := projection.project(line[pointIndex-1].Longitude, line[pointIndex-1].Latitude)
			endEast, endNorth := projection.project(line[pointIndex].Longitude, line[pointIndex].Latitude)
			segments = append(segments, segment{
				startEast:  startEast,
				startNorth: startNorth,
				endEast:    endEast,
				endNorth:   endNorth,
				wayIndex:   wayIndex,
			})
		}
	}

	return segments
}

// distanceTo returns the perpendicular distance from a projected point to the
// segment, clamped to its ends.
func (s segment) distanceTo(east, north float64) float64 {
	runEast, runNorth := s.endEast-s.startEast, s.endNorth-s.startNorth
	lengthSquared := runEast*runEast + runNorth*runNorth
	if lengthSquared == 0 {
		return math.Hypot(east-s.startEast, north-s.startNorth)
	}
	ratio := ((east-s.startEast)*runEast + (north-s.startNorth)*runNorth) / lengthSquared
	ratio = math.Max(0, math.Min(1, ratio))

	return math.Hypot(east-(s.startEast+ratio*runEast), north-(s.startNorth+ratio*runNorth))
}

// headingPenalty scores how far the segment's bearing is from the route's, as a
// distance. Undirected: a way is equally right whichever end was entered.
func (s segment) headingPenalty(headingEast, headingNorth float64) float64 {
	runEast, runNorth := s.endEast-s.startEast, s.endNorth-s.startNorth
	runLength := math.Hypot(runEast, runNorth)
	headingLength := math.Hypot(headingEast, headingNorth)
	if runLength == 0 || headingLength == 0 {
		return 0
	}
	alignment := math.Abs(runEast*headingEast+runNorth*headingNorth) / (runLength * headingLength)

	return headingWeightMetres * (1 - math.Min(1, alignment))
}

// heading returns the route's local direction at one point, taken across the
// neighbouring points so a closely spaced pair does not decide it.
func heading(projection projection, points []route.Point, index int) (east, north float64) {
	before := max(index-1, 0)
	after := min(index+1, len(points)-1)
	if before == after {
		return 0, 0
	}
	beforeEast, beforeNorth := projection.project(points[before].Longitude, points[before].Latitude)
	afterEast, afterNorth := projection.project(points[after].Longitude, points[after].Latitude)

	return afterEast - beforeEast, afterNorth - beforeNorth
}

// haversineMetres returns the great-circle distance between two stage points, on
// the same spherical model the route and elevation packages use.
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
