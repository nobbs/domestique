// Package osmindex builds and reads a local index of OpenStreetMap way geometry
// and surface tagging, so a stage can be classified from disk instead of from a
// public Overpass endpoint.
//
// It sits outside package surface deliberately. That package is pure geometry
// and tagging — it snaps points to ways and names the ground — and stays free of
// any concern about where ways come from. This one owns the file formats, the
// download, and the SQLite storage, and depends on surface for the shared types
// rather than the other way round.
package osmindex

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/nobbs/domestique/internal/surface"
)

// coordinateScale is the fixed-point multiplier a stored coordinate carries: a
// value of 1e6 means six decimal places, about 11 cm of latitude.
//
// Six was measured rather than assumed. Against a full-precision index the
// five-decimal grid disagreed on 0.493% of ridden distance, which is the same
// order as the disagreement with Overpass itself and so would have doubled the
// error for a quarter less file; six decimals costs 57 MB against 73 MB for
// OpenStreetMap's native seven and reproduces it to 99.876%.
const coordinateScale = 1e6

// cellDegrees is the side of one index cell in degrees.
//
// Cell size does not affect what the index answers — two indexes at the same
// precision on different grids agreed to 100.000% — so it is chosen purely for
// read cost. At 0.01° a cell is roughly 1.1 km north-south, and the corridor
// around a stage touches a few dozen of them.
const cellDegrees = 0.01

// nativeScale is the fixed-point precision OpenStreetMap itself stores, and the
// precision coordinates are held at between the node pass and quantisation.
const nativeScale = 1e7

// coordinateMissing marks a node the extract referenced but never supplied, and
// is deliberately not zero: (0, 0) is a real point in the Gulf of Guinea, so a
// zero default would make a node there indistinguishable from an absent one.
// At nativeScale a coordinate reaches 1.8e9 at the antimeridian, which leaves
// the int32 floor unreachable by any real place.
const coordinateMissing = math.MinInt32

// errShortRecord means a cell blob ended in the middle of a way record, which
// can only happen if the file is truncated or was written by another format.
var errShortRecord = errors.New("osmindex: cell record is truncated")

// cellKey addresses one cell of the grid. The origin is the intersection of the
// equator and the prime meridian, so a key is meaningful without reference to
// the region the index was built from.
type cellKey struct {
	x int32
	y int32
}

// quantisedPerCell is how many quantised coordinate units span one cell. It is
// the conversion between a cell key and the coordinate origin a blob's deltas
// are measured from.
func quantisedPerCell() int64 {
	return int64(math.Round(cellDegrees * coordinateScale))
}

// quantiseDivisor is what a native-precision coordinate is divided by to reach
// the stored precision.
func quantiseDivisor() int32 {
	return int32(math.Round(nativeScale / coordinateScale))
}

// cellOf returns the cell a quantised coordinate falls in.
func cellOf(x, y int32) cellKey {
	per := int32(quantisedPerCell()) //nolint:gosec // A cell is a hundredth of a degree; the span is five digits.

	return cellKey{x: floorDiv(x, per), y: floorDiv(y, per)}
}

// floorDiv divides rounding towards negative infinity, so cells west of the
// prime meridian and south of the equator are the same size as the rest. Go's
// integer division truncates towards zero, which would otherwise make the four
// cells around the origin share a key with their neighbours.
func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor
	if value%divisor != 0 && (value < 0) != (divisor < 0) {
		quotient--
	}

	return quotient
}

// appendRun packs one way's geometry within one cell onto a buffer.
//
// The blob for a cell is the concatenation of these records with nothing
// wrapping them: no count, no length prefix, no index. That is what lets the
// builder append a cell's later records to the ones already stored without
// reading back what it wrote, which is in turn what lets it hold only a bounded
// amount of packed output in memory at a time.
//
// Way identifiers are stored whole rather than delta-encoded against the
// previous record for the same reason — a record has to mean the same thing
// wherever in the blob it lands. It costs about five bytes per record against a
// file whose bulk is coordinates.
func appendRun(buffer []byte, key cellKey, wayID int64, kind surface.Kind, x, y []int32) []byte {
	buffer = binary.AppendUvarint(buffer, uint64(wayID)) //nolint:gosec // OSM identifiers are positive.
	buffer = append(buffer, byte(kind))
	buffer = binary.AppendUvarint(buffer, uint64(len(x)))

	per := quantisedPerCell()
	previousX, previousY := int64(key.x)*per, int64(key.y)*per
	for index := range x {
		buffer = binary.AppendVarint(buffer, int64(x[index])-previousX)
		buffer = binary.AppendVarint(buffer, int64(y[index])-previousY)
		previousX, previousY = int64(x[index]), int64(y[index])
	}

	return buffer
}

// decodeCell reverses appendRun for every record in one cell's blob.
//
// The ways are returned with geometry in degrees, which is what surface.Match
// consumes, so nothing downstream has to know the file stores fixed point.
func decodeCell(blob []byte, key cellKey) ([]surface.Way, error) {
	per := quantisedPerCell()
	baseX, baseY := int64(key.x)*per, int64(key.y)*per

	ways := make([]surface.Way, 0, 32)
	offset := 0
	for offset < len(blob) {
		wayID, size := binary.Uvarint(blob[offset:])
		if size <= 0 {
			return nil, errShortRecord
		}
		offset += size
		if offset >= len(blob) {
			return nil, errShortRecord
		}
		kind := surface.Kind(blob[offset])
		offset++

		count, size := binary.Uvarint(blob[offset:])
		if size <= 0 {
			return nil, errShortRecord
		}
		offset += size
		// The count is preallocated, so it has to be believed only as far as the
		// blob could actually carry: a point is two varints and so at least two
		// bytes. Without this a corrupt count allocates against a number nothing
		// in the file supports, which panics in make rather than returning the
		// error every other damaged record here returns.
		//
		// The subtraction cannot go negative: Uvarint reported a positive size
		// just above, so it read inside the blob and left offset within it.
		remaining := len(blob) - offset
		if count > uint64(remaining)/2 { //nolint:gosec // Non-negative, as above.
			return nil, errShortRecord
		}

		line := make([]surface.Coordinate, 0, count)
		currentX, currentY := baseX, baseY
		for range count {
			deltaX, xSize := binary.Varint(blob[offset:])
			if xSize <= 0 {
				return nil, errShortRecord
			}
			offset += xSize
			deltaY, ySize := binary.Varint(blob[offset:])
			if ySize <= 0 {
				return nil, errShortRecord
			}
			offset += ySize

			currentX += deltaX
			currentY += deltaY
			line = append(line, surface.Coordinate{
				Longitude: float64(currentX) / coordinateScale,
				Latitude:  float64(currentY) / coordinateScale,
			})
		}
		//nolint:gosec // The identifier was written from a positive int64 by appendRun.
		ways = append(ways, surface.Way{ID: int64(wayID), Kind: kind, Line: line})
	}

	return ways, nil
}

// splitIntoCells cuts one way into the maximal runs that lie in a single cell,
// appending each to the accumulator.
//
// Each run carries one point past the boundary it ends at, so a segment that
// crosses between two cells is complete on both sides. Without that overlap a
// stage point sitting near a cell edge would measure its distance to a way that
// appeared to stop at the edge, and would snap to something further away.
func splitIntoCells(into map[cellKey][]byte, wayID int64, kind surface.Kind, x, y []int32) int {
	written := 0
	current := cellOf(x[0], y[0])
	start := 0
	for index := 1; index < len(x); index++ {
		key := cellOf(x[index], y[index])
		if key == current {
			continue
		}
		before := len(into[current])
		into[current] = appendRun(into[current], current, wayID, kind, x[start:index+1], y[start:index+1])
		written += len(into[current]) - before
		current, start = key, index-1
	}
	before := len(into[current])
	into[current] = appendRun(into[current], current, wayID, kind, x[start:], y[start:])

	return written + len(into[current]) - before
}

// verifyScales fails a read whose file was written at a precision this build
// cannot interpret. The values live in the file rather than in configuration so
// an index and its reader cannot drift apart silently.
func verifyScales(fileCellDegrees, fileScale float64) error {
	if fileCellDegrees != cellDegrees || fileScale != coordinateScale {
		return fmt.Errorf(
			"osmindex: index was built at cell %g scale %g, this build reads cell %g scale %g",
			fileCellDegrees, fileScale, cellDegrees, coordinateScale,
		)
	}

	return nil
}
