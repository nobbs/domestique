package osmindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// Null Island is a real place, so a node there has to survive the pass that
// drops nodes the extract never supplied. The two cases are only
// distinguishable because an unresolved slot carries a sentinel rather than a
// zero, and a way reduced to one point by a wrongly dropped node vanishes
// entirely.
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
