// Package osmindex builds and reads a local index of OpenStreetMap way geometry
// and surface tagging, so a stage is classified from disk rather than from a
// public endpoint. It owns the file formats, the download and the storage, and
// depends on package surface for shared types rather than the other way round.
package osmindex

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/nobbs/domestique/internal/surface"
)

// coordinateScale is the fixed-point multiplier a stored coordinate carries: 1e6
// is six decimal places, about 11 cm of latitude. Five decimals disagreed on
// 0.493% of ridden distance; six costs 57 MB against 73 MB for OpenStreetMap's
// native seven and reproduces it to 99.876%.
const coordinateScale = 1e6

// cellDegrees is the side of one index cell in degrees. Cell size does not affect
// what the index answers, so it is chosen for read cost: at 0.01° a cell is
// roughly 1.1 km north-south and a stage's corridor touches a few dozen.
const cellDegrees = 0.01

// nativeScale is the fixed-point precision OpenStreetMap itself stores, and the
// precision coordinates are held at between the node pass and quantisation.
const nativeScale = 1e7

// coordinateMissing marks a node the extract referenced but never supplied, and
// is not zero: (0, 0) is a real point in the Gulf of Guinea. At nativeScale a
// coordinate reaches 1.8e9, leaving the int32 floor unreachable by a real place.
const coordinateMissing = math.MinInt32

// errShortRecord means a cell blob ended in the middle of a way record, which
// can only happen if the file is truncated or was written by another format.
var errShortRecord = errors.New("osmindex: cell record is truncated")

// cellKey addresses one cell of the grid. The origin is the equator and prime
// meridian, so a key is meaningful without reference to the region indexed.
type cellKey struct {
	x int32
	y int32
}

// quantisedPerCell is how many quantised coordinate units span one cell: the
// conversion between a cell key and the origin a blob's deltas are measured from.
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

// floorDiv divides rounding towards negative infinity, so cells west of the prime
// meridian and south of the equator are the same size as the rest. Go's integer
// division truncates towards zero, which would merge the four cells at the origin.
func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor
	if value%divisor != 0 && (value < 0) != (divisor < 0) {
		quotient--
	}

	return quotient
}

// appendRun packs one way's geometry within one cell onto a buffer. A cell's blob
// is the concatenation of these records with no count, length prefix or index, so
// the builder can append without reading back what it wrote. Way identifiers are
// stored whole for the same reason: a record must mean the same thing anywhere.
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

// decodeCell reverses appendRun for every record in one cell's blob. Geometry is
// returned in degrees, which is what surface.Match consumes.
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
		// The preallocated count is believed only as far as the blob could carry:
		// a point is two varints, so at least two bytes. A corrupt count would
		// otherwise panic in make rather than returning an error. The subtraction
		// cannot go negative: Uvarint read inside the blob just above.
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

// splitIntoCells cuts one way into the maximal runs lying in a single cell. Each
// run carries one point past the boundary it ends at, so a segment crossing cells
// is complete on both sides and a point near an edge does not snap further away.
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
// cannot interpret. The values live in the file so index and reader cannot drift.
func verifyScales(fileCellDegrees, fileScale float64) error {
	if fileCellDegrees != cellDegrees || fileScale != coordinateScale {
		return fmt.Errorf(
			"osmindex: index was built at cell %g scale %g, this build reads cell %g scale %g",
			fileCellDegrees, fileScale, cellDegrees, coordinateScale,
		)
	}

	return nil
}
