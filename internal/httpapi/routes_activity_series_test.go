package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	activities "github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

// seriesState is trackState with samples beside the positions: a ride carrying
// a heart rate throughout, a cadence that dropped out on the first sample, and
// nothing else.
func seriesState(subject string) *fakeState {
	state := trackState(subject)
	state.sampleRows = map[string][]activities.SampleRow{
		subject + "/1": {
			{
				Time:           activityClock(),
				DistanceMetres: activities.Reading{Value: 0, Known: true},
				HeartRateBPM:   activities.Reading{Value: 128, Known: true},
			},
			{
				Time:           activityClock().Add(time.Minute),
				DistanceMetres: activities.Reading{Value: 600, Known: true},
				HeartRateBPM:   activities.Reading{Value: 141, Known: true},
				CadenceRPM:     activities.Reading{Value: 84, Known: true},
			},
		},
		"rider-b/1": {{Time: activityClock(), HeartRateBPM: activities.Reading{Value: 90, Known: true}}},
	}

	return state
}

func getSeries(t *testing.T, handler *Handler, target string) (int, activitySeriesView) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
	var view activitySeriesView
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the series")
	}

	return response.Code, view
}

// The series is indexed with the track's coordinates, so a chart can lay it
// over the profile without knowing anything about how it was recorded.
func TestGetActivitySeriesIsIndexedWithTheTrack(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getSeries(t, handler, "/v1/activities/1/series/heartRate")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "heartRate", view.Series)
	require.Len(t, view.Values, 2, "one value per coordinate")
	require.NotNil(t, view.Values[0])
	assert.InDelta(t, 128.0, *view.Values[0], 1e-9)
}

// A sample the sensor missed is a null in place, not a dropped entry: dropping
// it would slide every later value onto the wrong coordinate.
func TestGetActivitySeriesKeepsAGapAsANull(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getSeries(t, handler, "/v1/activities/1/series/cadence")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Values, 2)
	assert.Nil(t, view.Values[0], "the sample before the sensor read")
	require.NotNil(t, view.Values[1])
	assert.InDelta(t, 84.0, *view.Values[1], 1e-9)
}

// A bicycle with no meter is not found, rather than a column of nulls the page
// would have to look through to learn there was nothing.
func TestGetActivitySeriesIsNotFoundWhenTheRideRecordedNone(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/power")
	assert.Equal(t, http.StatusNotFound, code)
}

// Speed is derived rather than recorded: no sample carries it, and the ride
// still answers with one.
func TestGetActivitySeriesDerivesSpeedFromTheDistanceCovered(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getSeries(t, handler, "/v1/activities/1/series/speed")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Values, 2)
	assert.Nil(t, view.Values[0], "nothing behind the first sample to measure")
	require.NotNil(t, view.Values[1])
	assert.InDelta(t, 36.0, *view.Values[1], 1e-9, "600 m in a minute")
}

// Scoped exactly as the track is: another rider's activity id reads as a ride
// that is not there, and an admin may name any target.
func TestGetActivitySeriesRefusesAnotherRidersActivity(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/heartRate?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivitySeriesServesAnyTargetToAnAdmin(t *testing.T) {
	handler := activityHandler(t, seriesState(testSubject), newFakeSessions())

	code, view := getSeries(t, handler, "/v1/activities/1/series/heartRate?target=rider-b")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, view.Values, 1)
}

// predictionState is seriesState with ride 1 matched forward to a route drawn
// along its own track, predicted at fifty minutes end to end.
func predictionState(subject string) *fakeState {
	state := seriesState(subject)
	track := state.tracks[subject+"/1"]
	// Eight seconds apart, inside a recording gap, so the step counts as moving.
	track[1].Time = activityClock().Add(8 * time.Second)
	state.sampleRows[subject+"/1"][1].Time = track[1].Time
	line := []measure.Coordinate{
		{Latitude: track[0].Latitude, Longitude: track[0].Longitude},
		{Latitude: track[1].Latitude, Longitude: track[1].Longitude},
	}
	state.stageProfiles = map[route.Key]fakeStageProfile{testRouteKey(): {line: line}}
	state.summaries = []route.Summary{{Provider: route.ProviderVeloPlanner, SourceRouteID: 7, StageOrder: 1}}
	state.cumulativeSeconds = json.RawMessage(`[0, 3000]`)
	state.routeClocks = map[string]fakeRouteClock{subject + "/1": {
		key:   testRouteKey(),
		clock: activities.ReadRouteClock(line, track, state.sampleRows[subject+"/1"], activities.DirectionForward),
	}}

	return state
}

// The ride took eight moving seconds over what was predicted at fifty minutes.
func TestGetActivitySeriesServesTheRideAheadOfItsPrediction(t *testing.T) {
	handler := activityHandler(t, predictionState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Values, 2, "one value per coordinate")
	require.NotNil(t, view.Values[0])
	assert.InDelta(t, 0.0, *view.Values[0], 1e-9, "both clocks start where the ride joined the route")
	require.NotNil(t, view.Values[1])
	assert.InDelta(t, 2992.0, *view.Values[1], 1e-6)
}

func TestGetActivitySeriesHasNoPredictionComparisonWithoutOne(t *testing.T) {
	t.Run("no clock", func(t *testing.T) {
		state := predictionState("rider-a")
		state.routeClocks = nil
		handler := activityHandler(t, state, nonAdminSessions("rider-a"))

		code, _ := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction")
		assert.Equal(t, http.StatusNotFound, code)
	})
	t.Run("no prediction", func(t *testing.T) {
		state := predictionState("rider-a")
		state.cumulativeSeconds = nil
		handler := activityHandler(t, state, nonAdminSessions("rider-a"))

		code, _ := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction")
		assert.Equal(t, http.StatusNotFound, code)
	})
	t.Run("route gone from the library", func(t *testing.T) {
		state := predictionState("rider-a")
		state.stageProfiles = nil
		handler := activityHandler(t, state, nonAdminSessions("rider-a"))

		code, _ := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction")
		assert.Equal(t, http.StatusNotFound, code)
	})
}

func TestGetActivitySeriesReportsAnUnreadableRouteClockOrStage(t *testing.T) {
	for name, spoil := range map[string]func(*fakeState){
		"clock":    func(state *fakeState) { state.routeClockErr = assert.AnError },
		"line":     func(state *fakeState) { state.stageProfileErr = assert.AnError },
		"geometry": func(state *fakeState) { state.stageGeometryErr = assert.AnError },
	} {
		t.Run(name, func(t *testing.T) {
			state := predictionState("rider-a")
			spoil(state)
			handler := activityHandler(t, state, nonAdminSessions("rider-a"))

			code, _ := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction")
			assert.Equal(t, http.StatusServiceUnavailable, code)
		})
	}
}

func TestGetActivitySeriesReportsAnUnreadablePrediction(t *testing.T) {
	state := predictionState("rider-a")
	state.cumulativeSeconds = json.RawMessage(`{"not":"an array"}`)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// Another rider's ride is not found here either, whatever clock it carries.
func TestGetActivitySeriesRefusesAnotherRidersPredictionComparison(t *testing.T) {
	state := predictionState("rider-a")
	state.routeClocks["rider-b/1"] = state.routeClocks["rider-a/1"]
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/aheadOfPrediction?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

// A ride this target has no summary for is not found, on the same terms the
// track endpoint uses.
func TestGetActivitySeriesIsNotFoundForARideThisTargetDoesNotHave(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/99/series/heartRate")
	assert.Equal(t, http.StatusNotFound, code)
}

// A series name the contract does not carry never reaches the handler.
func TestGetActivitySeriesRefusesAnUnknownSeries(t *testing.T) {
	handler := activityHandler(t, seriesState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/latitude")
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestGetActivitySeriesReportsAnUnreadableTargetList(t *testing.T) {
	state := seriesState("rider-a")
	state.targetErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/heartRate")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGetActivitySeriesReportsAnUnreadableStore(t *testing.T) {
	state := seriesState("rider-a")
	state.sampleRowsErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/heartRate")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGetActivitySeriesReportsAnUnreadableRecordsState(t *testing.T) {
	state := seriesState("rider-a")
	state.recordsStateErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getSeries(t, handler, "/v1/activities/1/series/heartRate")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}
