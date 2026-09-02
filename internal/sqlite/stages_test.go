package sqlite

import (
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reprocess removes the three answers the service would otherwise reuse, and
// keeps the one that says which Wahoo route it already owns.
func TestStoreReprocessesOneStageWithoutLosingItsRouteIdentity(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 7, 2, "revision", "encoded-hash", 4242), "UpsertTargetStage()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash", "index-gen", []byte(`[{"kind":"asphalt","startIndex":0,"endIndex":1}]`), 100,
	), "StoreStageSurface()")

	found, err := store.RequestStageReprocess(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "RequestStageReprocess()")
	require.True(t, found, "RequestStageReprocess() did not find the stored stage")

	var revision, contentHash string
	var wahooRouteID int64
	require.NoError(t, store.database.QueryRowContext(t.Context(), `
		SELECT source_revision, content_hash, wahoo_route_id FROM target_stages
		WHERE target_slot = 'rider-a' AND route_id = 7 AND stage_order = 2
	`).Scan(&revision, &contentHash, &wahooRouteID), "reading the target mapping")
	// Forgotten, but still a usable row: the reconciler rejects a mapping with an
	// empty field outright, which would fail the whole target phase instead of
	// rewriting this one route.
	assert.NotEmpty(t, revision, "the mapping's revision is not a value the reconciler can read")
	assert.NotEmpty(t, contentHash, "the mapping's content hash is not a value the reconciler can read")
	assert.NotEqual(t, "revision", revision, "the mapping still claims the revision it pushed")
	assert.NotEqual(t, "encoded-hash", contentHash, "the mapping still claims the content it pushed")
	// A reprocess rewrites the owned route; it never recreates it.
	assert.Equal(t, int64(4242), wahooRouteID, "wahoo route id")
	_, _, surfaceFound, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash")
	require.NoError(t, err, "StageSurface()")
	assert.False(t, surfaceFound, "the surface survived a reprocess instead of being asked for again")
}

// The geometry cache skips a stage whose content has not changed. A reprocess is
// the operator saying the derivation itself should be redone.
func TestStoreRewritesGeometryOfAReprocessedStage(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(), `
		UPDATE stage_geometry SET route_name = 'stale name' WHERE route_id = 7 AND stage_order = 2
	`)
	require.NoError(t, err, "ageing the cached geometry")

	// Without a request, an unchanged stage is left alone.
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	summary, _, _, _, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageGeometry()")
	// An unchanged stage is not rewritten, so the aged name is still there.
	require.Equal(t, "stale name", summary.SourceRouteName, "route name")

	_, requestErr := store.RequestStageReprocess(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, requestErr, "RequestStageReprocess()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory() after request")
	summary, _, _, _, err = store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageGeometry()")
	assert.Equal(t, "Alpine loop", summary.SourceRouteName, "the request did not rewrite the route name")

	// The mark is consumed, so the next pass leaves the stage alone again.
	var marks int
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM stage_reprocess").Scan(&marks), "counting reprocess requests")
	assert.Zero(t, marks, "the pass that honoured the request left it behind")
}

// A stage that is not in the inventory cannot be redone, and a mark nothing will
// consume is worse than an answer.
func TestStoreRefusesToReprocessAnUnknownStage(t *testing.T) {
	store := openTestStore(t, testKey(1))

	found, err := store.RequestStageReprocess(t.Context(), route.ProviderVeloPlanner, 99, 1)
	require.NoError(t, err, "RequestStageReprocess()")
	assert.False(t, found, "a stage that is not stored was marked for reprocessing")
}

func TestStoreCachesStageGeometryForTheMapView(t *testing.T) {
	store := openTestStore(t, testKey(1))
	elevation := 128.5
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	summary, coordinates, _, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.Equal(t, "Alpine loop — Descent", summary.Title(), "Title()")
	assert.Equal(t, 2, summary.PointCount, "PointCount")
	assert.Positive(t, summary.DistanceMetres, "DistanceMetres")
	wantBounds := route.Bounds{MinLongitude: 8.4, MinLatitude: 49.0, MaxLongitude: 8.5, MaxLatitude: 49.2}
	assert.Equal(t, wantBounds, summary.Bounds, "Bounds")
	assert.Equal(t, `[[8.4,49,128.5],[8.5,49.2]]`, string(coordinates), "coordinates")
}

func TestStoreCachesElevationStatistics(t *testing.T) {
	store := openTestStore(t, testKey(1))
	// A climb of 40 m followed by a descent of 15 m, so ascent and descent are
	// distinct — not just mirrors of each other — over roughly 500 m of northing.
	elevations := []float64{100, 110, 120, 130, 140, 125}
	geometry := make([]route.Point, 0, len(elevations))
	for index, elevation := range elevations {
		geometry = append(geometry, route.Point{
			Longitude: 8.4,
			Latitude:  49.0 + float64(index)*0.0009,
			Elevation: &elevation,
		})
	}
	stage := storeTestStageWithGeometry(t, 5, 1, "revision", "hash", "Climb", "", geometry)
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	summary, _, _, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 5, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.InDelta(t, 40.0, summary.AscentMetres, 0.001, "AscentMetres")
	assert.InDelta(t, 15.0, summary.DescentMetres, 0.001, "DescentMetres")
	assert.Positive(t, summary.MaxGradientPercent, "MaxGradientPercent")

	var listed route.Summary
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		listed = summary

		return nil
	}), "ForEachStageSummary()")
	assert.InDelta(t, summary.AscentMetres, listed.AscentMetres, 0.001, "listed AscentMetres")
	assert.InDelta(t, summary.DescentMetres, listed.DescentMetres, 0.001, "listed DescentMetres")
}

// A stage cached before the statistics existed must still be readable; the
// columns default to zero until a content change refills them.
func TestStoreReadsGeometryCachedBeforeElevationStatistics(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET ascent_metres = 0, descent_metres = 0, max_gradient_percent = 0`)
	require.NoError(t, err, "clearing statistics")

	summary, _, _, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.Zero(t, summary.AscentMetres, "AscentMetres")
	assert.Zero(t, summary.DescentMetres, "DescentMetres")
	assert.Zero(t, summary.MaxGradientPercent, "MaxGradientPercent")
}

func TestStoreDoesNotRewriteUnchangedStageGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	// A sentinel that a rewrite would necessarily overwrite. This is the
	// write-amplification guarantee: an unchanged library must not rewrite the
	// geometry cache on every scheduled run.
	const sentinel = 1
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET updated_at_unix = ?`, sentinel)
	require.NoError(t, err, "seeding sentinel")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "second StoreTrustedInventory()")
	assert.EqualValues(t, sentinel, stageGeometryUpdatedAt(t, store, 1, 1),
		"an unchanged sync rewrote the geometry cache")

	changed := storeTestStage(t, 1, 1, "revision", "different-content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{changed}), "changed StoreTrustedInventory()")
	assert.NotEqualValues(t, sentinel, stageGeometryUpdatedAt(t, store, 1, 1),
		"a changed content hash left the geometry cache untouched")
}

func TestStorePrunesGeometryForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first, second}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first}), "second StoreTrustedInventory()")

	_, _, _, removedFound, removedErr := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, removedErr, "StageGeometry() for a removed stage")
	assert.False(t, removedFound, "the geometry of a removed stage is still stored")

	_, _, _, retainedFound, retainedErr := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, retainedErr, "StageGeometry() for a retained stage")
	assert.True(t, retainedFound, "the geometry of a retained stage was dropped")
}

func TestStoreListsStageSummaries(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 3, 1, "revision", "hash", "Sunday", "", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.1},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	var summaries []route.Summary
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		summaries = append(summaries, summary)

		return nil
	}), "ForEachStageSummary()")
	require.Len(t, summaries, 1, "listed summaries")
	assert.Equal(t, "Sunday", summaries[0].Title(), "Title()")
	assert.Equal(t, "revision", summaries[0].SourceRevision, "SourceRevision")
	assert.Equal(t, 2, summaries[0].PointCount, "PointCount")
}

func TestStoreForEachStageSummaryReportsAPredictedMovingTime(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	movingSeconds := 555.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	var found *route.Summary
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		found = &summary

		return nil
	}), "ForEachStageSummary()")
	require.NotNil(t, found, "ForEachStageSummary() visited no stage")
	require.NotNil(t, found.MovingSeconds, "ForEachStageSummary() moving seconds")
	assert.InDelta(t, 555.0, *found.MovingSeconds, 0.001, "moving seconds")
}

func stageGeometryUpdatedAt(t *testing.T, store *Store, routeID int64, stageOrder int) int64 {
	t.Helper()

	var updatedAt int64
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT updated_at_unix FROM stage_geometry WHERE route_id = ? AND stage_order = ?`,
		routeID, stageOrder,
	).Scan(&updatedAt), "reading updated_at_unix")

	return updatedAt
}
