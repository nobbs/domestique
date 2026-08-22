package veloplanner

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestClientProviderIsVeloPlanner(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://example.invalid", Email: []byte("rider@example.invalid"), Password: []byte("secret")})
	require.NoError(t, err, "New()")

	assert.Equal(t, route.ProviderVeloPlanner, client.Provider(), "Provider()")
}

func TestClientInventoryUsesFreshAuthenticatedSession(t *testing.T) {
	var loginCount atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/login":
			writeBody(t, writer, `<input value="csrf-token" name="_csrf_token" type="hidden">`)
		case request.Method == http.MethodPost && request.URL.Path == "/login":
			if !assert.NoError(t, request.ParseForm()) {
				return
			}
			assert.Equal(t, "csrf-token", request.Form.Get("_csrf_token"), "csrf token")
			assert.Equal(t, "rider@example.test", request.Form.Get("user[email]"), "email")
			assert.Equal(t, "test-password", request.Form.Get("user[password]"), "password")
			http.SetCookie(writer, authenticatedCookie())
			loginCount.Add(1)
			writeBody(t, writer, `isUserLoggedIn: true, userId: 42`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/internal/users/42/routes":
			assertSessionCookie(t, request)
			assert.Equal(t, "1", request.URL.Query().Get("page"), "page")
			writeBody(t, writer, `{"data":[{"id":100}],"metadata":{"page":1,"total_pages":1,"total_count":1}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/internal/user_routes/100":
			assertSessionCookie(t, request)
			writeBody(t, writer, `{
                "data": {
                    "id": 100,
                    "name": "Morning route",
                    "updated_at": "2026-08-17T07:00:00",
                    "route_state": {
                        "stages": [{
                            "order": 1,
                            "name": "ignored for single stage",
                            "segments": [
                                {"startPointIndex": 1, "endPointIndex": 2, "path": {"decodedPoints": [[8.41, 49.01, 101], [8.42, 49.02, 102]]}},
                                {"startPointIndex": 0, "endPointIndex": 1, "path": {"decodedPoints": [[8.40, 49.00, 100], [8.41, 49.01, 101]]}}
                            ]
                        }]
                    }
                }
            }`)
		default:
			assert.Failf(t, "unexpected request", "%s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	stages, err := client.Inventory(t.Context())
	require.NoError(t, err)
	require.Len(t, stages, 1)

	stage := stages[0]
	assert.Equal(t, "domestique:veloplanner:100:stage:1", stage.Key().ExternalID())
	assert.Equal(t, "Morning route", stage.Title())
	assert.Len(t, stage.ContentHash(), 64, "the content hash is a hex SHA-256 digest")

	geometry := stage.Geometry()
	require.Len(t, geometry, 3, "the segments were not stitched into one line")
	for index, expected := range []struct {
		longitude float64
		latitude  float64
		elevation float64
	}{
		{longitude: 8.40, latitude: 49.00, elevation: 100},
		{longitude: 8.41, latitude: 49.01, elevation: 101},
		{longitude: 8.42, latitude: 49.02, elevation: 102},
	} {
		point := geometry[index]
		assert.InDelta(t, expected.longitude, point.Longitude, 1e-9, "geometry[%d].Longitude", index)
		assert.InDelta(t, expected.latitude, point.Latitude, 1e-9, "geometry[%d].Latitude", index)
		if assert.NotNilf(t, point.Elevation, "geometry[%d].Elevation", index) {
			assert.InDelta(t, expected.elevation, *point.Elevation, 1e-9, "geometry[%d].Elevation", index)
		}
	}

	_, err = client.Inventory(t.Context())
	require.NoError(t, err, "second Inventory()")
	assert.Equal(t, int32(2), loginCount.Load(), "each run must authenticate its own session")
}

func TestClientInventoryRejectsUnauthenticatedLogin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login":
			if request.Method == http.MethodGet {
				writeBody(t, writer, `<input name="_csrf_token" value="csrf-token">`)
				return
			}
			writeBody(t, writer, `isUserLoggedIn: false, userId: -42`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorIs(t, err, ErrAuthentication)
}

func TestClientInventoryRejectsMalformedGeometry(t *testing.T) {
	server := authenticatedRouteServer(t, `{
        "data": {
            "id": 100,
            "name": "Route",
            "updated_at": "2026-08-17T07:00:00",
            "route_state": {"stages": [{
                "order": 1,
                "segments": [{"path": {"decodedPoints": [[8.4]]}}]
            }]}
        }
    }`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "decoded point")
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{
			name:    "non HTTPS base URL",
			options: Options{BaseURL: "http://veloplanner.example.test", Email: []byte("email"), Password: []byte("password")},
		},
		{
			name:    "missing email",
			options: Options{BaseURL: "https://veloplanner.example.test", Password: []byte("password")},
		},
		{
			name:    "missing password",
			options: Options{BaseURL: "https://veloplanner.example.test", Email: []byte("email")},
		},
		{
			name:    "negative timeout",
			options: Options{BaseURL: "https://veloplanner.example.test", Email: []byte("email"), Password: []byte("password"), Timeout: -time.Second},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(&test.options)
			require.Error(t, err)
		})
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:   server.URL,
		Email:     []byte("rider@example.test"),
		Password:  []byte("test-password"),
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	return client
}

func authenticatedRouteServer(t *testing.T, detail string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login":
			if request.Method == http.MethodGet {
				writeBody(t, writer, `<input name="_csrf_token" value="csrf-token">`)
				return
			}
			http.SetCookie(writer, authenticatedCookie())
			writeBody(t, writer, `isUserLoggedIn: true, userId: 42`)
		case "/api/internal/users/42/routes":
			assertSessionCookie(t, request)
			writeBody(t, writer, `{"data":[{"id":100}],"metadata":{"page":1,"total_pages":1,"total_count":1}}`)
		case "/api/internal/user_routes/100":
			assertSessionCookie(t, request)
			writeBody(t, writer, detail)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

// assertSessionCookie runs on the server's goroutine, so it asserts rather than
// requires: FailNow off the test goroutine would leave the request hanging.
func assertSessionCookie(t *testing.T, request *http.Request) {
	t.Helper()
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		assert.Failf(t, "request did not carry the VeloPlanner session cookie", "%v", err)

		return
	}
	assert.NotEmpty(t, cookie.Value, "the VeloPlanner session cookie was empty")
}

func authenticatedCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    "authenticated",
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

func writeBody(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	_, err := io.WriteString(writer, body)
	assert.NoError(t, err, "writing the test response")
}
