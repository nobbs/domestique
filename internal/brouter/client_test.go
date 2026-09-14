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

	"github.com/nobbs/domestique/internal/plan"
)

func testWaypoints() []plan.Waypoint {
	return []plan.Waypoint{
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

	_, err = client.Route(context.Background(), testWaypoints(), plan.Trekking)
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod, "method")
	assert.Equal(t, "/brouter", gotPath, "path")
	query, parseErr := url.ParseQuery(gotQuery)
	require.NoError(t, parseErr)
	assert.Equal(t, "8.68,50.11|8.7,50.12", query.Get("lonlats"), "lonlats")
	assert.Equal(t, "trekking", query.Get("profile"), "profile")
	assert.Equal(t, "0", query.Get("alternativeidx"), "alternativeidx")
	assert.Equal(t, "geojson", query.Get("format"), "format")
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

	points, err := client.Route(context.Background(), testWaypoints(), plan.Trekking)
	require.NoError(t, err)
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

			_, routeErr := client.Route(context.Background(), testWaypoints(), plan.Trekking)
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

	_, routeErr := client.Route(context.Background(), testWaypoints(), plan.Trekking)
	require.Error(t, routeErr)
	var brouterErr *Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, FailureUnreachable, brouterErr.Category, "Category")
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

	_, routeErr := client.Route(context.Background(), []plan.Waypoint{{Longitude: 8.68, Latitude: 50.11}}, plan.Trekking)
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

	_, routeErr := client.Route(context.Background(), testWaypoints(), plan.Trekking)
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

	_, routeErr := client.Route(context.Background(), testWaypoints(), plan.Trekking)
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

	_, routeErr := client.Route(context.Background(), testWaypoints(), plan.Trekking)
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

	_, routeErr := client.Route(context.Background(), testWaypoints(), plan.Trekking)
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
