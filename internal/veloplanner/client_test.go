package veloplanner

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientInventoryUsesFreshAuthenticatedSession(t *testing.T) {
	var loginCount atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/login":
			writeBody(t, writer, `<input value="csrf-token" name="_csrf_token" type="hidden">`)
		case request.Method == http.MethodPost && request.URL.Path == "/login":
			if err := request.ParseForm(); err != nil {
				t.Errorf("ParseForm() error = %v", err)
				return
			}
			if got, want := request.Form.Get("_csrf_token"), "csrf-token"; got != want {
				t.Errorf("csrf token = %q, want %q", got, want)
			}
			if got, want := request.Form.Get("user[email]"), "rider@example.test"; got != want {
				t.Errorf("email = %q, want %q", got, want)
			}
			if got, want := request.Form.Get("user[password]"), "test-password"; got != want {
				t.Errorf("password = %q, want %q", got, want)
			}
			http.SetCookie(writer, authenticatedCookie())
			loginCount.Add(1)
			writeBody(t, writer, `isUserLoggedIn: true, userId: 42`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/internal/users/42/routes":
			requireSessionCookie(t, request)
			if got, want := request.URL.Query().Get("page"), "1"; got != want {
				t.Errorf("page = %q, want %q", got, want)
			}
			writeBody(t, writer, `{"data":[{"id":100}],"metadata":{"page":1,"total_pages":1,"total_count":1}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/internal/user_routes/100":
			requireSessionCookie(t, request)
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
			t.Errorf("unexpected request: %s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	stages, err := client.Inventory(t.Context())
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	if got, want := len(stages), 1; got != want {
		t.Fatalf("len(Inventory()) = %d, want %d", got, want)
	}

	stage := stages[0]
	if got, want := stage.Key().ExternalID(), "domestique:veloplanner:100:stage:1"; got != want {
		t.Errorf("ExternalID() = %q, want %q", got, want)
	}
	if got, want := stage.Title(), "Morning route"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if got := stage.ContentHash(); len(got) != 64 {
		t.Errorf("ContentHash() length = %d, want 64", len(got))
	}

	geometry := stage.Geometry()
	if got, want := len(geometry), 3; got != want {
		t.Fatalf("geometry length = %d, want %d", got, want)
	}
	for index, expected := range []struct {
		longitude float64
		latitude  float64
		elevation float64
	}{
		{longitude: 8.40, latitude: 49.00, elevation: 100},
		{longitude: 8.41, latitude: 49.01, elevation: 101},
		{longitude: 8.42, latitude: 49.02, elevation: 102},
	} {
		if got, want := geometry[index].Longitude, expected.longitude; got != want {
			t.Errorf("geometry[%d].Longitude = %v, want %v", index, got, want)
		}
		if got, want := geometry[index].Latitude, expected.latitude; got != want {
			t.Errorf("geometry[%d].Latitude = %v, want %v", index, got, want)
		}
		if geometry[index].Elevation == nil {
			t.Errorf("geometry[%d].Elevation = nil", index)
		} else if got, want := *geometry[index].Elevation, expected.elevation; got != want {
			t.Errorf("geometry[%d].Elevation = %v, want %v", index, got, want)
		}
	}

	if _, err := client.Inventory(t.Context()); err != nil {
		t.Fatalf("second Inventory() error = %v", err)
	}
	if got, want := loginCount.Load(), int32(2); got != want {
		t.Errorf("login count = %d, want %d", got, want)
	}
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
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("Inventory() error = %v, want ErrAuthentication", err)
	}
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
	if err == nil || !strings.Contains(err.Error(), "decoded point") {
		t.Fatalf("Inventory() error = %v, want malformed geometry error", err)
	}
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
			if _, err := New(&test.options); err == nil {
				t.Fatal("New() error = nil, want an error")
			}
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
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
			requireSessionCookie(t, request)
			writeBody(t, writer, `{"data":[{"id":100}],"metadata":{"page":1,"total_pages":1,"total_count":1}}`)
		case "/api/internal/user_routes/100":
			requireSessionCookie(t, request)
			writeBody(t, writer, detail)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

func requireSessionCookie(t *testing.T, request *http.Request) {
	t.Helper()
	cookie, err := request.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		t.Errorf("request did not carry the VeloPlanner session cookie")
	}
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
	if _, err := io.WriteString(writer, body); err != nil {
		t.Errorf("writing test response: %v", err)
	}
}
