package ridemodel

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

func stageWithElevation(t *testing.T, contentHash string) route.Route {
	t.Helper()
	elevationA, elevationB := 100.0, 110.0
	stage, err := route.NewRoute(
		route.ProviderVeloPlanner, 1, 1, "revision", "Route", "",
		[]route.Point{
			{Longitude: 8.4, Latitude: 49.0, Elevation: &elevationA},
			{Longitude: 8.401, Latitude: 49.001, Elevation: &elevationB},
		},
		contentHash,
	)
	require.NoError(t, err, "route.NewRoute()")

	return stage
}

func stageWithoutElevation(t *testing.T, contentHash string) route.Route {
	t.Helper()
	stage, err := route.NewRoute(
		route.ProviderVeloPlanner, 2, 1, "revision", "Route", "",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		contentHash,
	)
	require.NoError(t, err, "route.NewRoute()")

	return stage
}

type surfaceEntry struct {
	generation  string
	contentHash string
	ranges      json.RawMessage
}

type fakeSurfaceSource struct {
	entries map[route.Key]surfaceEntry
	err     error
	// surfaceReadErr fails StageSurface alone, for a fake that reports a real
	// classification generation via StageSurfaceHash but cannot read the
	// ranges themselves — a transient failure independent of whether anything
	// is classified.
	surfaceReadErr error
}

func (f *fakeSurfaceSource) StageSurfaceHash(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int,
) (contentHash, generation string, found bool, err error) {
	if f.err != nil {
		return "", "", false, f.err
	}
	entry, ok := f.entries[route.NewKey(provider, routeID, stageOrder)]
	if !ok {
		return "", "", false, nil
	}

	return entry.contentHash, entry.generation, true, nil
}

func (f *fakeSurfaceSource) StageSurface(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int, contentHash string,
) (ranges json.RawMessage, matchedMetres float64, found bool, err error) {
	if f.err != nil {
		return nil, 0, false, f.err
	}
	if f.surfaceReadErr != nil {
		return nil, 0, false, f.surfaceReadErr
	}
	entry, ok := f.entries[route.NewKey(provider, routeID, stageOrder)]
	if !ok || entry.contentHash != contentHash {
		return nil, 0, false, nil
	}

	return entry.ranges, 0, true, nil
}

type storedDuration struct {
	contentHash, surfaceGeneration, coefficientFingerprint string
	movingSeconds                                          *float64
	cumulativeSeconds                                      []byte
}

type fakeDurationCache struct {
	stored     map[route.Key]storedDuration
	hashErr    error
	storeCalls int
}

func newFakeDurationCache() *fakeDurationCache {
	return &fakeDurationCache{stored: map[route.Key]storedDuration{}}
}

func (f *fakeDurationCache) StageDurationFingerprint(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int,
) (contentHash, surfaceGeneration, coefficientFingerprint string, found bool, err error) {
	if f.hashErr != nil {
		return "", "", "", false, f.hashErr
	}
	entry, ok := f.stored[route.NewKey(provider, routeID, stageOrder)]
	if !ok {
		return "", "", "", false, nil
	}

	return entry.contentHash, entry.surfaceGeneration, entry.coefficientFingerprint, true, nil
}

func (f *fakeDurationCache) StoreStageDuration(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int,
	contentHash, surfaceGeneration, coefficientFingerprint string,
	movingSeconds *float64, cumulativeSeconds []byte,
) error {
	f.storeCalls++
	f.stored[route.NewKey(provider, routeID, stageOrder)] = storedDuration{
		contentHash: contentHash, surfaceGeneration: surfaceGeneration, coefficientFingerprint: coefficientFingerprint,
		movingSeconds: movingSeconds, cumulativeSeconds: cumulativeSeconds,
	}

	return nil
}

func TestPredictorPredictsAStageNotYetCached(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	cache := newFakeDurationCache()
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, testCoefficients())

	predicted, failed, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "predicted")
	assert.Zero(t, failed, "failed")
	assert.Equal(t, 1, cache.storeCalls, "store calls")

	stored := cache.stored[stage.Key()]
	require.NotNil(t, stored.movingSeconds, "stored moving seconds")
	assert.Positive(t, *stored.movingSeconds, "stored moving seconds value")
	assert.NotEmpty(t, stored.cumulativeSeconds, "stored cumulative series")
}

func TestPredictorSkipsAStageAlreadyCurrent(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	cache := newFakeDurationCache()
	cache.stored[stage.Key()] = storedDuration{
		contentHash: "hash-1", surfaceGeneration: "", coefficientFingerprint: coefficients.Fingerprint,
	}
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, coefficients)

	predicted, failed, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Zero(t, predicted, "predicted")
	assert.Zero(t, failed, "failed")
	assert.Zero(t, cache.storeCalls, "an already-current stage must not be recomputed")
}

func TestPredictorRecomputesWhenGeometryChanges(t *testing.T) {
	stage := stageWithElevation(t, "hash-2")
	coefficients := testCoefficients()
	cache := newFakeDurationCache()
	cache.stored[stage.Key()] = storedDuration{
		contentHash: "hash-1", surfaceGeneration: "", coefficientFingerprint: coefficients.Fingerprint,
	}
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, coefficients)

	predicted, _, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "a changed content hash should be recomputed")
}

func TestPredictorRecomputesWhenCoefficientFingerprintChanges(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	cache := newFakeDurationCache()
	cache.stored[stage.Key()] = storedDuration{
		contentHash: "hash-1", surfaceGeneration: "", coefficientFingerprint: "an-earlier-coefficient-file",
	}
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, coefficients)

	predicted, _, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "a re-fitted coefficient file should be recomputed")
}

func TestPredictorRecomputesWhenSurfaceClassificationChanges(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	cache := newFakeDurationCache()
	cache.stored[stage.Key()] = storedDuration{
		contentHash: "hash-1", surfaceGeneration: "generation-1", coefficientFingerprint: coefficients.Fingerprint,
	}
	source := &fakeSurfaceSource{entries: map[route.Key]surfaceEntry{
		stage.Key(): {contentHash: "hash-1", generation: "generation-2"},
	}}
	predictor := NewPredictor(source, cache, coefficients)

	predicted, _, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "a new surface index generation should be recomputed")
}

func TestPredictorFallsBackToAsphaltWhenNoSurfaceIsCached(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	cache := newFakeDurationCache()
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, coefficients)

	predicted, failed, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "predicted")
	assert.Zero(t, failed, "failed")

	stored := cache.stored[stage.Key()]
	require.NotNil(t, stored.movingSeconds, "stored moving seconds")

	asphaltOnly, ok := Predict(stage.Geometry(), nil, coefficients)
	require.True(t, ok, "Predict() reference")
	assert.InDelta(t, asphaltOnly.MovingSeconds, *stored.movingSeconds, 1e-9, "an unclassified stage is timed as asphalt")
}

// Regression: StageSurfaceHash can report a real, non-empty generation while
// StageSurface itself fails to read the ranges — a transient error, distinct
// from nothing being classified yet. The stored fingerprint must not still
// claim that generation, or a later successful read would look, to the cache,
// identical to one that already used it, and the asphalt fallback this run
// took would never be retried.
func TestPredictorDoesNotLockInAnAsphaltFallbackWhenSurfaceRangesFailToRead(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	source := &fakeSurfaceSource{
		entries: map[route.Key]surfaceEntry{
			stage.Key(): {contentHash: "hash-1", generation: "generation-1"},
		},
		surfaceReadErr: errors.New("surface ranges unavailable"),
	}
	cache := newFakeDurationCache()
	predictor := NewPredictor(source, cache, coefficients)

	predicted, failed, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "predicted")
	assert.Zero(t, failed, "failed")

	_, storedGeneration, _, found, err := cache.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	require.True(t, found, "no fingerprint was cached")
	assert.Empty(t, storedGeneration, "an asphalt fallback must not be cached against the real surface generation")

	// Once the ranges become readable, the mismatch against the cached empty
	// generation must trigger a recompute rather than being skipped as current.
	source.surfaceReadErr = nil
	source.entries[stage.Key()] = surfaceEntry{contentHash: "hash-1", generation: "generation-1"}
	predicted, _, err = predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "second Predict()")
	assert.Equal(t, 1, predicted, "a readable classification should trigger a recompute")
}

func TestPredictorReadsCachedSurfaceClassification(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	ranges, err := surface.EncodeRanges([]surface.Range{{StartIndex: 0, EndIndex: 1, Kind: surface.KindGravel}})
	require.NoError(t, err, "surface.EncodeRanges()")
	source := &fakeSurfaceSource{entries: map[route.Key]surfaceEntry{
		stage.Key(): {contentHash: "hash-1", generation: "generation-1", ranges: ranges},
	}}
	cache := newFakeDurationCache()
	predictor := NewPredictor(source, cache, coefficients)

	_, _, err = predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")

	stored := cache.stored[stage.Key()]
	require.NotNil(t, stored.movingSeconds, "stored moving seconds")

	gravelOnly, ok := Predict(stage.Geometry(), kindsAll(len(stage.Geometry()), surface.KindGravel), coefficients)
	require.True(t, ok, "Predict() reference")
	assert.InDelta(t, gravelOnly.MovingSeconds, *stored.movingSeconds, 1e-9, "a classified stage should read gravel")
}

func TestPredictorRecordsNoPredictionForAStageWithNoElevation(t *testing.T) {
	stage := stageWithoutElevation(t, "hash-1")
	cache := newFakeDurationCache()
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, testCoefficients())

	predicted, failed, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "predicted")
	assert.Zero(t, failed, "failed")

	stored, ok := cache.stored[stage.Key()]
	require.True(t, ok, "a stage with no usable elevation is still recorded")
	assert.Nil(t, stored.movingSeconds, "no prediction is stored for it")
	assert.Nil(t, stored.cumulativeSeconds, "no cumulative series is stored for it")
}

func TestPredictorReportsAnErrorWhenTheCacheIsUnavailable(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	cache := newFakeDurationCache()
	cache.hashErr = errors.New("state unavailable")
	predictor := NewPredictor(&fakeSurfaceSource{}, cache, testCoefficients())

	_, _, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.Error(t, err, "Predict()")
}

func TestPredictorStopsOnACancelledContext(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	predictor := NewPredictor(&fakeSurfaceSource{}, newFakeDurationCache(), testCoefficients())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := predictor.Predict(ctx, []route.Route{stage})
	require.Error(t, err, "Predict() on a cancelled context")
}
