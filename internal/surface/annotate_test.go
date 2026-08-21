package surface

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

// A stage whose cells are unreadable is only that stage's problem, and a long
// stage is only classified once its read lands. Stopping the pass at the first
// failure meant one such stage starved every stage behind it — in that run, and
// in every run after, because the inventory is always walked in the same order.
func TestAnnotateClassifiesTheStagesAfterOneThatFailed(t *testing.T) {
	source := &fakeSource{generation: "abc123", failFor: map[int64]error{2: errors.New("cell unreadable")}}
	cache := newFakeCache()
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1, 2, 3))
	require.NoError(t, err)
	assert.Equal(t, 2, classified)
	assert.Equal(t, 1, failed)
	assert.Contains(t, cache.stored, int64(3), "the stage after the failure was never classified")
}

// Until a first index has been built there is nothing to read, and recording
// every stage as unsurveyed would only mean reclassifying the lot as soon as one
// lands.
func TestAnnotateDoesNothingWithoutAnIndex(t *testing.T) {
	source := &fakeSource{}
	cache := newFakeCache()
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1, 2))
	require.NoError(t, err)
	assert.Zero(t, classified)
	assert.Zero(t, failed)
	assert.Empty(t, cache.stored, "classified stages against a source with no map behind it")
}

func TestAnnotateSkipsAStageAlreadyClassifiedAgainstItsGeometry(t *testing.T) {
	source := &fakeSource{generation: "abc123"}
	cache := newFakeCache()
	cache.hashes[1] = "hash-1"
	cache.generations[1] = "abc123"
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1, 2))
	require.NoError(t, err)
	assert.Equal(t, 1, classified)
	assert.Zero(t, failed)
	assert.False(t, source.asked[1],
		"read the index for a stage already classified for this geometry")
}

// A rebuilt map may have resurfaced a road under geometry that never moved, so
// the cached reading is stale even though the stage is not.
func TestAnnotateReclassifiesAStageMeasuredAgainstAnOlderIndex(t *testing.T) {
	source := &fakeSource{generation: "def456"}
	cache := newFakeCache()
	cache.hashes[1] = "hash-1"
	cache.generations[1] = "abc123"
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1))
	require.NoError(t, err)
	assert.Equal(t, 1, classified)
	assert.Zero(t, failed)
	assert.True(t, source.asked[1], "kept a classification measured against a retired index")
	assert.Equal(t, "def456", cache.generations[1], "the cached generation was not moved forward")
}

func testStages(t *testing.T, routeIDs ...int64) []route.Stage {
	t.Helper()
	stages := make([]route.Stage, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		// Each stage sits at its own latitude, which is how the fake source
		// below tells them apart: it is handed geometry, not identity.
		geometry := []route.Point{
			{Longitude: 8.0, Latitude: float64(routeID)},
			{Longitude: 8.001, Latitude: float64(routeID)},
		}
		stage, err := route.NewStage(
			routeID, 1, "revision", "Route", "Stage", geometry, "hash-"+strconv.FormatInt(routeID, 10),
		)
		require.NoError(t, err)
		stages = append(stages, stage)
	}

	return stages
}

type fakeSource struct {
	failFor    map[int64]error
	asked      map[int64]bool
	generation string
}

func (s *fakeSource) Generation() string { return s.generation }

func (s *fakeSource) Ways(_ context.Context, points []route.Point) ([]Way, error) {
	if s.asked == nil {
		s.asked = make(map[int64]bool)
	}
	routeID := int64(points[0].Latitude)
	s.asked[routeID] = true
	if err, found := s.failFor[routeID]; found {
		return nil, err
	}

	line := make([]Coordinate, 0, len(points))
	for _, point := range points {
		line = append(line, Coordinate{Longitude: point.Longitude, Latitude: point.Latitude})
	}

	return []Way{{ID: routeID, Kind: KindAsphalt, Line: line}}, nil
}

type fakeCache struct {
	hashes      map[int64]string
	generations map[int64]string
	stored      map[int64][]byte
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		hashes:      make(map[int64]string),
		generations: make(map[int64]string),
		stored:      make(map[int64][]byte),
	}
}

func (c *fakeCache) StageSurfaceHash(
	_ context.Context, routeID int64, _ int,
) (contentHash, generation string, found bool, err error) {
	hash, cached := c.hashes[routeID]

	return hash, c.generations[routeID], cached, nil
}

func (c *fakeCache) StoreStageSurface(
	_ context.Context, routeID int64, _ int, contentHash, generation string, ranges []byte, _ float64,
) error {
	c.hashes[routeID] = contentHash
	c.generations[routeID] = generation
	c.stored[routeID] = ranges

	return nil
}
