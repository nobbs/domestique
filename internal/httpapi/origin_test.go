package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mutableRoutes is every route that starts a run or writes state. A route added
// to this surface belongs here, and the provenance tests below cover it.
var mutableRoutes = []struct { //nolint:gochecknoglobals // test fixture, read-only
	method string
	target string
	body   string
}{
	{method: http.MethodPost, target: "/v1/sync"},
	{method: http.MethodPost, target: "/v1/sync/source"},
	{method: http.MethodPost, target: "/v1/sync/targets"},
	{method: http.MethodPut, target: "/v1/sync/schedule", body: `{"source":true,"targets":true}`},
	{method: http.MethodPost, target: "/v1/routes/12/stages/1/reprocess"},
}

// An Access session lives in an ordinary browser, so identity alone would let a
// page on any other site start a run in it. Every one of these requests carries
// a valid assertion and is still refused, and — the part that matters — neither
// the trigger nor the store is touched on the way out.
func TestMutableRoutesRejectForeignProvenance(t *testing.T) {
	origins := map[string]string{
		"absent":     "",
		"cross site": "https://evil.example.test",
		"opaque":     "null",
		"prefix":     "https://domestique.example.test.evil.example.test",
		"plaintext":  "http://domestique.example.test",
		"port":       "https://domestique.example.test:8443",
	}

	for name, origin := range origins {
		t.Run(name, func(t *testing.T) {
			for _, route := range mutableRoutes {
				trigger := &fakeSync{accepted: true}
				state := surfaceState()
				handler := newHandlerWithSync(t, &fakeOAuth{}, state, trigger)

				request := httptest.NewRequestWithContext(
					t.Context(), route.method, route.target, strings.NewReader(route.body),
				)
				request.Header.Set(assertionHeader, testAssertion)
				if origin != "" {
					request.Header.Set("Origin", origin)
				}

				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)

				assert.Equalf(t, http.StatusForbidden, response.Code, "%s %s", route.method, route.target)
				assert.Zerof(t, trigger.calls, "%s %s started a run", route.method, route.target)
				assert.Zerof(t, state.scheduleWrites, "%s %s wrote the schedule", route.method, route.target)
				assert.Emptyf(t, state.reprocessed, "%s %s reprocessed a stage", route.method, route.target)
			}
		})
	}
}

// The same requests, from the UI's own origin, must behave exactly as they did
// before the guard existed.
func TestMutableRoutesAcceptTheBrowserOrigin(t *testing.T) {
	for _, route := range mutableRoutes {
		trigger := &fakeSync{accepted: true}
		handler := newHandlerWithSync(t, &fakeOAuth{}, surfaceState(), trigger)

		request := httptest.NewRequestWithContext(
			t.Context(), route.method, route.target, strings.NewReader(route.body),
		)
		request.Header.Set(assertionHeader, testAssertion)
		request.Header.Set("Origin", testBrowserOrigin)

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		assert.NotEqualf(t, http.StatusForbidden, response.Code,
			"%s %s was refused, want the route's own answer", route.method, route.target)
	}
}

// An unverified caller learns nothing more from a mutable route than from any
// other: identity is settled before provenance, so a bad origin and a bad
// assertion are answered as the missing identity, not as a failed origin check.
func TestIdentityIsSettledBeforeProvenance(t *testing.T) {
	handler := newTestHandler(t)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/sync", http.NoBody)
	request.Header.Set("Origin", "https://evil.example.test")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// The OAuth callback is a cross-site GET the browser is redirected into. It is
// protected by its one-time, identity-bound, expiring state, and wrapping it in
// a provenance check would break the flow it is there to complete.
func TestOAuthRoutesStayOutsideTheProvenanceCheck(t *testing.T) {
	for _, target := range []string{"/oauth/wahoo/start/rider-a", "/oauth/wahoo/callback?state=s&code=c"} {
		handler := newTestHandler(t)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		request.Header.Set(assertionHeader, testAssertion)
		request.Header.Set("Origin", "https://api.wahooligan.example.test")

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		assert.NotEqualf(t, http.StatusForbidden, response.Code, "%s was refused, want the flow to proceed", target)
	}
}

// A read-only route carries no side effect to protect, and a browser sends no
// Origin on a GET, so requiring one there would refuse the whole UI.
func TestReadOnlyRoutesDoNotRequireAnOrigin(t *testing.T) {
	for _, target := range []string{
		"/v1/status",
		"/v1/routes",
		"/v1/providers/veloplanner/routes/12/stages/1",
		"/v1/providers/veloplanner/routes/12/stages/1/geometry",
		"/v1/webui/config",
	} {
		handler := newHandler(t, &fakeOAuth{}, surfaceState())
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		request.Header.Set(assertionHeader, testAssertion)

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		assert.Equalf(t, http.StatusOK, response.Code, "%s", target)
	}
}

// The origin is compared as a browser writes it, so the configured URL is
// reduced to a scheme and host first.
func TestBrowserOriginNormalisation(t *testing.T) {
	valid := map[string]string{
		"callback URL":  "https://domestique.example.test/oauth/wahoo/callback",
		"bare origin":   "https://domestique.example.test",
		"mixed case":    "https://Domestique.Example.Test/oauth/wahoo/callback",
		"default port":  "https://domestique.example.test:443/oauth/wahoo/callback",
		"surrounded":    "  https://domestique.example.test/oauth/wahoo/callback  ",
		"explicit port": "https://domestique.example.test:8443",
	}
	want := map[string]string{
		"callback URL":  "https://domestique.example.test",
		"bare origin":   "https://domestique.example.test",
		"mixed case":    "https://domestique.example.test",
		"default port":  "https://domestique.example.test",
		"surrounded":    "https://domestique.example.test",
		"explicit port": "https://domestique.example.test:8443",
	}

	for name, value := range valid {
		t.Run(name, func(t *testing.T) {
			got, err := browserOriginOf(value)
			require.NoErrorf(t, err, "browserOriginOf(%q)", value)
			assert.Equalf(t, want[name], got, "browserOriginOf(%q)", value)
		})
	}

	// A service with no origin to compare against has no guard, so it must not
	// start rather than fall back to accepting anything.
	for name, value := range map[string]string{
		"empty":     "",
		"plaintext": "http://domestique.example.test",
		"relative":  "/oauth/wahoo/callback",
		"no host":   "https://",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := browserOriginOf(value)
			require.Errorf(t, err, "browserOriginOf(%q) accepted an origin it cannot compare against", value)
		})
	}
}

// Without a configured origin there is nothing to compare a state-changing
// request against, so the handler must refuse to exist.
func TestNewRequiresABrowserOrigin(t *testing.T) {
	_, err := New(
		&Options{
			TargetIDs:      []string{"rider-a"},
			Basemaps:       testBasemaps(),
			AccessVerifier: &recordingVerifier{email: testAccessEmail},
			AccessEmail:    testAccessEmail,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.Error(t, err, "New() built a handler with no origin to compare against")
}
