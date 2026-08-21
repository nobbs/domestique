package osmindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"

	"github.com/nobbs/domestique/internal/surface"
)

// flushThresholdBytes is how much packed output the builder holds before
// writing it out.
//
// Packed cells are appended to whatever the file already holds, so the builder
// never needs a whole region's output in memory at once and this can be set for
// comfort rather than for correctness. It is the one number that decides the
// builder's own footprint; everything else it holds is a function of the region
// being read.
const flushThresholdBytes = 48 << 20

// DefaultMemoryLimit is the soft heap ceiling a build runs under.
//
// It is a constant rather than a setting because it is not a preference: a build
// measured at roughly half a gigabyte of live heap is given headroom, and the
// host it runs on has a few gigabytes free. An operator who tightened this would
// only make the collector work harder for the same result, and one who loosened
// it would only find out on the day the host ran out.
const DefaultMemoryLimit int64 = 1 << 30

// Options configures one build.
type Options struct {
	// Client fetches extracts. Empty means a client with no timeout, since the
	// caller's context is what bounds a download.
	Client *http.Client
	// Directory is where indexes live and where extracts are staged. It must be
	// on real disk: an extract is hundreds of megabytes and the service's /tmp is
	// a tmpfs, which would put it in the memory this build is trying to bound.
	Directory string
	// BaseURL is the extract host. Empty means DefaultBaseURL.
	BaseURL string
	// Regions are Geofabrik slugs such as "europe/germany/rheinland-pfalz".
	Regions []string
	// MemoryLimit is a soft ceiling applied for the duration of the build and
	// lifted afterwards. Zero leaves the runtime's limit alone.
	//
	// It is applied here rather than through GOMEMLIMIT because the setting is
	// runtime-wide: an environment variable sized for the build would make the
	// HTTP service collect against the same ceiling for the rest of the
	// process's life, which is a cost paid continuously to bound something that
	// happens weekly.
	MemoryLimit int64
}

// Result describes what a build did.
type Result struct {
	// Path is the new index file. Empty when Unchanged is true.
	Path string
	// Generation identifies the build.
	Generation string
	// Unchanged reports that every region's published extract is byte-identical
	// to the one the current index was built from, so nothing was downloaded and
	// no file was written.
	Unchanged bool
}

// Build produces an index for the configured regions.
//
// It fetches each region's published checksum first and derives the generation
// from them, so a scheduled rebuild that finds nothing changed upstream costs
// one small request per region and returns without downloading anything. When
// the generation differs from the one given, the extracts are downloaded,
// verified, decoded, and packed into a new file named for the new generation.
//
// The caller owns the returned file: this function does not touch whatever index
// is currently live.
func Build(ctx context.Context, options Options, currentGeneration string) (Result, error) {
	for _, region := range options.Regions {
		if err := ValidateRegion(region); err != nil {
			return Result{}, fmt.Errorf("osmindex: %w", err)
		}
	}
	if len(options.Regions) == 0 {
		return Result{}, fmt.Errorf("osmindex: no regions configured")
	}

	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	client := options.Client
	if client == nil {
		client = &http.Client{}
	}

	checksums, err := fetchChecksums(ctx, client, baseURL, options.Regions)
	if err != nil {
		return Result{}, err
	}
	generation := generationOf(checksums)
	if generation == currentGeneration {
		return Result{Generation: generation, Unchanged: true}, nil
	}

	if options.MemoryLimit > 0 {
		previous := debug.SetMemoryLimit(options.MemoryLimit)
		defer debug.SetMemoryLimit(previous)
	}

	path := IndexPath(options.Directory, generation)
	if err := buildInto(ctx, options, baseURL, client, checksums, generation, path); err != nil {
		removeFile(path)

		return Result{}, err
	}

	return Result{Path: path, Generation: generation}, nil
}

// buildInto does the work, so that Build can clean up a partial file whatever
// went wrong.
func buildInto(
	ctx context.Context,
	options Options,
	baseURL string,
	client *http.Client,
	checksums map[string]string,
	generation, path string,
) error {
	removeFile(path)
	writer, err := newCellWriter(ctx, path)
	if err != nil {
		return err
	}
	defer writer.close()

	// Regions are read one at a time and their working memory released before
	// the next, so the peak is set by the largest single region rather than by
	// the total. Extracts overlap slightly at their borders, which stores a
	// handful of boundary ways twice; a duplicate is the same way with the same
	// identifier, geometry, and class, so the match treats the two as one
	// candidate and nothing needs to deduplicate them.
	for _, region := range options.Regions {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("osmindex: building index: %w", err)
		}
		if err := addRegion(ctx, options, baseURL, client, checksums[region], region, writer); err != nil {
			return err
		}
	}

	if err := writer.flush(ctx); err != nil {
		return err
	}
	if err := writer.writeMetadata(ctx, generation, options.Regions); err != nil {
		return err
	}

	return writer.finish(ctx)
}

// addRegion downloads one extract, packs it into the writer, and removes it.
func addRegion(
	ctx context.Context,
	options Options,
	baseURL string,
	client *http.Client,
	checksum, region string,
	writer *cellWriter,
) error {
	extractPath, err := downloadExtract(ctx, client, baseURL, region, checksum, options.Directory)
	if err != nil {
		return err
	}
	// The extract is the largest thing on disk and is of no use once packed.
	defer removeFile(extractPath)

	return indexExtract(ctx, extractPath, writer)
}

// indexExtract reads one extract in two passes and packs what it finds.
//
// The two passes are what keep this affordable. A single pass would have to
// hold every node in the file against the chance that some later way refers to
// it; reading the ways first means the node pass knows exactly which
// coordinates matter and can discard the rest as it goes.
func indexExtract(ctx context.Context, extractPath string, writer *cellWriter) error {
	ways, references, err := scanWays(ctx, extractPath)
	if err != nil {
		return err
	}

	nodeIDs := slices.Clone(references)
	slices.Sort(nodeIDs)
	nodeIDs = slices.Compact(nodeIDs)

	latitude, longitude, err := scanNodes(ctx, extractPath, nodeIDs)
	if err != nil {
		return err
	}

	return packWays(ctx, ways, references, nodeIDs, latitude, longitude, writer)
}

// wayRecord is one candidate way: what it is, and where its geometry is to be
// found in the shared reference slice. The references are held in one slice
// rather than per way because a slice header per way would cost more than the
// references themselves.
type wayRecord struct {
	id       int64
	refStart int32
	refEnd   int32
	kind     surface.Kind
}

// scanWays reads the way pass, keeping the ways that can carry a rideable
// surface and classifying each as it goes.
func scanWays(ctx context.Context, path string) (ways []wayRecord, references []int64, err error) {
	file, err := os.Open(path) //nolint:gosec // The path was composed by this package when it staged the extract.
	if err != nil {
		return nil, nil, fmt.Errorf("osmindex: opening extract: %w", err)
	}
	defer closeFile(file)

	scanner := osmpbf.New(ctx, file, decoderCount())
	scanner.SkipNodes = true
	scanner.SkipRelations = true
	defer closeScanner(scanner)

	ways = make([]wayRecord, 0, 1<<19)
	references = make([]int64, 0, 1<<22)
	tags := make(map[string]string, 32)
	for scanner.Scan() {
		way, ok := scanner.Object().(*osm.Way)
		if !ok || len(way.Nodes) < 2 {
			continue
		}
		clear(tags)
		for _, tag := range way.Tags {
			tags[tag.Key] = tag.Value
		}
		if !candidateWay(tags) {
			continue
		}

		// A way record addresses the shared reference slice with int32 offsets,
		// which is what keeps it small enough to hold millions of them. No
		// regional extract comes close to two billion references, but a builder
		// that silently wrapped would produce an index with the wrong geometry
		// rather than no index at all.
		if len(references)+len(way.Nodes) > math.MaxInt32 {
			return nil, nil, errors.New("osmindex: extract holds more references than this builder can address")
		}

		start := int32(len(references)) //nolint:gosec // Bounded by the check above.
		for _, node := range way.Nodes {
			references = append(references, int64(node.ID))
		}
		ways = append(ways, wayRecord{
			id:       int64(way.ID),
			refStart: start,
			refEnd:   int32(len(references)), //nolint:gosec // Bounded by the check above.
			kind:     surface.Classify(tags),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("osmindex: reading ways: %w", err)
	}

	return ways, references, nil
}

// candidateWay decides whether a way can supply a surface to a route.
//
// This is the filter the Overpass query used to carry, kept whole so the index
// answers the same question the endpoint did. Everything tagged highway is a
// candidate except the features nothing is ridden on — a bus platform, a lift, a
// road still under construction — and areas, which are places rather than ways
// and would snap a route to the middle of a square.
//
// Note that an unclassified way is still kept. Knowing that a road exists and
// carries no surface tag is the difference between honestly unsurveyed ground
// and ground nobody looked for.
func candidateWay(tags map[string]string) bool {
	highway := tags["highway"]
	if highway == "" || tags["area"] == "yes" {
		return false
	}
	switch highway {
	case "proposed", "construction", "platform", "elevator", "corridor",
		"raceway", "bus_guideway", "rest_area", "services":
		return false
	}

	return true
}

// scanNodes reads the node pass, resolving only the coordinates the kept ways
// refer to.
//
// The identifiers arrive sorted, so this is a binary search per node rather than
// a hash of every node in the extract. The result is two parallel slices in the
// same order as nodeIDs: a sorted table of exactly the coordinates that will be
// used, and nothing else from a file that holds tens of millions of them.
func scanNodes(ctx context.Context, path string, nodeIDs []int64) (latitude, longitude []int32, err error) {
	file, err := os.Open(path) //nolint:gosec // The path was composed by this package when it staged the extract.
	if err != nil {
		return nil, nil, fmt.Errorf("osmindex: opening extract: %w", err)
	}
	defer closeFile(file)

	scanner := osmpbf.New(ctx, file, decoderCount())
	scanner.SkipWays = true
	scanner.SkipRelations = true
	defer closeScanner(scanner)

	// Every slot starts unresolved rather than at zero, because zero is a real
	// place. A slot still holding the sentinel after this pass is a node the
	// extract referenced but does not contain.
	latitude = make([]int32, len(nodeIDs))
	longitude = make([]int32, len(nodeIDs))
	for index := range latitude {
		latitude[index], longitude[index] = coordinateMissing, coordinateMissing
	}
	for scanner.Scan() {
		node, ok := scanner.Object().(*osm.Node)
		if !ok {
			continue
		}
		index, hit := slices.BinarySearch(nodeIDs, int64(node.ID))
		if !hit {
			continue
		}
		// Latitude and longitude are bounded by the globe, so at 1e7 they fit an
		// int32 with an order of magnitude to spare.
		latitude[index] = int32(math.Round(node.Lat * nativeScale))
		longitude[index] = int32(math.Round(node.Lon * nativeScale))
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("osmindex: reading nodes: %w", err)
	}

	return latitude, longitude, nil
}

// packWays resolves each way's geometry and cuts it into per-cell runs.
//
// Runs are packed to bytes here, at the moment they are cut, rather than being
// collected as geometry and packed at the end. That is what keeps the builder's
// footprint flat: the alternative holds a slice header and two coordinate slices
// per run for millions of runs, which for one German state is most of a
// gigabyte of live data, against a few tens of megabytes for the same runs as
// packed bytes.
func packWays(
	ctx context.Context,
	ways []wayRecord,
	references, nodeIDs []int64,
	latitude, longitude []int32,
	writer *cellWriter,
) error {
	divisor := quantiseDivisor()
	x := make([]int32, 0, 512)
	y := make([]int32, 0, 512)

	for index := range ways {
		if index%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("osmindex: packing ways: %w", err)
			}
		}

		way := &ways[index]
		x, y = x[:0], y[:0]
		for reference := way.refStart; reference < way.refEnd; reference++ {
			at, hit := slices.BinarySearch(nodeIDs, references[reference])
			// A node the extract references but does not contain is one cut off
			// at the region border. The way keeps whatever of it lies inside.
			if !hit || latitude[at] == coordinateMissing {
				continue
			}
			x = append(x, longitude[at]/divisor)
			y = append(y, latitude[at]/divisor)
		}
		if len(x) < 2 {
			continue
		}

		writer.add(way.id, way.kind, x, y)
		if err := writer.flushIfFull(ctx); err != nil {
			return err
		}
	}

	return nil
}

// decoderCount is how many goroutines decode the extract. It follows GOMAXPROCS
// rather than the core count so a service confined to two cores does not start
// eight decoders to contend for them.
func decoderCount() int {
	return max(1, runtime.GOMAXPROCS(0))
}

// cellWriter accumulates packed cells and writes them to the index file.
type cellWriter struct {
	database *sql.DB
	pending  map[cellKey][]byte
	path     string
	bytes    int
}

func newCellWriter(ctx context.Context, path string) (*cellWriter, error) {
	database, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("osmindex: creating index: %w", err)
	}
	database.SetMaxOpenConns(1)

	for _, statement := range []string{
		// The file is written once and read many times, and is rebuilt from
		// scratch if the write is interrupted, so durability during the build
		// buys nothing and costs a great deal.
		`PRAGMA journal_mode = OFF`,
		`PRAGMA synchronous = OFF`,
		`PRAGMA page_size = 8192`,
		`CREATE TABLE cell (
			x    INTEGER NOT NULL,
			y    INTEGER NOT NULL,
			ways BLOB    NOT NULL,
			PRIMARY KEY (x, y)
		) WITHOUT ROWID`,
		`CREATE TABLE meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			closeDatabase(database)

			return nil, fmt.Errorf("osmindex: preparing index: %w", err)
		}
	}

	return &cellWriter{
		database: database,
		pending:  make(map[cellKey][]byte, 1<<16),
		path:     path,
	}, nil
}

func (w *cellWriter) add(wayID int64, kind surface.Kind, x, y []int32) {
	w.bytes += splitIntoCells(w.pending, wayID, kind, x, y)
}

func (w *cellWriter) flushIfFull(ctx context.Context) error {
	if w.bytes < flushThresholdBytes {
		return nil
	}

	return w.flush(ctx)
}

// flush writes the accumulated cells, appending to whatever each cell already
// holds. Appending is what makes the accumulator disposable, and it is safe
// because a packed record carries no reference to its neighbours.
func (w *cellWriter) flush(ctx context.Context) error {
	if len(w.pending) == 0 {
		return nil
	}

	transaction, err := w.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("osmindex: writing cells: %w", err)
	}
	defer rollback(transaction)

	statement, err := transaction.PrepareContext(ctx,
		`INSERT INTO cell (x, y, ways) VALUES (?, ?, ?)
		 ON CONFLICT (x, y) DO UPDATE SET ways = ways || excluded.ways`,
	)
	if err != nil {
		return fmt.Errorf("osmindex: writing cells: %w", err)
	}
	defer closeStatement(statement)

	for key, blob := range w.pending {
		if _, err := statement.ExecContext(ctx, key.x, key.y, blob); err != nil {
			return fmt.Errorf("osmindex: writing cell %d/%d: %w", key.x, key.y, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("osmindex: writing cells: %w", err)
	}

	w.pending = make(map[cellKey][]byte, 1<<16)
	w.bytes = 0

	return nil
}

func (w *cellWriter) writeMetadata(ctx context.Context, generation string, regions []string) error {
	values := map[string]string{
		"generation":       generation,
		"regions":          joinRegions(regions),
		"built_at":         time.Now().UTC().Format(time.RFC3339),
		"cell_degrees":     fmt.Sprintf("%g", cellDegrees),
		"coordinate_scale": fmt.Sprintf("%g", coordinateScale),
		"builder_version":  builderVersion,
	}
	for key, value := range values {
		if _, err := w.database.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)`, key, value); err != nil {
			return fmt.Errorf("osmindex: writing index metadata: %w", err)
		}
	}

	return nil
}

// finish compacts the file. Appending to a blob rewrites it, so a cell that grew
// in several flushes leaves the pages it outgrew behind; vacuuming returns them
// and is what makes the file on disk the size the data actually is.
func (w *cellWriter) finish(ctx context.Context) error {
	if _, err := w.database.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("osmindex: compacting index: %w", err)
	}
	if err := w.database.Close(); err != nil {
		return fmt.Errorf("osmindex: closing index: %w", err)
	}

	return nil
}

func (w *cellWriter) close() { closeDatabase(w.database) }

//nolint:errcheck // A file opened for reading has nothing to report on close.
func closeFile(file *os.File) { _ = file.Close() }

//nolint:errcheck // The scan's own error is what the caller acts on.
func closeScanner(scanner *osmpbf.Scanner) { _ = scanner.Close() }

//nolint:errcheck // Rolling back an already-committed transaction is the normal path.
func rollback(transaction *sql.Tx) { _ = transaction.Rollback() }
