package brouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func testWaypoints() []Waypoint {
	return []Waypoint{
		{Longitude: 8.68, Latitude: 50.11},
		{Longitude: 8.70, Latitude: 50.12},
	}
}

func TestRouteSendsTheExpectedRequest(t *testing.T) {
	var gotPath, gotQuery, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/vnd.geo+json")
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11,100.0],[8.70,50.12,110.0]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, err = client.Route(context.Background(), testWaypoints(), "trekking")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod, "method")
	assert.Equal(t, "/brouter", gotPath, "path")
	query, parseErr := url.ParseQuery(gotQuery)
	require.NoError(t, parseErr)
	assert.Equal(t, "8.68,50.11|8.7,50.12", query.Get("lonlats"), "lonlats")
	assert.Equal(t, "trekking", query.Get("profile"), "profile")
	assert.Equal(t, "0", query.Get("alternativeidx"), "alternativeidx")
	assert.Equal(t, "geojson", query.Get("format"), "format")
	assert.Equal(t, "3", query.Get("timode"), "timode")
}

func TestRouteDecodesGeometryWithAndWithoutElevation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.geo+json")
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11,100.0],[8.70,50.12]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	answer, err := client.Route(context.Background(), testWaypoints(), "trekking")
	require.NoError(t, err)
	points := answer.Points
	require.Len(t, points, 2)
	require.NotNil(t, points[0].Elevation)
	assert.InDelta(t, 100.0, *points[0].Elevation, 0, "points[0].Elevation")
	assert.Nil(t, points[1].Elevation, "points[1].Elevation")
}

func TestRouteMapsFailureCategories(t *testing.T) {
	tests := []struct {
		handler  func(t *testing.T) http.HandlerFunc
		name     string
		category Failure
	}{
		{
			name: "unknown profile answers 500 with an empty body",
			handler: func(_ *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
			category: FailureEngine,
		},
		{
			name: "unroutable point answers 400 with a plain text body",
			handler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					_, writeErr := w.Write([]byte("datafile E0_N0.rd5 not found"))
					assert.NoError(t, writeErr)
				}
			},
			category: FailureRefused,
		},
		{
			name: "garbage 200 body",
			handler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					_, writeErr := w.Write([]byte("not json"))
					assert.NoError(t, writeErr)
				}
			},
			category: FailureResponse,
		},
		{
			name: "one point geometry",
			handler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
						`"geometry":{"type":"LineString","coordinates":[[8.68,50.11]]}}]}`))
					assert.NoError(t, writeErr)
				}
			},
			category: FailureResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler(t))
			defer server.Close()

			client, err := New(&Options{BaseURL: server.URL})
			require.NoError(t, err)

			_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
			require.Error(t, routeErr)
			var brouterErr *Error
			require.ErrorAs(t, routeErr, &brouterErr)
			assert.Equal(t, tt.category, brouterErr.Category, "Category")
			assert.NotContains(t, routeErr.Error(), "not found", "Error() leaked the response body")
		})
	}
}

func TestRouteTimesOutAsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL, Timeout: 50 * time.Millisecond})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureUnreachable, brouterErr.Category, "Category")
}

// A redirect is not a routing answer this adapter follows: it must not reach
// whatever the Location header names.
func TestRouteDoesNotFollowARedirect(t *testing.T) {
	var targetRequests int
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		targetRequests++
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureResponse, brouterErr.Category, "Category")
	assert.Equal(t, 0, targetRequests, "the redirect target must never be requested")
}

func TestNewRejectsABaseURLWithAPath(t *testing.T) {
	_, err := New(&Options{BaseURL: "http://brouter:17777/routing"})
	require.Error(t, err)
}

func TestNewRejectsNilOptions(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err)
}

func TestNewRejectsANegativeTimeout(t *testing.T) {
	_, err := New(&Options{BaseURL: "http://brouter:17777", Timeout: -time.Second})
	require.Error(t, err)
}

func TestRouteRejectsFewerThanTwoWaypoints(t *testing.T) {
	client, err := New(&Options{BaseURL: "http://brouter:17777"})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), []Waypoint{{Longitude: 8.68, Latitude: 50.11}}, "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureRefused, brouterErr.Category, "Category")
}

func TestRouteRejectsAnOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write(make([]byte, maximumBodyBytes+1))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureResponse, brouterErr.Category, "Category")
}

func TestRouteRejectsAnUnexpectedSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureResponse, brouterErr.Category, "Category")
}

func TestRouteRejectsAResponseWithNoLineStringFeature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureResponse, brouterErr.Category, "Category")
}

func TestRouteRejectsACoordinateMissingLatitude(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"geometry":{"type":"LineString","coordinates":[[8.68],[8.70,50.12]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureResponse, brouterErr.Category, "Category")
}

// A coordinate outside the valid range is never a real routing answer.
func TestRouteRejectsAnOutOfRangeCoordinate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"geometry":{"type":"LineString","coordinates":[[200,50.11],[8.70,50.12]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	_, routeErr := client.Route(context.Background(), testWaypoints(), "trekking")
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureResponse, brouterErr.Category, "Category")
}

// Error() names the category and status code only, and must still read
// sensibly for a failure that carries no status, such as a client-side
// validation refusal.
func TestErrorStringWithoutAStatus(t *testing.T) {
	err := &Error{Category: FailureRefused}
	assert.Contains(t, err.Error(), string(FailureRefused))
	assert.NotContains(t, err.Error(), "HTTP")
}

func TestRouteReadsTheWaysUnderTheLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"properties":{"messages":[` +
			`["Longitude","Latitude","Elevation","Distance","CostPerKm","WayTags"],` +
			`["8680000","50110000","100","34","2250","highway=pedestrian bicycle=yes"],` +
			`["8690000","50115000","101","62","5050","highway=footway surface=concrete"]]},` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11,100.0],[8.70,50.12,101.0]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	answer, err := client.Route(context.Background(), testWaypoints(), "trekking")

	require.NoError(t, err)
	require.Len(t, answer.Ways, 2)
	assert.InDelta(t, 34, answer.Ways[0].EndMetres, 0)
	assert.Equal(t, map[string]string{"highway": "pedestrian", "bicycle": "yes"}, answer.Ways[0].Tags)
	assert.InDelta(t, 96, answer.Ways[1].EndMetres, 0, "each stretch ends where the ones before it add up to")
	assert.Equal(t, "footway", answer.Ways[1].Tags["highway"])
}

func TestRouteRefusesAWaysTableWithADistanceThatIsNotOne(t *testing.T) {
	for name, distance := range map[string]string{"negative": "-1", "not a number": "NaN", "endless": "Inf"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
					`"properties":{"messages":[["Longitude","Latitude","Distance","WayTags"],` +
					`["8680000","50110000","` + distance + `","highway=footway"]]},` +
					`"geometry":{"type":"LineString","coordinates":[[8.68,50.11,100.0],[8.70,50.12,101.0]]}}]}`))
				assert.NoError(t, writeErr)
			}))
			defer server.Close()
			client, err := New(&Options{BaseURL: server.URL})
			require.NoError(t, err)

			answer, err := client.Route(context.Background(), testWaypoints(), "trekking")

			require.NoError(t, err)
			assert.Empty(t, answer.Ways)
		})
	}
}

func TestRouteKeepsTheLineWhenTheWaysTableIsUnreadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"properties":{"messages":[["Longitude","Latitude"],["8680000","50110000"]]},` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11,100.0],[8.70,50.12,101.0]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	answer, err := client.Route(context.Background(), testWaypoints(), "trekking")

	require.NoError(t, err)
	assert.Len(t, answer.Points, 2, "the line stands")
	assert.Empty(t, answer.Ways, "a table without distances or tags says nothing")
}

func TestRouteReadsTheEnginesTurns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"properties":{"voicehints":[[1,2,0,436.0,-102],[2,13,2,249.0,-210],[1,6,0,95,0],` +
			`[2,16,0,10,0],[9,5,0,1,0],[1.5,5,0,1,0],[1],[2,9,4,1,0]]},` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11],[8.69,50.115],[8.70,50.12]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	answer, err := client.Route(context.Background(), testWaypoints(), "trekking")

	require.NoError(t, err)
	assert.Equal(t, []Turn{
		{Turn: route.TurnLeft, Index: 1},
		{Turn: route.TurnRoundabout, Index: 2, Exit: 2},
		{Turn: route.TurnSlightRight, Index: 1},
		{Turn: route.TurnKeepRight, Index: 2},
	}, answer.Turns, "turns: beeline, an index off the line, a fractional index and a short row are left out, and only a roundabout keeps its exit")
}

func TestRouteKeepsTheLineWhenTheTurnsAreUnreadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"properties":{"voicehints":"none"},` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11],[8.70,50.12]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	client, err := New(&Options{BaseURL: server.URL})
	require.NoError(t, err)

	answer, err := client.Route(context.Background(), testWaypoints(), "trekking")

	require.NoError(t, err)
	assert.Len(t, answer.Points, 2, "the line stands")
	assert.Empty(t, answer.Turns, "turns")
}

func TestEngineTurnNamesEveryCommandItKeeps(t *testing.T) {
	want := map[int]route.Turn{
		1: route.TurnStraight, 2: route.TurnLeft, 3: route.TurnSlightLeft, 4: route.TurnSharpLeft,
		5: route.TurnRight, 6: route.TurnSlightRight, 7: route.TurnSharpRight, 8: route.TurnKeepLeft,
		9: route.TurnKeepRight, 10: route.TurnUTurn, 11: route.TurnUTurn, 13: route.TurnRoundabout,
		14: route.TurnRoundabout, 15: route.TurnUTurn, 17: route.TurnKeepLeft, 18: route.TurnKeepRight,
	}
	for command := 0; command <= 101; command++ {
		turn, known := engineTurn(command)
		expected, kept := want[command]
		assert.Equalf(t, kept, known, "command %d kept", command)
		assert.Equalf(t, expected, turn, "command %d turn", command)
	}
}
