package surface

import "math"

// segmentGrid indexes way segments by locality so each stage point compares
// itself against nearby candidates rather than every way in the area. A uniform
// grid rather than a tree: the data is uniformly dense at the only scale that
// matters, and a grid is a fraction of the code.
type segmentGrid struct {
	cells    map[cell][]int
	scratch  []int
	cellSize float64
}

type cell struct {
	east  int
	north int
}

// newSegmentGrid indexes segments for lookups within the given radius. The cell is
// twice the radius and each segment is sampled at the radius, which guarantees
// near() finds every segment within the radius while searching only the immediate
// ring: a candidate registers at most radius+radius/2 away, inside one cell width.
func newSegmentGrid(segments []segment, radiusMetres float64) *segmentGrid {
	grid := &segmentGrid{
		cells:    make(map[cell][]int, len(segments)),
		cellSize: radiusMetres * 2,
	}
	for index := range segments {
		grid.insert(index, segments[index], radiusMetres)
	}

	return grid
}

func (g *segmentGrid) insert(index int, target segment, sampleStepMetres float64) {
	runEast := target.endEast - target.startEast
	runNorth := target.endNorth - target.startNorth
	length := math.Hypot(runEast, runNorth)
	steps := 1
	if length > 0 {
		steps = int(math.Ceil(length/sampleStepMetres)) + 1
	}

	for step := range steps {
		ratio := 0.0
		if steps > 1 {
			ratio = float64(step) / float64(steps-1)
		}
		g.add(index, cell{
			east:  int(math.Floor((target.startEast + ratio*runEast) / g.cellSize)),
			north: int(math.Floor((target.startNorth + ratio*runNorth) / g.cellSize)),
		})
	}
}

// add registers a segment in one cell, skipping a repeat of the segment already
// at the tail. Segments are inserted one at a time, so the tail is the only
// place a duplicate of the current segment can be.
func (g *segmentGrid) add(index int, key cell) {
	existing := g.cells[key]
	if len(existing) > 0 && existing[len(existing)-1] == index {
		return
	}
	g.cells[key] = append(existing, index)
}

// near returns the segments registered in the ring of cells around a projected
// point. A segment in several cells is returned more than once; evaluating it
// twice is cheaper than the bookkeeping to avoid it. The result is owned by the
// grid and valid only until the next call.
func (g *segmentGrid) near(east, north float64) []int {
	centre := cell{
		east:  int(math.Floor(east / g.cellSize)),
		north: int(math.Floor(north / g.cellSize)),
	}

	g.scratch = g.scratch[:0]
	for eastOffset := -1; eastOffset <= 1; eastOffset++ {
		for northOffset := -1; northOffset <= 1; northOffset++ {
			g.scratch = append(g.scratch, g.cells[cell{
				east:  centre.east + eastOffset,
				north: centre.north + northOffset,
			}]...)
		}
	}

	return g.scratch
}
