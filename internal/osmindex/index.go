package osmindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver registration.

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

const driverName = "sqlite"

// cellRing is how many cells either side of a stage point are read.
//
// One ring is enough for any query radius shorter than a cell, which at 0.01°
// means anything under a kilometre; the snap radius is 25 m. It exists at all
// because a point sitting just inside a cell boundary has candidates in the
// neighbouring cell, and reading nine small blobs costs less than reasoning
// about which of them the corridor actually touches.
const cellRing = 1

// Index reads a built surface index. It is immutable once opened and safe for
// concurrent use; replacing one is the job of Current.
type Index struct {
	database *sql.DB
	lookup   *sql.Stmt
	path     string
	metadata Metadata
}

// Metadata describes the build a file came from. It is stored in the file rather
// than alongside it so an index can always say what it is, including after it
// has been copied somewhere its configuration did not follow.
type Metadata struct {
	BuiltAt    time.Time
	Generation string
	Regions    []string
}

// indexPrefix and indexSuffix bracket a generation in an index filename. The
// generation is in the name so a build can be written and opened beside the
// index still serving requests, and so the file itself says which build it is
// even to something that cannot read SQLite.
const (
	indexPrefix = "surface-"
	indexSuffix = ".sqlite"
)

// IndexPath is where the index for one generation lives.
func IndexPath(directory, generation string) string {
	return filepath.Join(directory, indexPrefix+generation+indexSuffix)
}

// Load opens the index for a generation, reporting a missing file as no index
// rather than as a failure.
//
// This is what makes a restart cheap: the last build's file is still on disk, so
// the service serves classifications from the moment it starts instead of going
// blind until the next scheduled build. A file that is missing or unreadable is
// not fatal — the next build replaces it.
func Load(ctx context.Context, directory, generation string) (index *Index, found bool, err error) {
	if generation == "" {
		return nil, false, nil
	}
	path := IndexPath(directory, generation)
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		return nil, false, nil
	}

	index, err = Open(ctx, path)
	if err != nil {
		return nil, false, err
	}

	return index, true, nil
}

// Open reads an existing index file.
func Open(ctx context.Context, path string) (*Index, error) {
	database, err := sql.Open(driverName, "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("osmindex: opening index: %w", err)
	}

	index := &Index{database: database, path: path}
	if metadataErr := index.readMetadata(ctx); metadataErr != nil {
		closeDatabase(database)

		return nil, metadataErr
	}

	lookup, err := database.PrepareContext(ctx, `SELECT ways FROM cell WHERE x = ? AND y = ?`)
	if err != nil {
		closeDatabase(database)

		return nil, fmt.Errorf("osmindex: preparing cell lookup: %w", err)
	}
	index.lookup = lookup

	return index, nil
}

// Metadata reports which build this index came from.
func (i *Index) Metadata() Metadata { return i.metadata }

// Path reports the file this index reads, which is what lets a swap delete the
// file it replaced.
func (i *Index) Path() string { return i.path }

// Close releases the file.
func (i *Index) Close() error {
	closeStatement(i.lookup)
	if err := i.database.Close(); err != nil {
		return fmt.Errorf("osmindex: closing index: %w", err)
	}

	return nil
}

// Ways implements surface.Source over the packed cells.
//
// Every cell the stage's corridor may reach is read and decoded whole. Decoding
// a cell the corridor only clips is wasted work, but a cell holds a kilometre of
// road network and the alternative is a second spatial test against geometry
// that surface.Match is about to test properly anyway.
func (i *Index) Ways(ctx context.Context, points []route.Point) ([]surface.Way, error) {
	ways := make([]surface.Way, 0, 1024)
	for _, key := range cellsFor(points) {
		var blob []byte
		switch err := i.lookup.QueryRowContext(ctx, key.x, key.y).Scan(&blob); {
		case errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return nil, fmt.Errorf("osmindex: reading cell %d/%d: %w", key.x, key.y, err)
		}

		decoded, err := decodeCell(blob, key)
		if err != nil {
			return nil, fmt.Errorf("osmindex: decoding cell %d/%d: %w", key.x, key.y, err)
		}
		ways = append(ways, decoded...)
	}

	return ways, nil
}

func (i *Index) readMetadata(ctx context.Context) error {
	rows, err := i.database.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return fmt.Errorf("osmindex: reading index metadata: %w", err)
	}
	defer closeRows(rows)

	values := make(map[string]string, 8)
	for rows.Next() {
		var key, value string
		if scanErr := rows.Scan(&key, &value); scanErr != nil {
			return fmt.Errorf("osmindex: reading index metadata: %w", scanErr)
		}
		values[key] = value
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return fmt.Errorf("osmindex: reading index metadata: %w", rowsErr)
	}

	fileCell, cellErr := parseFloat(values["cell_degrees"])
	fileScale, scaleErr := parseFloat(values["coordinate_scale"])
	if cellErr != nil || scaleErr != nil {
		return errors.New("osmindex: index metadata is missing its grid description")
	}
	if scaleErr := verifyScales(fileCell, fileScale); scaleErr != nil {
		return scaleErr
	}

	builtAt, err := time.Parse(time.RFC3339, values["built_at"])
	if err != nil {
		return fmt.Errorf("osmindex: index metadata has no usable build time: %w", err)
	}
	i.metadata = Metadata{
		Generation: values["generation"],
		Regions:    splitRegions(values["regions"]),
		BuiltAt:    builtAt.UTC(),
	}

	return nil
}

// cellsFor returns every cell a stage's corridor may reach, deduplicated.
func cellsFor(points []route.Point) []cellKey {
	seen := make(map[cellKey]struct{}, len(points))
	keys := make([]cellKey, 0, len(points))
	for _, point := range points {
		centre := cellKey{
			x: int32(math.Floor(point.Longitude / cellDegrees)),
			y: int32(math.Floor(point.Latitude / cellDegrees)),
		}
		for x := -cellRing; x <= cellRing; x++ {
			for y := -cellRing; y <= cellRing; y++ {
				key := cellKey{x: centre.x + int32(x), y: centre.y + int32(y)}
				if _, done := seen[key]; done {
					continue
				}
				seen[key] = struct{}{}
				keys = append(keys, key)
			}
		}
	}

	return keys
}

// Current holds whichever index is live and hands it to readers.
//
// A build produces a new file beside the old one and calls Swap. Readers hold
// the lock for the length of one Ways call — a fraction of a second against a
// local file — so a swap waits for work in flight rather than interrupting it,
// and the replaced file is closed and deleted only once nothing is reading it.
//
// A Current with no index yet is not an error. It reports that it has none, and
// classification is simply skipped until the first build lands.
type Current struct {
	index *Index
	mutex sync.RWMutex
}

// NewCurrent creates an empty holder.
func NewCurrent() *Current { return &Current{} }

// Ways implements surface.Source. It reports no ways at all when no index has
// been built yet, which leaves stages unclassified rather than failing a sync.
func (c *Current) Ways(ctx context.Context, points []route.Point) ([]surface.Way, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.index == nil {
		return nil, nil
	}

	return c.index.Ways(ctx, points)
}

// Metadata reports the live index's build, and false when there is none.
func (c *Current) Metadata() (Metadata, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.index == nil {
		return Metadata{}, false
	}

	return c.index.Metadata(), true
}

// Generation reports the live index's generation, empty when there is none. It
// is what the annotator keys its cache on.
func (c *Current) Generation() string {
	metadata, ok := c.Metadata()
	if !ok {
		return ""
	}

	return metadata.Generation
}

// Swap installs a new index and retires the one it replaces, deleting the
// retired file. Swapping in the index that is already live is a no-op, so a
// caller may re-announce the current generation without consequence.
func (c *Current) Swap(next *Index) {
	if next == nil {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	previous := c.index
	if previous != nil && previous.Path() == next.Path() {
		return
	}
	c.index = next

	if previous == nil {
		return
	}
	closeIndex(previous)
	removeFile(previous.Path())
}

// Close releases the live index without deleting its file, which is what a
// shutdown wants: the file is the next start's warm cache.
func (c *Current) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.index == nil {
		return nil
	}
	err := c.index.Close()
	c.index = nil

	return err
}

// The cleanup helpers below exist for the same reason as their counterparts in
// the state store: a failure to release something is not an answer to whatever
// the caller was actually doing, and each one names why in one place rather than
// at every call site.

//nolint:errcheck // A cleanup error cannot replace the already-returned result.
func closeDatabase(database *sql.DB) { _ = database.Close() }

//nolint:errcheck // A query cleanup error is superseded by the query result.
func closeRows(rows *sql.Rows) { _ = rows.Close() }

//nolint:errcheck // A statement that failed to close is released with its database.
func closeStatement(statement *sql.Stmt) {
	if statement == nil {
		return
	}
	_ = statement.Close()
}

//nolint:errcheck // A retired index is being discarded; its close cannot matter.
func closeIndex(index *Index) { _ = index.Close() }

//nolint:errcheck // A file that will not delete is reported by the next build's prune.
func removeFile(path string) { _ = os.Remove(path) }
