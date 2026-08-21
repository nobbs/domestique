//go:build verify

// Package-internal verification against the extract and index the design was
// measured on.
//
// This is not a unit test and is excluded from a normal run: it needs a 267 MB
// regional extract and the index built from it by the prototype, neither of
// which belongs in the repository. It exists to answer one question — does the
// production builder give the same answers as the index the 99.477% agreement
// was measured against — and it answers it the only way that counts, by running
// the production matcher over both and comparing classified distance.
//
//	go test -tags verify ./internal/osmindex -run TestVerifyAgainstPrototype -v \
//	  -extract /path/rheinland-pfalz.osm.pbf \
//	  -reference /path/surface-cell0.01-q1e-06.sqlite \
//	  -routes ~/Downloads/komoot-planned-gpx
package osmindex

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/xml"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

var (
	extractPath   = flag.String("extract", "", "regional .osm.pbf to build from")
	referencePath = flag.String("reference", "", "prototype index to compare against")
	routesPath    = flag.String("routes", "", "directory of GPX files")
	memoryLimit   = flag.Int64("limit", 0, "soft memory limit for -run TestMeasureBuild, in bytes")
)

const earthRadiusMetres = 6_371_000.0

func TestVerifyAgainstPrototype(t *testing.T) {
	if *extractPath == "" || *referencePath == "" || *routesPath == "" {
		t.Skip("needs -extract, -reference and -routes")
	}

	built := filepath.Join(t.TempDir(), "built.sqlite")
	start := time.Now()
	buildFixture(t, built)
	t.Logf("built %s in %s", size(t, built), time.Since(start).Round(time.Millisecond))
	t.Logf("reference is %s", size(t, *referencePath))

	fresh, err := Open(t.Context(), built)
	if err != nil {
		t.Fatalf("opening built index: %v", err)
	}
	defer func() { _ = fresh.Close() }()

	reference := openPrototype(t, *referencePath)
	defer func() { _ = reference.Close() }()

	routes := loadRoutes(t, *routesPath)
	t.Logf("comparing %d routes", len(routes))

	var agreed, total float64
	worst, worstName := 1.0, ""
	for _, item := range routes {
		freshWays, err := fresh.Ways(context.Background(), item.points)
		if err != nil {
			t.Fatalf("%s: built index: %v", item.name, err)
		}
		referenceWays, err := reference.ways(item.points)
		if err != nil {
			t.Fatalf("%s: reference index: %v", item.name, err)
		}

		freshKinds := surface.Match(item.points, freshWays)
		referenceKinds := surface.Match(item.points, referenceWays)

		weights := pointWeights(item.points)
		var routeAgreed, routeTotal float64
		for at := range item.points {
			routeTotal += weights[at]
			if freshKinds[at] == referenceKinds[at] {
				routeAgreed += weights[at]
			}
		}
		agreed += routeAgreed
		total += routeTotal

		if routeTotal > 0 {
			if share := routeAgreed / routeTotal; share < worst {
				worst, worstName = share, item.name
			}
		}
	}

	share := agreed / total
	t.Logf("agreement by distance: %.4f%% over %.1f km", share*100, total/1000)
	t.Logf("worst route: %.4f%% (%s)", worst*100, worstName)

	// The two indexes are built from the same extract at the same precision by
	// the same rules, so anything short of exact agreement is a difference in
	// the builder, not a rounding artefact.
	if share < 1.0 {
		t.Errorf("built index disagrees with the reference on %.4f%% of distance", (1-share)*100)
	}
}

// TestMeasureBuild builds without comparing, so the cost of a build can be read
// under a chosen memory limit and processor count. Peak resident size has to be
// read from outside the process; this reports what the runtime saw.
func TestMeasureBuild(t *testing.T) {
	if *extractPath == "" {
		t.Skip("needs -extract")
	}
	if *memoryLimit > 0 {
		defer debug.SetMemoryLimit(debug.SetMemoryLimit(*memoryLimit))
	}

	start := time.Now()
	built := filepath.Join(t.TempDir(), "built.sqlite")
	buildFixture(t, built)
	t.Logf("GOMAXPROCS=%d limit=%d MB: built %s in %s",
		runtime.GOMAXPROCS(0), *memoryLimit>>20, size(t, built), time.Since(start).Round(time.Millisecond))
}

// buildFixture runs the production packing path over a local extract, skipping
// only the download.
func buildFixture(t *testing.T, path string) {
	t.Helper()

	writer, err := newCellWriter(t.Context(), path)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}
	if err := indexExtract(context.Background(), *extractPath, writer); err != nil {
		writer.close()
		t.Fatalf("indexing extract: %v", err)
	}
	if err := writer.flush(t.Context()); err != nil {
		t.Fatalf("flushing cells: %v", err)
	}
	if err := writer.writeMetadata(t.Context(), "verify000000", []string{"europe/germany/rheinland-pfalz"}); err != nil {
		t.Fatalf("writing metadata: %v", err)
	}
	if err := writer.finish(t.Context()); err != nil {
		t.Fatalf("finishing index: %v", err)
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	t.Logf("peak heap during build: %.0f MB (total allocated %.0f MB)",
		float64(stats.HeapSys)/(1<<20), float64(stats.TotalAlloc)/(1<<20))
}

// prototypeIndex reads the format the validated index was written in: a count
// prefix per cell, way identifiers delta-encoded against the previous run.
//
// The production format drops both so that a cell can be appended to, which is
// what lets the builder flush and forget. Keeping the old reader here is what
// makes the comparison meaningful — the claim is that the two formats carry the
// same information, and the only way to test that is to decode both.
type prototypeIndex struct {
	database *sql.DB
	lookup   *sql.Stmt
}

func openPrototype(t *testing.T, path string) *prototypeIndex {
	t.Helper()

	database, err := sql.Open(driverName, "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening reference: %v", err)
	}
	lookup, err := database.Prepare(`SELECT ways FROM cell WHERE x = ? AND y = ?`)
	if err != nil {
		t.Fatalf("preparing reference lookup: %v", err)
	}

	return &prototypeIndex{database: database, lookup: lookup}
}

func (p *prototypeIndex) Close() error { return p.database.Close() }

func (p *prototypeIndex) ways(points []route.Point) ([]surface.Way, error) {
	ways := make([]surface.Way, 0, 4096)
	for _, key := range cellsFor(points) {
		var blob []byte
		switch err := p.lookup.QueryRow(key.x, key.y).Scan(&blob); {
		case err == sql.ErrNoRows:
			continue
		case err != nil:
			return nil, fmt.Errorf("reading cell %d/%d: %w", key.x, key.y, err)
		}
		ways = append(ways, decodePrototypeCell(blob, key)...)
	}

	return ways, nil
}

func decodePrototypeCell(blob []byte, key cellKey) []surface.Way {
	per := quantisedPerCell()
	baseX, baseY := int64(key.x)*per, int64(key.y)*per

	offset := 0
	count, size := binary.Uvarint(blob[offset:])
	offset += size
	ways := make([]surface.Way, 0, count)
	previousWay := int64(0)
	for range count {
		delta, size := binary.Varint(blob[offset:])
		offset += size
		previousWay += delta
		kind := surface.Kind(blob[offset])
		offset++
		points, size := binary.Uvarint(blob[offset:])
		offset += size

		line := make([]surface.Coordinate, 0, points)
		currentY, currentX := baseY, baseX
		for range points {
			deltaY, ySize := binary.Varint(blob[offset:])
			offset += ySize
			deltaX, xSize := binary.Varint(blob[offset:])
			offset += xSize
			currentY += deltaY
			currentX += deltaX
			line = append(line, surface.Coordinate{
				Latitude:  float64(currentY) / coordinateScale,
				Longitude: float64(currentX) / coordinateScale,
			})
		}
		ways = append(ways, surface.Way{ID: previousWay, Kind: kind, Line: line})
	}

	return ways
}

type namedRoute struct {
	name   string
	points []route.Point
}

type gpxFile struct {
	Trk struct {
		Trkseg []struct {
			Trkpt []struct {
				Lat float64 `xml:"lat,attr"`
				Lon float64 `xml:"lon,attr"`
			} `xml:"trkpt"`
		} `xml:"trkseg"`
	} `xml:"trk"`
}

func loadRoutes(t *testing.T, dir string) []namedRoute {
	t.Helper()

	entries, err := filepath.Glob(filepath.Join(dir, "*.gpx"))
	if err != nil {
		t.Fatalf("listing routes: %v", err)
	}
	sort.Strings(entries)

	routes := make([]namedRoute, 0, len(entries))
	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		var parsed gpxFile
		if err := xml.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		points := make([]route.Point, 0, 2048)
		for _, segment := range parsed.Trk.Trkseg {
			for _, trkpt := range segment.Trkpt {
				points = append(points, route.Point{Longitude: trkpt.Lon, Latitude: trkpt.Lat})
			}
		}
		if len(points) < 10 {
			continue
		}
		routes = append(routes, namedRoute{name: filepath.Base(path), points: points})
	}

	return routes
}

// pointWeights gives each point half the distance to each neighbour, so a
// disagreement is counted by the ground it covers rather than by how densely
// the track was sampled there.
func pointWeights(points []route.Point) []float64 {
	weights := make([]float64, len(points))
	for at := 1; at < len(points); at++ {
		half := haversineMetres(points[at-1], points[at]) / 2
		weights[at-1] += half
		weights[at] += half
	}

	return weights
}

func haversineMetres(left, right route.Point) float64 {
	latitude := (right.Latitude - left.Latitude) * math.Pi / 180
	longitude := (right.Longitude - left.Longitude) * math.Pi / 180
	mean := (left.Latitude + right.Latitude) / 2 * math.Pi / 180
	x := longitude * math.Cos(mean)

	return math.Hypot(x, latitude) * earthRadiusMetres
}

func size(t *testing.T, path string) string {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	return fmt.Sprintf("%.1f MB", float64(info.Size())/(1<<20))
}
