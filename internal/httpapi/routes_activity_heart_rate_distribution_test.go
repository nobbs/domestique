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
	"github.com/nobbs/domestique/internal/trainingload"
)

// heartRateDistributionState is one rider's own target with a derived ride
// whose zones and heart-rate samples the tests read, plus another rider's
// target under the same activity id.
func heartRateDistributionState(subject string) *fakeState {
	state := activityState(subject, time.Hour)
	hrSample := func(second int, value float64) trainingload.Sample {
		return trainingload.Sample{At: activityClock().Add(time.Duration(second) * time.Second), Value: value}
	}
	metrics := activities.RideMetrics{Load: trainingload.Metrics{
		Inputs: trainingload.Inputs{MaxHeartRateBPM: 180},
		Zones:  trainingload.Zones{10, 20, 30, 40, 50}, HasZones: true,
	}}
	// One plausible reading either side of a spike above the target's stored
	// maximum, so the cap's interpolation is exercised rather than a plain
	// pass-through.
	samples := activities.RideSamples{HeartRate: []trainingload.Sample{
		hrSample(0, 140), hrSample(10, 190), hrSample(20, 160), hrSample(30, 160),
	}}
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		subject: {1: metrics}, "rider-b": {1: metrics},
	}
	state.rideSamples = map[string]activities.RideSamples{
		subject + "/1": samples, "rider-b/1": samples,
	}

	return state
}

func getHeartRateDistribution(
	t *testing.T, handler *Handler, target string,
) (int, activityHeartRateDistributionView) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
	var view activityHeartRateDistributionView
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the distribution")
	}

	return response.Code, view
}

// The cap replaces the spike with a line interpolated between its plausible
// neighbours before the fold ever floors a value, so no bin above the
// target's stored maximum appears.
func TestGetActivityHeartRateDistributionAppliesTheCapBeforeFolding(t *testing.T) {
	handler := activityHandler(t, heartRateDistributionState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 140, view.FromBpm)
	require.Len(t, view.Seconds, 21, "140 through 160, the interpolated line's own range")
	assert.InDelta(t, 10.0, view.Seconds[0], 1e-9, "held at 140")
	assert.InDelta(t, 10.0, view.Seconds[10], 1e-9, "held at the interpolated 150")
	assert.InDelta(t, 10.0, view.Seconds[20], 1e-9, "held at 160")
	assert.Less(t, view.FromBpm+len(view.Seconds)-1, 180, "no bin reaches the stored maximum")
}

func TestGetActivityHeartRateDistributionIsNotFoundWithNoMetricsRow(t *testing.T) {
	state := heartRateDistributionState("rider-a")
	state.activityMetrics = nil
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivityHeartRateDistributionIsNotFoundWhenZonesAreWithheld(t *testing.T) {
	state := heartRateDistributionState("rider-a")
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {Load: trainingload.Metrics{HasZones: false}}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivityHeartRateDistributionIsNotFoundWithNoSamples(t *testing.T) {
	state := heartRateDistributionState("rider-a")
	state.rideSamples = nil
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivityHeartRateDistributionReportsAnUnreadableMetricsStore(t *testing.T) {
	state := heartRateDistributionState("rider-a")
	state.activityMetricsErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGetActivityHeartRateDistributionReportsAnUnreadableSampleStore(t *testing.T) {
	state := heartRateDistributionState("rider-a")
	state.rideSamplesErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// Scoped exactly as the track is: a target this caller cannot read is a ride
// that is not there, never their distribution and never a hint that it exists.
func TestGetActivityHeartRateDistributionRefusesAnotherRidersActivity(t *testing.T) {
	handler := activityHandler(t, heartRateDistributionState("rider-a"), nonAdminSessions("rider-a"))

	code, _ := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetActivityHeartRateDistributionServesAnyTargetToAnAdmin(t *testing.T) {
	handler := activityHandler(t, heartRateDistributionState(testSubject), newFakeSessions())

	code, view := getHeartRateDistribution(t, handler, "/v1/activities/1/heartRateDistribution?target=rider-b")
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, view.Seconds)
}
