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
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

const routeClimbsPath = "/v1/providers/veloplanner/sourceRoutes/7/routes/1/climbs"

// climbingLine is a route running due east, flat, then climbing at six percent
// for six hundred metres, then flat: one sustained climb in the middle.
func climbingLine() (line []measure.Coordinate, elevations []float64) {
	const step, metresPerDegree = 20.0, 111_320.0
	line = []measure.Coordinate{}
	elevations = []float64{}
	altitude := 100.0
	for metres := 0.0; metres <= 2600; metres += step {
		line = append(line, measure.Coordinate{Latitude: 0, Longitude: metres / metresPerDegree})
		if metres > 1000 && metres <= 1600 {
			altitude += step * 6 / 100
		}
		elevations = append(elevations, altitude)
	}

	return line, elevations
}

// climbState is one rider whose target holds two attempts at the route's climb.
func climbState(subject string) *fakeState {
	state := riddenState(subject)
	line, elevations := climbingLine()
	state.stageProfiles = map[route.Key]fakeStageProfile{
		testRouteKey(): {line: line, elevations: elevations},
	}
	slower := activities.ClimbAttempt{
		ClimbIndex: 0, Seconds: 780, HeartRateBPM: 162, HasHeartRate: true,
		PowerWatts: 268, HasPower: true,
	}
	quicker := activities.ClimbAttempt{ClimbIndex: 0, Seconds: 720}
	state.climbAttempts = map[route.Key][]activities.StoredClimbAttempt{
		testRouteKey(): {
			{
				RiddenAt:     time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC),
				ClimbAttempt: slower,
				WorkoutID:    1,
			},
			{
				RiddenAt:     time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC),
				ClimbAttempt: quicker,
				WorkoutID:    2,
			},
		},
	}

	return state
}

func getRouteClimbs(t *testing.T, handler *Handler, path string) (int, openapi.RouteClimbList) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, path))
	var list openapi.RouteClimbList
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &list), "decoding the route climb list")
	}

	return response.Code, list
}

func TestGetRouteClimbsServesTheClimbsAndTheCallersAttemptsQuickestFirst(t *testing.T) {
	state := climbState("rider-a")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getRouteClimbs(t, handler, routeClimbsPath)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Climbs, 1, "the route holds one sustained climb")
	climb := list.Climbs[0]
	assert.InDelta(t, 600.0, climb.DistanceMetres, 60)
	assert.InDelta(t, 36.0, climb.AscentMetres, 4)
	require.Len(t, climb.Attempts, 2)
	assert.InDelta(t, 720.0, climb.Attempts[0].Seconds, 0.001, "the quicker attempt leads")
	assert.InDelta(t, 780.0, climb.Attempts[1].Seconds, 0.001)
	// The climb's own ascent over the attempt's time, so two attempts at one
	// climb are comparable.
	assert.InDelta(t, climb.AscentMetres/720*3600, climb.Attempts[0].VamMetresPerHour, 0.001)
	require.NotNil(t, climb.Attempts[1].HeartRateBpm)
	assert.InDelta(t, 162.0, *climb.Attempts[1].HeartRateBpm, 0.001)
	require.NotNil(t, climb.Attempts[1].PowerWatts)
	assert.Nil(t, climb.Attempts[1].EstimatedPowerWatts, "a ride with a meter carries no estimate")
	assert.Equal(t, "rider-a", state.climbAttemptsFor, "the caller's own target alone")
}

// A route the library does not hold is not found, as it is on every other
// address under a route.
func TestGetRouteClimbsIsNotFoundForARouteTheLibraryLacks(t *testing.T) {
	handler := activityHandler(t, climbState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getRouteClimbs(t, handler,
		"/v1/providers/veloplanner/sourceRoutes/404/routes/1/climbs")
	assert.Equal(t, http.StatusNotFound, code)
}

// A route whose stored geometry carries no height has no climbs to serve, which
// is an empty list rather than a missing page.
func TestGetRouteClimbsIsEmptyForARouteWithNoHeight(t *testing.T) {
	state := climbState("rider-a")
	line, _ := climbingLine()
	state.stageProfiles[testRouteKey()] = fakeStageProfile{line: line}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getRouteClimbs(t, handler, routeClimbsPath)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, list.Climbs)
}

// A climb the caller has never ridden is served with no attempts rather than
// left out: the climb is the route's whoever has ridden it.
func TestGetRouteClimbsServeAClimbNobodyRode(t *testing.T) {
	state := climbState("rider-a")
	state.climbAttempts = nil
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getRouteClimbs(t, handler, routeClimbsPath)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Climbs, 1)
	assert.Empty(t, list.Climbs[0].Attempts)
}

func TestGetRouteClimbsReportsAnUnreadableStore(t *testing.T) {
	state := climbState("rider-a")
	state.climbAttemptErr = errors.New("the state is unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getRouteClimbs(t, handler, routeClimbsPath)
	assert.Equal(t, http.StatusServiceUnavailable, code)
}
