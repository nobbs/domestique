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
// to this surface belongs here, and the provenance tests below cover it. The
// task routes carry the names a real build registers, percent-encoded as the
// page sends them, so the guard is checked over the requests it actually sees.
var mutableRoutes = []struct { //nolint:gochecknoglobals // test fixture, read-only
	method string
	target string
	body   string
}{
	{method: http.MethodPut, target: "/v1/tasks/sync%3Asource/schedule", body: `{"enabled":true}`},
	{method: http.MethodPost, target: "/v1/routes/12/stages/1/reprocess"},
	{method: http.MethodPut, target: settingsWahooPath, body: wahooSubmission},
	{method: http.MethodPut, target: settingsKomootPath, body: komootSubmission},
	{method: http.MethodPut, target: settingsNotificationsPath, body: notificationsSubmission},
	{method: http.MethodPut, target: basemapsPath, body: basemapsSubmission},
	{method: http.MethodPut, target: settingsSurfacePath, body: surfaceSubmission},
	{method: http.MethodPut, target: settingsSyncPath, body: syncSubmission},
	{method: http.MethodPut, target: settingsAlertsPath, body: alertsSubmission},
	{method: http.MethodPut, target: settingsTimezonePath, body: `{"timezone": "Europe/Berlin"}`},
	{method: http.MethodPost, target: "/v1/tasks/sync%3Asource/run"},
	{method: http.MethodPost, target: "/v1/tasks/sync%3Atarget/run/rider-a"},
}

// askedTasks is what the handler's task list was asked for, so a refused
// request can be shown to have reached nothing behind the gate.
func askedTasks(t *testing.T, handler *Handler) []startedTask {
	t.Helper()
	tasks, ok := handler.tasks.(*fakeTasks)
	require.True(t, ok, "the handler was not built over a fake task list")

	return tasks.asked
}

// scheduledTasks is what the handler was told to switch, so a refused request
// can be shown to have written no schedule. The schedule is held by the task
// layer rather than the store, so watching the store would see nothing either
// way.
func scheduledTasks(t *testing.T, handler *Handler) []scheduledTask {
	t.Helper()
	tasks, ok := handler.tasks.(*fakeTasks)
	require.True(t, ok, "the handler was not built over a fake task list")

	return tasks.scheduled
}

// decidedAlerts is what the handler's matrix was told, so a refused request can
// be shown to have decided nothing.
func decidedAlerts(t *testing.T, handler *Handler) []AlertDecision {
	t.Helper()
	alerts, ok := handler.alerts.(*fakeAlerts)
	require.True(t, ok, "the handler was not built over a fake alert matrix")

	return alerts.decided
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
				withSession(request)
				if origin != "" {
					request.Header.Set("Origin", origin)
				}

				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)

				assert.Equalf(t, http.StatusForbidden, response.Code, "%s %s", route.method, route.target)
				assert.Zerof(t, trigger.calls, "%s %s started a run", route.method, route.target)
				assert.Emptyf(t, state.reprocessed, "%s %s reprocessed a stage", route.method, route.target)
				assert.Emptyf(t, decidedAlerts(t, handler), "%s %s decided an alert", route.method, route.target)
				assert.Emptyf(t, askedTasks(t, handler), "%s %s reached the task layer", route.method, route.target)
				assert.Emptyf(t, scheduledTasks(t, handler), "%s %s wrote a schedule", route.method, route.target)
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
		withSession(request)
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

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/tasks/sync/run", http.NoBody)
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
		withSession(request)
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
		"/v1/providers/veloplanner/sourceRoutes/12/routes/1",
		"/v1/providers/veloplanner/sourceRoutes/12/routes/1/geometry",
		"/v1/webui/config",
	} {
		handler := newHandler(t, &fakeOAuth{}, surfaceState())
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		withSession(request)

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
			Alerts:   &fakeAlerts{},
			Tasks:    &fakeTasks{},
			Settings: settingsWith(testBasemaps()),
			Sessions: newFakeSessions(),
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.Error(t, err, "New() built a handler with no origin to compare against")
}
