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

const testTileStyleURL = "https://tiles.example.test/styles/liberty"

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

	status.Header.Set(identityHeader, "rider@example.ts.net")
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

			forbidden := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
			forbidden.Header.Set(identityHeader, "someone-else@example.ts.net")
			forbiddenResponse := httptest.NewRecorder()
			handler.ServeHTTP(forbiddenResponse, forbidden)
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
	if got := response.Body.String(); !strings.Contains(got, testTileStyleURL) {
		t.Errorf("config body = %q, want the configured style URL", got)
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
		{name: "no login", options: &Options{TargetIDs: []string{"rider-a"}, TileStyleURL: testTileStyleURL}},
		{name: "no targets", options: &Options{TailnetUserLogin: "rider@example.ts.net", TileStyleURL: testTileStyleURL}},
		{name: "duplicate targets", options: &Options{
			TailnetUserLogin: "rider@example.ts.net",
			TargetIDs:        []string{"rider-a", "rider-a"},
			TileStyleURL:     testTileStyleURL,
		}},
		{name: "plaintext tile style", options: &Options{
			TailnetUserLogin: "rider@example.ts.net",
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     "http://tiles.example.test/style.json",
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

func newHandler(t *testing.T, oauthService OAuth, state State) *Handler {
	return newHandlerWithTrigger(t, oauthService, state, &fakeSyncTrigger{accepted: true})
}

func newHandlerWithTrigger(t *testing.T, oauthService OAuth, state State, syncTrigger SyncTrigger) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TailnetUserLogin: "rider@example.ts.net",
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
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
	request.Header.Set(identityHeader, "rider@example.ts.net")

	return request
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
	accepted bool
	calls    int
}

func (t *fakeSyncTrigger) Trigger() bool {
	t.calls++

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

type fakeState struct {
	coordinates   json.RawMessage
	surfaceRanges json.RawMessage
	surfaceHash   string
	surfaceErr    error
	summaries     []route.Summary
	surfaceMetres float64
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
