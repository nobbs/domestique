package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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
	testImageryStyleURL  = "https://imagery.example.test/maps/hybrid/style.json?key=abc"
	testSourceBaseURL    = "https://source.example.test"
	// The Wahoo redirect URL the composition root passes, and the origin a
	// browser derives from it for the requests it sends this service.
	testBrowserOriginURL = "https://domestique.example.test/oauth/wahoo/callback"
	testBrowserOrigin    = "https://domestique.example.test"
)

// testBasemaps is the one-entry list most of these tests configure: a map to
// paint on, and no choice to make about it.
func testBasemaps() []Basemap {
	return []Basemap{{Name: "Streets", StyleURL: testTileStyleURL}}
}

// twoProviderBasemaps is the shape this change exists for: two cartographies
// from two providers, which is also two origins in the policy.
func twoProviderBasemaps() []Basemap {
	return []Basemap{
		{Name: "Streets", StyleURL: testTileStyleURL, StyleURLDark: testTileStyleURLDark},
		{Name: "Satellite", StyleURL: testImageryStyleURL, DarkCartography: true},
	}
}

// testBasemapsWithDark adds the provider's dark twin, which is the shape that
// exercises the colour scheme without adding a second origin.
func testBasemapsWithDark() []Basemap {
	return []Basemap{{
		Name:         "Streets",
		StyleURL:     testTileStyleURL,
		StyleURLDark: testTileStyleURLDark,
	}}
}

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
		"/v1/sync/runs",
		"/v1/routes",
		"/v1/providers/veloplanner/routes/1/stages/1",
		"/v1/providers/veloplanner/routes/1/stages/1/geometry",
		"/v1/webui/config",
		"/oauth/wahoo/start/rider-a",
		"/oauth/wahoo/callback",
		"/assets/app-abc123.js",
		"/favicon.svg",
		"/icon-256.png",
		"/icon-512.png",
		"/manifest.webmanifest",
		"/",
		"/routes/veloplanner/1/1",
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
			Provider: route.ProviderVeloPlanner,
			RouteID:  12, StageOrder: 1, RouteName: "Alpine loop", StageName: "Descent",
			SourceRevision: "revision", ContentHash: "hash", PointCount: 2, DistanceMetres: 1234.5,
			Bounds: route.Bounds{MinLongitude: 8.4, MinLatitude: 49.0, MaxLongitude: 8.5, MaxLatitude: 49.2},
		}},
		coordinates: json.RawMessage(`[[8.4,49],[8.5,49.2]]`),
	}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/routes/12/stages/1/geometry"))
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
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/routes/12/stages/1/geometry"))
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
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/routes/12/stages/1/geometry"))
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
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/routes/12/stages/1/geometry"))
	require.Equal(t, http.StatusOK, response.Code, "geometry status")
	assert.NotContains(t, response.Body.String(), "surface", "the geometry carried a stale surface")
}

func TestHandlerReportsUnreadableSurfaceStateAsUnavailable(t *testing.T) {
	state := surfaceState()
	state.surfaceErr = errors.New("state unavailable")
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/routes/12/stages/1/geometry"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "geometry status")
}

// surfaceState holds one stage whose surface has been classified against the
// geometry that is stored for it.
func surfaceState() *fakeState {
	return &fakeState{
		summaries: []route.Summary{{
			Provider: route.ProviderVeloPlanner,
			RouteID:  12, StageOrder: 1, RouteName: "Alpine loop", StageName: "Descent",
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
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/routes/99/stages/1/geometry"))
	assert.Equal(t, http.StatusNotFound, response.Code, "geometry status")
}

// A second provider's own stage resolves the same way VeloPlanner's always
// has: state is keyed by provider, routeID and stageOrder together, so a
// stage stored under a different provider is found rather than refused for
// naming one the handler has not special-cased.
func TestHandlerServesAStageStoredUnderAnyProvider(t *testing.T) {
	state := &fakeState{summaries: []route.Summary{{
		Provider: route.ProviderKomoot,
		RouteID:  7, StageOrder: 1, RouteName: "Trail", PointCount: 2,
	}}}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/komoot/routes/7/stages/1"))
	assert.Equal(t, http.StatusOK, response.Code, "stage status")
	assert.Contains(t, response.Body.String(), `"provider":"komoot"`, "the stage names its provider")
}

func TestHandlerRejectsMalformedStageIdentifiers(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{
		"/v1/providers/veloplanner/routes/0/stages/1",
		"/v1/providers/veloplanner/routes/-1/stages/1/geometry",
		"/v1/providers/veloplanner/routes/abc/stages/1",
		"/v1/providers/veloplanner/routes/1/stages/0/geometry",
		"/v1/providers/komoot/routes/1/stages/1",
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
	// name goes with them, because it is what a reader picks by and what their
	// browser remembers. The provider base URL rides along, because the link
	// back to a stage's source route is built from it.
	for _, want := range []string{"Streets", testTileStyleURL, testTileStyleURLDark, testSourceBaseURL} {
		assert.Contains(t, body, want, "the config body omits a value the page is built from")
	}
}

func TestHandlerServesEveryConfiguredBasemapInOrder(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         twoProviderBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	require.Equal(t, http.StatusOK, response.Code, "config status")

	var body struct {
		Basemaps []struct {
			Name string `json:"name"`
			//nolint:tagliatelle // This v1 JSON contract uses snake_case.
			StyleURL string `json:"style_url"`
			//nolint:tagliatelle // This v1 JSON contract uses snake_case.
			DarkCartography bool `json:"dark_cartography"`
		} `json:"basemaps"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body), "decoding the config body")

	// The configured order is the order offered, because the first entry is what
	// a browser that has never chosen one loads.
	require.Len(t, body.Basemaps, 2, "basemaps")
	assert.Equal(t, "Streets", body.Basemaps[0].Name)
	assert.False(t, body.Basemaps[0].DarkCartography, "street cartography follows the colour scheme")
	assert.Equal(t, "Satellite", body.Basemaps[1].Name)
	assert.Equal(t, testImageryStyleURL, body.Basemaps[1].StyleURL)
	assert.True(t, body.Basemaps[1].DarkCartography, "imagery is dark ground in either scheme")
}

func TestHandlerOmitsAnUnconfiguredDarkTileStyle(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// Absent rather than empty: that is how the page knows to keep one style in
	// both colour schemes.
	assert.NotContains(t, response.Body.String(), "style_url_dark", "the config body carries a dark style key")
}

func TestHandlerOmitsAnUnconfiguredSourceBaseURL(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// Absent rather than an empty map: that is how the page knows to offer no
	// source link at all rather than one pointing at nowhere.
	assert.NotContains(t, response.Body.String(), "source_base_urls", "the config body carries a source base URLs key")
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
				Basemaps:         testBasemaps(),
				SourceBaseURL:    value,
				AccessVerifier:   &recordingVerifier{email: testAccessEmail},
				AccessEmail:      testAccessEmail,
				BrowserOriginURL: testBrowserOriginURL,
			},
			&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
		)
		require.Errorf(t, err, "New() accepted the source base URL %q", value)
	}
}

func TestHandlerSendsASourceBaseURLWithoutSurroundingWhitespace(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemaps(),
			SourceBaseURL:    "  " + testSourceBaseURL + "\n",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	// The page must receive the value that was validated, not a wider one: a
	// browser cannot parse a URL with whitespace around it, so an accepted
	// configuration would silently produce no link.
	var payload struct {
		//nolint:tagliatelle // Mirrors the wire field the page reads.
		SourceBaseURLs map[string]string `json:"source_base_urls"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload), "decoding config body")
	assert.Equal(t, testSourceBaseURL, payload.SourceBaseURLs["veloplanner"], "source base URL")
}

func TestHandlerAcceptsASourceBaseURLHostedUnderAPath(t *testing.T) {
	// A provider need not sit at the root of its host, and the page builds the
	// route path underneath whatever prefix it is given.
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemaps(),
			SourceBaseURL:    testSourceBaseURL + "/planner",
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
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
			Basemaps:         testBasemaps(),
			BuildRevision:    revision,
			BuildImageDigest: imageDigest,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// TestHandlerNamesEveryBasemapOriginInThePolicy is the counterpart of the
// single-origin assertion above: the policy has to admit each provider offered,
// or a reader who switches gets a blank map and a console full of refusals.
func TestHandlerNamesEveryBasemapOriginInThePolicy(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         twoProviderBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	policy := response.Header().Get("Content-Security-Policy")

	// Origins, not style URLs: a query string carrying an API key has no place
	// in a CSP source list, and would not match there anyway.
	for _, want := range []string{
		"img-src 'self' data: blob: https://imagery.example.test https://tiles.example.test",
		"connect-src 'self' https://imagery.example.test https://tiles.example.test",
	} {
		assert.Contains(t, policy, want, "the CSP omits a directive the page needs")
	}
	assert.NotContains(t, policy, "key=abc", "the CSP carries a style URL rather than its origin")
	// Two providers, each named once per directive. Sorted, so the header a
	// deployment sends does not depend on the order the entries were written in.
	assert.Equalf(t, 4, strings.Count(policy, "https://"),
		"the CSP names the wrong number of external origins: %q", policy)
}

func TestHandlerNamesOneOriginOnceForTwoBasemapsSharingIt(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs: []string{"rider-a"},
			Basemaps: []Basemap{
				{Name: "Streets", StyleURL: testTileStyleURL},
				{Name: "Outdoors", StyleURL: "https://tiles.example.test/styles/outdoors"},
			},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	policy := response.Header().Get("Content-Security-Policy")
	assert.Equalf(t, 2, strings.Count(policy, "https://"),
		"one origin serving two cartographies must be named once per directive: %q", policy)
}

// TestHandlerAcceptsADarkStyleOnItsOriginRegardlessOfHostCase mirrors the
// configuration layer's sameOrigin, which treats host case as insignificant.
// A dark style differing from its light counterpart only by host case is on
// the origin a browser would use, and must not fail startup here after
// passing that same check in the configuration.
func TestHandlerAcceptsADarkStyleOnItsOriginRegardlessOfHostCase(t *testing.T) {
	handler, err := New(
		&Options{
			TargetIDs: []string{"rider-a"},
			Basemaps: []Basemap{{
				Name:         "Streets",
				StyleURL:     testTileStyleURL,
				StyleURLDark: "https://TILES.example.test/styles/dark",
			}},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	policy := response.Header().Get("Content-Security-Policy")
	assert.Equalf(t, 2, strings.Count(policy, "https://"),
		"a host differing only by case must not be named as a second origin: %q", policy)
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
	// connect-src. One basemap's two styles must not become two origins.
	assert.Equalf(t, 2, strings.Count(policy, "https://"), "the CSP names the wrong number of external origins: %q", policy)
	assert.Equal(t, cacheAPI, api.Header().Get("Cache-Control"), "API Cache-Control")

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, authenticatedRequest(http.MethodGet, "/assets/app-abc123.js"))
	assert.Equal(t, cacheImmutable, asset.Header().Get("Cache-Control"), "asset Cache-Control")

	document := httptest.NewRecorder()
	handler.ServeHTTP(document, authenticatedRequest(http.MethodGet, "/"))
	assert.Equal(t, cacheDocument, document.Header().Get("Cache-Control"), "document Cache-Control")

	// Everything addressed by a fixed name revalidates rather than being cached
	// for a year, because a new one arrives at the URL the old one had. An
	// immutable icon is an installed copy showing last year's icon.
	for _, path := range []string{"/favicon.svg", "/icon-256.png", "/icon-512.png", "/manifest.webmanifest"} {
		stable := httptest.NewRecorder()
		handler.ServeHTTP(stable, authenticatedRequest(http.MethodGet, path))
		assert.Equalf(t, cacheDocument, stable.Header().Get("Cache-Control"), "%s Cache-Control", path)
	}

	// The manifest's type is set by hand because Go's table does not know the
	// extension and the responses forbid sniffing.
	manifest := httptest.NewRecorder()
	handler.ServeHTTP(manifest, authenticatedRequest(http.MethodGet, "/manifest.webmanifest"))
	assert.Equal(t, "application/manifest+json", manifest.Header().Get("Content-Type"), "manifest Content-Type")
}

func TestHandlerServesTheApplicationDocumentForDeepLinks(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{"/", "/routes/veloplanner/12/1"} {
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
	// The UI, not the JSON endpoint: whoever arrives here followed a link from
	// the page, and that page is where the account they just connected is
	// described.
	assert.Equal(t, "/", callbackResponse.Header().Get("Location"), "callback location")
}

func TestHandlerAcceptsManualSync(t *testing.T) {
	trigger := &fakeSync{accepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)
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
	trigger := &fakeSync{accepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, state, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/providers/veloplanner/routes/12/stages/1/reprocess"))
	require.Equal(t, http.StatusAccepted, response.Code, "reprocess status")
	assert.Equal(t, [][2]int64{{12, 1}}, state.reprocessed, "reprocessed stages")
	assert.Equal(t, []SyncPhase{SyncPhaseAll}, trigger.phases, "triggered phases")
}

// A run already in flight may be past this stage or may not include it, so the
// request waits for a pass that will honour it rather than being lost.
func TestHandlerKeepsAReprocessRequestWhenARunIsAlreadyActive(t *testing.T) {
	state := surfaceState()
	handler := newHandlerWithSync(t, &fakeOAuth{}, state, &fakeSync{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/providers/veloplanner/routes/12/stages/1/reprocess"))
	assert.Equal(t, http.StatusAccepted, response.Code, "reprocess status")
	assert.Len(t, state.reprocessed, 1, "the request was not recorded")
}

func TestHandlerReportsAnUnknownStageForReprocessingAsNotFound(t *testing.T) {
	trigger := &fakeSync{accepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/providers/veloplanner/routes/99/stages/1/reprocess"))
	assert.Equal(t, http.StatusNotFound, response.Code, "reprocess status")
	assert.Empty(t, trigger.phases, "a stage that is not stored triggered a run")
}

// A stage URL from before a second provider existed still resolves, redirected
// to the same stage under veloplanner, preserving suffix and method.
func TestHandlerRedirectsLegacyStagePaths(t *testing.T) {
	trigger := &fakeSync{accepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)

	for _, test := range []struct {
		method string
		target string
		want   string
	}{
		{
			method: http.MethodGet,
			target: "/v1/routes/12/stages/1",
			want:   "/v1/providers/veloplanner/routes/12/stages/1",
		},
		{
			method: http.MethodGet,
			target: "/v1/routes/12/stages/1/geometry",
			want:   "/v1/providers/veloplanner/routes/12/stages/1/geometry",
		},
		{
			method: http.MethodPost,
			target: "/v1/routes/12/stages/1/reprocess",
			want:   "/v1/providers/veloplanner/routes/12/stages/1/reprocess",
		},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(test.method, test.target))
		assert.Equalf(t, http.StatusPermanentRedirect, response.Code, "%s %s status", test.method, test.target)
		assert.Equalf(t, test.want, response.Header().Get("Location"), "%s %s location", test.method, test.target)
	}
}

// The legacy browser address redirects the same way, to the address a link
// shared after a second provider existed would have used.
func TestHandlerRedirectsLegacyBrowserRoute(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/routes/12/1"))
	assert.Equal(t, http.StatusPermanentRedirect, response.Code, "browser route status")
	assert.Equal(t, "/routes/veloplanner/12/1", response.Header().Get("Location"), "browser route location")
}

// A malformed legacy path is refused rather than redirected to a target that
// could never resolve.
func TestHandlerRejectsMalformedLegacyStagePaths(t *testing.T) {
	handler := newTestHandler(t)

	for _, target := range []string{
		"/v1/routes/not-a-number/stages/1",
		"/v1/routes/12/stages/not-a-number",
		"/v1/routes/0/stages/1",
		"/routes/not-a-number/1",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
		assert.Equalf(t, http.StatusNotFound, response.Code, "GET %s status", target)
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
		trigger := &fakeSync{accepted: true}
		handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, path))
		assert.Equalf(t, http.StatusAccepted, response.Code, "POST %s status", path)
		assert.Equalf(t, []SyncPhase{want}, trigger.phases, "POST %s triggered phases", path)
	}
}

// A configured slot is triggered by name and mutates no other target: the
// handler validates the path value before the sync process ever sees it.
func TestHandlerTriggersOneConfiguredTarget(t *testing.T) {
	trigger := &fakeSync{accepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync/targets/rider-a"))
	assert.Equal(t, http.StatusAccepted, response.Code, "POST /v1/sync/targets/rider-a status")
	assert.Equal(t, []string{"rider-a"}, trigger.targetTriggers, "triggered target")
}

// A target this build was never configured with is refused outright, the same
// way the OAuth start route refuses one: there is no slot to reconcile.
func TestHandlerRejectsAnUnconfiguredTargetTrigger(t *testing.T) {
	trigger := &fakeSync{accepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync/targets/unknown"))
	assert.Equal(t, http.StatusNotFound, response.Code, "POST /v1/sync/targets/unknown status")
	assert.Empty(t, trigger.targetTriggers, "an unconfigured target was triggered")
}

// A single-target trigger refuses the same way a full one does when a
// synchronization is already running.
func TestHandlerRefusesADuplicateTargetTrigger(t *testing.T) {
	trigger := &fakeSync{accepted: false}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync/targets/rider-a"))
	assert.Equal(t, http.StatusConflict, response.Code, "POST /v1/sync/targets/rider-a status")
}

// A surface retry is triggered independently of either sync phase, and never
// starts a synchronization: it must never read the source or write a target.
func TestHandlerTriggersASurfaceRetryWithoutStartingASync(t *testing.T) {
	trigger := &fakeSync{annotateAccepted: true}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync/surface"))
	assert.Equal(t, http.StatusAccepted, response.Code, "surface retry status")
	assert.Equal(t, 1, trigger.annotateCalls, "TriggerAnnotate calls")
	assert.Empty(t, trigger.phases, "the surface retry also triggered a phase")
}

// The surface retry shares the same single-flight guard as an ordinary sync.
func TestHandlerRejectsAnOverlappingSurfaceRetry(t *testing.T) {
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, &fakeSync{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync/surface"))
	assert.Equal(t, http.StatusConflict, response.Code, "surface retry status")
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

// historyStateFixture holds three recorded runs, newest first.
func historyStateFixture() *fakeState {
	endedAt := time.Date(2026, time.August, 18, 6, 30, 0, 0, time.UTC)

	return &fakeState{history: []recordedRun{
		{reference: "aaaaaaaaaaaa", phaseRun: phaseRun{
			phase: "targets", completedAt: endedAt, outcome: "failed", detail: "destination", created: 1,
		}},
		{reference: "bbbbbbbbbbbb", phaseRun: phaseRun{
			phase: "source", completedAt: endedAt.Add(-time.Hour), outcome: "succeeded", sourceStages: 12,
		}},
		{reference: "cccccccccccc", phaseRun: phaseRun{
			phase: "targets", completedAt: endedAt.Add(-2 * time.Hour), outcome: "succeeded", updated: 2,
		}},
	}}
}

func historyPage(t *testing.T, handler http.Handler, query string) syncRunsView {
	t.Helper()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/sync/runs"+query))
	require.Equal(t, http.StatusOK, response.Code, "status")
	var view syncRunsView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the history")

	return view
}

// The history is bounded on the way out as well as in storage: a caller reads
// it a page at a time, following the cursor the page before it ended with.
func TestHandlerServesTheRecordedHistoryOnePageAtATime(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, historyStateFixture())

	first := historyPage(t, handler, "?limit=2")
	require.Len(t, first.Runs, 2, "the first page")
	assert.Equal(t, []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb"},
		[]string{first.Runs[0].Reference, first.Runs[1].Reference}, "the newest runs, newest first")
	assert.Equal(t, "targets", first.Runs[0].Phase, "phase")
	assert.Equal(t, "failed", first.Runs[0].Result, "result")
	assert.Equal(t, "destination", first.Runs[0].Failure, "failure")
	assert.Equal(t, "2026-08-18T06:30:00Z", first.Runs[0].CompletedAt, "completion")
	assert.Equal(t, 12, first.Runs[1].SourceStages, "source stages")
	require.NotEmpty(t, first.Next, "a cursor for the page after the first")

	second := historyPage(t, handler, "?limit=2&after="+first.Next)
	require.Len(t, second.Runs, 1, "the page after the first")
	assert.Equal(t, "cccccccccccc", second.Runs[0].Reference, "the oldest run")
	assert.Empty(t, second.Next, "a cursor past the oldest recorded run")
}

// A page carries the aggregate record of a run and nothing else. Anything a run
// touched — the routes it moved, their geometry, the identifiers the provider
// knows them by, whatever it said when it refused — stays out of it.
func TestHandlerServesNothingAboutWhatARunTouched(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, historyStateFixture())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/sync/runs?limit=1"))
	require.Equal(t, http.StatusOK, response.Code, "status")
	var page struct {
		Runs []map[string]any `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page), "decoding the history")
	require.Len(t, page.Runs, 1, "the page")
	fields := make([]string, 0, len(page.Runs[0]))
	for field := range page.Runs[0] {
		fields = append(fields, field)
	}
	slices.Sort(fields)
	assert.Equal(t, []string{
		"completed_at", "created", "deleted", "failure", "phase", "reference", "result", "source_stages", "updated",
	}, fields, "the fields a recorded run is served as")
}

// State this service cannot read is its own fault, and it says so as one rather
// than serving an empty history that would read as "nothing has run".
func TestHandlerReportsAHistoryItCannotRead(t *testing.T) {
	state := historyStateFixture()
	state.historyErr = errors.New("state is unavailable")
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/sync/runs"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "status")
}

// A cursor this service did not issue is the caller's mistake, and answering it
// with the newest page would silently restart a walk through the history.
func TestHandlerRefusesAHistoryCursorItDidNotIssue(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, historyStateFixture())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/sync/runs?after=the-newest-one"))
	assert.Equal(t, http.StatusBadRequest, response.Code, "status")
	assert.Contains(t, response.Body.String(), "invalid_request", "the error code")
}

// The page size is bounded so one request cannot read the whole retained window.
func TestHandlerRefusesAPageSizeItWillNotServe(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, historyStateFixture())

	for _, limit := range []string{"0", "-1", "1000", "all"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/sync/runs?limit="+limit))
		assert.Equal(t, http.StatusBadRequest, response.Code, "status for limit="+limit)
	}
}

// An empty history is a page with nothing in it, not a missing list: a caller
// reading runs out of the response must not have to guard against null.
func TestHandlerServesAnEmptyHistoryAsAnEmptyPage(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, &fakeState{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/sync/runs"))
	require.Equal(t, http.StatusOK, response.Code, "status")
	assert.JSONEq(t, `{"runs":[]}`, response.Body.String(), "the empty page")
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
	assert.Empty(t, view.Sync.Surface.Generation, "a service with no index named one")
	assert.Empty(t, view.Sync.Surface.BuiltAt, "a service with no index dated one")
}

// A stage that keeps failing classification otherwise looks exactly like one
// nobody has asked about yet — the difference incomplete exists to draw.
func TestHandlerReportsHowMuchOfTheLibraryCouldNotBeClassified(t *testing.T) {
	state := &fakeState{surfaceClassified: 1, surfaceTotal: 3}
	trigger := &fakeSync{surfaceIncomplete: 1}
	handler := newHandlerWithSync(t, &fakeOAuth{}, state, trigger)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	require.Equal(t, http.StatusOK, response.Code, "status")
	var view statusView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding status")
	assert.Equal(t, 1, view.Sync.Surface.Incomplete, "incomplete")
}

// The counts alone cannot say whether a full library is classified against a map
// from this week or from last spring. The generation is what makes a rebuild that
// has silently stopped happening visible.
func TestHandlerNamesTheMapBuildTheClassificationsCameFrom(t *testing.T) {
	builtAt := time.Date(2026, time.August, 17, 3, 41, 0, 0, time.UTC)
	handler := newHandlerWithSurfaceIndex(t, func() (string, time.Time, bool) {
		return "9f2c41ab77de", builtAt, true
	})

	view := statusOf(t, handler)
	assert.Equal(t, "9f2c41ab77de", view.Sync.Surface.Generation, "generation")
	assert.Equal(t, "2026-08-17T03:41:00Z", view.Sync.Surface.BuiltAt, "built_at")
}

// A build the state database remembers whose file did not survive a restart is
// exactly the case worth seeing, so the status reports what is loaded rather than
// what was last recorded.
func TestHandlerNamesNoMapBuildBeforeOneIsLoaded(t *testing.T) {
	handler := newHandlerWithSurfaceIndex(t, func() (string, time.Time, bool) {
		return "", time.Time{}, false
	})

	view := statusOf(t, handler)
	assert.Empty(t, view.Sync.Surface.Generation, "generation")
	assert.Empty(t, view.Sync.Surface.BuiltAt, "built_at")
}

// A deployment that names no staleness bound gets no freshness claim at all,
// rather than one derived from a bound nobody configured.
func TestHandlerOmitsTrustedInventoryFreshnessWithNoConfiguredBound(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{}, &fakeState{})

	view := statusOf(t, handler)
	assert.Nil(t, view.Sync.TrustedInventory, "a service with no stale-after bound reported a freshness claim")
}

// A service that has never completed a source run has no trusted inventory
// yet, which is not the same claim as a stale one.
func TestHandlerReportsFreshBeforeAnySuccessfulSourceRun(t *testing.T) {
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	handler := newHandlerWithStaleAfter(t, &fakeState{}, 24*time.Hour, now)

	view := statusOf(t, handler)
	require.NotNil(t, view.Sync.TrustedInventory, "a configured bound reported no freshness claim")
	assert.True(t, view.Sync.TrustedInventory.Fresh, "a service with no successful source run was reported stale")
	assert.Empty(t, view.Sync.TrustedInventory.LastSuccessAt, "a service with no successful source run named one")
}

// An unreadable trusted-inventory record must fail the whole status request
// rather than silently omitting freshness or guessing it.
func TestHandlerReportsUnavailableWhenTrustedInventoryStateCannotBeRead(t *testing.T) {
	state := &fakeState{lastSuccessErr: errors.New("state unavailable")}
	handler := newHandlerWithStaleAfter(t, state, 24*time.Hour, time.Now())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "status")
}

// The reported age and freshness are read from local state alone, against the
// configured bound.
func TestHandlerReportsTrustedInventoryAgeAndFreshness(t *testing.T) {
	lastSuccess := time.Date(2026, time.August, 16, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC)
	state := &fakeState{lastSuccessAt: map[string]time.Time{"source": lastSuccess}}
	handler := newHandlerWithStaleAfter(t, state, 24*time.Hour, now)

	view := statusOf(t, handler)
	require.NotNil(t, view.Sync.TrustedInventory, "want a freshness claim")
	assert.False(t, view.Sync.TrustedInventory.Fresh, "a 24h30m-old inventory against a 24h bound was reported fresh")
	assert.Equal(t, "2026-08-16T08:00:00Z", view.Sync.TrustedInventory.LastSuccessAt, "last_success_at")
	assert.Equal(t, int64(24*time.Hour/time.Second), view.Sync.TrustedInventory.MaxAgeSeconds, "max_age_seconds")
	assert.Equal(t, int64((24*time.Hour+30*time.Minute)/time.Second), view.Sync.TrustedInventory.AgeSeconds, "age_seconds")
}

// An age of exactly zero, read immediately after a successful refresh, is
// still reported rather than omitted alongside an omitempty int's zero value.
func TestHandlerReportsAZeroAgeRatherThanOmittingIt(t *testing.T) {
	now := time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC)
	state := &fakeState{lastSuccessAt: map[string]time.Time{"source": now}}
	handler := newHandlerWithStaleAfter(t, state, 24*time.Hour, now)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	assert.Contains(t, response.Body.String(), `"age_seconds":0`, "a zero age was omitted from the response")
}

// A recorded success later than now — a clock that has moved backwards, or a
// success that races ahead of it — must never be reported as a negative age.
func TestHandlerClampsANegativeAgeToZero(t *testing.T) {
	now := time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC)
	state := &fakeState{lastSuccessAt: map[string]time.Time{"source": now.Add(time.Hour)}}
	handler := newHandlerWithStaleAfter(t, state, 24*time.Hour, now)

	view := statusOf(t, handler)
	require.NotNil(t, view.Sync.TrustedInventory, "want a freshness claim")
	assert.Zero(t, view.Sync.TrustedInventory.AgeSeconds, "a negative age was not clamped to zero")
	assert.True(t, view.Sync.TrustedInventory.Fresh, "a clamped age was reported stale")
}

// fresh must agree with age_seconds < max_age_seconds even when
// sync.stale_after carries sub-second precision: both are derived from the
// same truncated seconds rather than fresh comparing the untruncated duration.
func TestHandlerKeepsFreshConsistentWithASubSecondStaleAfter(t *testing.T) {
	lastSuccess := time.Date(2026, time.August, 17, 8, 29, 58, 600_000_000, time.UTC)
	now := time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC)
	state := &fakeState{lastSuccessAt: map[string]time.Time{"source": lastSuccess}}
	handler := newHandlerWithStaleAfter(t, state, 1500*time.Millisecond, now)

	view := statusOf(t, handler)
	require.NotNil(t, view.Sync.TrustedInventory, "want a freshness claim")
	// A 1.4s age against a 1.5s bound truncates to equal integer seconds on
	// both sides: the untruncated duration comparison (1.4s < 1.5s) says
	// fresh, but the documented contract compares the reported seconds
	// (1 < 1), which says stale. fresh must follow the documented contract.
	assert.Equal(t, int64(1), view.Sync.TrustedInventory.MaxAgeSeconds, "max_age_seconds")
	assert.Equal(t, int64(1), view.Sync.TrustedInventory.AgeSeconds, "age_seconds")
	assert.False(t, view.Sync.TrustedInventory.Fresh, "fresh disagreed with age_seconds < max_age_seconds")
}

// A run that has not finished must not be reported as the last one that did.
// An operator who has just pressed a button would read "succeeded" as their
// answer, and nothing in the response would say otherwise.
func TestHandlerReportsARunInFlightRatherThanTheLastResult(t *testing.T) {
	state := convergenceStateFixture()
	state.lastRun = &phaseRun{
		phase:       "targets",
		completedAt: time.Date(2026, time.August, 18, 6, 0, 0, 0, time.UTC),
		outcome:     "succeeded",
	}
	view := statusOf(t, newHandlerWithLiveSync(t, state, SyncActivityState{
		Phase: SyncPhaseTargets, Running: true,
	}))

	assert.Equal(t, runningState, view.Sync.State, "sync state")
	// The finished run is still reported beside it. It remains the last thing
	// that happened; it is only no longer the answer to "what is happening".
	assert.Equal(t, "succeeded", view.Sync.LastResult, "last result")
	require.NotNil(t, view.Sync.Active, "the status reports no work under way")
	assert.Equal(t, "targets", view.Sync.Active.Phase, "active phase")
	assert.Equal(t, 2, view.Sync.Active.Targets, "configured targets")
	// The aggregate of the two slots in the fixture: one holds both stages, the
	// other owes both.
	assert.Equal(t, targetStagesView{Current: 2, Pending: 2}, view.Sync.Active.Stages, "active stages")
	assert.Empty(t, view.Sync.Active.StartsAt, "a run under way is being held back")
}

// A run is accepted before its first half starts. That window is short, and it
// is exactly when an operator is looking at the page.
func TestHandlerReportsAnAcceptedRunBeforeItsFirstHalfStarts(t *testing.T) {
	view := statusOf(t, newHandlerWithLiveSync(t, convergenceStateFixture(), SyncActivityState{Running: true}))

	assert.Equal(t, queuedState, view.Sync.State, "sync state")
	require.NotNil(t, view.Sync.Active, "the status reports no work under way")
	assert.Empty(t, view.Sync.Active.Phase, "a half was named before one started")
}

// Startup holds the first run back deliberately. Reporting that as "idle" would
// describe a service with nothing to do.
func TestHandlerReportsAFirstRunHeldBackByTheInitialDelay(t *testing.T) {
	startsAt := time.Date(2026, time.August, 18, 6, 5, 0, 0, time.UTC)
	view := statusOf(t, newHandlerWithLiveSync(t, convergenceStateFixture(), SyncActivityState{StartsAt: startsAt}))

	assert.Equal(t, delayedState, view.Sync.State, "sync state")
	require.NotNil(t, view.Sync.Active, "the status reports no work under way")
	assert.Equal(t, "2026-08-18T06:05:00Z", view.Sync.Active.StartsAt, "the instant the run is held until")
	assert.Empty(t, view.Sync.Active.Phase, "a half was named before the run started")
}

// A manual trigger during that startup delay is work happening now, and the
// instant the held-back run is still due at belongs to a different run.
func TestHandlerOmitsTheHeldBackInstantFromARunAlreadyUnderWay(t *testing.T) {
	activity := SyncActivityState{
		StartsAt: time.Date(2026, time.August, 18, 6, 5, 0, 0, time.UTC),
		Phase:    SyncPhaseSource,
		Running:  true,
	}
	view := statusOf(t, newHandlerWithLiveSync(t, convergenceStateFixture(), activity))

	assert.Equal(t, runningState, view.Sync.State, "sync state")
	require.NotNil(t, view.Sync.Active, "the status reports no work under way")
	assert.Empty(t, view.Sync.Active.StartsAt, "a due instant was reported for a run already under way")
}

// Nothing under way is the absence of a state rather than one more state to
// report, so the group is absent too.
func TestHandlerOmitsWorkUnderWayWhenNothingIsRunning(t *testing.T) {
	view := statusOf(t, newHandlerWithLiveSync(t, convergenceStateFixture(), SyncActivityState{}))

	assert.Equal(t, "idle", view.Sync.State, "sync state")
	assert.Nil(t, view.Sync.Active, "the status reports work under way with nothing running")
}

// A refused trigger is a conflict and nothing else. It must not leave the
// status describing a second run that was never started.
func TestHandlerRefusesADuplicateTriggerWithoutManufacturingASecondRun(t *testing.T) {
	syncRuns := &fakeSync{activity: SyncActivityState{Phase: SyncPhaseSource, Running: true}}
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, syncRuns)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	require.Equal(t, http.StatusConflict, response.Code, "POST /v1/sync status")

	view := statusOf(t, handler)
	assert.Equal(t, runningState, view.Sync.State, "sync state")
	require.NotNil(t, view.Sync.Active, "the status reports no work under way")
	assert.Equal(t, "source", view.Sync.Active.Phase, "active phase")
}

func TestSyncFuncsAdaptAPairOfFunctions(t *testing.T) {
	activity := SyncActivityState{Phase: SyncPhaseSource, Running: true}

	var asked SyncPhase
	var annotateCalls int

	var askedTarget string

	funcs := SyncFuncs{
		TriggerFunc: func(phase SyncPhase) bool {
			asked = phase

			return true
		},
		TriggerTargetFunc: func(targetID string) bool {
			askedTarget = targetID

			return true
		},
		ActivityFunc: func() SyncActivityState { return activity },
		TriggerAnnotateFunc: func() bool {
			annotateCalls++

			return true
		},
		SurfaceIncompleteFunc: func() int { return 3 },
	}

	assert.True(t, funcs.Trigger(SyncPhaseTargets), "trigger")
	assert.Equal(t, SyncPhaseTargets, asked, "the phase the trigger was asked for")
	assert.True(t, funcs.TriggerTarget("rider-a"), "trigger target")
	assert.Equal(t, "rider-a", askedTarget, "the target the trigger was asked for")
	assert.Equal(t, activity, funcs.Activity(), "activity")
	assert.True(t, funcs.TriggerAnnotate(), "TriggerAnnotate()")
	assert.Equal(t, 1, annotateCalls, "TriggerAnnotate calls")
	assert.Equal(t, 3, funcs.SurfaceIncomplete(), "SurfaceIncomplete()")
}

// A process whose runs begin and end inside the request that asked for one has
// no in-flight window to describe, so it wires no ActivityFunc at all.
func TestSyncFuncsReportNothingUnderWayWithoutAnActivityFunc(t *testing.T) {
	funcs := SyncFuncs{
		TriggerFunc:       func(SyncPhase) bool { return false },
		TriggerTargetFunc: func(string) bool { return false },
	}

	assert.Equal(t, SyncActivityState{}, funcs.Activity(), "activity")
}

// A process that tracks no incomplete count wires no SurfaceIncompleteFunc,
// and zero is the honest answer for one that tracks none.
func TestSyncFuncsReportNoIncompleteCountWithoutAFunc(t *testing.T) {
	funcs := SyncFuncs{TriggerFunc: func(SyncPhase) bool { return false }}

	assert.Zero(t, funcs.SurfaceIncomplete(), "SurfaceIncomplete()")
}

// An operator who has started the browser flow and not finished it is in a
// state the targets table cannot hold, and it matters: told "not connected",
// they would start the flow a second time and invalidate the first.
func TestHandlerReportsAnInFlightAuthorizationAsPending(t *testing.T) {
	state := &fakeState{
		targets: []fakeTarget{
			{id: "rider-a", authorization: "not_authorized"},
			{id: "rider-b", authorization: "needs_reauthorization"},
		},
		pendingAuth: []string{"rider-a", "rider-b"},
	}
	handler := newHandlerWithTargets(t, state, "rider-a", "rider-b")

	view := statusOf(t, handler)

	require.Len(t, view.Targets, 2, "targets")
	for _, target := range view.Targets {
		assert.Equalf(t, "pending", target.Authorization, "%s authorisation", target.ID)
		// Pending is still not authorised. Nothing may be written to a slot whose
		// flow has not come back, and the one word each target gets says so.
		assert.Equalf(t, convergenceUnauthorized, target.Convergence, "%s convergence", target.ID)
	}
	assert.False(t, view.Ready, "a target midway through connecting reported the service ready")
}

// A slot that already holds a working refresh token keeps it until a fresh flow
// replaces it, so starting one must not report the account as unconnectable.
func TestHandlerKeepsAnAuthorizedTargetAuthorizedDuringAFreshFlow(t *testing.T) {
	state := &fakeState{
		targets:     []fakeTarget{{id: "rider-a", authorization: "authorized"}},
		pendingAuth: []string{"rider-a"},
	}
	handler := newHandlerWithTargets(t, state, "rider-a")

	view := statusOf(t, handler)

	require.Len(t, view.Targets, 1, "targets")
	assert.Equal(t, "authorized", view.Targets[0].Authorization, "authorisation")
	assert.True(t, view.Ready, "an authorised target stopped the service being ready")
}

func TestHandlerReportsUnreadablePendingAuthorizationsAsUnavailable(t *testing.T) {
	state := &fakeState{pendingAuthErr: errors.New("state unavailable")}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "status")
}

func TestHandlerReportsUnreadableScheduleAsUnavailable(t *testing.T) {
	state := &fakeState{scheduleErr: errors.New("state unavailable")}
	handler := newHandler(t, &fakeOAuth{}, state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/status"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "status")
}

func TestHandlerRejectsOverlappingManualSync(t *testing.T) {
	handler := newHandlerWithSync(t, &fakeOAuth{}, &fakeState{}, &fakeSync{})
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
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "duplicate targets", options: &Options{
			TargetIDs:        []string{"rider-a", "rider-a"},
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "no basemap at all", options: &Options{
			TargetIDs:        []string{"rider-a"},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "plaintext tile style", options: &Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         []Basemap{{Name: "Streets", StyleURL: "http://tiles.example.test/style.json"}},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "dark tile style on another origin", options: &Options{
			TargetIDs: []string{"rider-a"},
			Basemaps: []Basemap{{
				Name:         "Streets",
				StyleURL:     testTileStyleURL,
				StyleURLDark: "https://dark.example.test/styles/dark",
			}},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "unnamed basemap", options: &Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         []Basemap{{Name: "  ", StyleURL: testTileStyleURL}},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "two basemaps under one name", options: &Options{
			TargetIDs: []string{"rider-a"},
			Basemaps: []Basemap{
				{Name: "Streets", StyleURL: testTileStyleURL},
				{Name: "Streets", StyleURL: "https://imagery.example.test/styles/hybrid"},
			},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "plaintext dark tile style", options: &Options{
			TargetIDs: []string{"rider-a"},
			Basemaps: []Basemap{{
				Name:         "Streets",
				StyleURL:     testTileStyleURL,
				StyleURLDark: "http://tiles.example.test/styles/dark",
			}},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
		{name: "dark cartography with a dark style of its own", options: &Options{
			TargetIDs: []string{"rider-a"},
			Basemaps: []Basemap{{
				Name:            "Satellite",
				StyleURL:        testTileStyleURL,
				StyleURLDark:    "https://tiles.example.test/styles/dark",
				DarkCartography: true,
			}},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.options, &fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{})
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
			Basemaps:         testBasemaps(),
			AccessVerifier:   verifier,
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// newHandlerWithSurfaceIndex builds a handler that reads the live surface index
// through the given accessor, which is how the composition root supplies it.
func newHandlerWithSurfaceIndex(t *testing.T, index func() (string, time.Time, bool)) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
			SurfaceIndexFunc: index,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
}

func newHandler(t *testing.T, oauthService OAuth, state State) *Handler {
	return newHandlerWithSync(t, oauthService, state, &fakeSync{accepted: true})
}

// newHandlerWithStaleAfter builds a handler that reports trusted-inventory
// freshness, with now fixed so the reported age is deterministic.
func newHandlerWithStaleAfter(t *testing.T, state State, staleAfter time.Duration, now time.Time) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
			SourceStaleAfter: staleAfter,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")
	handler.now = func() time.Time { return now }

	return handler
}

// newHandlerWithTargets builds a handler configured for more than one slot, which
// is what a partial target failure needs to be visible at all.
func newHandlerWithTargets(t *testing.T, state State, targetIDs ...string) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        targetIDs,
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// newHandlerWithLiveSync builds a two-slot handler whose synchronization process
// reports the given work as unfinished.
func newHandlerWithLiveSync(t *testing.T, state State, activity SyncActivityState) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a", "rider-b"},
			Basemaps:         testBasemaps(),
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true, activity: activity}, &fakeAssets{},
	)
	require.NoError(t, err, "New()")

	return handler
}

func newHandlerWithSync(t *testing.T, oauthService OAuth, state State, syncRuns Sync) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Basemaps:         testBasemapsWithDark(),
			SourceBaseURL:    testSourceBaseURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		oauthService, state, syncRuns, &fakeAssets{},
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

type fakeSync struct {
	activity          SyncActivityState
	phases            []SyncPhase
	targetTriggers    []string
	calls             int
	annotateCalls     int
	surfaceIncomplete int
	accepted          bool
	annotateAccepted  bool
}

func (t *fakeSync) Trigger(phase SyncPhase) bool {
	t.calls++
	t.phases = append(t.phases, phase)

	return t.accepted
}

func (t *fakeSync) TriggerTarget(targetID string) bool {
	t.calls++
	t.targetTriggers = append(t.targetTriggers, targetID)

	return t.accepted
}

func (t *fakeSync) Activity() SyncActivityState { return t.activity }

func (t *fakeSync) TriggerAnnotate() bool {
	t.annotateCalls++

	return t.annotateAccepted
}

func (t *fakeSync) SurfaceIncomplete() int { return t.surfaceIncomplete }

type fakeAssets struct{}

func (*fakeAssets) Index(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := writer.Write([]byte("<!doctype html><title>domestique</title>")); err != nil {
		return
	}
}

func (*fakeAssets) Static(writer http.ResponseWriter, _ *http.Request) {
	// http.ServeContent leaves a type the caller already chose alone, which is
	// how the manifest keeps its own. The fake has to do the same or the test
	// would be checking the fake rather than the route.
	if writer.Header().Get("Content-Type") == "" {
		writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	if _, err := writer.Write([]byte("export default null;")); err != nil {
		return
	}
}

// recordedRun is one run of the fake's history, which it holds newest first.
type recordedRun struct {
	reference string
	phaseRun
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
	historyErr        error
	scheduleErr       error
	coverageErr       error
	reprocessErr      error
	sourceStageErr    error
	targetStageErr    error
	targetRunErr      error
	pendingAuthErr    error
	lastSuccessErr    error
	lastSuccessAt     map[string]time.Time
	targetStages      map[string][]storedStage
	reprocessed       [][2]int64
	targets           []fakeTarget
	pendingAuth       []string
	sourceStages      []storedStage
	targetRuns        []fakeTargetRun
	lastRun           *phaseRun
	surfaceHash       string
	coordinates       json.RawMessage
	surfaceRanges     json.RawMessage
	summaries         []route.Summary
	phaseRuns         []phaseRun
	history           []recordedRun
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

// ForEachPendingAuthorization reports the slots this fake is holding an
// in-flight browser flow for. Empty is the ordinary case, which is why every
// test that does not name one reads exactly as it did before.
func (s *fakeState) ForEachPendingAuthorization(_ context.Context, visit func(string) error) error {
	if s.pendingAuthErr != nil {
		return s.pendingAuthErr
	}
	for _, targetID := range s.pendingAuth {
		if err := visit(targetID); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) ForEachSourceStage(
	_ context.Context,
	visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string) error,
) error {
	if s.sourceStageErr != nil {
		return s.sourceStageErr
	}
	for _, stage := range s.sourceStages {
		if err := visit(route.ProviderVeloPlanner, stage.routeID, stage.stageOrder, stage.revision, "hash"); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) ForEachTargetStage(
	_ context.Context,
	targetID string,
	visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error,
) error {
	if s.targetStageErr != nil {
		return s.targetStageErr
	}
	for _, stage := range s.targetStages[targetID] {
		if err := visit(route.ProviderVeloPlanner, stage.routeID, stage.stageOrder, stage.revision, "encoded-hash", 900+stage.routeID); err != nil {
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
	provider route.Provider,
	routeID int64,
	stageOrder int,
) (route.Summary, json.RawMessage, bool, error) {
	for index := range s.summaries {
		summary := s.summaries[index]
		if summary.Provider == provider && summary.RouteID == routeID && summary.StageOrder == stageOrder {
			return summary, s.coordinates, true, nil
		}
	}

	return route.Summary{}, nil, false, nil
}

// StageSurface answers only for the geometry a classification was measured
// against, exactly as the store does.
func (s *fakeState) StageSurface(
	_ context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	contentHash string,
) (ranges json.RawMessage, matchedMetres float64, found bool, err error) {
	if s.surfaceErr != nil {
		return nil, 0, false, s.surfaceErr
	}
	for index := range s.summaries {
		summary := s.summaries[index]
		if summary.Provider != provider || summary.RouteID != routeID || summary.StageOrder != stageOrder {
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
func (s *fakeState) LastSyncRun(context.Context) (time.Time, string, string, int, int, int, int, bool, error) {
	if s.lastRun == nil {
		return time.Time{}, "", "", 0, 0, 0, 0, false, nil
	}
	run := s.lastRun

	return run.completedAt, run.outcome, run.detail,
		run.sourceStages, run.created, run.updated, run.deleted, true, nil
}

func (s *fakeState) LastSuccessfulPhaseCompletion(
	_ context.Context, phase string,
) (completedAt time.Time, found bool, err error) {
	if s.lastSuccessErr != nil {
		return time.Time{}, false, s.lastSuccessErr
	}
	completedAt, found = s.lastSuccessAt[phase]

	return completedAt, found, nil
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

// ForEachSyncRun pages over the held history. Its cursor is the position the
// next page starts at, which is what the store's own cursor is.
func (s *fakeState) ForEachSyncRun(
	_ context.Context,
	after string,
	limit int,
	visit func(reference, phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error,
) (next string, usable bool, err error) {
	if s.historyErr != nil {
		return "", false, s.historyErr
	}
	start := 0
	if after != "" {
		parsed, err := strconv.Atoi(after)
		// A cursor this fake did not issue is the caller's input, and the store
		// reports it as unusable rather than as a failure.
		if err != nil {
			return "", false, nil //nolint:nilerr // The cursor is unusable, not broken.
		}
		start = min(parsed, len(s.history))
	}
	for index := start; index < min(start+limit, len(s.history)); index++ {
		run := s.history[index]
		if err := visit(
			run.reference, run.phase, run.completedAt, run.outcome, run.detail,
			run.sourceStages, run.created, run.updated, run.deleted,
		); err != nil {
			return "", false, err
		}
	}
	if start+limit >= len(s.history) {
		return "", true, nil
	}

	return strconv.Itoa(start + limit), true, nil
}

func (s *fakeState) RequestStageReprocess(_ context.Context, provider route.Provider, routeID int64, stageOrder int) (bool, error) {
	if s.reprocessErr != nil {
		return false, s.reprocessErr
	}
	for index := range s.summaries {
		if s.summaries[index].Provider == provider && s.summaries[index].RouteID == routeID && s.summaries[index].StageOrder == stageOrder {
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
