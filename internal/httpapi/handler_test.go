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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Equal(t, http.StatusOK, healthResponse.Code, "health status")

	status := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/status", http.NoBody)
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	assert.Equal(t, http.StatusUnauthorized, statusResponse.Code, "unauthenticated status")

	status.Header.Set(assertionHeader, testAssertion)
	statusResponse = httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	assert.Equal(t, http.StatusOK, statusResponse.Code, "authenticated status")
	body := statusResponse.Body.String()
	assert.NotContains(t, body, "private-token", "the status body exposed a token")
	assert.Contains(t, body, "authorized", "the status body omits the authorization state")
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
			assert.Equalf(t, http.StatusUnauthorized, response.Code, "unauthenticated %s", path)

			forbiddenResponse := httptest.NewRecorder()
			foreign.ServeHTTP(forbiddenResponse, authenticatedRequest(http.MethodGet, path))
			assert.Equalf(t, http.StatusForbidden, forbiddenResponse.Code, "wrong identity %s", path)
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
	require.Equal(t, http.StatusOK, response.Code, "geometry status")

	var view geometryView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding geometry")
	assert.Equal(t, "Feature", view.Type, "type")
	assert.Equal(t, "LineString", view.Geometry.Type, "geometry type")
	assert.Equal(t, `[[8.4,49],[8.5,49.2]]`, string(view.Geometry.Coordinates), "coordinates")
	assert.Equal(t, "Alpine loop — Descent", view.Properties.Title, "title")
	// The bounding box is the corners of the two coordinates, which is what a map
	// frames the stage by.
	assert.InDeltaSlice(t, []float64{8.4, 49.0, 8.5, 49.2}, view.BBox, 1e-9, "bbox")
}

func TestHandlerServesTheStoredSurfaceWithGeometry(t *testing.T) {
	state := surfaceState()
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	require.Equal(t, http.StatusOK, response.Code, "geometry status")

	var view geometryView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding geometry")
	require.NotNil(t, view.Properties.Surface, "the geometry omitted the stored surface")
	assert.Equal(t, string(state.surfaceRanges), string(view.Properties.Surface.Ranges), "surface ranges")
	assert.InDelta(t, 1234.5, view.Properties.Surface.MatchedMetres, 0.001, "matched metres")
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
	require.Equal(t, http.StatusOK, response.Code, "geometry status")

	var view geometryView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding geometry")
	require.NotNil(t, view.Properties.Surface, "the geometry omitted a classification that matched nothing")
	assert.Equal(t, string(state.surfaceRanges), string(view.Properties.Surface.Ranges), "surface ranges")
	assert.Zero(t, view.Properties.Surface.MatchedMetres, "matched metres")
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
	require.Equal(t, http.StatusOK, response.Code, "geometry status")
	assert.NotContains(t, response.Body.String(), "surface", "the geometry carried a stale surface")
}

func TestHandlerReportsUnreadableSurfaceStateAsUnavailable(t *testing.T) {
	state := surfaceState()
	state.surfaceErr = errors.New("state unavailable")
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/routes/12/stages/1/geometry"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "geometry status")
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
	assert.Equal(t, http.StatusNotFound, response.Code, "geometry status")
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
			assert.Equal(t, http.StatusNotFound, response.Code, "status")
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
	require.Equal(t, http.StatusOK, response.Code, "routes status")
	body := response.Body.String()
	assert.Contains(t, body, `"title":"Sunday"`, "the routes body omits the stage title")
	assert.NotContains(t, body, "coordinates", "the routes body leaked geometry")
}

func TestHandlerServesTileStyleConfiguration(t *testing.T) {
	handler := newTestHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	require.Equal(t, http.StatusOK, response.Code, "config status")
	body := response.Body.String()
	// Both styles, because the page picks between them: the colour scheme is a
	// property of the browser, and this response is cached for the session. The
	// provider base URL rides along, because the link back to a stage's source
	// route is built from it.
	for _, want := range []string{testTileStyleURL, testTileStyleURLDark, testSourceBaseURL} {
		assert.Contains(t, body, want, "the config body omits a value the page is built from")
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
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// Absent rather than empty: that is how the page knows to keep one style in
	// both colour schemes.
	assert.NotContains(t, response.Body.String(), "tile_style_url_dark", "the config body carries a dark style key")
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
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// Absent rather than empty: that is how the page knows to offer no source
	// link at all rather than one pointing at nowhere.
	assert.NotContains(t, response.Body.String(), "source_base_url", "the config body carries a source base URL key")
}

func TestHandlerRefusesASourceBaseURLThatIsNotOne(t *testing.T) {
	// The value is echoed to the browser rather than only compared, so anything
	// riding on it is observable: credentials would be a secret in a JSON body,
	// and a query or fragment would be sent to the provider on every visit.
	for _, value := range []string{
		"http://source.example.test",
		"source.example.test",
		"/user-routes",
		"https://rider:hunter2@source.example.test",
		"https://source.example.test?utm_source=domestique",
		"https://source.example.test/#fragment",
		"https://",
	} {
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
		require.Errorf(t, err, "New() accepted the source base URL %q", value)
	}
}

func TestHandlerSendsASourceBaseURLWithoutSurroundingWhitespace(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			SourceBaseURL:    "  " + testSourceBaseURL + "\n",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// The page must receive the value that was validated, not a wider one: a
	// browser cannot parse a URL with whitespace around it, so an accepted
	// configuration would silently produce no link.
	var payload struct {
		//nolint:tagliatelle // Mirrors the wire field the page reads.
		SourceBaseURL string `json:"source_base_url"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload), "decoding config body")
	assert.Equal(t, testSourceBaseURL, payload.SourceBaseURL, "source base URL")
}

func TestHandlerAcceptsASourceBaseURLHostedUnderAPath(t *testing.T) {
	// A provider need not sit at the root of its host, and the page builds the
	// route path underneath whatever prefix it is given.
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			SourceBaseURL:    testSourceBaseURL + "/planner",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	assert.Contains(t, response.Body.String(), testSourceBaseURL+"/planner", "the config body omits the planner link")
}

func TestHandlerNamesTheBuildItIsRunning(t *testing.T) {
	revision := strings.Repeat("ab", 20)
	digest := "sha256:" + strings.Repeat("cd", 32)
	handler := newHandlerWithBuild(t, revision, digest)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	var view struct {
		Build *struct {
			Revision string `json:"revision"`
			//nolint:tagliatelle // Mirrors the wire field the page reads.
			ImageDigest string `json:"image_digest"`
		} `json:"build"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding status body")
	require.NotNil(t, view.Build, "the status body carries no build group")
	assert.Equal(t, revision, view.Build.Revision, "revision")
	assert.Equal(t, digest, view.Build.ImageDigest, "image digest")
}

func TestHandlerSaysNothingAboutAnUninjectedBuild(t *testing.T) {
	handler := newHandlerWithBuild(t, "", "")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	// Absent rather than an empty group: that is how the page tells a local
	// development process from a deployed one, instead of offering a link to a
	// commit nobody can look up.
	assert.NotContains(t, response.Body.String(), "\"build\"", "the status body carries a build group")
}

func TestHandlerRefusesToPublishABuildStampThatIsNotOne(t *testing.T) {
	digest := "sha256:" + strings.Repeat("cd", 32)

	// Dropped rather than served: a truncated or wrong revision becomes a link
	// to nowhere in a browser, and a tag cannot answer which image is running.
	for _, revision := range []string{
		"0123456",
		strings.Repeat("ab", 20) + "c",
		strings.ToUpper(strings.Repeat("ab", 20)),
		"refs/heads/main",
	} {
		handler := newHandlerWithBuild(t, revision, digest)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
		assert.NotContainsf(t, response.Body.String(), "\"build\"", "the status body for revision %q carries a build group", revision)
	}

	for _, value := range []string{
		"latest",
		"sha-0123456",
		"ghcr.io/nobbs/domestique@" + digest,
		"sha256:" + strings.Repeat("cd", 31),
		strings.ToUpper(digest),
	} {
		handler := newHandlerWithBuild(t, strings.Repeat("ab", 20), value)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
		body := response.Body.String()
		assert.NotContainsf(t, body, "image_digest", "the status body for image %q carries a digest", value)
		// The revision still stands on its own: one unusable value must not cost
		// the operator the other.
		assert.Containsf(t, body, strings.Repeat("ab", 20), "the status body for image %q lost the revision", value)
	}
}

func newHandlerWithBuild(t *testing.T, revision, imageDigest string) *Handler {
	t.Helper()

	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			BuildRevision:    revision,
			BuildImageDigest: imageDigest,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
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
		assert.Contains(t, policy, want, "the CSP omits a directive the page needs")
	}
	// Exactly one third-party origin, named once for img-src and once for
	// connect-src. Two configured basemap styles must not become two origins.
	assert.Equalf(t, 2, strings.Count(policy, "https://"), "the CSP names the wrong number of external origins: %q", policy)
	assert.Equal(t, cacheAPI, api.Header().Get("Cache-Control"), "API Cache-Control")

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, authenticatedRequest(http.MethodGet, "/assets/app-abc123.js"))
	assert.Equal(t, cacheImmutable, asset.Header().Get("Cache-Control"), "asset Cache-Control")

	document := httptest.NewRecorder()
	handler.ServeHTTP(document, authenticatedRequest(http.MethodGet, "/"))
	assert.Equal(t, cacheDocument, document.Header().Get("Cache-Control"), "document Cache-Control")
}

func TestHandlerServesTheApplicationDocumentForDeepLinks(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{"/", "/routes/12/1"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, path))
			require.Equal(t, http.StatusOK, response.Code, "status")
			assert.Contains(t, response.Body.String(), "<!doctype html>", "the response is not the application document")
		})
	}
}

func TestHandlerRunsCallerBoundOAuthFlow(t *testing.T) {
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := newHandler(t, oauthService, &fakeState{})
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, authenticatedRequest(http.MethodGet, "/oauth/wahoo/start/rider-a"))
	assert.Equal(t, http.StatusFound, startResponse.Code, "start status")
	assert.Equal(t, "rider-a", oauthService.targetID, "oauth target")

	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, authenticatedRequest(http.MethodGet, "/oauth/wahoo/callback?state=state&code=code"))
	assert.Equal(t, http.StatusSeeOther, callbackResponse.Code, "callback status")
	assert.Equal(t, "/v1/status", callbackResponse.Header().Get("Location"), "callback location")
}

func TestHandlerAcceptsManualSync(t *testing.T) {
	trigger := &fakeSyncTrigger{accepted: true}
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, trigger)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	assert.Equal(t, http.StatusAccepted, response.Code, "sync status")
	assert.Equal(t, 1, trigger.calls, "trigger calls")
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
	require.Equal(t, http.StatusAccepted, response.Code, "reprocess status")
	assert.Equal(t, [][2]int64{{12, 1}}, state.reprocessed, "reprocessed stages")
	assert.Equal(t, []SyncPhase{SyncPhaseAll}, trigger.phases, "triggered phases")
}

// A run already in flight may be past this stage or may not include it, so the
// request waits for a pass that will honour it rather than being lost.
func TestHandlerKeepsAReprocessRequestWhenARunIsAlreadyActive(t *testing.T) {
	state := surfaceState()
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, state, &fakeSyncTrigger{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/routes/12/stages/1/reprocess"))
	assert.Equal(t, http.StatusAccepted, response.Code, "reprocess status")
	assert.Len(t, state.reprocessed, 1, "the request was not recorded")
}

func TestHandlerReportsAnUnknownStageForReprocessingAsNotFound(t *testing.T) {
	trigger := &fakeSyncTrigger{accepted: true}
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/routes/99/stages/1/reprocess"))
	assert.Equal(t, http.StatusNotFound, response.Code, "reprocess status")
	assert.Empty(t, trigger.phases, "a stage that is not stored triggered a run")
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
		assert.Equalf(t, http.StatusAccepted, response.Code, "POST %s status", path)
		assert.Equalf(t, []SyncPhase{want}, trigger.phases, "POST %s triggered phases", path)
	}
}

func TestHandlerSwitchesEitherHalfOfTheSchedule(t *testing.T) {
	state := &fakeState{scheduleSource: true, scheduleTargets: true}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(
		http.MethodPut, "/v1/sync/schedule", `{"source":true,"targets":false}`,
	))
	require.Equal(t, http.StatusOK, response.Code, "schedule status")
	assert.True(t, state.scheduleSource, "the stored schedule switched the source half off")
	assert.False(t, state.scheduleTargets, "the stored schedule left the target half on")
	assert.Equal(t, 1, state.scheduleWrites, "schedule writes")
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
		assert.Equalf(t, http.StatusBadRequest, response.Code, "schedule status for %q", body)
		assert.Zerof(t, state.scheduleWrites, "a rejected body %q was written to the schedule", body)
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
	require.Equal(t, http.StatusOK, response.Code, "status")
	var view statusView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding status")
	assert.True(t, view.Sync.Schedule.Source, "the reported schedule has the source half off")
	assert.False(t, view.Sync.Schedule.Targets, "the reported schedule has the target half on")
	require.NotNil(t, view.Sync.Phases.Source, "the status reports no source run")
	require.NotNil(t, view.Sync.Phases.Targets, "the status reports no target run")
	assert.Equal(t, 12, view.Sync.Phases.Source.SourceStages, "source stages")
	assert.Equal(t, "destination", view.Sync.Phases.Targets.LastFailure, "target failure")
	assert.Equal(t, completedAt.Format(time.RFC3339), view.Sync.Phases.Source.LastCompletedAt, "source completion")
}

// A stage waiting its turn and a stage that fails every pass look identical on
// the map. The counts are what tell them apart.
func TestHandlerReportsHowMuchOfTheLibraryIsClassified(t *testing.T) {
	state := &fakeState{surfaceClassified: 1, surfaceTotal: 3}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	require.Equal(t, http.StatusOK, response.Code, "status")
	var view statusView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding status")
	assert.Equal(t, 1, view.Sync.Surface.Classified, "classified")
	assert.Equal(t, 3, view.Sync.Surface.Total, "total")
}

func TestHandlerReportsUnreadableScheduleAsUnavailable(t *testing.T) {
	state := &fakeState{scheduleErr: errors.New("state unavailable")}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "status")
}

func TestHandlerRejectsOverlappingManualSync(t *testing.T) {
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	assert.Equal(t, http.StatusConflict, response.Code, "sync status")
}

func TestHandlerRejectsInactiveTarget(t *testing.T) {
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := newHandler(t, oauthService, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/oauth/wahoo/start/rider-b"))
	assert.Equal(t, http.StatusNotFound, response.Code, "start status")
	assert.Empty(t, oauthService.targetID, "an unknown target still started an OAuth request")
}

func TestHandlerHidesOAuthFailure(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{completeErr: errors.New("private-token")}, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/oauth/wahoo/callback?state=state&code=code"))
	assert.Equal(t, http.StatusBadRequest, response.Code, "callback status")
	assert.NotContains(t, response.Body.String(), "private-token", "the callback body exposed the upstream error")
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
			_, err := New(test.options, &fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{})
			require.Error(t, err, "New() accepted the options")
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
	require.NoError(t, err, "New()")

	return handler
}

func newHandler(t *testing.T, oauthService OAuth, state State) *Handler {
	return newHandlerWithTrigger(t, oauthService, state, &fakeSyncTrigger{accepted: true})
}

// newHandlerWithTargets builds a handler configured for more than one slot, which
// is what a partial target failure needs to be visible at all.
func newHandlerWithTargets(t *testing.T, state State, targetIDs ...string) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        targetIDs,
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, state, &fakeSyncTrigger{accepted: true}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
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
	require.NoError(t, err, "New()")

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
	sourceStageErr    error
	targetStageErr    error
	targetRunErr      error
	targetStages      map[string][]storedStage
	reprocessed       [][2]int64
	targets           []fakeTarget
	sourceStages      []storedStage
	targetRuns        []fakeTargetRun
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

// fakeTarget is one configured slot and the authorisation state it is in.
type fakeTarget struct {
	id            string
	authorization string
}

// storedStage is one row of either stage table: which stage, and the source
// revision it is recorded at. Convergence is the comparison of the two tables on
// exactly these fields.
type storedStage struct {
	revision   string
	routeID    int64
	stageOrder int
}

// fakeTargetRun is one slot's own last reconciliation.
type fakeTargetRun struct {
	completedAt time.Time
	id          string
	outcome     string
	failure     string
}

// ForEachTarget reports the configured slots, defaulting to the single
// authorised slot the older tests were written against. The value is the one
// the store actually holds, so a test cannot pass on a state production can
// never produce.
func (s *fakeState) ForEachTarget(_ context.Context, visit func(string, string) error) error {
	if len(s.targets) == 0 {
		return visit("rider-a", "authorized")
	}
	for _, target := range s.targets {
		if err := visit(target.id, target.authorization); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) ForEachSourceStage(
	_ context.Context,
	visit func(routeID int64, stageOrder int, sourceRevision, contentHash string) error,
) error {
	if s.sourceStageErr != nil {
		return s.sourceStageErr
	}
	for _, stage := range s.sourceStages {
		if err := visit(stage.routeID, stage.stageOrder, stage.revision, "hash"); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) ForEachTargetStage(
	_ context.Context,
	targetID string,
	visit func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error,
) error {
	if s.targetStageErr != nil {
		return s.targetStageErr
	}
	for _, stage := range s.targetStages[targetID] {
		if err := visit(stage.routeID, stage.stageOrder, stage.revision, "encoded-hash", 900+stage.routeID); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) ForEachTargetRun(
	_ context.Context,
	visit func(targetID string, finishedAt time.Time, outcome, detail string) error,
) error {
	if s.targetRunErr != nil {
		return s.targetRunErr
	}
	for _, run := range s.targetRuns {
		if err := visit(run.id, run.completedAt, run.outcome, run.failure); err != nil {
			return err
		}
	}

	return nil
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

// statusOf reads the status document as the handler serves it.
func statusOf(t *testing.T, handler *Handler) statusView {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	require.Equal(t, http.StatusOK, response.Code)
	var view statusView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view))

	return view
}

// convergenceStateFixture builds a library of two stages where one slot holds
// both at their current revision and the other is a revision behind on one and
// has never been written the other.
func convergenceStateFixture() *fakeState {
	return &fakeState{
		targets: []fakeTarget{
			{id: "rider-a", authorization: "authorized"},
			{id: "rider-b", authorization: "authorized"},
		},
		sourceStages: []storedStage{
			{routeID: 12, stageOrder: 1, revision: "r2"},
			{routeID: 12, stageOrder: 2, revision: "r1"},
		},
		targetStages: map[string][]storedStage{
			"rider-a": {
				{routeID: 12, stageOrder: 1, revision: "r2"},
				{routeID: 12, stageOrder: 2, revision: "r1"},
			},
			"rider-b": {
				// Written before the first stage changed, and never written the
				// second stage at all.
				{routeID: 12, stageOrder: 1, revision: "r1"},
			},
		},
	}
}

// The operator's core question: is every stored stage, at the revision it is
// stored at, on every configured Wahoo account.
func TestHandlerReportsPerTargetConvergence(t *testing.T) {
	state := convergenceStateFixture()
	state.targetRuns = []fakeTargetRun{
		{id: "rider-a", completedAt: time.Date(2026, time.August, 18, 6, 0, 0, 0, time.UTC), outcome: "succeeded"},
	}
	view := statusOf(t, newHandlerWithTargets(t, state, "rider-a", "rider-b"))

	require.Len(t, view.Targets, 2)
	assert.Equal(t, convergenceCurrent, view.Targets[0].Convergence)
	assert.Equal(t, targetStagesView{Current: 2, Pending: 0}, view.Targets[0].Stages)
	require.NotNil(t, view.Targets[0].LastRun)
	assert.Equal(t, "succeeded", view.Targets[0].LastRun.Result)
	assert.Equal(t, "2026-08-18T06:00:00Z", view.Targets[0].LastRun.CompletedAt)

	assert.Equal(t, convergenceLagging, view.Targets[1].Convergence)
	assert.Equal(t, targetStagesView{Current: 0, Pending: 2}, view.Targets[1].Stages)
	// A slot that has never been reconciled is not a slot whose run succeeded
	// with nothing to do.
	assert.Nil(t, view.Targets[1].LastRun)

	assert.False(t, view.Converged, "one lagging slot is enough to say the library is not everywhere")
}

// Overall convergence is the conjunction, and is true only when every stored
// stage is current on every configured target.
func TestHandlerReportsOverallConvergenceOnlyWhenEveryTargetIsCurrent(t *testing.T) {
	state := convergenceStateFixture()
	state.targetStages["rider-b"] = state.targetStages["rider-a"]
	view := statusOf(t, newHandlerWithTargets(t, state, "rider-a", "rider-b"))

	assert.True(t, view.Converged)
	for _, target := range view.Targets {
		assert.Equal(t, convergenceCurrent, target.Convergence)
		assert.Equal(t, targetStagesView{Current: 2, Pending: 0}, target.Stages)
	}
}

// A run that wrote one account and could not write the other is recorded once as
// failed. Convergence has to say which account that was.
func TestHandlerReportsPartialTargetFailurePerTarget(t *testing.T) {
	state := convergenceStateFixture()
	state.targetStages["rider-b"] = state.targetStages["rider-a"]
	completedAt := time.Date(2026, time.August, 18, 6, 30, 0, 0, time.UTC)
	state.targetRuns = []fakeTargetRun{
		{id: "rider-a", completedAt: completedAt, outcome: "succeeded"},
		{id: "rider-b", completedAt: completedAt, outcome: "failed", failure: "destination"},
	}
	view := statusOf(t, newHandlerWithTargets(t, state, "rider-a", "rider-b"))

	require.Len(t, view.Targets, 2)
	assert.Equal(t, convergenceCurrent, view.Targets[0].Convergence)
	assert.Equal(t, convergenceFailed, view.Targets[1].Convergence)
	require.NotNil(t, view.Targets[1].LastRun)
	assert.Equal(t, "destination", view.Targets[1].LastRun.Failure)
	assert.False(t, view.Converged)
}

// A stage that has left the library is still on the target until a run removes
// it, so that removal is outstanding work rather than a stage nobody counts.
func TestHandlerCountsARouteTheLibraryNoLongerHasAsPending(t *testing.T) {
	state := convergenceStateFixture()
	state.sourceStages = []storedStage{{routeID: 12, stageOrder: 1, revision: "r2"}}
	state.targetStages["rider-a"] = []storedStage{
		{routeID: 12, stageOrder: 1, revision: "r2"},
		{routeID: 40, stageOrder: 1, revision: "r9"},
	}
	state.targetStages["rider-b"] = []storedStage{{routeID: 12, stageOrder: 1, revision: "r2"}}
	view := statusOf(t, newHandlerWithTargets(t, state, "rider-a", "rider-b"))

	require.Len(t, view.Targets, 2)
	assert.Equal(t, targetStagesView{Current: 1, Pending: 1}, view.Targets[0].Stages)
	assert.Equal(t, convergenceLagging, view.Targets[0].Convergence)
	assert.Equal(t, targetStagesView{Current: 1, Pending: 0}, view.Targets[1].Stages)
	assert.Equal(t, convergenceCurrent, view.Targets[1].Convergence)
	assert.False(t, view.Converged)
}

// An empty library is nothing owed, not something unknown. It must not read as
// lagging, and it must not fail the status request.
func TestHandlerReportsAnEmptyLibraryAsCurrentEverywhere(t *testing.T) {
	state := &fakeState{
		targets:      []fakeTarget{{id: "rider-a", authorization: "authorized"}},
		targetStages: map[string][]storedStage{},
	}
	view := statusOf(t, newHandlerWithTargets(t, state, "rider-a"))

	require.Len(t, view.Targets, 1)
	assert.Equal(t, convergenceCurrent, view.Targets[0].Convergence)
	assert.Equal(t, targetStagesView{}, view.Targets[0].Stages)
	assert.True(t, view.Converged)
}

// Nothing can be written to a slot waiting for its one-time browser visit, and
// saying it is merely "lagging" would suggest the next run will fix it.
func TestHandlerReportsAnUnauthorizedTargetAsUnconverged(t *testing.T) {
	state := convergenceStateFixture()
	state.targets[1].authorization = "not_authorized"
	view := statusOf(t, newHandlerWithTargets(t, state, "rider-a", "rider-b"))

	require.Len(t, view.Targets, 2)
	assert.Equal(t, convergenceUnauthorized, view.Targets[1].Convergence)
	assert.False(t, view.Converged)
}

// Convergence is derived from local state, so an unreadable table is an
// unavailable status rather than a status that quietly says everything is fine.
func TestHandlerReportsUnreadableConvergenceStateAsUnavailable(t *testing.T) {
	for name, state := range map[string]*fakeState{
		"source stages": {sourceStageErr: errors.New("state unavailable")},
		"target stages": {targetStageErr: errors.New("state unavailable")},
		"target runs":   {targetRunErr: errors.New("state unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			newHandlerWithTargets(t, state, "rider-a").
				ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
		})
	}
}

// Convergence is counts and categories. A route name, a Wahoo route identifier,
// or a revision would put the library's contents on a document that exists to be
// safe to look at.
func TestHandlerReportsConvergenceWithoutNamingAnything(t *testing.T) {
	state := convergenceStateFixture()
	state.summaries = []route.Summary{{
		RouteID: 12, StageOrder: 1, RouteName: "Alpine loop", StageName: "Descent",
		SourceRevision: "r2",
	}}
	response := httptest.NewRecorder()
	newHandlerWithTargets(t, state, "rider-a", "rider-b").
		ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	require.Equal(t, http.StatusOK, response.Code)

	body := response.Body.String()
	for _, secret := range []string{"Alpine loop", "Descent", "r2", "912", "wahoo"} {
		assert.NotContains(t, body, secret)
	}
}
