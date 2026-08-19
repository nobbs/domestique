package surface

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSegmentGridFindsEverySegmentWithinTheRadius checks the guarantee the grid
// is built on, by comparing it against the exhaustive search it replaces. The
// index is only allowed to be an optimisation: anything the naive scan would
// have considered a candidate has to come back from near() too.
func TestSegmentGridFindsEverySegmentWithinTheRadius(t *testing.T) {
	const radius = 25.0

	//nolint:gosec // A fixed seed keeps a failure reproducible; nothing here is a secret.
	generator := rand.New(rand.NewPCG(1, 2))
	segments := make([]segment, 0, 300)
	for range 300 {
		startEast := generator.Float64()*1000 - 500
		startNorth := generator.Float64()*1000 - 500
		segments = append(segments, segment{
			startEast:  startEast,
			startNorth: startNorth,
			endEast:    startEast + generator.Float64()*400 - 200,
			endNorth:   startNorth + generator.Float64()*400 - 200,
		})
	}
	grid := newSegmentGrid(segments, radius)

	for range 2000 {
		east := generator.Float64()*1000 - 500
		north := generator.Float64()*1000 - 500

		found := make(map[int]bool)
		for _, index := range grid.near(east, north) {
			found[index] = true
		}
		for index := range segments {
			distance := segments[index].distanceTo(east, north)
			if distance > radius {
				continue
			}
			require.Truef(t, found[index], "segment %d is %.2fm from (%.2f, %.2f) but near() did not return it",
				index, distance, east, north)
		}
	}
}

// TestSegmentGridFindsASegmentAcrossItsLength covers the case the sampling in
// insert exists for: a segment far longer than one cell, whose nearest point to
// the query is in the middle of it rather than at either end.
func TestSegmentGridFindsASegmentAcrossItsLength(t *testing.T) {
	const radius = 25.0

	tests := []struct {
		name       string
		target     segment
		queryEast  float64
		queryNorth float64
	}{
		{
			name:       "a long segment is found at its midpoint",
			target:     segment{startEast: -5000, startNorth: 0, endEast: 5000, endNorth: 0},
			queryEast:  0,
			queryNorth: 10,
		},
		{
			name:       "a segment exactly at the radius is still a candidate",
			target:     segment{startEast: -100, startNorth: radius, endEast: 100, endNorth: radius},
			queryEast:  0,
			queryNorth: 0,
		},
		{
			name:       "a degenerate segment is found at its point",
			target:     segment{startEast: 300, startNorth: -300, endEast: 300, endNorth: -300},
			queryEast:  310,
			queryNorth: -300,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			distance := test.target.distanceTo(test.queryEast, test.queryNorth)
			require.LessOrEqualf(t, distance, radius,
				"the test's own query is %.2fm away, beyond the %.2fm radius", distance, radius)

			grid := newSegmentGrid([]segment{test.target}, radius)
			assert.NotEmpty(t, grid.near(test.queryEast, test.queryNorth),
				"near() returned no candidates, want the segment")
		})
	}
}

// TestSegmentGridPrunesDistantSegments is the other half of the contract: the
// grid has to actually narrow the search, or Match pays for the index and still
// compares every point against every way.
func TestSegmentGridPrunesDistantSegments(t *testing.T) {
	const (
		radius       = 25.0
		segmentCount = 500
	)

	//nolint:gosec // A fixed seed keeps a failure reproducible; nothing here is a secret.
	generator := rand.New(rand.NewPCG(3, 4))
	segments := make([]segment, 0, segmentCount)
	for range segmentCount {
		startEast := generator.Float64()*4000 - 2000
		startNorth := generator.Float64()*4000 - 2000
		segments = append(segments, segment{
			startEast:  startEast,
			startNorth: startNorth,
			endEast:    startEast + generator.Float64()*100 - 50,
			endNorth:   startNorth + generator.Float64()*100 - 50,
		})
	}
	grid := newSegmentGrid(segments, radius)

	const queryCount = 500
	examined := 0
	for range queryCount {
		examined += len(grid.near(generator.Float64()*4000-2000, generator.Float64()*4000-2000))
	}

	naive := queryCount * segmentCount
	assert.LessOrEqualf(t, examined, naive/10,
		"near() examined %d candidates over %d queries, want well under a tenth of the %d a full scan costs",
		examined, queryCount, naive)
}

func TestSegmentGridDistanceToClampsToTheSegmentEnds(t *testing.T) {
	target := segment{startEast: 0, startNorth: 0, endEast: 100, endNorth: 0}

	tests := []struct {
		name  string
		east  float64
		north float64
		want  float64
	}{
		{name: "beside the middle", east: 50, north: 30, want: 30},
		{name: "beyond the start", east: -40, north: 0, want: 40},
		{name: "beyond the end", east: 140, north: 30, want: 50},
		{name: "on the segment", east: 25, north: 0, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.InDelta(t, test.want, target.distanceTo(test.east, test.north), 0.001,
				"distanceTo(%v, %v)", test.east, test.north)
		})
	}
}
