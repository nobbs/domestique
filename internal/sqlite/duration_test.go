package sqlite

import (
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCachesStageDurationAgainstTheFingerprintItWasComputedFrom(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 7, 2, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageDurationFingerprint() before prediction")
	assert.False(t, found, "a duration fingerprint was stored before prediction")

	movingSeconds := 987.5
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 7, 2,
		"content-hash", "surface-gen", "coefficient-fingerprint",
		&movingSeconds, []byte(`[0,10,20]`),
	), "StoreStageDuration()")

	contentHash, surfaceGeneration, coefficientFingerprint, found, err := store.StageDurationFingerprint(
		t.Context(), route.ProviderVeloPlanner, 7, 2,
	)
	require.NoError(t, err, "StageDurationFingerprint()")
	require.True(t, found, "StageDurationFingerprint() did not find the stored fingerprint")
	assert.Equal(t, "content-hash", contentHash, "content hash")
	assert.Equal(t, "surface-gen", surfaceGeneration, "surface generation")
	assert.Equal(t, "coefficient-fingerprint", coefficientFingerprint, "coefficient fingerprint")

	summary, _, cumulativeSeconds, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stage")
	require.NotNil(t, summary.MovingSeconds, "StageGeometry() moving seconds")
	assert.InDelta(t, 987.5, *summary.MovingSeconds, 0.001, "moving seconds")
	assert.JSONEq(t, `[0,10,20]`, string(cumulativeSeconds), "StageGeometry() cumulative seconds")
}

// A stage with no usable elevation is still recorded, with a nil prediction,
// so the next pass does not ask about it again every run — the same reasoning
// stage_surface's own "nothing to report" row follows.
func TestStoreStoresTheAbsenceOfAPrediction(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash", "", "coefficient-fingerprint", nil, nil,
	), "StoreStageDuration()")

	contentHash, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	require.True(t, found, "a stage with no usable elevation was not recorded")
	assert.Equal(t, "content-hash", contentHash, "content hash")

	summary, _, cumulativeSeconds, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stage")
	assert.Nil(t, summary.MovingSeconds, "a stage with no usable elevation must not report a moving time")
	assert.Empty(t, cumulativeSeconds, "a stage with no usable elevation must not report a cumulative series")
}

// A prediction measured against an earlier plan of the same stage addresses
// coordinates that no longer exist, so it must not be served for the stage's
// current geometry, on the same terms StageSurface hides a stale
// classification.
func TestStoreHidesADurationMeasuredAgainstOtherGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "current-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "earlier-hash", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	summary, _, cumulativeSeconds, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stage")
	assert.Nil(t, summary.MovingSeconds, "a prediction measured against replaced geometry was served for it")
	assert.Empty(t, cumulativeSeconds, "a cumulative series measured against replaced geometry was served for it")
}

func TestStorePrunesDurationForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first, second}), "StoreTrustedInventory()")
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 2, 1, "hash-two", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first}), "second StoreTrustedInventory()")

	_, _, _, removedFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, err, "StageDurationFingerprint() for a removed stage")
	assert.False(t, removedFound, "the duration of a removed stage is still stored")

	_, _, _, retainedFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint() for a retained stage")
	assert.True(t, retainedFound, "the duration of a retained stage was dropped")
}

func TestStorePrunesDurationMeasuredAgainstReplacedGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash-one")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	replanned := storeTestStage(t, 1, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{replanned}), "second StoreTrustedInventory()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint() after re-planning")
	assert.False(t, found, "stage_duration row survived re-planning")
}

func TestStorePruneStageDurationsWithDifferentFingerprintKeepsOnlyTheCurrentOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "current", &movingSeconds, nil,
	), "StoreStageDuration() current")
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 2, 1, "hash", "", "earlier", &movingSeconds, nil,
	), "StoreStageDuration() earlier")

	require.NoError(t, store.PruneStageDurationsWithDifferentFingerprint(t.Context(), "current"), "PruneStageDurationsWithDifferentFingerprint()")

	_, _, _, currentFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint() current")
	assert.True(t, currentFound, "a prediction matching the current fingerprint was pruned")

	_, _, _, earlierFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, err, "StageDurationFingerprint() earlier")
	assert.False(t, earlierFound, "a prediction from an earlier coefficient file survived pruning")
}

// An empty currentFingerprint is what an unconfigured deployment passes: it
// matches no stored row, so every prediction is pruned — the read path must
// serve nothing, not whatever an earlier configuration left behind.
func TestStorePruneStageDurationsWithDifferentFingerprintClearsEverythingWhenUnconfigured(t *testing.T) {
	store := openTestStore(t, testKey(1))
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "a-since-removed-coefficient-file", &movingSeconds, nil,
	), "StoreStageDuration()")

	require.NoError(t, store.PruneStageDurationsWithDifferentFingerprint(t.Context(), ""), "PruneStageDurationsWithDifferentFingerprint()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	assert.False(t, found, "a prediction survived pruning with no fingerprint configured")
}
