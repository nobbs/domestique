package sqlite

import (
	"fmt"
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorePersistsTrustedInventoryAndTargetStages(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	count, err := store.TrustedInventoryCount(t.Context(), route.ProviderVeloPlanner)
	require.NoError(t, err, "TrustedInventoryCount()")
	assert.Equal(t, 1, count, "TrustedInventoryCount()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 1, 1, "revision", "content-hash", 42), "UpsertTargetStage()")

	var got []string
	require.NoError(t, store.ForEachTargetStage(
		t.Context(),
		"rider-a",
		func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error {
			got = append(got, fmt.Sprintf("%s/%d/%d/%s/%s/%d", provider, routeID, stageOrder, sourceRevision, contentHash, wahooRouteID))

			return nil
		},
	), "ForEachTargetStage()")
	assert.Equal(t, []string{"veloplanner/1/1/revision/content-hash/42"}, got, "target mappings")
	require.NoError(t, store.DeleteTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 1, 1), "DeleteTargetStage()")
	require.NoError(t, store.ForEachTargetStage(t.Context(), "rider-a", func(route.Provider, int64, int, string, string, int64) error {
		assert.Fail(t, "ForEachTargetStage() invoked the visitor after deletion")

		return nil
	}), "ForEachTargetStage() after deletion")
}

// The stored inventory is what the target phase reconciles from, so it has to
// come back as the same stages that went in, elevation included.
func TestStoreReadsTheTrustedInventoryBackAsStages(t *testing.T) {
	store := openTestStore(t, testKey(1))
	elevation := 128.5
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	stages, err := store.TrustedInventory(t.Context())
	require.NoError(t, err, "TrustedInventory()")
	require.Len(t, stages, 1, "stages")
	restored := stages[0]
	assert.Equal(t, stage.Key(), restored.Key(), "key")
	assert.Equal(t, "content-hash", restored.ContentHash(), "content hash")
	assert.Equal(t, "revision", restored.Revision(), "revision")
	assert.Equal(t, "Alpine loop — Descent", restored.Title(), "title")
	points := restored.Geometry()
	require.Len(t, points, 2, "points")
	require.NotNil(t, points[0].Elevation, "the first point lost its elevation")
	assert.InDelta(t, elevation, *points[0].Elevation, 0.001, "first elevation")
	assert.Nil(t, points[1].Elevation, "the second point gained an elevation")
}

// Storing one provider's inventory must never disturb another's: a source
// phase that reads several sources replaces each one's stored stages on its
// own, so a source that failed to read this run keeps what it last had.
func TestStoreTrustedInventoryIsScopedToItsProvider(t *testing.T) {
	const secondProvider route.Provider = "second-provider"
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "content-hash")
	second, err := route.NewRoute(
		secondProvider, 1, 1, "revision", "Route", "",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		"content-hash",
	)
	require.NoError(t, err, "NewRoute()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first}), "StoreTrustedInventory() first provider")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), secondProvider, []route.Route{second}), "StoreTrustedInventory() second provider")

	firstCount, err := store.TrustedInventoryCount(t.Context(), route.ProviderVeloPlanner)
	require.NoError(t, err, "TrustedInventoryCount() first provider")
	assert.Equal(t, 1, firstCount, "first provider count")
	secondCount, err := store.TrustedInventoryCount(t.Context(), secondProvider)
	require.NoError(t, err, "TrustedInventoryCount() second provider")
	assert.Equal(t, 1, secondCount, "second provider count")

	stages, err := store.TrustedInventory(t.Context())
	require.NoError(t, err, "TrustedInventory()")
	assert.ElementsMatch(t, []route.Key{first.Key(), second.Key()}, []route.Key{stages[0].Key(), stages[1].Key()}, "union of both providers")

	// Replacing the first provider's inventory must not touch the second's.
	replacement := storeTestStage(t, 2, 1, "replacement", "replacement-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{replacement}), "StoreTrustedInventory() replacing first provider")

	secondCountAfter, err := store.TrustedInventoryCount(t.Context(), secondProvider)
	require.NoError(t, err, "TrustedInventoryCount() second provider after replacement")
	assert.Equal(t, 1, secondCountAfter, "second provider count is unaffected by the first provider's replacement")
	_, _, _, secondGeometryFound, err := store.StageGeometry(t.Context(), secondProvider, 1, 1)
	require.NoError(t, err, "StageGeometry() second provider")
	assert.True(t, secondGeometryFound, "the second provider's geometry cache must survive the first provider's replacement")
}

// A stage claiming a provider other than the one it is being stored under
// would let one source's write corrupt another's scoped share.
func TestStoreRefusesATrustedInventoryStageUnderTheWrongProvider(t *testing.T) {
	const secondProvider route.Provider = "second-provider"
	store := openTestStore(t, testKey(1))
	mismatched := storeTestStage(t, 1, 1, "revision", "content-hash")

	err := store.StoreTrustedInventory(t.Context(), secondProvider, []route.Route{mismatched})
	require.Error(t, err, "StoreTrustedInventory() with a stage under the wrong provider")
}

// A partial library reads as a library whose missing stages should be deleted,
// so a stage without geometry for its current hash fails the whole read.
func TestStoreRefusesATrustedInventoryMissingGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(), "DELETE FROM stage_geometry")
	require.NoError(t, err, "clearing geometry cache")

	_, inventoryErr := store.TrustedInventory(t.Context())
	require.Error(t, inventoryErr, "TrustedInventory() described a library it could not read whole")
}
