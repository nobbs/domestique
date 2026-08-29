package osmindex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// Null Island is a real place, so a node there has to survive the pass that drops
// nodes the extract never supplied. The two cases are distinguishable only
// because an unresolved slot carries a sentinel rather than a zero.
func TestPackWaysKeepsANodeAtTheOrigin(t *testing.T) {
	path := IndexPath(t.TempDir(), "aaaaaaaaaaaa")
	writer, err := newCellWriter(t.Context(), path)
	require.NoError(t, err, "newCellWriter()")

	// Node 1 sits exactly on the origin; node 2 is a little north-east of it.
	nodeIDs := []int64{1, 2}
	latitude := []int32{0, 10_000}
	longitude := []int32{0, 10_000}
	ways := []wayRecord{{id: 7, refStart: 0, refEnd: 2, kind: surface.KindAsphalt}}

	require.NoError(t, packWays(t.Context(), ways, []int64{1, 2}, nodeIDs, latitude, longitude, writer), "packWays()")
	require.NoError(t, writer.flush(t.Context()), "flush()")
	require.NoError(t, writer.writeMetadata(t.Context(), "aaaaaaaaaaaa", []string{"africa/ghana"}), "writeMetadata()")
	require.NoError(t, writer.finish(t.Context()), "finish()")

	index, err := Open(t.Context(), path)
	require.NoError(t, err, "Open()")
	t.Cleanup(func() { assert.NoError(t, index.Close(), "Close()") })

	ways2, err := index.Ways(t.Context(), []route.Point{{Longitude: 0, Latitude: 0}})
	require.NoError(t, err, "Ways()")
	require.Len(t, ways2, 1, "ways at the origin")
	assert.Equal(t, int64(7), ways2[0].ID, "way identifier")
}

// A way the region border cut in half keeps the part that lies inside, and one
// left with a single point is not a line and is dropped.
func TestPackWaysDropsTheNodesTheExtractNeverSupplied(t *testing.T) {
	path := IndexPath(t.TempDir(), "aaaaaaaaaaaa")
	writer, err := newCellWriter(t.Context(), path)
	require.NoError(t, err, "newCellWriter()")

	// Only node 1 was resolved; nodes 2 and 3 lie beyond the region border.
	nodeIDs := []int64{1, 2, 3}
	latitude := []int32{499_010_000, coordinateMissing, coordinateMissing}
	longitude := []int32{83_010_000, coordinateMissing, coordinateMissing}
	ways := []wayRecord{{id: 9, refStart: 0, refEnd: 3, kind: surface.KindAsphalt}}

	require.NoError(t, packWays(t.Context(), ways, []int64{1, 2, 3}, nodeIDs, latitude, longitude, writer), "packWays()")
	require.NoError(t, writer.flush(t.Context()), "flush()")
	require.NoError(t, writer.writeMetadata(t.Context(), "aaaaaaaaaaaa", []string{"europe/germany/rheinland-pfalz"}), "writeMetadata()")
	require.NoError(t, writer.finish(t.Context()), "finish()")

	index, err := Open(t.Context(), path)
	require.NoError(t, err, "Open()")
	t.Cleanup(func() { assert.NoError(t, index.Close(), "Close()") })

	ways2, err := index.Ways(t.Context(), []route.Point{{Longitude: 8.301, Latitude: 49.901}})
	require.NoError(t, err, "Ways()")
	assert.Empty(t, ways2, "a way left with a single point is not a line")
}

// The way pass answers the question the Overpass query used to: everything that
// could carry a rideable surface, and nothing that could not.
func TestScanWaysKeepsOnlyWhatCanCarryASurface(t *testing.T) {
	path := writeExtract(t, testExtract(t))

	ways, references, err := scanWays(t.Context(), path)
	require.NoError(t, err, "scanWays()")

	kinds := map[int64]surface.Kind{}
	for _, way := range ways {
		kinds[way.id] = way.kind
		assert.Less(t, way.refStart, way.refEnd, "way %d spans no references", way.id)
	}
	assert.Equal(t, map[int64]surface.Kind{
		10: surface.KindAsphalt,
		11: surface.KindGround,
		12: surface.KindUnknown,
	}, kinds, "the ways a surface can be read from")
	assert.Len(t, references, 7, "references of the kept ways, in one shared slice")
}

func TestCandidateWayRejectsWhatNobodyRides(t *testing.T) {
	assert.True(t, candidateWay(map[string]string{"highway": "residential"}), "a road")
	assert.True(t, candidateWay(map[string]string{"highway": "path"}), "a path")
	assert.False(t, candidateWay(map[string]string{"building": "yes"}), "an untagged-for-highway feature")
	assert.False(t, candidateWay(map[string]string{"highway": "pedestrian", "area": "yes"}), "a square")
	for _, highway := range []string{"proposed", "construction", "platform", "elevator", "raceway"} {
		assert.False(t, candidateWay(map[string]string{"highway": highway}), "highway=%s", highway)
	}
}

// The node pass resolves the identifiers the way pass asked for and nothing
// else, which is what keeps a file of tens of millions of nodes affordable.
func TestScanNodesResolvesOnlyWhatTheWaysReferenced(t *testing.T) {
	path := writeExtract(t, testExtract(t))

	// Node 3 is in the extract; node 50 is not, standing for one cut off at a
	// region border. Node 99 is in the extract but referenced by nothing.
	latitude, longitude, err := scanNodes(t.Context(), path, []int64{1, 3, 50})
	require.NoError(t, err, "scanNodes()")

	assert.Equal(t, int32(499_010_000), latitude[0], "node 1 latitude")
	assert.Equal(t, int32(83_010_000), longitude[0], "node 1 longitude")
	assert.Equal(t, int32(499_030_000), latitude[1], "node 3 latitude")
	assert.Equal(t, int32(coordinateMissing), latitude[2], "a node the extract does not contain")
}

// The whole chain, from the published checksum to a file that answers a query.
func TestBuildProducesAnIndexTheRegionsCover(t *testing.T) {
	extract := testExtract(t)
	digest := digestOf(extract)
	server := extractServer(t, map[string]string{
		"/europe/germany/rheinland-pfalz-latest.osm.pbf":     string(extract),
		"/europe/germany/rheinland-pfalz-latest.osm.pbf.md5": digest + "  rheinland-pfalz-latest.osm.pbf\n",
	})

	directory := t.TempDir()
	options := Options{
		Client:    server.Client(),
		Directory: directory,
		BaseURL:   server.URL,
		Regions:   []string{"europe/germany/rheinland-pfalz"},
	}

	result, err := Build(t.Context(), options, "")
	require.NoError(t, err, "Build()")
	require.False(t, result.Unchanged, "a first build is not unchanged")
	assert.Equal(t, IndexPath(directory, result.Generation), result.Path, "the built file")

	index, err := Open(t.Context(), result.Path)
	require.NoError(t, err, "Open()")
	t.Cleanup(func() { assert.NoError(t, index.Close(), "Close()") })

	ways, err := index.Ways(t.Context(), []route.Point{{Longitude: 8.3015, Latitude: 49.9015}})
	require.NoError(t, err, "Ways()")

	kinds := map[int64]surface.Kind{}
	for _, way := range ways {
		kinds[way.ID] = way.Kind
	}
	assert.Equal(t, map[int64]surface.Kind{
		10: surface.KindAsphalt,
		11: surface.KindGround,
		12: surface.KindUnknown,
	}, kinds, "the ways the index serves")

	// The extract is the largest thing on disk and is of no use once packed.
	// Asserting on the whole directory rather than one composed name keeps this
	// honest if the staged file is ever named differently.
	entries, err := os.ReadDir(directory)
	require.NoError(t, err, "reading the index directory")
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{filepath.Base(result.Path)}, names, "what the build left behind")
}

// An extract nobody has republished produces the same generation, and the build
// stops before downloading anything.
func TestBuildStopsWhenNoExtractHasChanged(t *testing.T) {
	extract := testExtract(t)
	digest := digestOf(extract)
	server := extractServer(t, map[string]string{
		"/europe/germany/rheinland-pfalz-latest.osm.pbf.md5": digest + "  rheinland-pfalz-latest.osm.pbf\n",
	})
	options := Options{
		Client:    server.Client(),
		Directory: t.TempDir(),
		BaseURL:   server.URL,
		Regions:   []string{"europe/germany/rheinland-pfalz"},
	}

	first, err := Build(t.Context(), options, "")
	require.Error(t, err, "the extract itself is not served, so a real build cannot finish")

	// The generation is decided by the checksums alone, so it is known even
	// though that build failed.
	generation := generationOf(map[string]string{"europe/germany/rheinland-pfalz": digest})
	assert.Empty(t, first.Generation, "a failed build reports no generation")

	second, err := Build(t.Context(), options, generation)
	require.NoError(t, err, "Build()")
	assert.True(t, second.Unchanged, "nothing upstream moved")
	assert.Equal(t, generation, second.Generation, "the generation already held")
}

func TestBuildRefusesAConfigurationItCannotAct(t *testing.T) {
	_, err := Build(t.Context(), Options{Directory: t.TempDir()}, "")
	require.Error(t, err, "no regions configured")

	_, err = Build(t.Context(), Options{Directory: t.TempDir(), Regions: []string{"../etc"}}, "")
	require.Error(t, err, "a region that is not a slug")
}

func writeExtract(t *testing.T, body []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "extract.osm.pbf")
	require.NoError(t, os.WriteFile(path, body, 0o600), "writing the extract")

	return path
}
