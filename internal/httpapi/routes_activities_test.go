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

// With no from, the whole history is served: the account's first recorded
// activity onward, newest first, and never another rider's.
func TestGetActivitiesServesTheCallersWholeHistoryByDefault(t *testing.T) {
	state := activityState("rider-a", time.Hour, 48*time.Hour, 3*365*24*time.Hour)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 3, "no from means no lower bound")
	assert.Equal(t, []int64{1, 2, 3}, []int64{list.Activities[0].ID, list.Activities[1].ID, list.Activities[2].ID}, "newest first")
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

func TestGetActivitiesRefusesAnInvertedOrMalformedWindow(t *testing.T) {
	handler := activityHandler(t, activityState("rider-a"), nonAdminSessions("rider-a"))

	for name, query := range map[string]string{
		"inverted": "?from=2026-08-24T00:00:00Z&to=2026-08-23T00:00:00Z",
		"bad from": "?from=yesterday",
		"bad to":   "?to=tomorrow",
	} {
		t.Run(name, func(t *testing.T) {
			code, _ := getActivities(t, handler, "/v1/activities"+query)
			assert.Equal(t, http.StatusBadRequest, code)
		})
	}
}

// A from more than two years back is no longer refused: there is no maximum
// span, only from <= to.
func TestGetActivitiesAcceptsAFromMoreThanTwoYearsBack(t *testing.T) {
	handler := activityHandler(t, activityState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities?from=2020-01-01T00:00:00Z&to=2026-08-24T00:00:00Z")
	assert.Equal(t, http.StatusOK, code)
}

// The 5000-activity cap is unaffected by the unbounded window.
func TestGetActivitiesCapsAtFiveThousand(t *testing.T) {
	ages := make([]time.Duration, 5001)
	for i := range ages {
		ages[i] = time.Duration(i+1) * time.Hour
	}
	handler := activityHandler(t, activityState("rider-a", ages...), nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, list.Activities, maximumActivities)
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

// trackState is one rider's own target carrying a recorded track, plus another
// rider's target with a track of its own under the same activity id.
func trackState(subject string) *fakeState {
	state := activityState(subject, time.Hour)
	state.tracks = map[string][]activities.TrackPoint{
		subject + "/1": {
			{Time: activityClock(), Latitude: 49.0, Longitude: 8.4, AltitudeMetres: 110, HasAltitude: true},
			{Time: activityClock().Add(time.Minute), Latitude: 49.2, Longitude: 8.5, AltitudeMetres: 180, HasAltitude: true},
		},
		"rider-b/1": {
			{Time: activityClock(), Latitude: 1, Longitude: 1, HasAltitude: true},
			{Time: activityClock(), Latitude: 2, Longitude: 2, HasAltitude: true},
		},
	}

	return state
}

func getTrack(t *testing.T, handler *Handler, target string) (int, activityTrackView) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
	var view activityTrackView
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the track")
		assert.Contains(t, response.Header().Get("Content-Type"), "application/geo+json")
	}

	return response.Code, view
}

func TestGetActivityTrackServesTheCallersOwnRide(t *testing.T) {
	handler := activityHandler(t, trackState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Feature", view.Type)
	require.NotNil(t, view.Geometry, "the line")
	assert.Equal(t, "LineString", view.Geometry.Type)
	assert.Equal(t, trackStateStored, view.Properties.State)
	assert.Equal(t, [][2]float64{{8.4, 49.0}, {8.5, 49.2}}, view.Geometry.Coordinates, "longitude first")
	assert.InDeltaSlice(t, []float64{8.4, 49.0, 8.5, 49.2}, view.BBox, 1e-9, "bbox")
	assert.Equal(t, []*float64{new(110.0), new(180.0)}, view.Properties.AltitudeMetres, "altitudes")
}

// The track is scoped to the caller's own target exactly as the list is, so
// another rider's activity id reads as a ride that is not there.
func TestGetActivityTrackRefusesAnotherRidersActivity(t *testing.T) {
	handler := activityHandler(t, trackState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getTrack(t, handler, "/v1/activities/1/track?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivityTrackServesAnyTargetToAnAdmin(t *testing.T) {
	handler := activityHandler(t, trackState(testSubject), newFakeSessions())

	code, view := getTrack(t, handler, "/v1/activities/1/track?target=rider-b")
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, view.Geometry, "the line")
	assert.Len(t, view.Geometry.Coordinates, 2)
}

// A ride with no line to draw is served, not hidden: the state is the whole
// point, since "not downloaded yet" and "too few positions" are answers a
// rider acts on differently.
func TestGetActivityTrackNamesWhyItHasNoLine(t *testing.T) {
	for name, expected := range map[string]struct {
		state activities.RecordsState
		want  string
	}{
		"samples not stored yet": {state: activities.RecordsPending, want: trackStatePending},
		"file did not decode":    {state: activities.RecordsUnreadable, want: trackStateUnreadable},
		"too few positions":      {state: activities.RecordsStored, want: trackStateEmpty},
	} {
		t.Run(name, func(t *testing.T) {
			state := trackState("rider-a")
			delete(state.tracks, "rider-a/1")
			state.recordsStates = map[string]activities.RecordsState{"rider-a/1": expected.state}
			handler := activityHandler(t, state, nonAdminSessions("rider-a"))

			code, view := getTrack(t, handler, "/v1/activities/1/track")
			require.Equal(t, http.StatusOK, code)
			assert.Equal(t, "Feature", view.Type)
			assert.Nil(t, view.Geometry, "an unlocated Feature carries no line")
			assert.Empty(t, view.BBox, "there is no box around nothing")
			assert.Equal(t, expected.want, view.Properties.State)
		})
	}
}

// A single positioned sample draws no line either, and the ride that recorded
// it is stored, not pending.
func TestGetActivityTrackReportsASingleSampleAsEmpty(t *testing.T) {
	state := trackState("rider-a")
	state.tracks["rider-a/1"] = state.tracks["rider-a/1"][:1]
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Nil(t, view.Geometry, "one sample is not a line")
	assert.Equal(t, trackStateEmpty, view.Properties.State)
}

// Only a ride this target has no summary for is missing.
func TestGetActivityTrackIsNotFoundForAnUnknownRide(t *testing.T) {
	handler := activityHandler(t, trackState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getTrack(t, handler, "/v1/activities/7/track")
	assert.Equal(t, http.StatusNotFound, code, "an activity this target does not have")

	code, _ = getTrack(t, handler, "/v1/activities/one/track")
	assert.Equal(t, http.StatusBadRequest, code, "the served surface refuses an id that is not a number")

	// Called directly, past the document validator that refuses it first: the
	// handler must not read an unaddressable id as activity zero.
	request := authenticatedRequest(http.MethodGet, "/v1/activities/one/track")
	request.SetPathValue("activityId", "one")
	response := httptest.NewRecorder()
	handler.GetActivityTrack(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

// The state read is a store read like any other, so a store that cannot answer
// it is unavailable rather than a ride that does not exist.
func TestGetActivityTrackReportsAnUnreadableRecordsState(t *testing.T) {
	state := trackState("rider-a")
	state.recordsStateErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getTrack(t, handler, "/v1/activities/1/track")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// Altitude is served for every positioned sample, null where that one recorded
// none: a barometer commonly needs a leading run of the ride to warm up.
func TestGetActivityTrackServesNullsForALeadingAltitudeGap(t *testing.T) {
	state := trackState("rider-a")
	state.tracks["rider-a/1"] = append([]activities.TrackPoint{
		{Time: activityClock().Add(-time.Minute), Latitude: 48.9, Longitude: 8.3},
	}, state.tracks["rider-a/1"]...)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []*float64{nil, new(110.0), new(180.0)}, view.Properties.AltitudeMetres)
}

// The same holds for a gap in the middle of the ride: a sibling series can
// drop out for a run of samples with a complete series either side of it.
func TestGetActivityTrackServesANullForAnInteriorAltitudeGap(t *testing.T) {
	state := trackState("rider-a")
	state.tracks["rider-a/1"][1].HasAltitude = false
	state.tracks["rider-a/1"][1].AltitudeMetres = 0
	state.tracks["rider-a/1"] = append(state.tracks["rider-a/1"], activities.TrackPoint{
		Time: activityClock().Add(2 * time.Minute), Latitude: 49.4, Longitude: 8.6,
		AltitudeMetres: 220, HasAltitude: true,
	})
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []*float64{new(110.0), nil, new(220.0)}, view.Properties.AltitudeMetres)
}

// Only when nothing in the ride recorded an altitude does the property
// disappear rather than travel as an array of nothing but nulls.
func TestGetActivityTrackOmitsAltitudeWhenNoSampleRecordedOne(t *testing.T) {
	state := trackState("rider-a")
	for i := range state.tracks["rider-a/1"] {
		state.tracks["rider-a/1"][i].HasAltitude = false
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, view.Properties.AltitudeMetres)
}

func TestGetActivityTrackReportsAnUnreadableStore(t *testing.T) {
	state := trackState("rider-a")
	state.trackErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getTrack(t, handler, "/v1/activities/1/track")
	assert.Equal(t, http.StatusServiceUnavailable, code)

	state.trackErr = nil
	state.targetErr = errors.New("unreadable")
	code, _ = getTrack(t, handler, "/v1/activities/1/track")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}
