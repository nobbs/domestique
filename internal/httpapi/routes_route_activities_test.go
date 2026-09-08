package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	activities "github.com/nobbs/domestique/internal/activity"
	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/session"
)

func testRouteKey() route.Key {
	return route.NewKey(route.ProviderVeloPlanner, 7, 1)
}

const routeActivitiesPath = "/v1/providers/veloplanner/sourceRoutes/7/routes/1/activities"

// riddenState is one rider's target holding two rides on the route, and another
// rider's target holding one of its own on the same route.
func riddenState(subject string) *fakeState {
	state := activityState(subject, time.Hour, 2*time.Hour)
	state.routeMatches = map[string]map[int64]activities.RouteMatch{
		subject: {
			// Both shares clear the gate the matcher applies, so nothing here is a
			// state the service could not have stored.
			1: {
				Key: testRouteKey(), RouteCoverage: 0.98, RideCoverage: 0.95,
				Direction: activities.DirectionReverse,
			},
			2: {Key: route.NewKey(route.ProviderVeloPlanner, 8, 1), RouteCoverage: 0.94, RideCoverage: 0.93},
		},
		"rider-b": {99: {
			Key: testRouteKey(), RouteCoverage: 1, RideCoverage: 1, Direction: activities.DirectionForward,
		}},
	}

	return state
}

func getRouteActivities(t *testing.T, handler *Handler, path string) (int, openapi.RouteActivityList) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, path))
	var list openapi.RouteActivityList
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &list), "decoding the route activity list")
	}

	return response.Code, list
}

func TestGetRouteActivitiesServesOnlyTheCallersOwnRides(t *testing.T) {
	handler := activityHandler(t, riddenState("rider-a"), nonAdminSessions("rider-a"))

	code, list := getRouteActivities(t, handler, routeActivitiesPath)

	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 1, "another rider's ride on this route is not this rider's history")
	assert.Equal(t, int64(1), list.Activities[0].ID)
	assert.InDelta(t, 0.98, list.Activities[0].RouteCoverage, 1e-9)
	assert.InDelta(t, 0.95, list.Activities[0].RideCoverage, 1e-9)
	assert.Equal(t, openapi.RouteRideDirectionReverse, list.Activities[0].Direction,
		"a route's history says which way round each ride went")
}

// A route nobody rode is an empty history, not a missing page: the route exists
// and the answer about it is "none".
func TestGetRouteActivitiesIsEmptyForARouteNobodyRode(t *testing.T) {
	handler := activityHandler(t, riddenState("rider-a"), nonAdminSessions("rider-a"))

	code, list := getRouteActivities(
		t, handler, "/v1/providers/veloplanner/sourceRoutes/404/routes/1/activities")

	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, list.Activities)
}

// Naming a target the caller may not read is not found rather than forbidden,
// so the surface never confirms which targets exist.
func TestGetRouteActivitiesHidesAnotherRidersTarget(t *testing.T) {
	handler := activityHandler(t, riddenState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getRouteActivities(t, handler, routeActivitiesPath+"?target=rider-b")

	assert.Equal(t, http.StatusNotFound, code)
}

// An admin reads the target they name, which is how one rider's history of a
// route is read at all from outside their own session.
func TestGetRouteActivitiesLetsAnAdminNameATarget(t *testing.T) {
	sessions := newFakeSessions()
	sessions.identity = session.Identity{Subject: "rider-a", Display: "rider-a", Admin: true}
	handler := activityHandler(t, riddenState("rider-a"), sessions)

	code, list := getRouteActivities(t, handler, routeActivitiesPath+"?target=rider-b")

	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 1)
	assert.Equal(t, int64(99), list.Activities[0].ID)
}

func TestGetRouteActivitiesReportsAnUnreadableStore(t *testing.T) {
	state := riddenState("rider-a")
	state.routeRidesErr = errors.New("unavailable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getRouteActivities(t, handler, routeActivitiesPath)

	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// The match travels with the ride, so a ride page says which route it was
// without a second read.
func TestGetActivitiesCarriesTheRouteEachRideWasRiddenOn(t *testing.T) {
	handler := activityHandler(t, riddenState("rider-a"), nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")

	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 2)
	matched := list.Activities[0]
	require.NotNil(t, matched.RouteMatch)
	assert.Equal(t, "veloplanner", matched.RouteMatch.Provider)
	assert.Equal(t, int64(7), matched.RouteMatch.SourceRouteID)
	assert.Equal(t, 1, matched.RouteMatch.StageOrder)
	assert.InDelta(t, 0.98, matched.RouteMatch.RouteCoverage, 1e-9)
	assert.Equal(t, openapi.RouteRideDirectionReverse, matched.RouteMatch.Direction)
}

// A ride on no library route carries no match rather than an empty one.
func TestGetActivitiesOmitsTheMatchForARideOnNoRoute(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")

	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 1)
	assert.Nil(t, list.Activities[0].RouteMatch)
}

func TestGetActivitiesReportsAnUnreadableRouteMatchStore(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.routeMatchErr = errors.New("unavailable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities")

	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// The served contract refuses a route address that is not numeric before the
// handler is reached, so no store is asked about a route that cannot exist.
func TestGetRouteActivitiesRefusesAMalformedRouteAddress(t *testing.T) {
	handler := activityHandler(t, riddenState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getRouteActivities(
		t, handler, "/v1/providers/veloplanner/sourceRoutes/not-a-number/routes/1/activities")

	assert.Equal(t, http.StatusBadRequest, code)
}

// Which targets the caller may read is itself a stored read, and a route's
// history must not be served at all when that read fails.
func TestGetRouteActivitiesReportsAnUnreadableTargetList(t *testing.T) {
	state := riddenState("rider-a")
	state.targetErr = errors.New("unavailable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getRouteActivities(t, handler, routeActivitiesPath)

	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// A caller with no target of their own has ridden nothing, which is an empty
// history rather than a missing route.
func TestGetRouteActivitiesIsEmptyForACallerWithNoTarget(t *testing.T) {
	state := riddenState("rider-a")
	handler := activityHandler(t, state, nonAdminSessions("rider-nobody"))

	code, list := getRouteActivities(t, handler, routeActivitiesPath)

	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, list.Activities)
}
