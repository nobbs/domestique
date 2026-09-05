package ridemodel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
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

type storedDuration struct {
	contentHash, surfaceGeneration, coefficientFingerprint string
	movingSeconds                                          *float64
	cumulativeSeconds                                      []byte
}

type fakeDurationCache struct {
	stored      map[route.Key]storedDuration
	failures    map[route.Key]string
	hashErr     error
	storeErr    error
	failureErr  error
	whileStored func()
	storeCalls  int
}

func newFakeDurationCache() *fakeDurationCache {
	return &fakeDurationCache{
		stored:   map[route.Key]storedDuration{},
		failures: map[route.Key]string{},
	}
}

func (f *fakeDurationCache) RecordStageDurationFailure(
	_ context.Context, provider route.Provider, routeID int64, stageOrder int, reason string,
) error {
	f.failures[route.NewKey(provider, routeID, stageOrder)] = reason

	return f.failureErr
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
	if f.whileStored != nil {
		f.whileStored()
	}
	if f.storeErr != nil {
		return f.storeErr
	}
	f.stored[route.NewKey(provider, routeID, stageOrder)] = storedDuration{
		contentHash: contentHash, surfaceGeneration: surfaceGeneration, coefficientFingerprint: coefficientFingerprint,
		movingSeconds: movingSeconds, cumulativeSeconds: cumulativeSeconds,
	}

	return nil
}

func TestPredictorPredictsAStageNotYetCached(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	cache := newFakeDurationCache()
	predictor := NewPredictor(cache, testCoefficients())

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
	predictor := NewPredictor(cache, coefficients)

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
	predictor := NewPredictor(cache, coefficients)

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
	predictor := NewPredictor(cache, coefficients)

	predicted, _, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "a re-fitted coefficient file should be recomputed")
}

// A row cached while prediction still read the ground carries that generation.
// It is no longer part of what makes a prediction current, so such a row is
// recomputed once and stored with none.
func TestPredictorRecomputesARowCachedAgainstASurfaceGeneration(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	coefficients := testCoefficients()
	cache := newFakeDurationCache()
	cache.stored[stage.Key()] = storedDuration{
		contentHash: "hash-1", surfaceGeneration: "generation-1", coefficientFingerprint: coefficients.Fingerprint,
	}

	predicted, _, err := NewPredictor(cache, coefficients).Predict(t.Context(), []route.Route{stage})
	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, predicted, "a row cached against a surface generation should be recomputed")
	assert.Empty(t, cache.stored[stage.Key()].surfaceGeneration, "the recomputed row carries no surface generation")
}

func TestPredictorRecordsNoPredictionForAStageWithNoElevation(t *testing.T) {
	stage := stageWithoutElevation(t, "hash-1")
	cache := newFakeDurationCache()
	predictor := NewPredictor(cache, testCoefficients())

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
	predictor := NewPredictor(cache, testCoefficients())

	_, _, err := predictor.Predict(t.Context(), []route.Route{stage})
	require.Error(t, err, "Predict()")
}

func TestPredictorStopsOnACancelledContext(t *testing.T) {
	stage := stageWithElevation(t, "hash-1")
	predictor := NewPredictor(newFakeDurationCache(), testCoefficients())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := predictor.Predict(ctx, []route.Route{stage})
	require.Error(t, err, "Predict() on a cancelled context")
}

func TestPredictNamesWhatStoppedAStage(t *testing.T) {
	t.Parallel()

	cache := newFakeDurationCache()
	cache.storeErr = errors.New("state unavailable")
	stage := stageWithElevation(t, "hash-1")

	_, failed, err := NewPredictor(cache, testCoefficients()).
		Predict(t.Context(), []route.Route{stage})

	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, failed, "failed stages")
	assert.Equal(t, map[route.Key]string{stage.Key(): ReasonCache}, cache.failures, "recorded failures")
}

func TestPredictCarriesOnWhenAFailureCannotBeRecorded(t *testing.T) {
	t.Parallel()

	cache := newFakeDurationCache()
	cache.storeErr = errors.New("state unavailable")
	cache.failureErr = errors.New("state unavailable")

	_, failed, err := NewPredictor(cache, testCoefficients()).
		Predict(t.Context(), []route.Route{stageWithElevation(t, "hash-1")})

	require.NoError(t, err, "Predict()")
	assert.Equal(t, 1, failed, "failed stages")
}

// A shutdown reaching the cache mid-pass is not something the stage did.
func TestPredictRecordsNothingForAStageAShutdownInterrupted(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cache := newFakeDurationCache()
	cache.whileStored = cancel
	cache.storeErr = context.Canceled

	predicted, failed, err := NewPredictor(cache, testCoefficients()).
		Predict(ctx, []route.Route{stageWithElevation(t, "hash-1")})

	require.ErrorIs(t, err, context.Canceled, "Predict()")
	assert.Zero(t, predicted, "predicted stages")
	assert.Zero(t, failed, "a stage interrupted by a shutdown counted as failed")
	assert.Empty(t, cache.failures, "a shutdown was recorded as a stage failure")
}
