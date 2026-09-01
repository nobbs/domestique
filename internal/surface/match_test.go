package surface

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

// The tests lay their geometry out in metres east and north of a fixed origin,
// because every tolerance in this package is a distance and a test written in
// degrees would hide which distances it is exercising.
const (
	originLatitude  = 49.0
	originLongitude = 8.0
)

func TestMatchSnapsToTheWayUnderTheRoute(t *testing.T) {
	ways := []Way{
		metreWay(1, KindAsphalt, [2]float64{0, 0}, [2]float64{200, 0}),
		metreWay(2, KindGravel, [2]float64{0, 60}, [2]float64{200, 60}),
	}
	points := metreRoute(0, 200, 20)

	kinds := Match(points, ways)

	assertEveryPointIs(t, points, kinds, KindAsphalt)
}

// TestMatchLeavesUnreachedPointsUnknown covers the case the Overpass query
// itself cannot rule out: geometry came back, but none of it is near enough to
// this stretch of the route to say anything about it.
func TestMatchLeavesUnreachedPointsUnknown(t *testing.T) {
	ways := []Way{metreWay(1, KindAsphalt, [2]float64{0, 60}, [2]float64{200, 60})}
	points := metreRoute(0, 200, 20)

	kinds := Match(points, ways)

	assertEveryPointIs(t, points, kinds, KindUnknown)
}

// TestMatchPrefersTheRoadOverACrossingWay exercises the heading penalty. At the
// junction the crossing track is exactly as close as the road, and only its
// bearing distinguishes the two.
func TestMatchPrefersTheRoadOverACrossingWay(t *testing.T) {
	ways := []Way{
		metreWay(1, KindAsphalt, [2]float64{0, 0}, [2]float64{200, 0}),
		metreWay(2, KindGround, [2]float64{100, -50}, [2]float64{100, 50}),
	}
	points := metreRoute(0, 200, 20)

	kinds := Match(points, ways)

	assertEveryPointIs(t, points, kinds, KindAsphalt)
}

// TestMatchIgnoresAFlickerOntoAParallelPath is the failure this package is
// mostly built to avoid: a gravel cycleway running alongside the road, close
// enough that a couple of route points fall nearer to it than to the road they
// are actually on.
func TestMatchIgnoresAFlickerOntoAParallelPath(t *testing.T) {
	ways := []Way{
		metreWay(1, KindAsphalt, [2]float64{0, 0}, [2]float64{400, 0}),
		metreWay(2, KindGravel, [2]float64{0, 20}, [2]float64{400, 20}),
	}
	points := make([][2]float64, 0, 21)
	for step := range 21 {
		east := float64(step) * 20
		north := 0.0
		if step == 10 || step == 11 {
			north = 18
		}
		points = append(points, [2]float64{east, north})
	}
	geometry := metrePoints(points)

	kinds := Match(geometry, ways)

	assertEveryPointIs(t, geometry, kinds, KindAsphalt)
}

func TestMatchHandlesEmptyInput(t *testing.T) {
	assert.Empty(t, Match(nil, []Way{metreWay(1, KindAsphalt, [2]float64{0, 0}, [2]float64{10, 0})}),
		"Match() classified points it was never given")

	points := metreRoute(0, 100, 20)
	for name, ways := range map[string][]Way{
		"no ways":            nil,
		"a way with no line": {{ID: 1, Kind: KindAsphalt}},
	} {
		t.Run(name, func(t *testing.T) {
			assertEveryPointIs(t, points, Match(points, ways), KindUnknown)
		})
	}
}

func TestDespeckleReplacesShortRunsWithTheLongerNeighbour(t *testing.T) {
	tests := []struct {
		name  string
		kinds []Kind
		want  []Kind
	}{
		{
			name:  "a brief flicker takes the surrounding class",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindGravel, KindGravel, KindAsphalt, KindAsphalt, KindAsphalt},
			want:  []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt},
		},
		{
			name:  "a gap in the match is filled the same way",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindUnknown, KindAsphalt, KindAsphalt, KindAsphalt},
			want:  []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt},
		},
		{
			name:  "a run at the start takes its only neighbour",
			kinds: []Kind{KindGravel, KindGravel, KindAsphalt, KindAsphalt, KindAsphalt},
			want:  []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt},
		},
		{
			name:  "a run at the end takes its only neighbour",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindGravel},
			want:  []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt},
		},
		{
			name:  "the longer of two neighbours wins",
			kinds: []Kind{KindGravel, KindGravel, KindGravel, KindAsphalt, KindAsphalt, KindPaving, KindPaving, KindPaving, KindPaving},
			want:  []Kind{KindGravel, KindGravel, KindGravel, KindPaving, KindPaving, KindPaving, KindPaving, KindPaving, KindPaving},
		},
		{
			name:  "equally long neighbours resolve to the one already ridden",
			kinds: []Kind{KindGravel, KindGravel, KindGravel, KindAsphalt, KindPaving, KindPaving, KindPaving},
			want:  []Kind{KindGravel, KindGravel, KindGravel, KindGravel, KindPaving, KindPaving, KindPaving},
		},
		{
			name:  "a run long enough to be real survives",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindGravel, KindGravel, KindGravel, KindAsphalt, KindAsphalt, KindAsphalt},
			want:  []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindGravel, KindGravel, KindGravel, KindAsphalt, KindAsphalt, KindAsphalt},
		},
		{
			name:  "an entirely unmatched stage stays unmatched",
			kinds: []Kind{KindUnknown, KindUnknown, KindUnknown, KindUnknown},
			want:  []Kind{KindUnknown, KindUnknown, KindUnknown, KindUnknown},
		},
		{
			name:  "a stage too short to hold a run is left alone",
			kinds: []Kind{KindGravel, KindAsphalt},
			want:  []Kind{KindGravel, KindAsphalt},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, despeckle(test.kinds))
		})
	}
}

func TestCompressFoldsRunsIntoRanges(t *testing.T) {
	tests := []struct {
		name  string
		kinds []Kind
		want  []Range
	}{
		{
			name:  "no points compress to no ranges",
			kinds: nil,
			want:  []Range{},
		},
		{
			name:  "one class is one range",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindAsphalt},
			want:  []Range{{StartIndex: 0, EndIndex: 2, Kind: KindAsphalt}},
		},
		{
			name:  "each change starts a range",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindGravel, KindGravel, KindGravel, KindPaving},
			want: []Range{
				{StartIndex: 0, EndIndex: 1, Kind: KindAsphalt},
				{StartIndex: 2, EndIndex: 4, Kind: KindGravel},
				{StartIndex: 5, EndIndex: 5, Kind: KindPaving},
			},
		},
		{
			name:  "a class recurring later is a separate range",
			kinds: []Kind{KindAsphalt, KindGravel, KindAsphalt},
			want: []Range{
				{StartIndex: 0, EndIndex: 0, Kind: KindAsphalt},
				{StartIndex: 1, EndIndex: 1, Kind: KindGravel},
				{StartIndex: 2, EndIndex: 2, Kind: KindAsphalt},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Compress(test.kinds)
			// Non-nil so an unclassified stage serves [] rather than null.
			require.NotNil(t, got)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestExpandIsCompressesInverse(t *testing.T) {
	kinds := []Kind{KindAsphalt, KindAsphalt, KindGravel, KindGravel, KindGravel}
	assert.Equal(t, kinds, Expand(Compress(kinds), len(kinds)))
}

func TestExpandLeavesUncoveredPositionsUnknown(t *testing.T) {
	ranges := []Range{{StartIndex: 1, EndIndex: 1, Kind: KindAsphalt}}
	assert.Equal(t, []Kind{KindUnknown, KindAsphalt, KindUnknown}, Expand(ranges, 3))
}

// A negative StartIndex cannot come from Compress, but Expand reads whatever a
// caller decoded — a corrupted stored row, in the worst case — and must
// degrade the position to KindUnknown rather than panic on a negative index.
func TestExpandDoesNotPanicOnANegativeStartIndex(t *testing.T) {
	ranges := []Range{{StartIndex: -5, EndIndex: 1, Kind: KindGravel}}
	assert.NotPanics(t, func() {
		assert.Equal(t, []Kind{KindGravel, KindGravel, KindUnknown}, Expand(ranges, 3))
	})
}

// Equally, an EndIndex beyond the current geometry — stale ranges from a
// longer, since-replaced stage — must not run past the slice it is filling.
func TestExpandDoesNotPanicOnAnEndIndexPastPointCount(t *testing.T) {
	ranges := []Range{{StartIndex: 0, EndIndex: 100, Kind: KindGravel}}
	assert.NotPanics(t, func() {
		assert.Equal(t, []Kind{KindGravel, KindGravel}, Expand(ranges, 2))
	})
}

func TestMatchedMetresCountsOnlyClassifiedLength(t *testing.T) {
	points := metreRoute(0, 300, 100)

	tests := []struct {
		name  string
		kinds []Kind
		want  float64
	}{
		{
			name:  "a fully matched stage counts in full",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindAsphalt, KindAsphalt},
			want:  300,
		},
		{
			name:  "an unmatched stage counts nothing",
			kinds: []Kind{KindUnknown, KindUnknown, KindUnknown, KindUnknown},
			want:  0,
		},
		{
			name:  "a boundary segment still counts once",
			kinds: []Kind{KindAsphalt, KindAsphalt, KindUnknown, KindUnknown},
			want:  200,
		},
		{
			name:  "a mismatched class count is not measured",
			kinds: []Kind{KindAsphalt},
			want:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.InDelta(t, test.want, MatchedMetres(points, test.kinds), 1)
		})
	}
}

// assertEveryPointIs checks that the whole stage was classified, and classified
// the one way. The length check is what stops a Match that returned nothing from
// passing the loop vacuously.
func assertEveryPointIs(t *testing.T, points []route.Point, kinds []Kind, want Kind) {
	t.Helper()
	require.Len(t, kinds, len(points), "Match() did not classify every point")
	for index, kind := range kinds {
		assert.Equal(t, want, kind, "point %d", index)
	}
}

// offset returns the coordinate the given number of metres east and north of the
// test origin.
func offset(eastMetres, northMetres float64) Coordinate {
	metresPerDegree := route.EarthRadiusMetres * math.Pi / 180

	return Coordinate{
		Longitude: originLongitude + eastMetres/(metresPerDegree*math.Cos(originLatitude*math.Pi/180)),
		Latitude:  originLatitude + northMetres/metresPerDegree,
	}
}

// metreWay builds a way through the given east/north metre offsets.
func metreWay(id int64, kind Kind, coordinates ...[2]float64) Way {
	line := make([]Coordinate, 0, len(coordinates))
	for _, coordinate := range coordinates {
		line = append(line, offset(coordinate[0], coordinate[1]))
	}

	return Way{Line: line, ID: id, Kind: kind}
}

// metrePoints builds stage geometry through the given east/north metre offsets.
func metrePoints(coordinates [][2]float64) []route.Point {
	points := make([]route.Point, 0, len(coordinates))
	for _, coordinate := range coordinates {
		position := offset(coordinate[0], coordinate[1])
		points = append(points, route.Point{Longitude: position.Longitude, Latitude: position.Latitude})
	}

	return points
}

// metreRoute builds stage geometry running due east along the origin's latitude.
func metreRoute(fromEastMetres, toEastMetres, stepMetres float64) []route.Point {
	coordinates := make([][2]float64, 0)
	for east := fromEastMetres; east <= toEastMetres; east += stepMetres {
		coordinates = append(coordinates, [2]float64{east, 0})
	}

	return metrePoints(coordinates)
}
