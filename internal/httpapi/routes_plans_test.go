package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

const planRoutePath = "/v1/plans/route"

const (
	validPlanRouteBody = `{"profile":"trekking","waypoints":[{"longitude":8,"latitude":49},{"longitude":8.1,"latitude":49.1}]}`
	validPlanWriteBody = `{"name":"Alpine loop","profile":"trekking","waypoints":[{"longitude":8,"latitude":49},{"longitude":8.1,"latitude":49.1}],"published":false}`
)

// fakePlans is a minimal, version-checked stand-in for *plan.Service: Replace
// and Delete refuse an expectedVersion that does not match what is "stored",
// the same as the real service.
type fakePlans struct {
	routeErr  error
	createErr error
	listErr   error
	getErr    error
	list      []plan.Plan
	measured  plan.Measured
	stored    plan.Plan
	hasStored bool
}

type fakeSurfaceClassifier struct {
	classification *SurfaceClassification
	err            error
	points         []route.Point
}

func (f *fakeSurfaceClassifier) Classify(_ context.Context, points []route.Point) (*SurfaceClassification, error) {
	f.points = append([]route.Point(nil), points...)

	return f.classification, f.err
}

func (p *fakePlans) Route(context.Context, []plan.Waypoint, plan.Profile) (plan.Measured, error) {
	return p.measured, p.routeErr
}

func (p *fakePlans) Create(
	_ context.Context, name string, profile plan.Profile, waypoints []plan.Waypoint, cues bool,
) (plan.Plan, error) {
	if p.createErr != nil {
		return plan.Plan{}, p.createErr
	}
	p.stored = plan.Plan{
		ID: 7, Name: name, Profile: profile, Waypoints: waypoints,
		Geometry: p.measured.Geometry, DistanceMetres: p.measured.DistanceMetres, AscentMetres: p.measured.AscentMetres,
		Published: false, Cues: cues, Version: 1,
	}
	p.hasStored = true

	return p.stored, nil
}

func (p *fakePlans) Replace(
	_ context.Context, id, expectedVersion int64, name string, profile plan.Profile, waypoints []plan.Waypoint,
	published, cues bool,
) (plan.Plan, error) {
	if !p.hasStored || p.stored.ID != id {
		return plan.Plan{}, plan.ErrNotFound
	}
	if p.stored.Version != expectedVersion {
		return plan.Plan{}, plan.ErrVersionMismatch
	}
	p.stored = plan.Plan{
		ID: id, Name: name, Profile: profile, Waypoints: waypoints,
		Geometry: p.measured.Geometry, DistanceMetres: p.measured.DistanceMetres, AscentMetres: p.measured.AscentMetres,
		Published: published, Cues: cues, Version: expectedVersion + 1,
	}

	return p.stored, nil
}

func (p *fakePlans) Delete(_ context.Context, id, expectedVersion int64) error {
	if !p.hasStored || p.stored.ID != id {
		return plan.ErrNotFound
	}
	if p.stored.Version != expectedVersion {
		return plan.ErrVersionMismatch
	}
	p.hasStored = false

	return nil
}

func (p *fakePlans) Get(_ context.Context, id int64) (plan.Plan, bool, error) {
	if p.getErr != nil {
		return plan.Plan{}, false, p.getErr
	}
	if !p.hasStored || p.stored.ID != id {
		return plan.Plan{}, false, nil
	}

	return p.stored, true, nil
}

func (p *fakePlans) List(_ context.Context) ([]plan.Plan, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	if p.list != nil {
		return p.list, nil
	}
	if p.hasStored {
		return []plan.Plan{p.stored}, nil
	}

	return nil, nil
}

// plansHandler builds a handler over the given session identity and Plans
// port. plans nil is a build with no routing engine configured.
func plansHandler(t *testing.T, sessions Sessions, plans Plans) *Handler {
	return plansHandlerWithSurface(t, sessions, plans, nil)
}

func plansHandlerWithSurface(
	t *testing.T, sessions Sessions, plans Plans, classifier SurfaceClassifier,
) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			schemaCache:       testSchemaCache,
			Alerts:            &fakeAlerts{},
			Tasks:             &fakeTasks{},
			Settings:          settingsWith(testBasemaps()),
			Sessions:          sessions,
			BrowserOriginURL:  testBrowserOriginURL,
			Plans:             plans,
			SurfaceClassifier: classifier,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// planOperations is every plan endpoint, with a body the contract validator
// accepts, so a refusal under test is the handler's own and not the
// document's.
func planOperations() []struct{ name, method, path, body, ifMatch string } {
	return []struct{ name, method, path, body, ifMatch string }{
		{"preview", http.MethodPost, planRoutePath, validPlanRouteBody, ""},
		{"list", http.MethodGet, plansPath, "", ""},
		{"create", http.MethodPost, plansPath, validPlanWriteBody, ""},
		{"get", http.MethodGet, plansPath + "/7", "", ""},
		{"replace", http.MethodPut, plansPath + "/7", validPlanWriteBody, "1"},
		{"delete", http.MethodDelete, plansPath + "/7", "", "1"},
	}
}

// planRequest is one plan operation as a signed-in browser sends it.
func planRequest(method, path, body, ifMatchValue string) *http.Request {
	request := authenticatedRequest(method, path)
	if body != "" {
		request = authenticatedRequestWithBody(method, path, body)
	}
	if ifMatchValue != "" {
		request.Header.Set("If-Match", ifMatchValue)
	}

	return request
}

func TestPlanRoutesRefuseANonAdminSession(t *testing.T) {
	for _, operation := range planOperations() {
		t.Run(operation.name, func(t *testing.T) {
			handler := plansHandler(t, nonAdminSessions("rider-a"), &fakePlans{})

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, planRequest(operation.method, operation.path, operation.body, operation.ifMatch))

			assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		})
	}
}

// Without a routing engine configured, the whole group is unregistered: every
// address answers the same not-found shape as one the document never named.
func TestPlanRoutesAreUnregisteredWithoutAPlansPort(t *testing.T) {
	for _, operation := range planOperations() {
		t.Run(operation.name, func(t *testing.T) {
			handler := plansHandler(t, newFakeSessions(), nil)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, planRequest(operation.method, operation.path, operation.body, operation.ifMatch))

			assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), `"error"`)
		})
	}
}

func TestPlanDocumentsFollowPlanningAndAdminAccess(t *testing.T) {
	for _, path := range []string{"/plan", "/plan/7"} {
		t.Run(path, func(t *testing.T) {
			configured := plansHandler(t, newFakeSessions(), &fakePlans{})
			anonymous := httptest.NewRecorder()
			configured.ServeHTTP(anonymous, httptest.NewRequestWithContext(
				t.Context(), http.MethodGet, path, http.NoBody))
			assert.Equal(t, http.StatusUnauthorized, anonymous.Code, anonymous.Body.String())

			browser := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
			browser.Header.Set("Accept", "text/html")
			redirect := httptest.NewRecorder()
			configured.ServeHTTP(redirect, browser)
			assert.Equal(t, http.StatusFound, redirect.Code, redirect.Body.String())
			assert.Equal(t, loginPath, redirect.Header().Get("Location"))

			admin := httptest.NewRecorder()
			configured.ServeHTTP(admin, authenticatedRequest(http.MethodGet, path))
			assert.Equal(t, http.StatusOK, admin.Code, admin.Body.String())
			assert.Equal(t, cacheDocument, admin.Header().Get("Cache-Control"))
			assert.Contains(t, admin.Body.String(), "<!doctype html>")

			nonAdmin := plansHandler(t, nonAdminSessions("rider-a"), &fakePlans{})
			refused := httptest.NewRecorder()
			nonAdmin.ServeHTTP(refused, authenticatedRequest(http.MethodGet, path))
			assert.Equal(t, http.StatusNotFound, refused.Code, refused.Body.String())

			unconfigured := plansHandler(t, newFakeSessions(), nil)
			missing := httptest.NewRecorder()
			unconfigured.ServeHTTP(missing, authenticatedRequest(http.MethodGet, path))
			assert.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
		})
	}
}

func TestWebUIConfigReportsWhetherPlanningIsConfigured(t *testing.T) {
	for name, plans := range map[string]Plans{"configured": &fakePlans{}, "unconfigured": nil} {
		t.Run(name, func(t *testing.T) {
			handler := plansHandler(t, newFakeSessions(), plans)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())

			var body struct {
				Planning bool `json:"planning"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, plans != nil, body.Planning)
		})
	}
}

func TestPreviewPlanRouteReturnsGeometryDistanceAndAscent(t *testing.T) {
	elevationMetres := 410.5
	handler := plansHandler(t, newFakeSessions(), &fakePlans{measured: plan.Measured{
		Geometry: []route.Point{
			{Longitude: 8, Latitude: 49},
			{Longitude: 8.1, Latitude: 49.1, Elevation: &elevationMetres},
		},
		DistanceMetres: 1200, AscentMetres: 42,
	}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body openapi.PlanRoutePreview
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.InDelta(t, 1200, body.DistanceMetres, 0)
	assert.InDelta(t, 42, body.AscentMetres, 0)
	require.NotNil(t, body.DescentMetres, "a line that only climbs still has a descent: none")
	assert.InDelta(t, 0, *body.DescentMetres, 0)
	require.Len(t, body.Geometry.Coordinates, 2)
	assert.Len(t, body.Geometry.Coordinates[0], 2, "the first point carries no elevation")
	require.Len(t, body.Geometry.Coordinates[1], 3, "the second point carries its elevation")
	assert.InDelta(t, elevationMetres, body.Geometry.Coordinates[1][2], 0)
}

func TestPreviewPlanRouteCarriesThePredictedTimePerWaypoint(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{measured: plan.Measured{
		Geometry:       []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.1, Latitude: 49.1}},
		DistanceMetres: 1200, AscentMetres: 42, MovingSeconds: 300,
		Progress: []plan.Progress{
			{DistanceMetres: 0},
			{DistanceMetres: 1200, MovingSeconds: 300},
		},
	}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body openapi.PlanRoutePreview
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotNil(t, body.MovingSeconds)
	assert.InDelta(t, 300, *body.MovingSeconds, 0)
	require.Len(t, body.WaypointProgress, 2)
	assert.Nil(t, body.WaypointProgress[0].MovingSeconds, "the start has taken no time")
	require.NotNil(t, body.WaypointProgress[1].MovingSeconds)
	assert.InDelta(t, 300, *body.WaypointProgress[1].MovingSeconds, 0)
	assert.InDelta(t, 1200, body.WaypointProgress[1].DistanceMetres, 0)
}

func TestPreviewPlanRouteCarriesWhereTheRiderWalks(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{measured: plan.Measured{
		Geometry:       []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.1, Latitude: 49.1}},
		DistanceMetres: 1200, AscentMetres: 42,
		Pushing: []plan.Window{{StartMetres: 300, EndMetres: 420}},
	}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body openapi.PlanRoutePreview
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, []openapi.PlanWindow{{StartMetres: 300, EndMetres: 420}}, body.Pushing)
}

func TestPreviewPlanRouteOmitsATimeItCannotPredict(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{measured: plan.Measured{
		Geometry:       []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.1, Latitude: 49.1}},
		DistanceMetres: 1200, AscentMetres: 42,
		Progress: []plan.Progress{{DistanceMetres: 0}, {DistanceMetres: 1200}},
	}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body openapi.PlanRoutePreview
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Nil(t, body.MovingSeconds)
	require.Len(t, body.WaypointProgress, 2, "a waypoint still knows its distance")
	assert.Nil(t, body.WaypointProgress[1].MovingSeconds)
}

func TestPlanResponsesCarryCurrentSurfaceClassification(t *testing.T) {
	geometry := []route.Point{
		{Longitude: 8, Latitude: 49},
		{Longitude: 8.1, Latitude: 49.1},
	}
	classification := &SurfaceClassification{
		Ranges:        []SurfaceRange{{Kind: "gravel", StartIndex: 0, EndIndex: 1}},
		MatchedMetres: 1200,
	}
	for _, operation := range []struct {
		readResult func(*testing.T, []byte)
		name       string
		method     string
		path       string
		body       string
		ifMatch    string
		status     int
	}{
		{name: "preview", method: http.MethodPost, path: planRoutePath, body: validPlanRouteBody, status: http.StatusOK,
			readResult: func(t *testing.T, body []byte) {
				var response openapi.PlanRoutePreview
				require.NoError(t, json.Unmarshal(body, &response))
				require.NotNil(t, response.Surface)
				assert.Equal(t, "gravel", string(response.Surface.Ranges[0].Kind))
				assert.InDelta(t, 1200, response.Surface.MatchedMetres, 0)
			}},
		{name: "create", method: http.MethodPost, path: plansPath, body: validPlanWriteBody, status: http.StatusCreated,
			readResult: func(t *testing.T, body []byte) {
				var response openapi.Plan
				require.NoError(t, json.Unmarshal(body, &response))
				require.NotNil(t, response.Surface)
				assert.Equal(t, "gravel", string(response.Surface.Ranges[0].Kind))
			}},
		{name: "replace", method: http.MethodPut, path: plansPath + "/7", body: validPlanWriteBody, ifMatch: "1", status: http.StatusOK,
			readResult: func(t *testing.T, body []byte) {
				var response openapi.Plan
				require.NoError(t, json.Unmarshal(body, &response))
				require.NotNil(t, response.Surface)
				assert.Equal(t, "gravel", string(response.Surface.Ranges[0].Kind))
			}},
		{name: "get", method: http.MethodGet, path: plansPath + "/7", status: http.StatusOK,
			readResult: func(t *testing.T, body []byte) {
				var response openapi.Plan
				require.NoError(t, json.Unmarshal(body, &response))
				require.NotNil(t, response.Surface)
				assert.Equal(t, "gravel", string(response.Surface.Ranges[0].Kind))
			}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			fake := &fakePlans{
				measured: plan.Measured{Geometry: geometry},
				stored:   plan.Plan{ID: 7, Version: 1, Geometry: geometry}, hasStored: true,
			}
			classifier := &fakeSurfaceClassifier{classification: classification}
			handler := plansHandlerWithSurface(t, newFakeSessions(), fake, classifier)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, planRequest(operation.method, operation.path, operation.body, operation.ifMatch))
			require.Equal(t, operation.status, response.Code, response.Body.String())
			operation.readResult(t, response.Body.Bytes())
			assert.Equal(t, geometry, classifier.points)
		})
	}
}

func TestPlanSurfaceClassificationFailureIsOmitted(t *testing.T) {
	classifier := &fakeSurfaceClassifier{err: errors.New("surface lookup failed at 8,49")}
	handler := plansHandlerWithSurface(t, newFakeSessions(), &fakePlans{measured: plan.Measured{
		Geometry: []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.1, Latitude: 49.1}},
	}}, classifier)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "surface lookup failed")
	assert.NotContains(t, response.Body.String(), "matchedMetres")
}

func TestPlanSurfaceClassificationUnavailableIsOmitted(t *testing.T) {
	classifier := &fakeSurfaceClassifier{}
	handler := plansHandlerWithSurface(t, newFakeSessions(), &fakePlans{measured: plan.Measured{
		Geometry: []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.1, Latitude: 49.1}},
	}}, classifier)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "surface")
}

func TestCreatePlanStoresADraft(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{measured: plan.Measured{DistanceMetres: 500, AscentMetres: 10}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, plansPath, validPlanWriteBody, ""))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())

	var body openapi.Plan
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.False(t, body.Published, "a create always stores a draft")
	assert.Equal(t, int64(1), body.Version)
}

func TestCreatePlanRejectsAValidationError(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{
		createErr: fmt.Errorf("%w: name is required", plan.ErrInvalid),
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, plansPath, validPlanWriteBody, ""))
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "name is required")
}

func TestCreatePlanReportsARoutingFailureWithoutLeakingItsDetail(t *testing.T) {
	upstream := errors.New("no route found near 8.123456,49.654321")
	handler := plansHandler(t, newFakeSessions(), &fakePlans{
		createErr: fmt.Errorf("plan: routing waypoints: %w: %w", plan.ErrRouting, upstream),
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, plansPath, validPlanWriteBody, ""))
	assert.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "routing_failed")
	assert.NotContains(t, response.Body.String(), "8.123456", "a coordinate reached the response")
	assert.NotContains(t, response.Body.String(), "no route found", "the engine's own error text reached the response")
}

func TestReplacePlanEnforcesIfMatch(t *testing.T) {
	fake := &fakePlans{stored: plan.Plan{ID: 7, Version: 3}, hasStored: true}
	handler := plansHandler(t, newFakeSessions(), fake)

	stale := httptest.NewRecorder()
	handler.ServeHTTP(stale, planRequest(http.MethodPut, plansPath+"/7", validPlanWriteBody, "2"))
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code, stale.Body.String())
	assert.Contains(t, stale.Body.String(), "precondition_failed")

	current := httptest.NewRecorder()
	handler.ServeHTTP(current, planRequest(http.MethodPut, plansPath+"/7", validPlanWriteBody, "3"))
	require.Equal(t, http.StatusOK, current.Code, current.Body.String())

	var body openapi.Plan
	require.NoError(t, json.Unmarshal(current.Body.Bytes(), &body))
	assert.Equal(t, int64(4), body.Version, "a successful replace increments the version")
}

func TestDeletePlanEnforcesIfMatch(t *testing.T) {
	fake := &fakePlans{stored: plan.Plan{ID: 7, Version: 3}, hasStored: true}
	handler := plansHandler(t, newFakeSessions(), fake)

	stale := httptest.NewRecorder()
	handler.ServeHTTP(stale, planRequest(http.MethodDelete, plansPath+"/7", "", "2"))
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code, stale.Body.String())

	current := httptest.NewRecorder()
	handler.ServeHTTP(current, planRequest(http.MethodDelete, plansPath+"/7", "", "3"))
	assert.Equal(t, http.StatusNoContent, current.Code, current.Body.String())
}

func TestListPlansCarriesNoGeometry(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{list: []plan.Plan{
		{ID: 7, Name: "Alpine loop", Profile: plan.Trekking, Version: 1, Waypoints: []plan.Waypoint{{}, {}}},
	}})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, plansPath))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "geometry", "the list carries a geometry field")
}

// A plan body carries at most 50 waypoints; one built to overrun the 8 KiB
// bound this surface holds every plan request to is refused rather than read.
func TestCreatePlanRejectsAnOversizedBody(t *testing.T) {
	oversized := `{"name":"` + strings.Repeat("a", int(maximumPlanBytes)) + `","profile":"trekking","waypoints":[]}`
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, plansPath, oversized, ""))
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}

// directPlanRequest builds a request for calling a plan handler method
// directly, bypassing the mux and the contract validator: some of these
// branches (an If-Match the document's own string schema does not refuse, or
// a malformed body the validator would otherwise catch first) are only
// reachable that way.
func directPlanRequest(method, path, body string) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
}

func TestReplacePlanRejectsAnUnparseableOrMissingIfMatch(t *testing.T) {
	for name, ifMatchValue := range map[string]string{"unparseable": "abc", "missing": "", "zero": "0"} {
		t.Run(name, func(t *testing.T) {
			handler := plansHandler(t, newFakeSessions(), &fakePlans{})
			request := directPlanRequest(http.MethodPut, plansPath+"/7", validPlanWriteBody)
			request.SetPathValue("planId", "7")
			if ifMatchValue != "" {
				request.Header.Set("If-Match", ifMatchValue)
			}

			response := httptest.NewRecorder()
			handler.ReplacePlan(response, request)
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

func TestDeletePlanRejectsAnUnparseableOrMissingIfMatch(t *testing.T) {
	for name, ifMatchValue := range map[string]string{"unparseable": "abc", "missing": "", "zero": "0"} {
		t.Run(name, func(t *testing.T) {
			handler := plansHandler(t, newFakeSessions(), &fakePlans{})
			request := directPlanRequest(http.MethodDelete, plansPath+"/7", "")
			request.SetPathValue("planId", "7")
			if ifMatchValue != "" {
				request.Header.Set("If-Match", ifMatchValue)
			}

			response := httptest.NewRecorder()
			handler.DeletePlan(response, request)
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

// A body the contract validator would otherwise catch first is exercised by
// calling the handler directly with a valid If-Match, so ReplacePlan's own
// decode failure is what answers.
func TestReplacePlanRejectsAMalformedBody(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{stored: plan.Plan{ID: 7, Version: 3}, hasStored: true})
	request := directPlanRequest(http.MethodPut, plansPath+"/7", "{not json")
	request.SetPathValue("planId", "7")
	request.Header.Set("If-Match", "3")

	response := httptest.NewRecorder()
	handler.ReplacePlan(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}

// DeletePlan answers not found for a planId this handler cannot parse, the
// same as an address naming a plan that was never stored.
func TestDeletePlanRejectsAnUnparseablePlanId(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})
	request := directPlanRequest(http.MethodDelete, plansPath+"/x", "")
	request.SetPathValue("planId", "x")
	request.Header.Set("If-Match", "1")

	response := httptest.NewRecorder()
	handler.DeletePlan(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

// planFailed maps plan.ErrNotFound to 404 wherever a Plans method returns it,
// not only for a planId this handler could not itself parse.
func TestReplaceAndDeletePlanReportNotFoundForAnUnknownID(t *testing.T) {
	t.Run("replace", func(t *testing.T) {
		handler := plansHandler(t, newFakeSessions(), &fakePlans{})

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, planRequest(http.MethodPut, plansPath+"/7", validPlanWriteBody, "1"))
		assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	})

	t.Run("delete", func(t *testing.T) {
		handler := plansHandler(t, newFakeSessions(), &fakePlans{})

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, planRequest(http.MethodDelete, plansPath+"/7", "", "1"))
		assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	})
}

// An error a Plans method returns that is neither a known sentinel nor a
// validation leaf — a store failure, say — is answered as unavailable rather
// than guessed at.
func TestCreatePlanReportsAnUnclassifiedErrorAsUnavailable(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{
		createErr: fmt.Errorf("plan: storing plan: %w", errors.New("disk full")),
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, plansPath, validPlanWriteBody, ""))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

// A body the contract validator would otherwise catch first is exercised by
// calling the handler directly, so PreviewPlanRoute's own decode failure is
// what answers.
func TestPreviewPlanRouteRejectsAMalformedBody(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})
	request := directPlanRequest(http.MethodPost, planRoutePath, "{not json")

	response := httptest.NewRecorder()
	handler.PreviewPlanRoute(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}

func TestPreviewPlanRouteReportsARoutingFailure(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{
		routeErr: fmt.Errorf("plan: routing waypoints: %w: %w", plan.ErrRouting, errors.New("upstream")),
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPost, planRoutePath, validPlanRouteBody, ""))
	assert.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "routing_failed")
}

func TestCreatePlanRejectsAMalformedBody(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})
	request := directPlanRequest(http.MethodPost, plansPath, "{not json")

	response := httptest.NewRecorder()
	handler.CreatePlan(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}

func TestListPlansReportsAnUnavailableStore(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{listErr: errors.New("boom")})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, plansPath))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

func TestGetPlanReturnsAPlanWhole(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{
		stored: plan.Plan{ID: 7, Name: "Alpine loop", Profile: plan.Trekking, Version: 1}, hasStored: true,
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, plansPath+"/7"))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body openapi.Plan
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "Alpine loop", body.Name)
}

func TestGetPlanReportsNotFoundForAnUnknownID(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, plansPath+"/7"))
	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

func TestGetPlanReportsAnUnavailableStore(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{getErr: errors.New("boom")})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, plansPath+"/7"))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

func TestGetPlanRejectsAnUnparseablePlanId(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})
	request := directPlanRequest(http.MethodGet, plansPath+"/x", "")
	request.SetPathValue("planId", "x")

	response := httptest.NewRecorder()
	handler.GetPlan(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

// ReplacePlan answers not found for a planId this handler cannot parse, on
// the same terms as DeletePlan and GetPlan.
func TestReplacePlanRejectsAnUnparseablePlanId(t *testing.T) {
	handler := plansHandler(t, newFakeSessions(), &fakePlans{})
	request := directPlanRequest(http.MethodPut, plansPath+"/x", validPlanWriteBody)
	request.SetPathValue("planId", "x")
	request.Header.Set("If-Match", "1")

	response := httptest.NewRecorder()
	handler.ReplacePlan(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

type fakePlanDeliveries struct {
	err        error
	revision   string
	deliveries []PlanDelivery
	asked      bool
}

func (d *fakePlanDeliveries) PlanDelivery(_ context.Context, _ int64, revision string) ([]PlanDelivery, error) {
	d.asked, d.revision = true, revision

	return d.deliveries, d.err
}

func deliveryHandler(
	t *testing.T, sessions Sessions, plans Plans, deliveries PlanDeliveries, state *fakeState, tasks *fakeTasks,
) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			schemaCache:      testSchemaCache,
			Alerts:           &fakeAlerts{},
			Tasks:            tasks,
			Settings:         settingsWith(testBasemaps()),
			Sessions:         sessions,
			BrowserOriginURL: testBrowserOriginURL,
			Plans:            plans,
			PlanDeliveries:   deliveries,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// A stored replace or delete asks for that one plan to be pushed; a refused
// one asks for nothing.
func TestPlanWritesAskForTheirPlansPush(t *testing.T) {
	tests := map[string]struct{ method, body string }{
		"replace": {http.MethodPut, validPlanWriteBody},
		"delete":  {http.MethodDelete, ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tasks := &fakeTasks{}
			fake := &fakePlans{stored: plan.Plan{ID: 7, Version: 3}, hasStored: true}
			handler := deliveryHandler(t, newFakeSessions(), fake, nil, &fakeState{}, tasks)

			stale := httptest.NewRecorder()
			handler.ServeHTTP(stale, planRequest(test.method, plansPath+"/7", test.body, "2"))
			require.Equal(t, http.StatusPreconditionFailed, stale.Code, stale.Body.String())
			assert.Empty(t, tasks.asked, "tasks asked for after a refused write")

			stored := httptest.NewRecorder()
			handler.ServeHTTP(stored, planRequest(test.method, plansPath+"/7", test.body, "3"))
			require.Less(t, stored.Code, http.StatusMultipleChoices, stored.Body.String())
			assert.Equal(t, []startedTask{{name: TaskSyncPlan, argument: "7"}}, tasks.asked, "tasks asked for")
		})
	}
}

// A push the task layer refuses still leaves the write answered: the sweep
// picks the plan up.
func TestAPlanWriteSucceedsWhenItsPushIsRefused(t *testing.T) {
	tasks := &fakeTasks{refuse: true}
	fake := &fakePlans{stored: plan.Plan{ID: 7, Version: 3}, hasStored: true}
	handler := deliveryHandler(t, newFakeSessions(), fake, nil, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodPut, plansPath+"/7", validPlanWriteBody, "3"))

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
}

func TestGetPlanDeliveryNamesEachRiderAndTheirState(t *testing.T) {
	updatedAt := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	deliveredAt := time.Date(2026, 9, 18, 10, 1, 0, 0, time.FixedZone("CEST", 2*60*60))
	fake := &fakePlans{stored: plan.Plan{ID: 7, Version: 3, Published: true, UpdatedAt: updatedAt}, hasStored: true}
	deliveries := &fakePlanDeliveries{deliveries: []PlanDelivery{
		{TargetID: "rider-a", State: "current", DeliveredAt: deliveredAt},
		{TargetID: "rider-b", State: "failed", Failure: "authorization"},
		{TargetID: "rider-c", State: "pending"},
	}}
	state := &fakeState{
		targets: []fakeTarget{
			{id: "rider-a", authorization: "authorized", owner: testSubject},
			{id: "rider-b", authorization: "needs_reauthorization", owner: "github|2"},
			{id: "rider-c", authorization: "authorized", owner: "github|3"},
		},
		nicknames: map[string]string{testSubject: "Alex", "github|2": "Marta"},
	}
	handler := deliveryHandler(t, newFakeSessions(), fake, deliveries, state, &fakeTasks{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodGet, plansPath+"/7/delivery", "", ""))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body openapi.PlanDelivery
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, fake.stored.Revision(), deliveries.revision, "revision asked about")
	delivered := deliveredAt.UTC()
	assert.Equal(t, []openapi.PlanTargetDelivery{
		{ID: "rider-a", Own: true, OwnerNickname: new("Alex"), State: "current", DeliveredAt: &delivered},
		{ID: "rider-b", OwnerNickname: new("Marta"), State: "failed", Failure: new("authorization")},
		{ID: "rider-c", State: "pending"},
	}, body.Targets, "targets")
}

// A draft is owed nowhere, so every rider is asked about against no revision.
func TestGetPlanDeliveryAsksAboutADraftAgainstNoRevision(t *testing.T) {
	fake := &fakePlans{stored: plan.Plan{ID: 7, Version: 1, UpdatedAt: time.Now()}, hasStored: true}
	deliveries := &fakePlanDeliveries{}
	handler := deliveryHandler(t, newFakeSessions(), fake, deliveries, &fakeState{}, &fakeTasks{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, planRequest(http.MethodGet, plansPath+"/7/delivery", "", ""))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.True(t, deliveries.asked, "deliveries asked")
	assert.Empty(t, deliveries.revision, "revision asked about")
}

func TestGetPlanDeliveryRefusals(t *testing.T) {
	stored := plan.Plan{ID: 7, Version: 1}
	tests := map[string]struct {
		sessions   Sessions
		plans      *fakePlans
		deliveries PlanDeliveries
		state      *fakeState
		want       int
	}{
		"non-admin":          {nonAdminSessions("rider-a"), &fakePlans{stored: stored, hasStored: true}, &fakePlanDeliveries{}, &fakeState{}, http.StatusForbidden},
		"unknown plan":       {newFakeSessions(), &fakePlans{}, &fakePlanDeliveries{}, &fakeState{}, http.StatusNotFound},
		"no delivery port":   {newFakeSessions(), &fakePlans{stored: stored, hasStored: true}, nil, &fakeState{}, http.StatusNotFound},
		"plan unreadable":    {newFakeSessions(), &fakePlans{getErr: errors.New("down")}, &fakePlanDeliveries{}, &fakeState{}, http.StatusServiceUnavailable},
		"state unreadable":   {newFakeSessions(), &fakePlans{stored: stored, hasStored: true}, &fakePlanDeliveries{err: errors.New("down")}, &fakeState{}, http.StatusServiceUnavailable},
		"targets unreadable": {newFakeSessions(), &fakePlans{stored: stored, hasStored: true}, &fakePlanDeliveries{}, &fakeState{targetErr: errors.New("down")}, http.StatusServiceUnavailable},
		"nicknames unreadable": {
			newFakeSessions(), &fakePlans{stored: stored, hasStored: true}, &fakePlanDeliveries{},
			&fakeState{nicknamesErr: errors.New("down")}, http.StatusServiceUnavailable,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler := deliveryHandler(t, test.sessions, test.plans, test.deliveries, test.state, &fakeTasks{})

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, planRequest(http.MethodGet, plansPath+"/7/delivery", "", ""))

			assert.Equal(t, test.want, response.Code, response.Body.String())
		})
	}
}

func TestGetPlanDeliveryRejectsAnUnparseablePlanId(t *testing.T) {
	handler := deliveryHandler(t, newFakeSessions(), &fakePlans{}, &fakePlanDeliveries{}, &fakeState{}, &fakeTasks{})
	request := directPlanRequest(http.MethodGet, plansPath+"/x/delivery", "")
	request.SetPathValue("planId", "x")

	response := httptest.NewRecorder()
	handler.GetPlanDelivery(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

// The cue switch travels to the service on both writes, absent reads as off,
// and a plan says whether it carries cues and how many turns it has.
func TestPlanWritesCarryTheCueSwitch(t *testing.T) {
	fake := &fakePlans{}
	handler := plansHandler(t, newFakeSessions(), fake)
	cuedBody := strings.Replace(validPlanWriteBody, `"published":false`, `"published":false,"cues":true`, 1)

	created := httptest.NewRecorder()
	handler.ServeHTTP(created, planRequest(http.MethodPost, plansPath, cuedBody, ""))
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var body openapi.Plan
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &body))
	assert.True(t, body.Cues, "cues after create")
	require.NotNil(t, body.TurnCount, "turn count")
	assert.Zero(t, *body.TurnCount, "turn count")

	replaced := httptest.NewRecorder()
	handler.ServeHTTP(replaced, planRequest(http.MethodPut, plansPath+"/7", validPlanWriteBody, "1"))
	require.Equal(t, http.StatusOK, replaced.Code, replaced.Body.String())
	assert.False(t, fake.stored.Cues, "an absent switch is off")
}
