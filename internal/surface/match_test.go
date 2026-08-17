package surface

import (
	"math"
	"testing"

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

	kinds := Match(metreRoute(0, 200, 20), ways)

	for index, kind := range kinds {
		if kind != KindAsphalt {
			t.Errorf("kind[%d] = %v, want %v", index, kind, KindAsphalt)
		}
	}
}

// TestMatchLeavesUnreachedPointsUnknown covers the case the Overpass query
// itself cannot rule out: geometry came back, but none of it is near enough to
// this stretch of the route to say anything about it.
func TestMatchLeavesUnreachedPointsUnknown(t *testing.T) {
	ways := []Way{metreWay(1, KindAsphalt, [2]float64{0, 60}, [2]float64{200, 60})}

	kinds := Match(metreRoute(0, 200, 20), ways)

	for index, kind := range kinds {
		if kind != KindUnknown {
			t.Errorf("kind[%d] = %v, want %v", index, kind, KindUnknown)
		}
	}
}

// TestMatchPrefersTheRoadOverACrossingWay exercises the heading penalty. At the
// junction the crossing track is exactly as close as the road, and only its
// bearing distinguishes the two.
func TestMatchPrefersTheRoadOverACrossingWay(t *testing.T) {
	ways := []Way{
		metreWay(1, KindAsphalt, [2]float64{0, 0}, [2]float64{200, 0}),
		metreWay(2, KindGround, [2]float64{100, -50}, [2]float64{100, 50}),
	}

	kinds := Match(metreRoute(0, 200, 20), ways)

	for index, kind := range kinds {
		if kind != KindAsphalt {
			t.Errorf("kind[%d] = %v, want %v", index, kind, KindAsphalt)
		}
	}
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

	kinds := Match(metrePoints(points), ways)

	for index, kind := range kinds {
		if kind != KindAsphalt {
			t.Errorf("kind[%d] = %v, want %v", index, kind, KindAsphalt)
		}
	}
}

func TestMatchHandlesEmptyInput(t *testing.T) {
	if got := Match(nil, []Way{metreWay(1, KindAsphalt, [2]float64{0, 0}, [2]float64{10, 0})}); len(got) != 0 {
		t.Errorf("Match(no points) = %v, want empty", got)
	}

	points := metreRoute(0, 100, 20)
	for name, ways := range map[string][]Way{
		"no ways":            nil,
		"a way with no line": {{ID: 1, Kind: KindAsphalt}},
	} {
		t.Run(name, func(t *testing.T) {
			kinds := Match(points, ways)
			if len(kinds) != len(points) {
				t.Fatalf("kind count = %d, want %d", len(kinds), len(points))
			}
			for index, kind := range kinds {
				if kind != KindUnknown {
					t.Errorf("kind[%d] = %v, want %v", index, kind, KindUnknown)
				}
			}
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
			got := despeckle(test.kinds)
			if len(got) != len(test.want) {
				t.Fatalf("despeckle() length = %d, want %d", len(got), len(test.want))
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("despeckle() = %v, want %v", got, test.want)
				}
			}
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
			if got == nil {
				t.Fatal("Compress() = nil, want a non-nil slice")
			}
			if len(got) != len(test.want) {
				t.Fatalf("Compress() = %v, want %v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("Compress() = %v, want %v", got, test.want)
				}
			}
		})
	}
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
			if got := MatchedMetres(points, test.kinds); math.Abs(got-test.want) > 1 {
				t.Errorf("MatchedMetres() = %v, want %v within 1", got, test.want)
			}
		})
	}
}

// offset returns the coordinate the given number of metres east and north of the
// test origin.
func offset(eastMetres, northMetres float64) Coordinate {
	metresPerDegree := earthRadiusMetres * math.Pi / 180

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
