package demo_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

func TestLibraryIsTheSameOnEveryRun(t *testing.T) {
	t.Parallel()

	first, err := demo.Stages()
	require.NoError(t, err)
	second, err := demo.Stages()
	require.NoError(t, err)

	require.Len(t, second, len(first))
	for index := range first {
		left, right := &first[index], &second[index]
		assert.Equal(t, left.ContentHash(), right.ContentHash(),
			"stage %d is not the stage it was a moment ago", index)
		assert.Equal(t, left.Geometry(), right.Geometry())
	}
}

func TestLibraryCoversTheShapesTheUIHasToDraw(t *testing.T) {
	t.Parallel()

	stages, err := demo.Stages()
	require.NoError(t, err)

	routes := map[int64]int{}
	var loops, longest, shortest int
	shortestMetres := math.Inf(1)
	longestMetres := 0.0
	for index := range stages {
		stage := &stages[index]
		routes[stage.Key().RouteID()]++

		geometry := stage.Geometry()
		start, finish := geometry[0], geometry[len(geometry)-1]
		if metresApart(start, finish) < 100 {
			loops++
		}
		if metres := stage.DistanceMetres(); metres < shortestMetres {
			shortestMetres, shortest = metres, index
		}
		if metres := stage.DistanceMetres(); metres > longestMetres {
			longestMetres, longest = metres, index
		}
	}

	assert.Contains(t, routes, int64(4101), "a multi-stage route is what a route list has to group")
	assert.Equal(t, 3, routes[4101])
	assert.GreaterOrEqual(t, loops, 2, "a loop and an out-and-back both finish where they started")
	assert.Less(t, shortestMetres, 3_000.0, "a stage short enough to crowd its own map markers")
	assert.Greater(t, longestMetres, 50_000.0, "a stage long enough to need its cues spaced out")
	assert.NotEqual(t, shortest, longest)
}

func TestLibraryCoversEveryElevationCase(t *testing.T) {
	t.Parallel()

	stages, err := demo.Stages()
	require.NoError(t, err)

	var none, partial, complete int
	gradients := map[string]bool{}
	for index := range stages {
		stage := &stages[index]
		heights, holes := 0, 0
		for _, point := range stage.Geometry() {
			if point.Elevation == nil {
				holes++

				continue
			}
			heights++
		}
		switch {
		case heights == 0:
			none++
		case holes == 0:
			complete++
		default:
			partial++
		}

		switch gradient := stage.MaxGradientPercent(); {
		case gradient < 1:
			gradients["flat"] = true
		case gradient < 5:
			gradients["rolling"] = true
		default:
			gradients["steep"] = true
		}
	}

	assert.Positive(t, none, "a stage the source stored with no profile at all")
	assert.Positive(t, partial, "a stage whose profile stops partway")
	assert.Positive(t, complete)
	assert.Len(t, gradients, 3, "a gradient key with one band lit tells a developer nothing: %v", gradients)
}

func TestClassificationsCoverEverySurfaceOutcome(t *testing.T) {
	t.Parallel()

	stages, err := demo.Stages()
	require.NoError(t, err)
	classifications, err := demo.Classifications(stages)
	require.NoError(t, err)

	require.Less(t, len(classifications), len(stages),
		"a stage that was never classified looks different from one classified as unknown")

	kinds := map[string]bool{}
	partiallyMatched := 0
	byKey := map[[2]int64]*route.Stage{}
	for index := range stages {
		key := stages[index].Key()
		byKey[[2]int64{key.RouteID(), int64(key.StageOrder())}] = &stages[index]
	}
	for _, classification := range classifications {
		stage := byKey[[2]int64{classification.RouteID, int64(classification.StageOrder)}]
		require.NotNil(t, stage, "a classification addresses a stage that is not in the library")
		assert.Equal(t, stage.ContentHash(), classification.ContentHash,
			"a surface stored against another revision's geometry is pruned on the next read")

		for _, band := range decodeRanges(t, classification.Ranges) {
			kinds[band.Kind] = true
			assert.LessOrEqual(t, band.EndIndex, len(stage.Geometry())-1,
				"a range addresses a point the geometry does not have")
		}
		if classification.MatchedMetres < 0.95*stage.DistanceMetres() {
			partiallyMatched++
		}
	}

	for _, expected := range []surface.Kind{
		surface.KindUnknown, surface.KindAsphalt, surface.KindPaving,
		surface.KindCompacted, surface.KindGravel, surface.KindGround,
	} {
		assert.Contains(t, kinds, expected.String())
	}
	assert.Positive(t, partiallyMatched,
		"a classification that covers part of a stage is the honest denominator case")
}

func TestRevisionsDifferSoATargetCanLookBehind(t *testing.T) {
	t.Parallel()

	assert.NotEqual(t, demo.Revision(4101, 1), demo.EarlierRevision(4101, 1))
	assert.NotEqual(t, demo.Revision(4101, 1), demo.Revision(4101, 2))
}

// storedRange is the wire form the geometry endpoint serves, restated here so
// the test reads the bytes a consumer would rather than the producer's types.
//
//nolint:tagliatelle // the stored wire form is snake_case; this reads it, it does not choose it.
type storedRange struct {
	Kind       string `json:"kind"`
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
}

func decodeRanges(t *testing.T, encoded []byte) []storedRange {
	t.Helper()

	var ranges []storedRange
	require.NoError(t, json.Unmarshal(encoded, &ranges))
	require.NotEmpty(t, ranges)

	return ranges
}

func metresApart(from, to route.Point) float64 {
	const metresPerDegree = 111_320.0

	northward := (to.Latitude - from.Latitude) * metresPerDegree
	eastwards := (to.Longitude - from.Longitude) * metresPerDegree *
		math.Cos(from.Latitude*math.Pi/180)

	return math.Hypot(northward, eastwards)
}
