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
