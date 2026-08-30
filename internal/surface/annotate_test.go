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
	assert.Contains(t, cache.stored, testKey(3), "the stage after the failure was never classified")
	assert.Equal(t, map[route.Key]string{testKey(2): ReasonWays},
		cache.failures, "the stage that failed was not named")
}

// A count says how many stages are missing a classification; the record says
// which, and what stopped each one.
func TestAnnotateNamesWhatStoppedEachStage(t *testing.T) {
	source := &fakeSource{generation: "abc123", failFor: map[int64]error{1: errors.New("cell unreadable")}}
	cache := newFakeCache()
	cache.storeErr = map[route.Key]error{testKey(2): errors.New("state unavailable")}

	_, failed, err := NewAnnotator(source, cache).Annotate(t.Context(), testStages(t, 1, 2))
	require.NoError(t, err)
	assert.Equal(t, 2, failed, "failed stages")
	assert.Equal(t, map[route.Key]string{
		testKey(1): ReasonWays,
		testKey(2): ReasonCache,
	}, cache.failures, "recorded failures")
}

// Losing the record of a failure leaves the count as the only account of it,
// which is what there was before. It must not stop the pass.
func TestAnnotateCarriesOnWhenAFailureCannotBeRecorded(t *testing.T) {
	source := &fakeSource{generation: "abc123", failFor: map[int64]error{1: errors.New("cell unreadable")}}
	cache := newFakeCache()
	cache.failureErr = errors.New("state unavailable")

	classified, failed, err := NewAnnotator(source, cache).Annotate(t.Context(), testStages(t, 1, 2))
	require.NoError(t, err, "Annotate()")
	assert.Equal(t, 1, classified, "classified stages")
	assert.Equal(t, 1, failed, "failed stages")
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
	cache.hashes[testKey(1)] = "hash-1"
	cache.generations[testKey(1)] = "abc123"
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
	cache.hashes[testKey(1)] = "hash-1"
	cache.generations[testKey(1)] = "abc123"
	annotator := NewAnnotator(source, cache)

	classified, failed, err := annotator.Annotate(t.Context(), testStages(t, 1))
	require.NoError(t, err)
	assert.Equal(t, 1, classified)
	assert.Zero(t, failed)
	assert.True(t, source.asked[1], "kept a classification measured against a retired index")
	assert.Equal(t, "def456", cache.generations[testKey(1)], "the cached generation was not moved forward")
}

func testStages(t *testing.T, routeIDs ...int64) []route.Route {
	t.Helper()
	stages := make([]route.Route, 0, len(routeIDs))
	for _, routeID := range routeIDs {
		// Each stage sits at its own latitude, which is how the fake source
		// below tells them apart: it is handed geometry, not identity.
		geometry := []route.Point{
			{Longitude: 8.0, Latitude: float64(routeID)},
			{Longitude: 8.001, Latitude: float64(routeID)},
		}
		stage, err := route.NewRoute(
			route.ProviderVeloPlanner, routeID, 1, "revision", "Route", "Stage", geometry, "hash-"+strconv.FormatInt(routeID, 10),
		)
		require.NoError(t, err)
		stages = append(stages, stage)
	}

	return stages
}

type fakeSource struct {
	failFor    map[int64]error
	asked      map[int64]bool
	whileAsked func()
	generation string
}

func (s *fakeSource) Generation() string { return s.generation }

func (s *fakeSource) Ways(_ context.Context, points []route.Point) ([]Way, error) {
	if s.asked == nil {
		s.asked = make(map[int64]bool)
	}
	routeID := int64(points[0].Latitude)
	s.asked[routeID] = true
	if s.whileAsked != nil {
		s.whileAsked()
	}
	if err, found := s.failFor[routeID]; found {
		return nil, err
	}

	line := make([]Coordinate, 0, len(points))
	for _, point := range points {
		line = append(line, Coordinate{Longitude: point.Longitude, Latitude: point.Latitude})
	}

	return []Way{{ID: routeID, Kind: KindAsphalt, Line: line}}, nil
}

// fakeCache keys on the whole stage identity rather than the route ID alone. The
// cache it stands in for is addressed by provider, route and stage order, so a
// fake dropping any of the three would answer for a stage it was never asked about.
type fakeCache struct {
	hashes      map[route.Key]string
	generations map[route.Key]string
	stored      map[route.Key][]byte
	failures    map[route.Key]string
	storeErr    map[route.Key]error
	failureErr  error
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		hashes:      make(map[route.Key]string),
		generations: make(map[route.Key]string),
		stored:      make(map[route.Key][]byte),
		failures:    make(map[route.Key]string),
	}
}

func (c *fakeCache) RecordStageSurfaceFailure(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int, reason string,
) error {
	c.failures[route.NewKey(provider, routeID, stageOrder)] = reason

	return c.failureErr
}

func (c *fakeCache) StageSurfaceHash(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int,
) (contentHash, generation string, found bool, err error) {
	key := route.NewKey(provider, routeID, stageOrder)
	hash, cached := c.hashes[key]

	return hash, c.generations[key], cached, nil
}

func (c *fakeCache) StoreStageSurface(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int, contentHash, generation string, ranges []byte, _ float64,
) error {
	key := route.NewKey(provider, routeID, stageOrder)
	if err := c.storeErr[key]; err != nil {
		return err
	}
	c.hashes[key] = contentHash
	c.generations[key] = generation
	c.stored[key] = ranges

	return nil
}

// testKey is the identity testStages gives a stage built from a route ID.
func testKey(routeID int64) route.Key {
	return route.NewKey(route.ProviderVeloPlanner, routeID, 1)
}

// A shutdown reaching the map mid-pass is not something the stage did. Recording
// it would leave a row blaming the map for a service that was stopping.
func TestAnnotateRecordsNothingForAStageAShutdownInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	source := &fakeSource{
		generation: "abc123",
		failFor:    map[int64]error{1: context.Canceled},
		whileAsked: cancel,
	}
	cache := newFakeCache()

	classified, failed, err := NewAnnotator(source, cache).Annotate(ctx, testStages(t, 1, 2))

	require.ErrorIs(t, err, context.Canceled, "Annotate()")
	assert.Zero(t, classified, "classified stages")
	assert.Zero(t, failed, "a stage interrupted by a shutdown counted as failed")
	assert.Empty(t, cache.failures, "a shutdown was recorded as a stage failure")
}
