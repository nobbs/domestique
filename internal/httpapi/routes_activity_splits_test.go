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

// splitsState is trackState with two kilometres of samples beside the
// positions: the first ridden in a minute and climbing, the second in two and
// flat, and half of a third left over.
func splitsState(subject string) *fakeState {
	state := trackState(subject)
	sample := func(second int, metres, altitude, bpm float64) activities.SampleRow {
		return activities.SampleRow{
			Time:           activityClock().Add(time.Duration(second) * time.Second),
			DistanceMetres: activities.Reading{Value: metres, Known: true},
			AltitudeMetres: activities.Reading{Value: altitude, Known: true},
			HeartRateBPM:   activities.Reading{Value: bpm, Known: true},
		}
	}
	state.sampleRows = map[string][]activities.SampleRow{
		subject + "/1": {
			sample(0, 0, 100, 130),
			sample(60, 1000, 150, 150),
			sample(180, 2000, 150, 140),
			sample(240, 2500, 150, 120),
		},
		"rider-b/1": {sample(0, 0, 0, 90), sample(60, 1000, 0, 95)},
	}

	return state
}

func getSplits(t *testing.T, handler *Handler, target string) (int, activitySplitsView) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
	var view activitySplitsView
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the splits")
	}

	return response.Code, view
}

func TestGetActivitySplitsCutsTheRideIntoKilometres(t *testing.T) {
	handler := activityHandler(t, splitsState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getSplits(t, handler, "/v1/activities/1/splits")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Splits, 3)
	assert.InDelta(t, 1000.0, view.Splits[0].DistanceMetres, 1e-9)
	assert.InDelta(t, 60.0, view.Splits[0].MovingSeconds, 1e-9)
	assert.InDelta(t, 50.0, view.Splits[0].AscentMetres, 1e-9, "only the climbing part")
	assert.InDelta(t, 120.0, view.Splits[1].MovingSeconds, 1e-9)
	assert.Zero(t, view.Splits[1].AscentMetres, "a flat kilometre climbs nothing")
	assert.InDelta(t, 500.0, view.Splits[2].DistanceMetres, 1e-9, "what was left over")
}

// A stretch whose samples carried no meter has no average power, which is not
// the same as an average of nought.
func TestGetActivitySplitsLeavesOutASeriesTheStretchDidNotCarry(t *testing.T) {
	handler := activityHandler(t, splitsState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getSplits(t, handler, "/v1/activities/1/splits")
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, view.Splits)
	require.NotNil(t, view.Splits[0].HeartRateBPM)
	assert.InDelta(t, 150.0, *view.Splits[0].HeartRateBPM, 1e-9)
	assert.Nil(t, view.Splits[0].PowerWatts)
}

// Scoped exactly as the track is: a target this caller cannot read is a ride
// that is not there, never their splits and never a hint that it exists.
func TestGetActivitySplitsRefusesAnotherRidersActivity(t *testing.T) {
	handler := activityHandler(t, splitsState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getSplits(t, handler, "/v1/activities/1/splits?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivitySplitsServesAnyTargetToAnAdmin(t *testing.T) {
	handler := activityHandler(t, splitsState(testSubject), newFakeSessions())

	code, view := getSplits(t, handler, "/v1/activities/1/splits?target=rider-b")
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, view.Splits)
}

// A ride whose samples are not stored has nothing to cut, and a table with no
// rows says that on its own.
func TestGetActivitySplitsServesAnEmptyListForARideWithNoSamples(t *testing.T) {
	state := splitsState("rider-a")
	state.sampleRows = nil
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getSplits(t, handler, "/v1/activities/1/splits")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, view.Splits)
}

func TestGetActivitySplitsIsNotFoundForARideThisTargetHasNot(t *testing.T) {
	handler := activityHandler(t, splitsState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getSplits(t, handler, "/v1/activities/99/splits")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivitySplitsReportsAnUnreadableSampleStore(t *testing.T) {
	state := splitsState("rider-a")
	state.sampleRowsErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getSplits(t, handler, "/v1/activities/1/splits")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}
