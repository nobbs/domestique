package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const (
	testTileStyleURL     = "https://tiles.example.test/styles/liberty"
	testTileStyleURLDark = "https://tiles.example.test/styles/liberty-dark"
	testSourceBaseURL    = "https://source.example.test"
	// The Wahoo redirect URL the composition root passes, and the origin a
	// browser derives from it for the requests it sends this service.
	testBrowserOriginURL = "https://domestique.example.test/oauth/wahoo/callback"
	testBrowserOrigin    = "https://domestique.example.test"
)

func TestHandlerGatesStateAndKeepsHealthLocal(t *testing.T) {
	handler := newTestHandler(t)
	health := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, health)
	if got, want := healthResponse.Code, http.StatusOK; got != want {
		t.Errorf("health status = %d, want %d", got, want)
	}

	status := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/status", http.NoBody)
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	if got, want := statusResponse.Code, http.StatusUnauthorized; got != want {
		t.Errorf("unauthenticated status = %d, want %d", got, want)
	}

	status.Header.Set(assertionHeader, testAssertion)
	statusResponse = httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	if got, want := statusResponse.Code, http.StatusOK; got != want {
		t.Errorf("authenticated status = %d, want %d", got, want)
	}
	if body := statusResponse.Body.String(); strings.Contains(body, "private-token") || !strings.Contains(body, "authorised") {
		t.Errorf("status body = %q, want safe authorization state", body)
	}
}

// Every route added for the browser UI must sit behind the same identity gate.
func TestHandlerGatesEveryNonHealthRoute(t *testing.T) {
	handler := newTestHandler(t)
	foreign := newHandlerWithVerifier(t, &recordingVerifier{email: "someone-else@example.com"})
	paths := []string{
		"/v1/status",
		"/v1/routes",
		"/v1/routes/1/stages/1",
		"/v1/routes/1/stages/1/geometry",
		"/v1/webui/config",
		"/oauth/wahoo/start/rider-a",
		"/oauth/wahoo/callback",
		"/assets/app-abc123.js",
		"/favicon.svg",
		"/",
		"/routes/1/1",
		"/unknown",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequestWithContext(
				t.Context(), http.MethodGet, path, http.NoBody))
			if got, want := response.Code, http.StatusUnauthorized; got != want {
				t.Errorf("unauthenticated %s = %d, want %d", path, got, want)
			}

			forbiddenResponse := httptest.NewRecorder()
			foreign.ServeHTTP(forbiddenResponse, authenticatedRequest(http.MethodGet, path))
			if got, want := forbiddenResponse.Code, http.StatusForbidden; got != want {
				t.Errorf("wrong identity %s = %d, want %d", path, got, want)
			}
		})
	}
}

func TestHandlerServesStageGeometryAsGeoJSON(t *testing.T) {
	state := &fakeState{
		summaries: []route.Summary{{
			RouteID: 12, StageOrder: 1, RouteName: "Alpine loop", StageName: "Descent",
			SourceRevision: "revision", ContentHash: "hash", PointCount: 2, DistanceMetres: 1234.5,
			Bounds: route.Bounds{MinLongitude: 8.4, MinLatitude: 49.0, MaxLongitude: 8.5, MaxLatitude: 49.2},
		}},
		coordinates: json.RawMessage(`[[8.4,49],[8.5,49.2]]`),
	}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("geometry status = %d, want %d", got, want)
	}

	var view geometryView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding geometry = %v", err)
	}
	if got, want := view.Type, "Feature"; got != want {
		t.Errorf("type = %q, want %q", got, want)
	}
	if got, want := view.Geometry.Type, "LineString"; got != want {
		t.Errorf("geometry type = %q, want %q", got, want)
	}
	if got, want := string(view.Geometry.Coordinates), `[[8.4,49],[8.5,49.2]]`; got != want {
		t.Errorf("coordinates = %s, want %s", got, want)
	}
	if got, want := view.Properties.Title, "Alpine loop — Descent"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if want := []float64{8.4, 49.0, 8.5, 49.2}; len(view.BBox) != 4 {
		t.Errorf("bbox = %v, want %v", view.BBox, want)
	}
}

func TestHandlerServesTheStoredSurfaceWithGeometry(t *testing.T) {
	state := surfaceState()
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("geometry status = %d, want %d", got, want)
	}

	var view geometryView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding geometry = %v", err)
	}
	if view.Properties.Surface == nil {
		t.Fatalf("geometry omitted the stored surface")
	}
	if got, want := string(view.Properties.Surface.Ranges), string(state.surfaceRanges); got != want {
		t.Errorf("surface ranges = %s, want %s", got, want)
	}
	if got, want := view.Properties.Surface.MatchedMetres, 1234.5; got != want {
		t.Errorf("matched metres = %v, want %v", got, want)
	}
}

// A stage nobody has surveyed is still a stage that was asked about. It is
// served as a present surface whose ranges cover the line as unsurveyed and
// whose matched length is zero, because that is what tells a client the question
// was answered — an absent surface would say it never was.
func TestHandlerServesAnUnsurveyedSurfaceAsClassified(t *testing.T) {
	state := surfaceState()
	state.surfaceRanges = json.RawMessage(`[{"kind":"unknown","start_index":0,"end_index":1}]`)
	state.surfaceMetres = 0
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("geometry status = %d, want %d", got, want)
	}

	var view geometryView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding geometry = %v", err)
	}
	if view.Properties.Surface == nil {
		t.Fatalf("geometry omitted a classification that matched nothing")
	}
	if got, want := string(view.Properties.Surface.Ranges), string(state.surfaceRanges); got != want {
		t.Errorf("surface ranges = %s, want %s", got, want)
	}
	if got := view.Properties.Surface.MatchedMetres; got != 0 {
		t.Errorf("matched metres = %v, want 0", got)
	}
}

// A classification is a set of positions in one stored coordinate array, so one
// measured against an earlier plan of the same stage must not be served against
// the current line.
func TestHandlerOmitsASurfaceMeasuredAgainstOtherGeometry(t *testing.T) {
	state := surfaceState()
	state.surfaceHash = "earlier-hash"
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("geometry status = %d, want %d", got, want)
	}
	if body := response.Body.String(); strings.Contains(body, "surface") {
		t.Errorf("geometry carried a stale surface: %q", body)
	}
}

func TestHandlerReportsUnreadableSurfaceStateAsUnavailable(t *testing.T) {
	state := surfaceState()
	state.surfaceErr = errors.New("state unavailable")
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	if got, want := response.Code, http.StatusInternalServerError; got != want {
		t.Errorf("geometry status = %d, want %d", got, want)
	}
}

// surfaceState holds one stage whose surface has been classified against the
// geometry that is stored for it.
func surfaceState() *fakeState {
	return &fakeState{
		summaries: []route.Summary{{
			RouteID: 12, StageOrder: 1, RouteName: "Alpine loop", StageName: "Descent",
			SourceRevision: "revision", ContentHash: "hash", PointCount: 2,
			Bounds: route.Bounds{MinLongitude: 8.4, MinLatitude: 49.0, MaxLongitude: 8.5, MaxLatitude: 49.2},
		}},
		coordinates:   json.RawMessage(`[[8.4,49],[8.5,49.2]]`),
		surfaceRanges: json.RawMessage(`[{"kind":"asphalt","start_index":0,"end_index":1}]`),
		surfaceHash:   "hash",
		surfaceMetres: 1234.5,
	}
}

func TestHandlerReportsMissingGeometryAsNotFound(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/99/stages/1/geometry"))
	if got, want := response.Code, http.StatusNotFound; got != want {
		t.Errorf("geometry status = %d, want %d", got, want)
	}
}

func TestHandlerRejectsMalformedStageIdentifiers(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{
		"/v1/routes/0/stages/1",
		"/v1/routes/-1/stages/1/geometry",
		"/v1/routes/abc/stages/1",
		"/v1/routes/1/stages/0/geometry",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, path))
			if got, want := response.Code, http.StatusNotFound; got != want {
				t.Errorf("status = %d, want %d", got, want)
			}
		})
	}
}

func TestHandlerListsStagesWithoutGeometry(t *testing.T) {
	state := &fakeState{summaries: []route.Summary{{
		RouteID: 3, StageOrder: 1, RouteName: "Sunday", PointCount: 2, DistanceMetres: 900,
	}}}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("routes status = %d, want %d", got, want)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"title":"Sunday"`) {
		t.Errorf("routes body = %q, want the stage title", body)
	}
	if strings.Contains(body, "coordinates") {
		t.Errorf("routes body leaked geometry: %q", body)
	}
}

func TestHandlerServesTileStyleConfiguration(t *testing.T) {
	handler := newTestHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("config status = %d, want %d", got, want)
	}
	body := response.Body.String()
	// Both styles, because the page picks between them: the colour scheme is a
	// property of the browser, and this response is cached for the session. The
	// provider base URL rides along, because the link back to a stage's source
	// route is built from it.
	for _, want := range []string{testTileStyleURL, testTileStyleURLDark, testSourceBaseURL} {
		if !strings.Contains(body, want) {
			t.Errorf("config body = %q, want it to contain %q", body, want)
		}
	}
}

func TestHandlerOmitsAnUnconfiguredDarkTileStyle(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// Absent rather than empty: that is how the page knows to keep one style in
	// both colour schemes.
	if got := response.Body.String(); strings.Contains(got, "tile_style_url_dark") {
		t.Errorf("config body = %q, want no dark style key", got)
	}
}

func TestHandlerOmitsAnUnconfiguredSourceBaseURL(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// Absent rather than empty: that is how the page knows to offer no source
	// link at all rather than one pointing at nowhere.
	if got := response.Body.String(); strings.Contains(got, "source_base_url") {
		t.Errorf("config body = %q, want no source base URL key", got)
	}
}

func TestHandlerRefusesASourceBaseURLThatIsNotOne(t *testing.T) {
	for _, value := range []string{"http://source.example.test", "source.example.test", "/user-routes"} {
		_, err := New(
			&Options{
				TargetIDs:        []string{"rider-a"},
				TileStyleURL:     testTileStyleURL,
				SourceBaseURL:    value,
				AccessVerifier:   &recordingVerifier{email: testAccessEmail},
				AccessEmail:      testAccessEmail,
				BrowserOriginURL: testBrowserOriginURL,
			},
			&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{},
		)
		if err == nil {
			t.Errorf("New() with source base URL %q error = nil, want an error", value)
		}
	}
}

func TestHandlerSetsPolicyAndCacheHeaders(t *testing.T) {
	handler := newTestHandler(t)

	api := httptest.NewRecorder()
	handler.ServeHTTP(api, authenticatedRequest(http.MethodGet, "/v1/status"))
	policy := api.Header().Get("Content-Security-Policy")
	// worker-src must allow 'self': MapLibre loads its worker from a bundled
	// same-origin module, and a blob-only policy blocks the map from rendering.
	for _, want := range []string{
		"default-src 'self'",
		"frame-ancestors 'none'",
		"worker-src 'self' blob:",
		"https://tiles.example.test",
	} {
		if !strings.Contains(policy, want) {
			t.Errorf("CSP = %q, want it to contain %q", policy, want)
		}
	}
	// Exactly one third-party origin, named once for img-src and once for
	// connect-src. Two configured basemap styles must not become two origins.
	if got, want := strings.Count(policy, "https://"), 2; got != want {
		t.Errorf("CSP names %d external origins, want %d: %q", got, want, policy)
	}
	if got, want := api.Header().Get("Cache-Control"), cacheAPI; got != want {
		t.Errorf("API Cache-Control = %q, want %q", got, want)
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, authenticatedRequest(http.MethodGet, "/assets/app-abc123.js"))
	if got, want := asset.Header().Get("Cache-Control"), cacheImmutable; got != want {
		t.Errorf("asset Cache-Control = %q, want %q", got, want)
	}

	document := httptest.NewRecorder()
	handler.ServeHTTP(document, authenticatedRequest(http.MethodGet, "/"))
	if got, want := document.Header().Get("Cache-Control"), cacheDocument; got != want {
		t.Errorf("document Cache-Control = %q, want %q", got, want)
	}
}

func TestHandlerServesTheApplicationDocumentForDeepLinks(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{"/", "/routes/12/1"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, path))
			if got, want := response.Code, http.StatusOK; got != want {
				t.Fatalf("status = %d, want %d", got, want)
			}
			if got := response.Body.String(); !strings.Contains(got, "<!doctype html>") {
				t.Errorf("body = %q, want the application document", got)
			}
		})
	}
}

func TestHandlerRunsCallerBoundOAuthFlow(t *testing.T) {
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := newHandler(t, oauthService, &fakeState{})
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, authenticatedRequest(http.MethodGet, "/oauth/wahoo/start/rider-a"))
	if got, want := startResponse.Code, http.StatusFound; got != want {
		t.Errorf("start status = %d, want %d", got, want)
	}
	if got, want := oauthService.targetID, "rider-a"; got != want {
		t.Errorf("oauth target = %q, want %q", got, want)
	}

	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, authenticatedRequest(http.MethodGet, "/oauth/wahoo/callback?state=state&code=code"))
	if got, want := callbackResponse.Code, http.StatusSeeOther; got != want {
		t.Errorf("callback status = %d, want %d", got, want)
	}
	if got, want := callbackResponse.Header().Get("Location"), "/v1/status"; got != want {
		t.Errorf("callback location = %q, want %q", got, want)
	}
}

func TestHandlerAcceptsManualSync(t *testing.T) {
	trigger := &fakeSyncTrigger{accepted: true}
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, trigger)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	if got, want := response.Code, http.StatusAccepted; got != want {
		t.Errorf("sync status = %d, want %d", got, want)
	}
	if got, want := trigger.calls, 1; got != want {
		t.Errorf("trigger calls = %d, want %d", got, want)
	}
}

// Redoing a stage is one request: mark it, then start the run that will honour
// the mark. Both halves, because the stage has to be read again before it can be
// written again.
func TestHandlerReprocessesOneStageAndStartsARun(t *testing.T) {
	state := surfaceState()
	trigger := &fakeSyncTrigger{accepted: true}
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, state, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/routes/12/stages/1/reprocess"))
	if got, want := response.Code, http.StatusAccepted; got != want {
		t.Fatalf("reprocess status = %d, want %d", got, want)
	}
	if got, want := state.reprocessed, [][2]int64{{12, 1}}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("reprocessed = %v, want %v", got, want)
	}
	if got := trigger.phases; len(got) != 1 || got[0] != SyncPhaseAll {
		t.Errorf("triggered %v, want [%s]", got, SyncPhaseAll)
	}
}

// A run already in flight may be past this stage or may not include it, so the
// request waits for a pass that will honour it rather than being lost.
func TestHandlerKeepsAReprocessRequestWhenARunIsAlreadyActive(t *testing.T) {
	state := surfaceState()
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, state, &fakeSyncTrigger{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/routes/12/stages/1/reprocess"))
	if got, want := response.Code, http.StatusAccepted; got != want {
		t.Errorf("reprocess status = %d, want %d", got, want)
	}
	if len(state.reprocessed) != 1 {
		t.Errorf("reprocessed = %v, want the request recorded anyway", state.reprocessed)
	}
}

func TestHandlerReportsAnUnknownStageForReprocessingAsNotFound(t *testing.T) {
	trigger := &fakeSyncTrigger{accepted: true}
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/routes/99/stages/1/reprocess"))
	if got, want := response.Code, http.StatusNotFound; got != want {
		t.Errorf("reprocess status = %d, want %d", got, want)
	}
	if len(trigger.phases) != 0 {
		t.Errorf("triggered %v, want no run for a stage that is not stored", trigger.phases)
	}
}

// Each half is triggerable on its own, because each is separately switched off
// and separately worth starting by hand.
func TestHandlerTriggersEachPhaseOnItsOwn(t *testing.T) {
	for path, want := range map[string]SyncPhase{
		"/v1/sync":         SyncPhaseAll,
		"/v1/sync/source":  SyncPhaseSource,
		"/v1/sync/targets": SyncPhaseTargets,
	} {
		trigger := &fakeSyncTrigger{accepted: true}
		handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, trigger)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, path))
		if got := response.Code; got != http.StatusAccepted {
			t.Errorf("POST %s status = %d, want %d", path, got, http.StatusAccepted)
		}
		if got := trigger.phases; len(got) != 1 || got[0] != want {
			t.Errorf("POST %s triggered %v, want [%s]", path, got, want)
		}
	}
}

func TestHandlerSwitchesEitherHalfOfTheSchedule(t *testing.T) {
	state := &fakeState{scheduleSource: true, scheduleTargets: true}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(
		http.MethodPut, "/v1/sync/schedule", `{"source":true,"targets":false}`,
	))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("schedule status = %d, want %d", got, want)
	}
	if !state.scheduleSource || state.scheduleTargets {
		t.Errorf("stored schedule = %v, %v, want the target half off", state.scheduleSource, state.scheduleTargets)
	}
	if got, want := state.scheduleWrites, 1; got != want {
		t.Errorf("schedule writes = %d, want %d", got, want)
	}
}

// Half a schedule is not a schedule: a body naming one switch would leave the
// other at whatever the caller happened to assume.
func TestHandlerRejectsAnIncompleteScheduleChange(t *testing.T) {
	bodies := []string{
		`{"source":true}`,
		`{}`,
		`{"source":true,"targets":true,"other":1}`,
		"not json",
		// A second object after the first: a caller who believes they sent
		// something this service never read.
		`{"source":true,"targets":false}{"source":false,"targets":true}`,
	}
	for _, body := range bodies {
		state := &fakeState{}
		handler := newHandler(t, &fakeOAuth{}, state)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, "/v1/sync/schedule", body))
		if got, want := response.Code, http.StatusBadRequest; got != want {
			t.Errorf("schedule status for %q = %d, want %d", body, got, want)
		}
		if state.scheduleWrites != 0 {
			t.Errorf("schedule writes for %q = %d, want 0", body, state.scheduleWrites)
		}
	}
}

func TestHandlerReportsTheScheduleAndEachPhaseInStatus(t *testing.T) {
	completedAt := time.Date(2026, time.August, 18, 6, 30, 0, 0, time.UTC)
	state := &fakeState{
		scheduleSource:  true,
		scheduleTargets: false,
		phaseRuns: []phaseRun{
			{phase: "source", completedAt: completedAt, outcome: "succeeded", sourceStages: 12},
			{phase: "targets", completedAt: completedAt, outcome: "failed", detail: "destination", created: 1},
		},
	}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var view statusView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding status = %v", err)
	}
	if !view.Sync.Schedule.Source || view.Sync.Schedule.Targets {
		t.Errorf("schedule = %+v, want the source half on and the target half off", view.Sync.Schedule)
	}
	if view.Sync.Phases.Source == nil || view.Sync.Phases.Targets == nil {
		t.Fatalf("phases = %+v, want a run for each", view.Sync.Phases)
	}
	if got, want := view.Sync.Phases.Source.SourceStages, 12; got != want {
		t.Errorf("source stages = %d, want %d", got, want)
	}
	if got, want := view.Sync.Phases.Targets.LastFailure, "destination"; got != want {
		t.Errorf("target failure = %q, want %q", got, want)
	}
	if got, want := view.Sync.Phases.Source.LastCompletedAt, completedAt.Format(time.RFC3339); got != want {
		t.Errorf("source completion = %q, want %q", got, want)
	}
}

// A stage waiting its turn and a stage that fails every pass look identical on
// the map. The counts are what tell them apart.
func TestHandlerReportsHowMuchOfTheLibraryIsClassified(t *testing.T) {
	state := &fakeState{surfaceClassified: 1, surfaceTotal: 3}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var view statusView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding status = %v", err)
	}
	if got, want := view.Sync.Surface.Classified, 1; got != want {
		t.Errorf("classified = %d, want %d", got, want)
	}
	if got, want := view.Sync.Surface.Total, 3; got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
}

func TestHandlerReportsUnreadableScheduleAsUnavailable(t *testing.T) {
	state := &fakeState{scheduleErr: errors.New("state unavailable")}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	if got, want := response.Code, http.StatusInternalServerError; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func TestHandlerRejectsOverlappingManualSync(t *testing.T) {
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	if got, want := response.Code, http.StatusConflict; got != want {
		t.Errorf("sync status = %d, want %d", got, want)
	}
}

func TestHandlerRejectsInactiveTarget(t *testing.T) {
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := newHandler(t, oauthService, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/oauth/wahoo/start/rider-b"))
	if got, want := response.Code, http.StatusNotFound; got != want {
		t.Errorf("start status = %d, want %d", got, want)
	}
	if got := oauthService.targetID; got != "" {
		t.Errorf("OAuth start target = %q, want no OAuth request", got)
	}
}

func TestHandlerHidesOAuthFailure(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{completeErr: errors.New("private-token")}, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/oauth/wahoo/callback?state=state&code=code"))
	if got, want := response.Code, http.StatusBadRequest; got != want {
		t.Errorf("callback status = %d, want %d", got, want)
	}
	if body := response.Body.String(); strings.Contains(body, "private-token") {
		t.Errorf("callback body exposed upstream error: %q", body)
	}
}

func TestNewRejectsIncompleteOptions(t *testing.T) {
	tests := []struct {
		options *Options
		name    string
	}{
		{name: "nil options"},
		{name: "no targets", options: &Options{
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "duplicate targets", options: &Options{
			TargetIDs:        []string{"rider-a", "rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "plaintext tile style", options: &Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     "http://tiles.example.test/style.json",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "dark tile style on another origin", options: &Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			TileStyleURLDark: "https://dark.example.test/styles/dark",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "plaintext dark tile style", options: &Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			TileStyleURLDark: "http://tiles.example.test/styles/dark",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.options, &fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{}); err == nil {
				t.Error("New() error = nil, want an error")
			}
		})
	}
}

func newTestHandler(t *testing.T) *Handler { return newHandler(t, &fakeOAuth{}, &fakeState{}) }

// newHandlerWithVerifier builds a handler whose assertions resolve to whatever
// the given verifier reports, for exercising a refused identity.
func newHandlerWithVerifier(t *testing.T, verifier AccessVerifier) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   verifier,
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{accepted: true}, &fakeAssets{},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return handler
}

func newHandler(t *testing.T, oauthService OAuth, state State) *Handler {
	return newHandlerWithTrigger(t, oauthService, state, &fakeSyncTrigger{accepted: true})
}

func newHandlerWithTrigger(t *testing.T, oauthService OAuth, state State, syncTrigger SyncTrigger) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			TileStyleURLDark: testTileStyleURLDark,
			SourceBaseURL:    testSourceBaseURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		oauthService, state, syncTrigger, &fakeAssets{},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return handler
}

func authenticatedRequest(method, target string) *http.Request {
	request := httptest.NewRequestWithContext(context.Background(), method, target, http.NoBody)
	request.Header.Set(assertionHeader, testAssertion)
	withBrowserOrigin(request)

	return request
}

func authenticatedRequestWithBody(method, target, body string) *http.Request {
	request := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	request.Header.Set(assertionHeader, testAssertion)
	withBrowserOrigin(request)

	return request
}

// withBrowserOrigin attaches Origin exactly where a browser attaches it: to
// every request whose method is not GET or HEAD, same-origin ones included.
func withBrowserOrigin(request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		request.Header.Set("Origin", testBrowserOrigin)
	}
}

type fakeOAuth struct {
	completeErr        error
	location, targetID string
}

func (o *fakeOAuth) Start(_ context.Context, _, targetID string) (string, error) {
	o.targetID = targetID

	return o.location, nil
}

func (o *fakeOAuth) Complete(context.Context, string, string, string) error { return o.completeErr }

type fakeSyncTrigger struct {
	phases   []SyncPhase
	calls    int
	accepted bool
}

func (t *fakeSyncTrigger) Trigger(phase SyncPhase) bool {
	t.calls++
	t.phases = append(t.phases, phase)

	return t.accepted
}

type fakeAssets struct{}

func (*fakeAssets) Index(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := writer.Write([]byte("<!doctype html><title>domestique</title>")); err != nil {
		return
	}
}

func (*fakeAssets) Static(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	if _, err := writer.Write([]byte("export default null;")); err != nil {
		return
	}
}

type phaseRun struct {
	phase        string
	completedAt  time.Time
	outcome      string
	detail       string
	sourceStages int
	created      int
	updated      int
	deleted      int
}

type fakeState struct {
	surfaceErr        error
	phaseRunErr       error
	scheduleErr       error
	coverageErr       error
	reprocessErr      error
	reprocessed       [][2]int64
	surfaceHash       string
	coordinates       json.RawMessage
	surfaceRanges     json.RawMessage
	summaries         []route.Summary
	phaseRuns         []phaseRun
	surfaceMetres     float64
	scheduleWrites    int
	surfaceClassified int
	surfaceTotal      int
	scheduleSource    bool
	scheduleTargets   bool
}

func (*fakeState) ForEachTarget(_ context.Context, visit func(string, string) error) error {
	return visit("rider-a", "authorised")
}

func (s *fakeState) ForEachStageSummary(_ context.Context, visit func(route.Summary) error) error {
	for index := range s.summaries {
		if err := visit(s.summaries[index]); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) StageGeometry(
	_ context.Context,
	routeID int64,
	stageOrder int,
) (route.Summary, json.RawMessage, bool, error) {
	for index := range s.summaries {
		summary := s.summaries[index]
		if summary.RouteID == routeID && summary.StageOrder == stageOrder {
			return summary, s.coordinates, true, nil
		}
	}

	return route.Summary{}, nil, false, nil
}

// StageSurface answers only for the geometry a classification was measured
// against, exactly as the store does.
func (s *fakeState) StageSurface(
	_ context.Context,
	routeID int64,
	stageOrder int,
	contentHash string,
) (ranges json.RawMessage, matchedMetres float64, found bool, err error) {
	if s.surfaceErr != nil {
		return nil, 0, false, s.surfaceErr
	}
	for index := range s.summaries {
		summary := s.summaries[index]
		if summary.RouteID != routeID || summary.StageOrder != stageOrder {
			continue
		}
		if s.surfaceRanges == nil || contentHash != s.surfaceHash {
			break
		}

		return s.surfaceRanges, s.surfaceMetres, true, nil
	}

	return nil, 0, false, nil
}

//nolint:gocritic // This fake conforms to the persistence-free state boundary.
func (*fakeState) LastSyncRun(context.Context) (time.Time, string, string, int, int, int, int, bool, error) {
	return time.Time{}, "", "", 0, 0, 0, 0, false, nil
}

func (s *fakeState) ForEachPhaseRun(
	_ context.Context,
	visit func(phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error,
) error {
	if s.phaseRunErr != nil {
		return s.phaseRunErr
	}
	for _, run := range s.phaseRuns {
		if err := visit(
			run.phase, run.completedAt, run.outcome, run.detail,
			run.sourceStages, run.created, run.updated, run.deleted,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) RequestStageReprocess(_ context.Context, routeID int64, stageOrder int) (bool, error) {
	if s.reprocessErr != nil {
		return false, s.reprocessErr
	}
	for index := range s.summaries {
		if s.summaries[index].RouteID == routeID && s.summaries[index].StageOrder == stageOrder {
			s.reprocessed = append(s.reprocessed, [2]int64{routeID, int64(stageOrder)})

			return true, nil
		}
	}

	return false, nil
}

func (s *fakeState) SurfaceCoverage(context.Context) (classified, total int, err error) {
	if s.coverageErr != nil {
		return 0, 0, s.coverageErr
	}

	return s.surfaceClassified, s.surfaceTotal, nil
}

func (s *fakeState) SyncSchedule(context.Context) (source, targets bool, err error) {
	if s.scheduleErr != nil {
		return false, false, s.scheduleErr
	}

	return s.scheduleSource, s.scheduleTargets, nil
}

func (s *fakeState) SetSyncSchedule(_ context.Context, source, targets bool) error {
	if s.scheduleErr != nil {
		return s.scheduleErr
	}
	s.scheduleSource, s.scheduleTargets = source, targets
	s.scheduleWrites++

	return nil
}
