package surface

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestOverpassWaysClassifiesTheWaysItIsGiven(t *testing.T) {
	var query string
	// Every check inside a handler runs on the server's goroutine, where FailNow is
	// unsafe, so the handlers in this file assert and return rather than require.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			assert.Failf(t, "parsing the query form", "%v", err)

			return
		}
		query = request.PostFormValue("data")
		assert.Equal(t, userAgent, request.Header.Get("User-Agent"), "User-Agent")
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
	require.NoError(t, err)

	// The node and the single-coordinate way are absent from want on purpose:
	// neither can be snapped to, so both must be dropped rather than carried
	// through as candidates that match nothing.
	want := []Way{{ID: 10, Kind: KindPaving}, {ID: 11, Kind: KindGravel}, {ID: 12, Kind: KindUnknown}}
	require.Len(t, ways, len(want), "way count")
	for index, way := range ways {
		assert.Equalf(t, want[index].ID, way.ID, "way[%d] id", index)
		assert.Equalf(t, want[index].Kind, way.Kind, "way[%d] kind", index)
		assert.GreaterOrEqualf(t, len(way.Line), 2, "way[%d] carries %d coordinates, want at least 2", index, len(way.Line))
	}
	for _, want := range []string{
		"[out:json][timeout:150];",
		"way(around:35,",
		`["highway"]`,
		`["area"!="yes"]`,
		"out tags geom;",
	} {
		assert.Contains(t, query, want, "the query omits a clause it needs")
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
	_, err := newTestOverpass(t, server).Ways(t.Context(), points)
	require.NoError(t, err)

	assert.Contains(t, query, coordinatePair(points[2]), "the query omits the corner")
	assert.Contains(t, query, coordinatePair(points[0]), "the query omits the start")
	assert.Contains(t, query, coordinatePair(points[len(points)-1]), "the query omits the end")
	// The points midway along each straight leg say nothing the leg's ends do not.
	assert.NotContains(t, query, coordinatePair(points[1]), "the query carries a redundant point")
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
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(queries), 2, "the route was not split across several queries")
	for index, query := range queries {
		named := strings.Count(query, ".")/2 + 1
		assert.LessOrEqualf(t, named, maximumQueryPoints+1, "query[%d] names about %d points", index, named)
	}
	assert.Len(t, ways, 1, "the repeated way was not reported once")
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
	require.NoError(t, err)
	require.Len(t, ways, 1)
	assert.Equal(t, KindGravel, ways[0].Kind, "the way the retry returned")
	assert.Equal(t, 7*time.Second, waited, "the client did not wait the advertised pause")
	assert.Equal(t, 2, requests, "request count")
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
	require.ErrorIs(t, err, ErrRateLimited)
	assert.Equal(t, busyAttempts, requests, "requests made before giving up")
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
	require.NoError(t, err)
	assert.Len(t, ways, 1)
	// Each refusal waits longer than the last, so a saturated endpoint is asked
	// less often rather than more.
	require.GreaterOrEqualf(t, len(pauses), 2, "pauses = %v, want one per refusal", pauses)
	assert.Greaterf(t, pauses[1], pauses[0], "pauses = %v, want each one longer than the last", pauses)
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
		_, err := client.Ways(t.Context(), metreRoute(0, 200, 50))
		require.ErrorIs(t, err, ErrRateLimited)
		server.Close()
		for _, pause := range pauses {
			assert.Equalf(t, time.Duration(seconds)*time.Second, pause, "want the %ds the endpoint asked for", seconds)
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

	_, err := client.Ways(t.Context(), metreRoute(0, 200, 50))
	require.ErrorIs(t, err, ErrRateLimited)
	require.GreaterOrEqualf(t, len(pauses), 2, "pauses = %v, want one per refusal", pauses)
	assert.Greaterf(t, pauses[1], pauses[0], "pauses = %v, want each one longer than the last", pauses)
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
				_, err := writer.Write([]byte(test.body))
				assert.NoError(t, err, "writing the response")
			}))
			defer server.Close()

			_, err := newTestOverpass(t, server).Ways(t.Context(), metreRoute(0, 200, 50))
			require.ErrorContains(t, err, test.want)
		})
	}
}

// TestOverpassWaysSkipsGeometryItCannotAskAbout guards against spending a
// request on a stage that cannot produce a corridor at all.
func TestOverpassWaysSkipsGeometryItCannotAskAbout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		assert.Fail(t, "the endpoint was queried for geometry with no extent")
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
			require.NoError(t, err)
			assert.Empty(t, ways, "geometry with no extent produced ways")
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
			_, err := NewOverpass(test.options)
			require.Error(t, err, "NewOverpass() accepted the options")
		})
	}
}

func TestNewOverpassDefaultsToThePublicEndpoint(t *testing.T) {
	client, err := NewOverpass(nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultEndpoint, client.endpoint.String(), "endpoint")
	assert.Equal(t, defaultTimeout, client.timeout, "timeout")
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
			assert.Equalf(t, test.want, retryAfter(test.header), "retryAfter(%q)", test.header)
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
	require.Lessf(t, len(kept), len(points), "kept %d of %d points, want a reduction", len(kept), len(points))

	projection := newProjection(points[0].Longitude, points[0].Latitude)
	keptSegments := buildSegments(projection, []Way{lineOf(kept)})
	for index := range points {
		east, north := projection.project(points[index].Longitude, points[index].Latitude)
		nearest := math.Inf(1)
		for _, candidate := range keptSegments {
			nearest = math.Min(nearest, candidate.distanceTo(east, north))
		}
		require.LessOrEqualf(t, nearest, tolerance,
			"point %d sits %.2fm from the simplified line, beyond the %.2fm tolerance", index, nearest, tolerance)
	}
}

func TestDecimateKeepsTheShapeItNeeds(t *testing.T) {
	t.Run("a straight run collapses to its ends", func(t *testing.T) {
		points := metreRoute(0, 1000, 25)
		assert.Len(t, decimate(points, queryToleranceMetres), 2, "a straight run did not collapse to its ends")
	})

	t.Run("geometry with no extent is returned unchanged", func(t *testing.T) {
		for _, points := range [][]route.Point{nil, metreRoute(0, 0, 25), metreRoute(0, 50, 50)} {
			assert.Lenf(t, decimate(points, queryToleranceMetres), len(points),
				"geometry with no extent lost one of its %d points", len(points))
		}
	})

	t.Run("a turn sharper than the tolerance is kept", func(t *testing.T) {
		points := metrePoints([][2]float64{{0, 0}, {100, 0}, {100, 100}})
		assert.Len(t, decimate(points, queryToleranceMetres), 3, "the corner was not kept")
	})
}

// Every query has to stay inside the bound that the endpoint actually feels: a
// corridor of fifty kilometres went unanswered for three minutes where the same
// ground in four corridors answered in seconds.
func TestOverpassAsksAboutBoundedLengthsOfRoute(t *testing.T) {
	var corridors []int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			assert.Failf(t, "parsing the query form", "%v", err)

			return
		}
		corridors = append(corridors, strings.Count(request.PostFormValue("data"), ","))
		writeOverpass(t, writer, `{"elements":[]}`)
	}))
	defer server.Close()

	// Fifty kilometres of straight road: a handful of vertices once simplified,
	// and far more ground than one query may cover.
	_, err := newTestOverpass(t, server).Ways(t.Context(), metreRoute(0, 50_000, 100))
	require.NoError(t, err)

	wantQueries := int(math.Ceil(50_000 / maximumQueryMetres))
	assert.GreaterOrEqualf(t, len(corridors), wantQueries, "want at least %d queries for fifty kilometres", wantQueries)
	for index, commas := range corridors {
		// Two commas per vertex; the radius is not followed by one.
		assert.LessOrEqualf(t, commas/2, maximumQueryPoints, "query %d carried too many vertices", index)
	}
}

// Vertices are not spread evenly along a route: a town's worth of turns inside
// one kilometre can outnumber the cap while covering almost no ground, and the
// distance cuts alone would hand that to one query.
func TestChunkPointsCapsVerticesWhereTheyBunchUp(t *testing.T) {
	coordinates := make([][2]float64, 0, 900)
	// Six hundred points inside the first kilometre, then fifty kilometres with
	// almost nothing on it.
	for step := range 600 {
		coordinates = append(coordinates, [2]float64{float64(step) * 1.6, 0})
	}
	for step := range 300 {
		coordinates = append(coordinates, [2]float64{1000 + float64(step)*170, 0})
	}
	points := metrePoints(coordinates)

	chunks := chunkPoints(points, 250, 12000)
	for index, chunk := range chunks {
		assert.LessOrEqualf(t, len(chunk), 250, "chunk[%d] holds too many points", index)
		assert.LessOrEqualf(t, pathMetres(chunk), 12001.0, "chunk[%d] covers more than 12000 m", index)
	}
	covered := 0.0
	for _, chunk := range chunks {
		covered += pathMetres(chunk)
	}
	assert.InDelta(t, pathMetres(points), covered, 1, "the chunks do not cover the route they were cut from")
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
		// No distance bound is not the same as no bound at all.
		{name: "the vertex cap holds without a span", total: 600, size: 250, span: 0, wantChunks: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinates := make([][2]float64, 0, test.total)
			for step := range test.total {
				coordinates = append(coordinates, [2]float64{float64(step) * 100, 0})
			}
			points := metrePoints(coordinates)

			chunks := chunkPoints(points, test.size, test.span)
			require.Len(t, chunks, test.wantChunks, "chunk count")

			// A cut can land mid-segment, so the runs are checked by the ground
			// they cover rather than by how many points that took.
			covered := 0.0
			for index, chunk := range chunks {
				if test.total >= 2 {
					assert.GreaterOrEqualf(t, len(chunk), 2, "chunk[%d] is not a queryable run", index)
				}
				if test.size >= 2 {
					assert.LessOrEqualf(t, len(chunk), test.size, "chunk[%d] holds more than %d points", index, test.size)
				}
				if index > 0 {
					previous := chunks[index-1]
					assert.Equalf(t, previous[len(previous)-1], chunk[0], "chunk[%d] does not start where chunk[%d] ended", index, index-1)
				}
				covered += pathMetres(chunk)
			}
			assert.InDelta(t, pathMetres(points), covered, 1, "the chunks do not cover the route they were cut from")
			assert.Equal(t, points[0], chunks[0][0], "the first chunk does not start where the route does")
			last := chunks[len(chunks)-1]
			assert.Equal(t, points[len(points)-1], last[len(last)-1], "the last chunk does not end where the route does")
			if test.span > 0 && len(chunks) > 1 {
				for index, chunk := range chunks {
					// Allow the metre of slack the equal-length split rounds by.
					assert.LessOrEqualf(t, pathMetres(chunk), test.span+1, "chunk[%d] covers more than the %.1f m span", index, test.span)
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
	require.NoError(t, err, "NewOverpass()")

	return client
}

func writeOverpass(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	_, err := writer.Write([]byte(body))
	assert.NoError(t, err, "writing the response")
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
