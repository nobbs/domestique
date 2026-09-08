package measure

import "math"

// SnapIndex answers which of a fixed set of polylines pass within a radius of a
// coordinate, and how far away each is. Two matchers want that question — the
// surface classifier snapping a route to candidate ways, and the ride matcher
// snapping a recorded track to the library — and neither wants its own copy.
type SnapIndex struct {
	grid       *segmentGrid
	segments   []snapSegment
	hits       []SnapHit
	projection projection
	radius     float64
}

// SnapHit is one indexed segment lying within the index's radius of a query
// coordinate. Line addresses the polyline it belongs to, as given.
type SnapHit struct {
	DistanceMetres float64
	// AlongMetres is how far along its polyline the nearest point of this
	// segment lies, measured from the line's start in the index's own projected
	// frame. It says where on a route a position falls, which is what tells one
	// direction of travel from the other.
	AlongMetres float64
	runEast     float64
	runNorth    float64
	Line        int
}

// NewSnapIndex indexes polylines for lookups within the given radius. A line of
// fewer than two coordinates contributes nothing; an empty set yields an index
// that finds nothing rather than a nil to guard at every call.
func NewSnapIndex(lines [][]Coordinate, radiusMetres float64) *SnapIndex {
	index := &SnapIndex{radius: radiusMetres, projection: projectionFor(lines)}
	// A radius of nothing asks for nothing. Indexing at one sizes every grid cell
	// at zero, leaving the step count and the cell keys to float conversions the
	// language does not define.
	if radiusMetres <= 0 {
		index.grid = newSegmentGrid(nil, 1)

		return index
	}

	index.segments = buildSnapSegments(index.projection, lines)
	index.grid = newSegmentGrid(index.segments, radiusMetres)

	return index
}

// projectionFor anchors the frame on the first line that contributes a segment:
// a line too short to index is not where the geometry is, and anchoring on one
// far from the rest would scale every east-west distance by the wrong latitude.
// Anything at all beats the zero value, whose scale of zero would collapse east
// and west together.
func projectionFor(lines [][]Coordinate) projection {
	spare, hasSpare := Coordinate{}, false
	for _, line := range lines {
		if len(line) >= 2 {
			return newProjection(line[0].Longitude, line[0].Latitude)
		}
		if len(line) == 1 && !hasSpare {
			spare, hasSpare = line[0], true
		}
	}
	if hasSpare {
		return newProjection(spare.Longitude, spare.Latitude)
	}

	return newProjection(0, 0)
}

// Near returns the indexed segments within the index's radius of the
// coordinate. One polyline appears once per segment of it in range, and a
// segment spanning several cells may repeat: evaluating it twice is cheaper
// than the bookkeeping to avoid it, and every caller reduces over the hits. The
// result is owned by the index and valid only until the next call.
func (i *SnapIndex) Near(at Coordinate) []SnapHit {
	east, north := i.projection.project(at.Longitude, at.Latitude)

	i.hits = i.hits[:0]
	for _, segmentIndex := range i.grid.near(east, north) {
		candidate := i.segments[segmentIndex]
		distance, ratio := candidate.distanceTo(east, north)
		if distance > i.radius {
			continue
		}
		i.hits = append(i.hits, SnapHit{
			Line:           candidate.line,
			DistanceMetres: distance,
			AlongMetres:    candidate.startAlong + ratio*candidate.length(),
			runEast:        candidate.endEast - candidate.startEast,
			runNorth:       candidate.endNorth - candidate.startNorth,
		})
	}

	return i.hits
}

// NearestMetres returns the distance to the closest indexed polyline, and
// whether anything lay within the radius at all.
func (i *SnapIndex) NearestMetres(at Coordinate) (metres float64, found bool) {
	nearest := math.Inf(1)
	for _, hit := range i.Near(at) {
		nearest = math.Min(nearest, hit.DistanceMetres)
	}

	return nearest, !math.IsInf(nearest, 1)
}

// Nearest returns the closest indexed segment to a coordinate, and whether
// anything lay within the radius. A caller that wants where on a line the
// position fell reads the hit's AlongMetres.
func (i *SnapIndex) Nearest(at Coordinate) (SnapHit, bool) {
	best, found := SnapHit{DistanceMetres: math.Inf(1)}, false
	for _, hit := range i.Near(at) {
		if hit.DistanceMetres < best.DistanceMetres {
			best, found = hit, true
		}
	}

	return best, found
}

// LineMetres is the length of one indexed polyline in the projected frame
// AlongMetres is measured in, so a caller can tell a position near the end of a
// line from one near its start.
func (i *SnapIndex) LineMetres(line int) float64 {
	length := 0.0
	for _, segment := range i.segments {
		if segment.line == line {
			length = math.Max(length, segment.startAlong+segment.length())
		}
	}

	return length
}

// Offset returns the vector between two coordinates in projected metres, so a
// caller can express a direction in the frame the hits are aligned against.
//
// It scales longitude at the latitude it was asked about rather than at the
// index's own reference, which is correct wherever the pair sits and holds even
// for an index that snapped to nothing. Over the span where a direction is
// compared against a segment the two references agree to within a millionth.
func (i *SnapIndex) Offset(from, to Coordinate) (east, north float64) {
	local := newProjection(from.Longitude, from.Latitude)
	toEast, toNorth := local.project(to.Longitude, to.Latitude)

	return toEast, toNorth
}

// Alignment scores how nearly the hit's segment runs along the given direction,
// 1 for parallel and 0 for square to it. Undirected: a line is equally aligned
// whichever end it was entered from. A zero-length direction scores 1, having
// nothing to disagree with.
func (h SnapHit) Alignment(east, north float64) float64 {
	runLength := math.Hypot(h.runEast, h.runNorth)
	directionLength := math.Hypot(east, north)
	if runLength == 0 || directionLength == 0 {
		return 1
	}

	return math.Min(1, math.Abs(h.runEast*east+h.runNorth*north)/(runLength*directionLength))
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
	metresPerDegree := EarthRadiusMetres * math.Pi / 180

	return (longitude - p.referenceLongitude) * metresPerDegree * p.longitudeScale,
		(latitude - p.referenceLatitude) * metresPerDegree
}

// snapSegment is one straight piece of one indexed polyline, in projected metres.
type snapSegment struct {
	startEast  float64
	startNorth float64
	endEast    float64
	endNorth   float64
	// startAlong is the distance from the polyline's start to this segment's,
	// so a hit can say where along the line it fell.
	startAlong float64
	line       int
}

func (s snapSegment) length() float64 {
	return math.Hypot(s.endEast-s.startEast, s.endNorth-s.startNorth)
}

func buildSnapSegments(projection projection, lines [][]Coordinate) []snapSegment {
	segments := make([]snapSegment, 0, len(lines))
	for lineIndex, line := range lines {
		along := 0.0
		for pointIndex := 1; pointIndex < len(line); pointIndex++ {
			startEast, startNorth := projection.project(line[pointIndex-1].Longitude, line[pointIndex-1].Latitude)
			endEast, endNorth := projection.project(line[pointIndex].Longitude, line[pointIndex].Latitude)
			one := snapSegment{
				startEast:  startEast,
				startNorth: startNorth,
				endEast:    endEast,
				endNorth:   endNorth,
				startAlong: along,
				line:       lineIndex,
			}
			along += one.length()
			segments = append(segments, one)
		}
	}

	return segments
}

// distanceTo returns the perpendicular distance from a projected point to the
// segment, clamped to its ends, and how far along the segment that nearest
// point fell as a share of its length.
func (s snapSegment) distanceTo(east, north float64) (distance, ratio float64) {
	runEast, runNorth := s.endEast-s.startEast, s.endNorth-s.startNorth
	lengthSquared := runEast*runEast + runNorth*runNorth
	if lengthSquared == 0 {
		return math.Hypot(east-s.startEast, north-s.startNorth), 0
	}
	ratio = ((east-s.startEast)*runEast + (north-s.startNorth)*runNorth) / lengthSquared
	ratio = math.Max(0, math.Min(1, ratio))

	return math.Hypot(east-(s.startEast+ratio*runEast), north-(s.startNorth+ratio*runNorth)), ratio
}
