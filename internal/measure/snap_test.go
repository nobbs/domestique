package measure

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// metresEast offsets a coordinate by a distance east, at the latitude given.
func metresEast(from Coordinate, metres float64) Coordinate {
	degrees := metres / (EarthRadiusMetres * math.Pi / 180) / math.Cos(from.Latitude*math.Pi/180)

	return Coordinate{Latitude: from.Latitude, Longitude: from.Longitude + degrees}
}

// metresNorth offsets a coordinate by a distance north.
func metresNorth(from Coordinate, metres float64) Coordinate {
	return Coordinate{
		Latitude:  from.Latitude + metres/(EarthRadiusMetres*math.Pi/180),
		Longitude: from.Longitude,
	}
}

// snapOrigin is the arbitrary point every case here is laid out around.
func snapOrigin() Coordinate {
	return Coordinate{Latitude: 49.9, Longitude: 8.2}
}

func TestSnapIndexFindsTheLineRunningThroughAPoint(t *testing.T) {
	line := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 1000)}
	index := NewSnapIndex([][]Coordinate{line}, 25)

	metres, found := index.NearestMetres(metresNorth(metresEast(snapOrigin(), 500), 10))

	require.True(t, found)
	assert.InDelta(t, 10, metres, 0.5)
}

func TestSnapIndexFindsNothingBeyondItsRadius(t *testing.T) {
	line := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 1000)}
	index := NewSnapIndex([][]Coordinate{line}, 25)

	_, found := index.NearestMetres(metresNorth(metresEast(snapOrigin(), 500), 60))

	assert.False(t, found)
}

func TestSnapIndexReportsEveryLineWithinTheRadius(t *testing.T) {
	near := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 1000)}
	parallel := []Coordinate{metresNorth(snapOrigin(), 15), metresNorth(metresEast(snapOrigin(), 1000), 15)}
	index := NewSnapIndex([][]Coordinate{near, parallel}, 25)

	lines := map[int]bool{}
	for _, hit := range index.Near(metresEast(snapOrigin(), 400)) {
		lines[hit.Line] = true
	}

	assert.Equal(t, map[int]bool{0: true, 1: true}, lines)
}

func TestSnapIndexAlignmentSeparatesAParallelLineFromASquareOne(t *testing.T) {
	alongside := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 200)}
	crossing := []Coordinate{metresNorth(metresEast(snapOrigin(), 100), -100), metresNorth(metresEast(snapOrigin(), 100), 100)}
	index := NewSnapIndex([][]Coordinate{alongside, crossing}, 25)

	east, north := index.Offset(snapOrigin(), metresEast(snapOrigin(), 100))
	alignments := map[int]float64{}
	for _, hit := range index.Near(metresEast(snapOrigin(), 100)) {
		alignments[hit.Line] = hit.Alignment(east, north)
	}

	assert.InDelta(t, 1, alignments[0], 0.01)
	assert.InDelta(t, 0, alignments[1], 0.01)
}

// A direction with no length cannot disagree with anything, and a caller that
// weights alignment must not be charged a penalty for the ends of a track.
func TestSnapIndexAlignmentIsWholeForADirectionOfNoLength(t *testing.T) {
	assert.InDelta(t, 1, SnapHit{runEast: 1, runNorth: 0}.Alignment(0, 0), 0.0001)
	assert.InDelta(t, 1, SnapHit{}.Alignment(1, 0), 0.0001)
}

func TestSnapIndexOverNoLinesFindsNothingRatherThanPanicking(t *testing.T) {
	index := NewSnapIndex(nil, 25)

	_, found := index.NearestMetres(snapOrigin())

	assert.False(t, found)
	assert.Empty(t, index.Near(snapOrigin()))
}

// A line of one coordinate has no segment to measure against, and must not
// stop the lines around it being indexed.
func TestSnapIndexSkipsALineTooShortToHaveASegment(t *testing.T) {
	index := NewSnapIndex([][]Coordinate{
		{},
		{snapOrigin()},
		{metresNorth(snapOrigin(), 5), metresNorth(metresEast(snapOrigin(), 100), 5)},
	}, 25)

	lines := map[int]bool{}
	for _, hit := range index.Near(metresEast(snapOrigin(), 50)) {
		lines[hit.Line] = true
	}

	assert.Equal(t, map[int]bool{2: true}, lines)
}

func TestSnapIndexOffsetIsMeasuredInMetres(t *testing.T) {
	index := NewSnapIndex([][]Coordinate{{snapOrigin(), metresEast(snapOrigin(), 100)}}, 25)

	east, north := index.Offset(snapOrigin(), metresNorth(metresEast(snapOrigin(), 300), 400))

	assert.InDelta(t, 300, east, 1)
	assert.InDelta(t, 400, north, 1)
}

func TestSnapIndexReportsHowFarAlongTheLineAPositionFell(t *testing.T) {
	line := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 400), metresNorth(metresEast(snapOrigin(), 400), 300)}
	index := NewSnapIndex([][]Coordinate{line}, 25)

	quarter, found := index.Nearest(metresEast(snapOrigin(), 100))
	require.True(t, found)
	assert.InDelta(t, 100, quarter.AlongMetres, 1)

	past, found := index.Nearest(metresNorth(metresEast(snapOrigin(), 400), 150))
	require.True(t, found)
	assert.InDelta(t, 550, past.AlongMetres, 1, "the corner is 400 m along, then 150 m up")

	assert.InDelta(t, 700, index.LineMetres(0), 1)
}

// The position along the line is what tells one direction of travel from the
// other, so it must grow with the line's own order rather than with proximity.
func TestSnapIndexAlongGrowsWithTheLinesOwnOrder(t *testing.T) {
	line := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 1000)}
	index := NewSnapIndex([][]Coordinate{line}, 25)

	previous := -1.0
	for metres := 0.0; metres <= 1000; metres += 100 {
		hit, found := index.Nearest(metresEast(snapOrigin(), metres))
		require.True(t, found)
		assert.Greater(t, hit.AlongMetres, previous)
		previous = hit.AlongMetres
	}
}

func TestSnapIndexNearestFindsNothingBeyondTheRadius(t *testing.T) {
	index := NewSnapIndex([][]Coordinate{{snapOrigin(), metresEast(snapOrigin(), 400)}}, 25)

	_, found := index.Nearest(metresNorth(metresEast(snapOrigin(), 200), 90))

	assert.False(t, found)
}

// A line the index does not hold has no length, rather than the length of
// whichever line happens to be first.
func TestSnapIndexLineMetresIsZeroForALineItDoesNotHold(t *testing.T) {
	index := NewSnapIndex([][]Coordinate{{snapOrigin(), metresEast(snapOrigin(), 400)}}, 25)

	assert.InDelta(t, 0, index.LineMetres(7), 1e-9)
}

// A line too short to contribute a segment is not where the indexed geometry
// is. Anchoring the projection on one scales east-west distance by the wrong
// latitude, so the line here runs north and the query sits east of it, which is
// the offset that distortion falls on.
func TestSnapIndexAnchorsOnALineItActuallyIndexes(t *testing.T) {
	stray := []Coordinate{{Latitude: 0, Longitude: 0}}
	northward := []Coordinate{snapOrigin(), metresNorth(snapOrigin(), 1000)}

	withStray := NewSnapIndex([][]Coordinate{stray, northward}, 25)
	alone := NewSnapIndex([][]Coordinate{northward}, 25)

	query := metresEast(metresNorth(snapOrigin(), 500), 10)
	strayed, found := withStray.NearestMetres(query)
	require.True(t, found)
	clean, found := alone.NearestMetres(query)
	require.True(t, found)

	assert.InDelta(t, clean, strayed, 0.01, "a stray point must not move the reading")
	assert.InDelta(t, 10, strayed, 0.5)
}

// A radius of nothing has no answer to give, and says so rather than indexing
// at a cell size of zero, where the step count and the cell keys both come from
// out-of-range float conversions the language does not define.
func TestSnapIndexOverANonPositiveRadiusFindsNothing(t *testing.T) {
	line := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 1000)}

	for _, radius := range []float64{0, -25} {
		index := NewSnapIndex([][]Coordinate{line}, radius)

		_, found := index.NearestMetres(metresEast(snapOrigin(), 500))
		assert.False(t, found, "radius %v", radius)
		assert.Empty(t, index.Near(snapOrigin()), "radius %v", radius)
		assert.InDelta(t, 0, index.LineMetres(0), 1e-9, "radius %v", radius)
	}
}

// Callers read the frame through Offset whether or not anything was indexed,
// and the zero-value projection scales longitude by nothing, putting east and
// west together. Wherever a coordinate was given the frame is anchored on it.
func TestSnapIndexKeepsAUsableFrameWhenItIndexedNothing(t *testing.T) {
	line := []Coordinate{snapOrigin(), metresEast(snapOrigin(), 1000)}
	far := metresNorth(metresEast(snapOrigin(), 300), 400)

	for name, index := range map[string]*SnapIndex{
		"a radius of nothing":      NewSnapIndex([][]Coordinate{line}, 0),
		"no line long enough":      NewSnapIndex([][]Coordinate{{snapOrigin()}}, 25),
		"a line and a stray point": NewSnapIndex([][]Coordinate{{far}, line}, 25),
	} {
		east, north := index.Offset(snapOrigin(), far)
		assert.InDelta(t, 300, east, 2, name)
		assert.InDelta(t, 400, north, 2, name)
	}
}

// An index given no coordinate at all has nowhere to anchor. The frame is then
// equatorial rather than collapsed, so a direction still has an east to it, and
// nothing is indexed for that frame to misjudge.
func TestSnapIndexFrameOverNoCoordinatesIsNotCollapsed(t *testing.T) {
	east, _ := NewSnapIndex(nil, 25).Offset(snapOrigin(), metresEast(snapOrigin(), 300))

	assert.Positive(t, east)
}
