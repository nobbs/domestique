package wahoo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

func TestClientCompletesOAuthAndFindsAuthenticatedUser(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/oauth/token":
			if request.Method != http.MethodPost {
				t.Errorf("token method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("ParseForm() error = %v", err)
				return
			}
			if got, want := request.Form.Get("client_id"), "client-id"; got != want {
				t.Errorf("client id = %q, want %q", got, want)
			}
			if got, want := request.Form.Get("client_secret"), "test-client-secret"; got != want {
				t.Errorf("client secret = %q, want %q", got, want)
			}
			if got, want := request.Form.Get("code"), "authorization-code"; got != want {
				t.Errorf("code = %q, want %q", got, want)
			}
			writeJSON(t, writer, map[string]string{"access_token": "access-token", "refresh_token": "refresh-token"})
		case "/v1/user":
			if got, want := request.Header.Get("Authorization"), "Bearer access-token"; got != want {
				t.Errorf("authorization = %q, want %q", got, want)
			}
			writeJSON(t, writer, map[string]int64{"id": 42})
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	authorizationURL, err := client.AuthorizationURL("state-value")
	if err != nil {
		t.Fatalf("AuthorizationURL() error = %v", err)
	}
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatalf("parsing authorization URL: %v", err)
	}
	if got, want := parsed.Path, "/oauth/authorize"; got != want {
		t.Errorf("authorization path = %q, want %q", got, want)
	}
	if got, want := parsed.Query().Get("scope"), "routes_read routes_write user_read"; got != want {
		t.Errorf("authorization scope = %q, want %q", got, want)
	}
	if got, want := parsed.Query().Get("state"), "state-value"; got != want {
		t.Errorf("authorization state = %q, want %q", got, want)
	}

	accessToken, refreshToken, err := client.ExchangeAuthorizationCode(t.Context(), "authorization-code")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	if got, want := refreshToken, "refresh-token"; got != want {
		t.Errorf("refresh token = %q, want %q", got, want)
	}
	userID, err := client.AuthenticatedUser(t.Context(), accessToken)
	if err != nil {
		t.Fatalf("AuthenticatedUser() error = %v", err)
	}
	if got, want := userID, "42"; got != want {
		t.Errorf("user id = %q, want %q", got, want)
	}
}

func TestClientWritesAndFindsOwnedRoute(t *testing.T) {
	stage := testStage(t)
	externalID := stage.Key().ExternalID()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/routes":
			switch request.Method {
			case http.MethodPost:
				requireRouteForm(t, request, externalID, true)
				writeJSON(t, writer, map[string]any{"id": 51, "external_id": externalID})
			case http.MethodGet:
				if got, want := request.URL.Query().Get("external_id"), externalID; got != want {
					t.Errorf("external id query = %q, want %q", got, want)
				}
				writeJSON(t, writer, []map[string]any{{"id": 51, "external_id": externalID}})
			default:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "/v1/routes/51":
			switch request.Method {
			case http.MethodPut:
				requireRouteForm(t, request, externalID, false)
				writeJSON(t, writer, map[string]any{"id": 51, "external_id": externalID})
			case http.MethodDelete:
				writer.WriteHeader(http.StatusNoContent)
			default:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	createdRouteID, err := client.CreateRoute(t.Context(), "access-token", &stage, []byte("fit-data"))
	if err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if got, want := createdRouteID, int64(51); got != want {
		t.Errorf("created route id = %d, want %d", got, want)
	}
	if _, updateErr := client.UpdateRoute(t.Context(), createdRouteID, "access-token", &stage, []byte("new-fit-data")); updateErr != nil {
		t.Fatalf("UpdateRoute() error = %v", updateErr)
	}
	foundRouteID, found, err := client.RouteByExternalID(t.Context(), "access-token", externalID)
	if err != nil {
		t.Fatalf("RouteByExternalID() error = %v", err)
	}
	if !found {
		t.Fatal("RouteByExternalID() found = false, want true")
	}
	if got, want := foundRouteID, int64(51); got != want {
		t.Errorf("found route id = %d, want %d", got, want)
	}
	if err := client.DeleteRoute(t.Context(), createdRouteID, "access-token"); err != nil {
		t.Fatalf("DeleteRoute() error = %v", err)
	}
}

func TestClientWaitsForAdvertisedRateLimit(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writer.Header().Set("X-RateLimit-Remaining", "10, 5, 0")
			writer.Header().Set("X-RateLimit-Reset", "5")
		}
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	var waited time.Duration
	client.wait = func(_ context.Context, duration time.Duration) error {
		waited = duration
		now = now.Add(duration)
		return nil
	}

	if _, err := client.AuthenticatedUser(t.Context(), "access-token"); err != nil {
		t.Fatalf("first AuthenticatedUser() error = %v", err)
	}
	if _, err := client.AuthenticatedUser(t.Context(), "access-token"); err != nil {
		t.Fatalf("second AuthenticatedUser() error = %v", err)
	}
	if got, want := waited, 5*time.Second; got != want {
		t.Errorf("rate limit wait = %s, want %s", got, want)
	}
}

func TestClientClassifiesRejectedRefreshToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).RefreshAccessToken(t.Context(), "refresh-token")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("RefreshAccessToken() error = %v, want ErrUnauthorized", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		APIBaseURL:   server.URL,
		OAuthBaseURL: server.URL,
		ClientID:     "client-id",
		RedirectURL:  "https://pi.example.ts.net/oauth/wahoo/callback",
		ClientSecret: []byte("test-client-secret"),
		Timeout:      time.Second,
		Transport:    server.Client().Transport,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return client
}

func testStage(t *testing.T) route.Stage {
	t.Helper()
	elevation := 100.0
	stage, err := route.NewStage(
		100,
		1,
		"2026-08-17T07:00:00Z",
		"Morning route",
		"",
		[]route.Point{
			{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
			{Longitude: 8.401, Latitude: 49.001, Elevation: &elevation},
		},
		"hash",
	)
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	return stage
}

func requireRouteForm(t *testing.T, request *http.Request, externalID string, expectExternalID bool) {
	t.Helper()
	if err := request.ParseForm(); err != nil {
		t.Errorf("ParseForm() error = %v", err)
		return
	}
	if got := request.Form.Get("route[file]"); !strings.HasPrefix(got, "data:application/vnd.fit;base64,") {
		t.Errorf("route file = %q, want FIT data URI", got)
	}
	if got, want := request.Form.Get("route[name]"), "Morning route"; got != want {
		t.Errorf("route name = %q, want %q", got, want)
	}
	if got := request.Form.Get("route[distance]"); got == "" || got == "0" {
		t.Errorf("route distance = %q, want a positive value", got)
	}
	if expectExternalID {
		if got, want := request.Form.Get("route[external_id]"), externalID; got != want {
			t.Errorf("route external id = %q, want %q", got, want)
		}
	} else if got := request.Form.Get("route[external_id]"); got != "" {
		t.Errorf("update external id = %q, want empty", got)
	}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("writing JSON response: %v", err)
	}
}
