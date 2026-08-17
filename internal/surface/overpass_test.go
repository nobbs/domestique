package surface

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

func TestOverpassWaysClassifiesTheWaysItIsGiven(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		query = request.PostFormValue("data")
		if got := request.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		writeOverpass(t, writer, `{"elements":[
			{"type":"way","id":10,"tags":{"highway":"residential","surface":"sett"},
			 "geometry":[{"lat":49.0,"lon":8.0},{"lat":49.001,"lon":8.0}]},
			{"type":"way","id":11,"tags":{"highway":"track","tracktype":"grade2"},
			 "geometry":[{"lat":49.0,"lon":8.001},{"lat":49.001,"lon":8.001}]},
			{"type":"way","id":12,"tags":{"highway":"service"},
			 "geometry":[{"lat":49.0,"lon":8.002},{"lat":49.001,"lon":8.002}]},
			{"type":"node","id":13,"tags":{"highway":"crossing"}},
			{"type":"way","id":14,"tags":{"highway":"residential","surface":"asphalt"},
			 "geometry":[{"lat":49.0,"lon":8.003}]}
		]}`)
	}))
	defer server.Close()

	ways, err := newTestOverpass(t, server).Ways(t.Context(), metreRoute(0, 200, 50))
	if err != nil {
		t.Fatalf("Ways() error = %v", err)
	}

	want := []Way{{ID: 10, Kind: KindPaving}, {ID: 11, Kind: KindGravel}, {ID: 12, Kind: KindUnknown}}
	if len(ways) != len(want) {
		t.Fatalf("way count = %d, want %d", len(ways), len(want))
	}
	for index, way := range ways {
		if way.ID != want[index].ID || way.Kind != want[index].Kind {
			t.Errorf("way[%d] = {id %d, %v}, want {id %d, %v}", index, way.ID, way.Kind, want[index].ID, want[index].Kind)
		}
		if len(way.Line) < 2 {
			t.Errorf("way[%d] carries %d coordinates, want at least 2", index, len(way.Line))
		}
	}

	// A way with only one coordinate cannot be snapped to and must be dropped
	// rather than carried through as a zero-length candidate.
	if strings.Contains(query, "") && len(ways) == 0 {
		t.Fatal("no ways decoded")
	}
	for _, want := range []string{
		"[out:json][timeout:150];",
		"way(around:35,",
		`["highway"]`,
		`["area"!="yes"]`,
		"out tags geom;",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q does not contain %q", query, want)
		}
	}
}

// TestOverpassWaysAsksAboutTheRouteAsRidden checks the corridor: the query has to
// name the route's own coordinates, because the around filter buffers the
// polyline it is given and anything omitted is simply not asked about.
func TestOverpassWaysAsksAboutTheRouteAsRidden(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query = request.PostFormValue("data")
		writeOverpass(t, writer, `{"elements":[]}`)
	}))
	defer server.Close()

	// A right-angle turn: the corner is the one point a simplified corridor must
	// not drop, or the query cuts across ground the route never touches.
	points := metrePoints([][2]float64{{0, 0}, {200, 0}, {400, 0}, {400, 200}, {400, 400}})
	if _, err := newTestOverpass(t, server).Ways(t.Context(), points); err != nil {
		t.Fatalf("Ways() error = %v", err)
	}

	corner := points[2]
	if got := coordinatePair(corner); !strings.Contains(query, got) {
		t.Errorf("query %q omits the corner %q", query, got)
	}
	if got := coordinatePair(points[0]); !strings.Contains(query, got) {
		t.Errorf("query %q omits the start %q", query, got)
	}
	if got := coordinatePair(points[len(points)-1]); !strings.Contains(query, got) {
		t.Errorf("query %q omits the end %q", query, got)
	}
	// The points midway along each straight leg say nothing the leg's ends do not.
	if got := coordinatePair(points[1]); strings.Contains(query, got) {
		t.Errorf("query %q carries the redundant point %q", query, got)
	}
}

func TestOverpassWaysSplitsALongRouteAndReportsEachWayOnce(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		queries = append(queries, request.PostFormValue("data"))
		writeOverpass(t, writer, `{"elements":[
			{"type":"way","id":10,"tags":{"surface":"asphalt"},
			 "geometry":[{"lat":49.0,"lon":8.0},{"lat":49.001,"lon":8.0}]}
		]}`)
	}))
	defer server.Close()

	// A zig-zag so simplification cannot collapse the geometry: every vertex is a
	// turn, which is the worst case the chunking exists for.
	coordinates := make([][2]float64, 0, 600)
	for step := range 600 {
		north := 0.0
		if step%2 == 1 {
			north = 40
		}
		coordinates = append(coordinates, [2]float64{float64(step) * 100, north})
	}

	ways, err := newTestOverpass(t, server).Ways(t.Context(), metrePoints(coordinates))
	if err != nil {
		t.Fatalf("Ways() error = %v", err)
	}

	if len(queries) < 2 {
		t.Fatalf("query count = %d, want the route split across several", len(queries))
	}
	for index, query := range queries {
		if got := strings.Count(query, ".")/2 + 1; got > maximumQueryPoints+1 {
			t.Errorf("query[%d] names about %d points, want at most %d", index, got, maximumQueryPoints)
		}
	}
	if len(ways) != 1 {
		t.Errorf("way count = %d, want the repeated way reported once", len(ways))
	}
}

func TestOverpassWaysRetriesOnceWhenTheEndpointIsBusy(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writer.Header().Set("Retry-After", "7")
			writer.WriteHeader(http.StatusTooManyRequests)

			return
		}
		writeOverpass(t, writer, `{"elements":[
			{"type":"way","id":10,"tags":{"surface":"gravel"},
			 "geometry":[{"lat":49.0,"lon":8.0},{"lat":49.001,"lon":8.0}]}
		]}`)
	}))
	defer server.Close()

	client := newTestOverpass(t, server)
	var waited time.Duration
	client.wait = func(_ context.Context, duration time.Duration) error {
		waited = duration

		return nil
	}

	ways, err := client.Ways(t.Context(), metreRoute(0, 200, 50))
	if err != nil {
		t.Fatalf("Ways() error = %v", err)
	}
	if len(ways) != 1 || ways[0].Kind != KindGravel {
		t.Errorf("ways = %v, want one gravel way", ways)
	}
	if got, want := waited, 7*time.Second; got != want {
		t.Errorf("wait = %s, want the advertised %s", got, want)
	}
	if requests != 2 {
		t.Errorf("request count = %d, want 2", requests)
	}
}

func TestOverpassWaysReportsPersistentRateLimiting(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestOverpass(t, server)
	client.wait = func(context.Context, time.Duration) error { return nil }

	_, err := client.Ways(t.Context(), metreRoute(0, 200, 50))
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Ways() error = %v, want ErrRateLimited", err)
	}
	if requests != 2 {
		t.Errorf("request count = %d, want the endpoint left alone after the retry", requests)
	}
}

func TestOverpassWaysRejectsUnusableResponses(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		want   string
		status int
	}{
		{
			name:   "a server failure",
			status: http.StatusInternalServerError,
			body:   "",
			want:   "HTTP 500",
		},
		{
			name:   "geometry that is not json",
			status: http.StatusOK,
			body:   "<html>overloaded</html>",
			want:   "decoding overpass response",
		},
		{
			name:   "a query the server abandoned",
			status: http.StatusOK,
			body:   `{"remark":"runtime error: Query timed out in \"query\" at line 1","elements":[]}`,
			want:   "runtime error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				if _, err := writer.Write([]byte(test.body)); err != nil {
					t.Errorf("Write() error = %v", err)
				}
			}))
			defer server.Close()

			_, err := newTestOverpass(t, server).Ways(t.Context(), metreRoute(0, 200, 50))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Ways() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

// TestOverpassWaysSkipsGeometryItCannotAskAbout guards against spending a
// request on a stage that cannot produce a corridor at all.
func TestOverpassWaysSkipsGeometryItCannotAskAbout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the endpoint was queried for geometry with no extent")
	}))
	defer server.Close()

	client := newTestOverpass(t, server)
	for name, points := range map[string][]route.Point{
		"no points":  nil,
		"one point":  metreRoute(0, 0, 50),
		"nil points": {},
	} {
		t.Run(name, func(t *testing.T) {
			ways, err := client.Ways(t.Context(), points)
			if err != nil {
				t.Fatalf("Ways() error = %v", err)
			}
			if len(ways) != 0 {
				t.Errorf("ways = %v, want none", ways)
			}
		})
	}
}

func TestNewOverpassRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		options *Options
		name    string
	}{
		{name: "a relative endpoint", options: &Options{Endpoint: "/api/interpreter"}},
		{name: "an endpoint with no host", options: &Options{Endpoint: "https://"}},
		{name: "an unsupported scheme", options: &Options{Endpoint: "ftp://overpass.example/api"}},
		{name: "a negative timeout", options: &Options{Timeout: -time.Second}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewOverpass(test.options); err == nil {
				t.Error("NewOverpass() error = nil, want an error")
			}
		})
	}
}

func TestNewOverpassDefaultsToThePublicEndpoint(t *testing.T) {
	client, err := NewOverpass(nil)
	if err != nil {
		t.Fatalf("NewOverpass() error = %v", err)
	}
	if got := client.endpoint.String(); got != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", got, DefaultEndpoint)
	}
	if got := client.timeout; got != defaultTimeout {
		t.Errorf("timeout = %s, want %s", got, defaultTimeout)
	}
}

func TestRetryAfterFallsBackToTheDocumentedPause(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{name: "an advertised pause is honoured", header: "12", want: 12 * time.Second},
		{name: "a padded value is still read", header: " 4 ", want: 4 * time.Second},
		{name: "no header falls back", header: "", want: rateLimitPause},
		{name: "an http date falls back", header: "Wed, 21 Oct 2026 07:28:00 GMT", want: rateLimitPause},
		{name: "zero falls back", header: "0", want: rateLimitPause},
		{name: "an unreasonable wait is capped", header: "86400", want: 2 * rateLimitPause},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := retryAfter(test.header); got != test.want {
				t.Errorf("retryAfter(%q) = %s, want %s", test.header, got, test.want)
			}
		})
	}
}

// TestDecimateStaysWithinItsTolerance is the property the corridor depends on:
// every point that was dropped has to remain within the tolerance of the
// simplified line, or the query stops covering ground the route crosses.
func TestDecimateStaysWithinItsTolerance(t *testing.T) {
	const tolerance = 10.0

	coordinates := make([][2]float64, 0, 400)
	for step := range 400 {
		east := float64(step) * 25
		coordinates = append(coordinates, [2]float64{east, 300 * math.Sin(east/900)})
	}
	points := metrePoints(coordinates)

	kept := decimate(points, tolerance)
	if len(kept) >= len(points) {
		t.Fatalf("kept %d of %d points, want a reduction", len(kept), len(points))
	}

	projection := newProjection(points[0].Longitude, points[0].Latitude)
	keptSegments := buildSegments(projection, []Way{lineOf(kept)})
	for index := range points {
		east, north := projection.project(points[index].Longitude, points[index].Latitude)
		nearest := math.Inf(1)
		for _, candidate := range keptSegments {
			nearest = math.Min(nearest, candidate.distanceTo(east, north))
		}
		if nearest > tolerance {
			t.Fatalf("point %d sits %.2fm from the simplified line, beyond the %.2fm tolerance", index, nearest, tolerance)
		}
	}
}

func TestDecimateKeepsTheShapeItNeeds(t *testing.T) {
	t.Run("a straight run collapses to its ends", func(t *testing.T) {
		points := metreRoute(0, 1000, 25)
		if got := decimate(points, queryToleranceMetres); len(got) != 2 {
			t.Errorf("kept %d points, want 2", len(got))
		}
	})

	t.Run("geometry with no extent is returned unchanged", func(t *testing.T) {
		for _, points := range [][]route.Point{nil, metreRoute(0, 0, 25), metreRoute(0, 50, 50)} {
			if got := decimate(points, queryToleranceMetres); len(got) != len(points) {
				t.Errorf("kept %d of %d points, want all of them", len(got), len(points))
			}
		}
	})

	t.Run("a turn sharper than the tolerance is kept", func(t *testing.T) {
		points := metrePoints([][2]float64{{0, 0}, {100, 0}, {100, 100}})
		if got := decimate(points, queryToleranceMetres); len(got) != 3 {
			t.Errorf("kept %d points, want the corner kept", len(got))
		}
	})
}

func TestChunkPointsCoversTheWholeRouteWithoutGaps(t *testing.T) {
	tests := []struct {
		name       string
		total      int
		size       int
		wantChunks int
	}{
		{name: "a short route is one chunk", total: 10, size: 250, wantChunks: 1},
		{name: "a route at the cap is one chunk", total: 250, size: 250, wantChunks: 1},
		{name: "one point over the cap splits in two", total: 251, size: 250, wantChunks: 2},
		{name: "a long route splits evenly", total: 1000, size: 250, wantChunks: 5},
		{name: "a size too small to split is ignored", total: 100, size: 1, wantChunks: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinates := make([][2]float64, 0, test.total)
			for step := range test.total {
				coordinates = append(coordinates, [2]float64{float64(step) * 100, 0})
			}
			points := metrePoints(coordinates)

			chunks := chunkPoints(points, test.size)
			if len(chunks) != test.wantChunks {
				t.Fatalf("chunk count = %d, want %d", len(chunks), test.wantChunks)
			}

			covered := len(chunks[0])
			for index, chunk := range chunks {
				if len(chunk) < 2 && test.total >= 2 {
					t.Errorf("chunk[%d] holds %d points, want a queryable run", index, len(chunk))
				}
				if test.size >= 2 && len(chunk) > test.size {
					t.Errorf("chunk[%d] holds %d points, want at most %d", index, len(chunk), test.size)
				}
				if index > 0 {
					previous := chunks[index-1]
					if chunk[0] != previous[len(previous)-1] {
						t.Errorf("chunk[%d] does not start where chunk[%d] ended", index, index-1)
					}
					covered += len(chunk) - 1
				}
			}
			if len(chunks) > 1 && covered != test.total {
				t.Errorf("chunks cover %d points, want %d", covered, test.total)
			}
		})
	}
}

func newTestOverpass(t *testing.T, server *httptest.Server) *Overpass {
	t.Helper()
	client, err := NewOverpass(&Options{
		Endpoint:  server.URL,
		Timeout:   5 * time.Second,
		Transport: server.Client().Transport,
	})
	if err != nil {
		t.Fatalf("NewOverpass() error = %v", err)
	}

	return client
}

func writeOverpass(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Errorf("Write() error = %v", err)
	}
}

// coordinatePair renders a point the way buildQuery does, so a test can look for
// it in the query text.
func coordinatePair(point route.Point) string {
	var builder strings.Builder
	builder.WriteString(strconv.FormatFloat(point.Latitude, 'f', 5, 64))
	builder.WriteByte(',')
	builder.WriteString(strconv.FormatFloat(point.Longitude, 'f', 5, 64))

	return builder.String()
}

// lineOf reads stage geometry back as a way, so a test can measure against it
// with the same segment helpers the match uses.
func lineOf(points []route.Point) Way {
	line := make([]Coordinate, 0, len(points))
	for _, point := range points {
		line = append(line, Coordinate{Longitude: point.Longitude, Latitude: point.Latitude})
	}

	return Way{Line: line}
}
