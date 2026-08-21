package osmindex

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

func TestIndexServesTheWaysNearAStage(t *testing.T) {
	directory := t.TempDir()
	path := writeTestIndex(t, directory, "aaaaaaaaaaaa")

	index, err := Open(t.Context(), path)
	require.NoError(t, err, "Open()")
	t.Cleanup(func() { assert.NoError(t, index.Close(), "Close()") })

	metadata := index.Metadata()
	assert.Equal(t, "aaaaaaaaaaaa", metadata.Generation, "Metadata().Generation")
	assert.Equal(t, []string{"europe/germany/rheinland-pfalz"}, metadata.Regions, "Metadata().Regions")
	assert.False(t, metadata.BuiltAt.IsZero(), "Metadata().BuiltAt")

	ways, err := index.Ways(t.Context(), []route.Point{{Longitude: 8.3015, Latitude: 49.9015}})
	require.NoError(t, err, "Ways()")
	require.Len(t, ways, 1, "ways near the stage")
	assert.Equal(t, int64(42), ways[0].ID, "way identifier")
	assert.Equal(t, surface.KindGravel, ways[0].Kind, "way class")
}

// Geometry outside every region simply has no candidates, which is what leaves a
// stage unclassified rather than wrongly classified.
func TestIndexAnswersNothingOutsideTheRegionsItHolds(t *testing.T) {
	index, err := Open(t.Context(), writeTestIndex(t, t.TempDir(), "aaaaaaaaaaaa"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, index.Close()) })

	ways, err := index.Ways(t.Context(), []route.Point{{Longitude: -73.98, Latitude: 40.75}})
	require.NoError(t, err, "Ways()")
	assert.Empty(t, ways, "ways for geometry no configured region covers")
}

// One ring either side is what makes a point just inside a cell boundary see the
// candidates in the cell next door.
func TestCellsForExpandsByOneRingAndDeduplicates(t *testing.T) {
	single := cellsFor([]route.Point{{Longitude: 8.3015, Latitude: 49.9015}})
	assert.Len(t, single, 9, "cells around one point")

	pair := cellsFor([]route.Point{
		{Longitude: 8.3015, Latitude: 49.9015},
		{Longitude: 8.3016, Latitude: 49.9016},
	})
	assert.Len(t, pair, 9, "two points in the same cell name the same nine cells once")
}

// A restart serves classifications from the last build's file, so the reopen has
// to be a normal start rather than a recovery.
func TestLoadReopensTheLastBuild(t *testing.T) {
	directory := t.TempDir()
	writeTestIndex(t, directory, "bbbbbbbbbbbb")

	index, found, err := Load(t.Context(), directory, "bbbbbbbbbbbb")
	require.NoError(t, err, "Load()")
	require.True(t, found, "Load() found no index")
	t.Cleanup(func() { assert.NoError(t, index.Close()) })
	assert.Equal(t, "bbbbbbbbbbbb", index.Metadata().Generation, "Metadata().Generation")
}

// A build the state database remembers whose file did not survive is the
// ordinary state of a host that lost its disk, not a failure to report: the
// service starts without classifications and the next build fills them in.
func TestLoadReportsAMissingFileAsNoIndex(t *testing.T) {
	index, found, err := Load(t.Context(), t.TempDir(), "cccccccccccc")
	require.NoError(t, err, "Load() for a generation with no file")
	assert.False(t, found, "Load() invented an index")
	assert.Nil(t, index, "Load() invented an index")

	index, found, err = Load(t.Context(), t.TempDir(), "")
	require.NoError(t, err, "Load() for a service that has never built")
	assert.False(t, found, "Load() invented an index")
	assert.Nil(t, index, "Load() invented an index")
}

// Until the first build lands there is nothing to read, and saying so is what
// keeps the annotator from recording every stage as unsurveyed.
func TestCurrentReportsNoIndexBeforeTheFirstBuild(t *testing.T) {
	current := NewCurrent()

	assert.Empty(t, current.Generation(), "Generation() before the first build")
	_, ok := current.Metadata()
	assert.False(t, ok, "Metadata() before the first build")

	ways, err := current.Ways(t.Context(), []route.Point{{Longitude: 8.3015, Latitude: 49.9015}})
	require.NoError(t, err, "Ways() before the first build")
	assert.Empty(t, ways, "ways before the first build")

	require.NoError(t, current.Close(), "Close() before the first build")
}

// The retired file is hundreds of megabytes on a host with a few gigabytes free,
// so a swap that left it behind would fill the disk within a month.
func TestSwapRetiresTheFileItReplaces(t *testing.T) {
	directory := t.TempDir()
	firstPath := writeTestIndex(t, directory, "aaaaaaaaaaaa")
	secondPath := writeTestIndex(t, directory, "dddddddddddd")

	current := NewCurrent()
	first, err := Open(t.Context(), firstPath)
	require.NoError(t, err)
	current.Swap(first)

	second, err := Open(t.Context(), secondPath)
	require.NoError(t, err)
	current.Swap(second)

	assert.Equal(t, "dddddddddddd", current.Generation(), "Generation() after the swap")
	assert.NoFileExists(t, firstPath, "the replaced index was left on disk")
	assert.FileExists(t, secondPath, "the installed index was removed")

	// Shutdown keeps the file: it is the next start's warm cache.
	require.NoError(t, current.Close(), "Close()")
	assert.FileExists(t, secondPath, "closing deleted the file the next start reopens")
}

// The runner announces the current generation on an unchanged build, so
// re-installing what is already live has to be free rather than destructive.
func TestSwapIgnoresTheIndexAlreadyLive(t *testing.T) {
	directory := t.TempDir()
	path := writeTestIndex(t, directory, "aaaaaaaaaaaa")

	current := NewCurrent()
	index, err := Open(t.Context(), path)
	require.NoError(t, err)
	current.Swap(index)
	current.Swap(index)

	assert.Equal(t, "aaaaaaaaaaaa", current.Generation(), "Generation()")
	assert.FileExists(t, path, "re-installing the live index deleted its own file")
	require.NoError(t, current.Close())
}

func TestIndexPathNamesAFileForItsGeneration(t *testing.T) {
	assert.Equal(t,
		"/var/lib/domestique/surface-aaaaaaaaaaaa.sqlite",
		IndexPath("/var/lib/domestique", "aaaaaaaaaaaa"),
	)
}

// An index written at a precision this build cannot interpret is refused rather
// than read as though it were current.
func TestOpenRejectsAnIndexFromAnotherFormat(t *testing.T) {
	path := writeTestIndex(t, t.TempDir(), "aaaaaaaaaaaa")
	writeMeta(t, path, "coordinate_scale", "100000")

	_, err := Open(t.Context(), path)
	require.Error(t, err, "Open() accepted an index at another precision")
	assert.Contains(t, err.Error(), "this build reads")
}

// writeTestIndex writes a small but real index: the same writer the builder
// uses, the same packed format, the same metadata. Only the OpenStreetMap decode
// is left out, which is what the fixture-gated verification harness covers.
func writeTestIndex(t *testing.T, directory, generation string) string {
	t.Helper()

	path := IndexPath(directory, generation)
	writer, err := newCellWriter(t.Context(), path)
	require.NoError(t, err, "newCellWriter()")

	x, y := absolute(t, [][2]float64{{8.301, 49.901}, {8.302, 49.902}, {8.303, 49.903}})
	writer.add(42, surface.KindGravel, x, y)
	require.NoError(t, writer.flush(t.Context()), "flush()")
	require.NoError(t, writer.writeMetadata(t.Context(), generation, []string{"europe/germany/rheinland-pfalz"}), "writeMetadata()")
	require.NoError(t, writer.finish(t.Context()), "finish()")

	return path
}

func writeMeta(t *testing.T, path, key, value string) {
	t.Helper()

	database, err := sql.Open(driverName, path)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	require.NoError(t, err, "rewriting index metadata")
	require.NoError(t, database.Close())
}
