package sqlite

import (
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A classification measured against a shape the stage no longer has describes a
// line the map cannot draw, so it does not count as classified.
func TestStoreCountsOnlyClassificationsOfTheCurrentGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	geometry := []route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.5, Latitude: 49.2}}
	first := storeTestStageWithGeometry(t, 7, 1, "revision", "hash-a", "Alpine loop", "Descent", geometry)
	second := storeTestStageWithGeometry(t, 8, 1, "revision", "hash-b", "Coast road", "Return", geometry)
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first, second}), "StoreTrustedInventory()")

	classified, total, err := store.SurfaceCoverage(t.Context())
	require.NoError(t, err, "SurfaceCoverage()")
	require.Zero(t, classified, "a stage counted as classified before anything was")
	require.Equal(t, 2, total, "total stages")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 7, 1, "hash-a", "index-gen", []byte(`[{"kind":"asphalt","startIndex":0,"endIndex":1}]`), 100,
	), "StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 8, 1, "an-earlier-shape", "index-gen", []byte(`[{"kind":"gravel","startIndex":0,"endIndex":1}]`), 100,
	), "StoreStageSurface()")

	classified, total, err = store.SurfaceCoverage(t.Context())
	require.NoError(t, err, "SurfaceCoverage()")
	// The stale classification does not count towards the coverage.
	assert.Equal(t, 1, classified, "classified stages")
	assert.Equal(t, 2, total, "total stages")
}

func TestStoreCachesStageSurfaceAgainstTheGeometryItDescribes(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 7, 2, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")

	_, _, found, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageSurfaceHash() before enrichment")
	assert.False(t, found, "a surface hash was stored before enrichment")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash", "index-gen", []byte(testSurfaceRanges), 1234.5,
	), "StoreStageSurface()")

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash")
	require.NoError(t, err, "StageSurface()")
	require.True(t, found, "StageSurface() did not find the stored surface")
	// Byte-identical, because the endpoint serves the stored ranges as they are.
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")
	assert.InDelta(t, 1234.5, matchedMetres, 0.001, "matchedMetres")

	hash, generation, found, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, found, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "content-hash", hash, "StageSurfaceHash()")
	assert.Equal(t, "index-gen", generation, "StageSurfaceHash() generation")
}

func TestStoreHidesASurfaceMeasuredAgainstOtherGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "current-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "earlier-hash", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	// The ranges index the coordinates of the geometry they were measured
	// against, so beside a re-planned stage they are absent, never approximate.
	_, _, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "current-hash")
	require.NoError(t, err, "StageSurface() for other geometry")
	assert.False(t, found, "ranges measured against replaced geometry were served for it")
	// The hash is still readable, which is how the enrichment pass knows the
	// stage needs asking about again.
	hash, _, hashFound, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, hashFound, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "earlier-hash", hash, "StageSurfaceHash()")
}

// The mirror of the test above, and the opposite answer. A stale generation is
// not a stale geometry: those ranges still index the coordinates the stage has,
// so they are old rather than wrong. One row per stage, so withholding it would
// blank the library after every rebuild. The next pass corrects it.
func TestStoreKeepsASurfaceMeasuredAgainstAnEarlierIndex(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash", "earlier-generation", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	ranges, _, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash")
	require.NoError(t, err, "StageSurface() after a rebuild")
	require.True(t, found, "a stage was blanked because the index had been rebuilt")
	// Byte-identical, as everywhere here: the endpoint serves the stored ranges
	// as they are rather than re-encoding them.
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")

	// What the enrichment pass compares against the live generation, and so what
	// makes the stage be measured again rather than left as it is forever.
	_, generation, hashFound, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, hashFound, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "earlier-generation", generation, "the generation the row was measured against")
}

func TestStoreReplacesAStageSurfaceRatherThanAccumulatingOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "second-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "first-hash", "index-gen", []byte(`[]`), 1), "first StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "second-hash", "index-gen", []byte(testSurfaceRanges), 2,
	), "second StoreStageSurface()")

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "second-hash")
	require.NoError(t, err, "StageSurface()")
	require.True(t, found, "StageSurface() did not find the stored surface")
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")
	assert.InDelta(t, 2.0, matchedMetres, 0.001, "matchedMetres")
	assert.Equal(t, 1, countStageSurfaceRows(t, store), "stage_surface rows for one stage")
}

func TestStorePrunesSurfaceForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first, second}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 2, 1, "hash-two", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{first}), "second StoreTrustedInventory()")

	_, _, removedFound, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, err, "StageSurfaceHash() for a removed stage")
	assert.False(t, removedFound, "the surface hash of a removed stage is still stored")

	_, _, retainedFound, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one")
	require.NoError(t, err, "StageSurface() for a retained stage")
	assert.True(t, retainedFound, "the surface of a retained stage was dropped")
}

func TestStorePrunesSurfaceMeasuredAgainstReplacedGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash-one")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	replanned := storeTestStage(t, 1, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Route{replanned}), "second StoreTrustedInventory()")

	// The row goes rather than lingering as something to be matched around: the
	// coordinate array its ranges address has been replaced.
	assert.Zero(t, countStageSurfaceRows(t, store), "stage_surface rows after re-planning")
}

// testSurfaceRanges is one stored classification, in the shape the annotator
// writes and the geometry endpoint serves.
const testSurfaceRanges = `[{"kind":"asphalt","startIndex":0,"endIndex":1}]`

func countStageSurfaceRows(t *testing.T, store *Store) int {
	t.Helper()

	var rows int
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM stage_surface`,
	).Scan(&rows), "counting stage_surface rows")

	return rows
}
