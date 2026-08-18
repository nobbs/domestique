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

	// The node and the single-coordinate way are absent from want on purpose:
	// neither can be snapped to, so both must be dropped rather than carried
	// through as candidates that match nothing.
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
	if requests != busyAttempts {
		t.Errorf("request count = %d, want %d before giving up", requests, busyAttempts)
	}
}

// A refusal is usually a moment's contention on a shared server, and the query
// that was refused is answerable a minute later. Giving up on the first retry
// left a long stage — which is only classified once every one of its chunks
// lands — failing indefinitely.
func TestOverpassWaysRetriesARefusedQueryUntilItLands(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests < busyAttempts {
			writer.WriteHeader(http.StatusGatewayTimeout)

			return
		}
		writeOverpass(t, writer, `{"elements":[
			{"type":"way","id":10,"tags":{"highway":"residential","surface":"asphalt"},
			 "geometry":[{"lat":49.0,"lon":8.0},{"lat":49.001,"lon":8.0}]}
		]}`)
	}))
	defer server.Close()

	client := newTestOverpass(t, server)
	var pauses []time.Duration
	client.wait = func(_ context.Context, pause time.Duration) error {
		pauses = append(pauses, pause)

		return nil
	}

	ways, err := client.Ways(t.Context(), metreRoute(0, 200, 50))
	if err != nil {
		t.Fatalf("Ways() error = %v", err)
	}
	if got, want := len(ways), 1; got != want {
		t.Errorf("ways = %d, want %d", got, want)
	}
	// Each refusal waits longer than the last, so a saturated endpoint is asked
	// less often rather than more.
	if len(pauses) < 2 || pauses[1] <= pauses[0] {
		t.Errorf("pauses = %v, want each one longer than the last", pauses)
	}
}

// A wait the endpoint asked for is taken as given, including when it happens to
// name the same interval this client would have chosen on its own.
func TestOverpassHonoursTheWaitTheEndpointAsksFor(t *testing.T) {
	for _, seconds := range []int{5, int(rateLimitPause / time.Second)} {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			requests++
			writer.Header().Set("Retry-After", strconv.Itoa(seconds))
			writer.WriteHeader(http.StatusTooManyRequests)
		}))

		client := newTestOverpass(t, server)
		var pauses []time.Duration
		client.wait = func(_ context.Context, pause time.Duration) error {
			pauses = append(pauses, pause)

			return nil
		}
		if _, err := client.Ways(t.Context(), metreRoute(0, 200, 50)); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("Ways() error = %v, want ErrRateLimited", err)
		}
		server.Close()
		for _, pause := range pauses {
			if pause != time.Duration(seconds)*time.Second {
				t.Errorf("pause = %v, want the %ds the endpoint asked for", pause, seconds)
			}
		}
	}
}

func TestOverpassChoosesAGrowingPauseWhenTheEndpointNamesNone(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestOverpass(t, server)
	var pauses []time.Duration
	client.wait = func(_ context.Context, pause time.Duration) error {
		pauses = append(pauses, pause)

		return nil
	}

	if _, err := client.Ways(t.Context(), metreRoute(0, 200, 50)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Ways() error = %v, want ErrRateLimited", err)
	}
	if len(pauses) < 2 || pauses[1] <= pauses[0] {
		t.Errorf("pauses = %v, want each one longer than the last", pauses)
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
		{name: "no header is no answer", header: "", want: 0},
		{name: "an http date is no answer", header: "Wed, 21 Oct 2026 07:28:00 GMT", want: 0},
		{name: "zero is no answer", header: "0", want: 0},
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

// Every query has to stay inside the bound that the endpoint actually feels: a
// corridor of fifty kilometres went unanswered for three minutes where the same
// ground in four corridors answered in seconds.
func TestOverpassAsksAboutBoundedLengthsOfRoute(t *testing.T) {
	var corridors []int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		corridors = append(corridors, strings.Count(request.PostFormValue("data"), ","))
		writeOverpass(t, writer, `{"elements":[]}`)
	}))
	defer server.Close()

	// Fifty kilometres of straight road: a handful of vertices once simplified,
	// and far more ground than one query may cover.
	if _, err := newTestOverpass(t, server).Ways(t.Context(), metreRoute(0, 50_000, 100)); err != nil {
		t.Fatalf("Ways() error = %v", err)
	}

	wantQueries := int(math.Ceil(50_000 / maximumQueryMetres))
	if len(corridors) < wantQueries {
		t.Errorf("queries = %d, want at least %d for fifty kilometres", len(corridors), wantQueries)
	}
	for index, commas := range corridors {
		// Two commas per vertex, plus the one after the radius.
		if vertices := (commas - 1) / 2; vertices > maximumQueryPoints {
			t.Errorf("query %d carried %d vertices, want at most %d", index, vertices, maximumQueryPoints)
		}
	}
}

func TestChunkPointsCoversTheWholeRouteWithoutGaps(t *testing.T) {
	// The points are a hundred metres apart, so a route of n points covers
	// (n-1) × 100 m and a span cap converts directly into a chunk count.
	const unbounded = 1e9
	tests := []struct {
		name       string
		total      int
		size       int
		span       float64
		wantChunks int
	}{
		{name: "a short route is one chunk", total: 10, size: 250, span: unbounded, wantChunks: 1},
		{name: "a route at the cap is one chunk", total: 250, size: 250, span: unbounded, wantChunks: 1},
		{name: "one point over the cap splits in two", total: 251, size: 250, span: unbounded, wantChunks: 2},
		{name: "a long route splits evenly", total: 1000, size: 250, span: unbounded, wantChunks: 5},
		{name: "a size too small to split is ignored", total: 100, size: 1, span: unbounded, wantChunks: 1},
		// Simplification strips vertices without shortening the route, so a long
		// stage arrives here well under the vertex cap and would go out as one
		// corridor no endpoint will answer.
		{name: "distance splits a route inside the vertex cap", total: 200, size: 250, span: 5000, wantChunks: 4},
		{name: "distance at the cap is one chunk", total: 51, size: 250, span: 5000, wantChunks: 1},
		{name: "whichever bound is tighter decides", total: 1000, size: 250, span: 5000, wantChunks: 20},
		{name: "a span too small to split is ignored", total: 100, size: 250, span: 0, wantChunks: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinates := make([][2]float64, 0, test.total)
			for step := range test.total {
				coordinates = append(coordinates, [2]float64{float64(step) * 100, 0})
			}
			points := metrePoints(coordinates)

			chunks := chunkPoints(points, test.size, test.span)
			if len(chunks) != test.wantChunks {
				t.Fatalf("chunk count = %d, want %d", len(chunks), test.wantChunks)
			}

			// A cut can land mid-segment, so the runs are checked by the ground
			// they cover rather than by how many points that took.
			covered := 0.0
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
				}
				covered += pathMetres(chunk)
			}
			if got, want := covered, pathMetres(points); math.Abs(got-want) > 1 {
				t.Errorf("chunks cover %.1f m, want %.1f m", got, want)
			}
			if chunks[0][0] != points[0] {
				t.Error("the first chunk does not start where the route does")
			}
			last := chunks[len(chunks)-1]
			if last[len(last)-1] != points[len(points)-1] {
				t.Error("the last chunk does not end where the route does")
			}
			if test.span > 0 && len(chunks) > 1 {
				for index, chunk := range chunks {
					// Allow the metre of slack the equal-length split rounds by.
					if got := pathMetres(chunk); got > test.span+1 {
						t.Errorf("chunk[%d] covers %.1f m, want at most %.1f m", index, got, test.span)
					}
				}
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
