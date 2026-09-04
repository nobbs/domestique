package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	activities "github.com/nobbs/domestique/internal/activity"
	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// activityClock is the moment every window in this file is measured from.
func activityClock() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) }

// activityState is one rider's own target carrying activities at the given
// ages, plus another rider's target with one of its own.
func activityState(subject string, ages ...time.Duration) *fakeState {
	state := &fakeState{
		targets: []fakeTarget{
			{id: subject, authorization: "authorized", owner: subject},
			{id: "rider-b", authorization: "authorized", owner: "rider-b"},
		},
		activities: map[string][]activities.Stored{
			"rider-b": {{ID: 99, StartedAt: activityClock().Add(-time.Hour), DistanceMetres: 1}},
		},
	}
	for index, age := range ages {
		state.activities[subject] = append(state.activities[subject], activities.Stored{
			ID: int64(index + 1), StartedAt: activityClock().Add(-age),
			DistanceMetres: 1000, MovingSeconds: 60, ElapsedSeconds: 90, AscentMetres: 10,
			TypeID: 15, LocationID: 1,
		})
	}

	return state
}

// activityHandler builds a handler over state, as the given identity, with the
// clock fixed so a default window is deterministic.
func activityHandler(t *testing.T, state State, sessions Sessions) *Handler {
	t.Helper()
	handler := handlerFor(t, sessions, &fakeOAuth{}, state, nil)
	handler.now = activityClock

	return handler
}

func getActivities(t *testing.T, handler *Handler, target string) (int, openapi.ActivityList) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
	var list openapi.ActivityList
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &list), "decoding the activity list")
	}

	return response.Code, list
}

// The default window is the last year of the caller's own target, newest
// first, and never another rider's.
func TestGetActivitiesServesTheCallersOwnTargetOverTheDefaultWindow(t *testing.T) {
	state := activityState("rider-a", time.Hour, 48*time.Hour, 400*24*time.Hour)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 2, "the activity older than a year is outside the default window")
	assert.Equal(t, []int64{1, 2}, []int64{list.Activities[0].ID, list.Activities[1].ID}, "newest first")
	assert.InDelta(t, 1000.0, list.Activities[0].DistanceMetres, 1e-9)
	assert.Equal(t, 15, list.Activities[0].TypeID)
	assert.Equal(t, 1, list.Activities[0].LocationID)
}

// An explicit window is half-open on the start time.
func TestGetActivitiesHonoursAnExplicitWindow(t *testing.T) {
	state := activityState("rider-a", time.Hour, 48*time.Hour)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	from := activityClock().Add(-24 * time.Hour).Format(time.RFC3339)
	to := activityClock().Add(-time.Hour).Format(time.RFC3339)
	code, list := getActivities(t, handler, "/v1/activities?from="+from+"&to="+to)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, list.Activities, "an activity starting exactly at to is outside the window")
}

func TestGetActivitiesReadsAWindowToWholeSeconds(t *testing.T) {
	state := activityState("rider-a", time.Hour, 48*time.Hour)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	// Fractional seconds are valid RFC 3339 and are truncated, so an edge a few
	// milliseconds past the ride's whole-second start still includes it.
	from := activityClock().Add(-time.Hour).Add(400 * time.Millisecond).In(time.FixedZone("CEST", 2*3600)).Format(time.RFC3339Nano)
	code, list := getActivities(t, handler, "/v1/activities?from="+url.QueryEscape(from))
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, list.Activities, 1)
}

// Naming another rider's target is not found rather than forbidden, so the
// surface never confirms which targets exist.
func TestGetActivitiesRefusesATargetTheCallerDoesNotOwn(t *testing.T) {
	handler := activityHandler(t, activityState("rider-a", time.Hour), nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivitiesServesAnyTargetToAnAdmin(t *testing.T) {
	state := activityState(testSubject, time.Hour)
	handler := activityHandler(t, state, newFakeSessions())

	code, list := getActivities(t, handler, "/v1/activities?target=rider-b")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 1)
	assert.Equal(t, int64(99), list.Activities[0].ID)
}

// A rider who has not connected Wahoo yet has no target at all, and reads an
// empty history rather than a missing page.
func TestGetActivitiesIsEmptyForARiderWithNoTarget(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{{id: "rider-b", authorization: "authorized", owner: "rider-b"}}}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, list.Activities)
}

func TestGetActivitiesRefusesAWindowItWillNotRead(t *testing.T) {
	handler := activityHandler(t, activityState("rider-a"), nonAdminSessions("rider-a"))

	for name, query := range map[string]string{
		"inverted": "?from=2026-08-24T00:00:00Z&to=2026-08-23T00:00:00Z",
		"too long": "?from=2020-01-01T00:00:00Z&to=2026-08-24T00:00:00Z",
		"bad from": "?from=yesterday",
		"bad to":   "?to=tomorrow",
	} {
		t.Run(name, func(t *testing.T) {
			code, _ := getActivities(t, handler, "/v1/activities"+query)
			assert.Equal(t, http.StatusBadRequest, code)
		})
	}
}

func TestGetActivitiesReportsAnUnreadableTargetList(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.targetErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGetActivitiesReportsAnUnreadableStore(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activitiesErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}
