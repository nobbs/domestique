package osmindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/surface"
)

// The blob has no header and no index, so the only thing that makes it readable
// is that a record ends exactly where the next one begins.
func TestCellRoundTripsEveryRecordItHolds(t *testing.T) {
	key := cellKey{x: 830, y: 4990}
	x, y := quantised(t, key, [][2]float64{{8.301, 49.901}, {8.3035, 49.9025}, {8.3049, 49.9038}})

	var blob []byte
	blob = appendRun(blob, key, 1234, surface.KindGravel, x, y)
	blob = appendRun(blob, key, 5678, surface.KindAsphalt, x[:2], y[:2])

	ways, err := decodeCell(blob, key)
	require.NoError(t, err)
	require.Len(t, ways, 2, "records in the blob")

	assert.Equal(t, int64(1234), ways[0].ID, "first way identifier")
	assert.Equal(t, surface.KindGravel, ways[0].Kind, "first way class")
	require.Len(t, ways[0].Line, 3, "first way points")
	assert.InDelta(t, 8.301, ways[0].Line[0].Longitude, 1e-6, "first point longitude")
	assert.InDelta(t, 49.9038, ways[0].Line[2].Latitude, 1e-6, "last point latitude")

	assert.Equal(t, int64(5678), ways[1].ID, "second way identifier")
	assert.Equal(t, surface.KindAsphalt, ways[1].Kind, "second way class")
	assert.Len(t, ways[1].Line, 2, "second way points")
}

// Appending is what lets the builder write a cell in several flushes without
// reading back what it already wrote, so two blobs joined end to end have to
// decode as one.
func TestConcatenatedBlobsDecodeAsOne(t *testing.T) {
	key := cellKey{x: 830, y: 4990}
	x, y := quantised(t, key, [][2]float64{{8.301, 49.901}, {8.302, 49.902}})

	first := appendRun(nil, key, 1, surface.KindAsphalt, x, y)
	second := appendRun(nil, key, 2, surface.KindGround, x, y)

	ways, err := decodeCell(append(append([]byte(nil), first...), second...), key)
	require.NoError(t, err)
	require.Len(t, ways, 2)
	assert.Equal(t, int64(1), ways[0].ID)
	assert.Equal(t, int64(2), ways[1].ID)
}

func TestDecodeCellRejectsATruncatedRecord(t *testing.T) {
	key := cellKey{x: 830, y: 4990}
	x, y := quantised(t, key, [][2]float64{{8.301, 49.901}, {8.302, 49.902}})
	blob := appendRun(nil, key, 1, surface.KindAsphalt, x, y)

	_, err := decodeCell(blob[:len(blob)-1], key)
	require.ErrorIs(t, err, errShortRecord)
}

// A way that crosses a boundary has to be complete on both sides of it: a stage
// point just inside one cell measures its distance to a way that would otherwise
// appear to stop at the edge, and would snap to something further away.
func TestSplitIntoCellsCarriesTheCrossingSegmentIntoBothCells(t *testing.T) {
	// A straight line west to east across one cell boundary at 8.30°.
	x, y := absolute(t, [][2]float64{{8.298, 49.901}, {8.299, 49.901}, {8.301, 49.901}, {8.302, 49.901}})

	cells := make(map[cellKey][]byte)
	splitIntoCells(cells, 42, surface.KindAsphalt, x, y)

	west, east := cellKey{x: 829, y: 4990}, cellKey{x: 830, y: 4990}
	require.Contains(t, cells, west, "the cell the way starts in")
	require.Contains(t, cells, east, "the cell the way ends in")

	westWays, err := decodeCell(cells[west], west)
	require.NoError(t, err)
	eastWays, err := decodeCell(cells[east], east)
	require.NoError(t, err)

	require.Len(t, westWays, 1)
	require.Len(t, eastWays, 1)
	assert.InDelta(t, 8.301, westWays[0].Line[len(westWays[0].Line)-1].Longitude, 1e-6,
		"the western run stops at the boundary instead of crossing it")
	assert.InDelta(t, 8.299, eastWays[0].Line[0].Longitude, 1e-6,
		"the eastern run starts at the boundary instead of before it")
}

// Cells west of the prime meridian and south of the equator are the same size as
// the rest. Go's integer division truncates towards zero, which would otherwise
// make the four cells around the origin share a key with their neighbours.
func TestCellOfDividesTowardsNegativeInfinity(t *testing.T) {
	perCell := int32(quantisedPerCell()) //nolint:gosec // A cell spans five digits at this precision.

	assert.Equal(t, cellKey{x: 0, y: 0}, cellOf(1, 1), "just east and north of the origin")
	assert.Equal(t, cellKey{x: -1, y: -1}, cellOf(-1, -1), "just west and south of the origin")
	assert.Equal(t, cellKey{x: -1, y: -1}, cellOf(-perCell, -perCell), "exactly one cell out")
	assert.Equal(t, cellKey{x: -2, y: -2}, cellOf(-perCell-1, -perCell-1), "just past one cell out")
}

func TestVerifyScalesRejectsAFileThisBuildCannotRead(t *testing.T) {
	require.NoError(t, verifyScales(cellDegrees, coordinateScale), "the scales this build writes")
	require.Error(t, verifyScales(cellDegrees, 1e5), "a file written at a coarser precision")
	require.Error(t, verifyScales(0.05, coordinateScale), "a file written on a different grid")
}

// quantised converts degrees to the stored fixed point and asserts the points
// land in the cell the caller says they do, so a fixture cannot drift into
// testing the wrong cell.
func quantised(t *testing.T, key cellKey, degrees [][2]float64) (x, y []int32) {
	t.Helper()
	x, y = absolute(t, degrees)
	for index := range x {
		require.Equalf(t, key, cellOf(x[index], y[index]), "point %d is not in cell %v", index, key)
	}

	return x, y
}

func absolute(t *testing.T, degrees [][2]float64) (x, y []int32) {
	t.Helper()
	x = make([]int32, 0, len(degrees))
	y = make([]int32, 0, len(degrees))
	for _, point := range degrees {
		x = append(x, int32(point[0]*coordinateScale))
		y = append(y, int32(point[1]*coordinateScale))
	}

	return x, y
}
