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
	"github.com/nobbs/domestique/internal/trainingload"
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

// A ride's derived numbers ride along with the ride, so the ride page and the
// volume page both work from the one read. Each part is omitted where the ride
// or the profile did not allow it, rather than sent as a zero.
func TestGetActivitiesCarriesTheDerivedMetricsOfEachRide(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Load: trainingload.Metrics{
				Inputs: trainingload.Inputs{ThresholdHeartRateBPM: 170},
				Zones:  trainingload.Zones{60, 120, 180, 240, 300}, HasZones: true,
				TRIMP: 42.5, HasTRIMP: true,
				EstimatedPowerWatts: 168.5, HasEstimatedPower: true,
			},
			Averages: activities.RideAverages{
				HeartRateBPM: 142.5, MaxHeartRateBPM: 178, HasHeartRate: true,
				CadenceRPM: 81.5, HasCadence: true,
				PowerWatts: 196.25, HasPower: true,
			},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 2)

	// Newest first, and the derived ride is the one an hour old.
	derived, plain := list.Activities[0], list.Activities[1]
	require.NotNil(t, derived.Metrics, "the ride that was derived")
	assert.Equal(t, []float64{60, 120, 180, 240, 300}, derived.Metrics.ZoneSeconds)
	// The rates the zones were cut at, from the threshold this row was derived
	// against rather than whatever the profile holds now.
	assert.Equal(t, []float64{144.5, 153, 161.5, 170}, derived.Metrics.ZoneBoundsBpm)
	require.NotNil(t, derived.Metrics.Trimp)
	assert.InDelta(t, 42.5, *derived.Metrics.Trimp, 1e-9)
	assert.Nil(t, derived.Metrics.PowerTss, "the rider has entered no threshold power")
	require.NotNil(t, derived.Metrics.EstimatedPowerWatts, "which is why it has an estimate at all")
	assert.InDelta(t, 168.5, *derived.Metrics.EstimatedPowerWatts, 1e-9)
	require.NotNil(t, derived.Metrics.AverageHeartRateBpm)
	assert.InDelta(t, 142.5, *derived.Metrics.AverageHeartRateBpm, 1e-9)
	require.NotNil(t, derived.Metrics.MaxHeartRateBpm)
	assert.InDelta(t, 178.0, *derived.Metrics.MaxHeartRateBpm, 1e-9)
	require.NotNil(t, derived.Metrics.AverageCadenceRpm)
	assert.InDelta(t, 81.5, *derived.Metrics.AverageCadenceRpm, 1e-9)
	require.NotNil(t, derived.Metrics.AveragePowerWatts, "which the average power does not need")
	assert.InDelta(t, 196.25, *derived.Metrics.AveragePowerWatts, 1e-9)
	assert.Nil(t, plain.Metrics, "and a ride with no row carries none at all")
}

// The listing card gets one line about the ride: the range the temperature
// moved over, the wind, and the whole of what fell.
func TestGetActivitiesSummarisesTheWeatherEachRideWasRiddenThrough(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activityWeather = map[string]map[int64][]activities.WeatherStep{
		"rider-a": {1: {
			{At: activityClock(), Step: time.Hour, TemperatureCelsius: 12, PrecipitationMillimetres: 0.4,
				WindSpeedKMH: 10, WeatherCode: 61},
			{At: activityClock().Add(time.Hour), Step: time.Hour, TemperatureCelsius: 18,
				PrecipitationMillimetres: 0.2, WindSpeedKMH: 20, WeatherCode: 3},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 2)

	summary := list.Activities[0].Weather
	require.NotNil(t, summary, "the ride that was asked about")
	assert.InDelta(t, 12.0, summary.TemperatureMinCelsius, 1e-9)
	assert.InDelta(t, 18.0, summary.TemperatureMaxCelsius, 1e-9)
	assert.InDelta(t, 15.0, summary.WindSpeedKmh, 1e-9, "the mean over the steps")
	assert.InDelta(t, 0.6, summary.PrecipitationMillimetres, 1e-9, "the whole of what fell")
	assert.Equal(t, 61, summary.WeatherCode, "the worst step, not a mean of codes")
	assert.Nil(t, list.Activities[1].Weather, "and a ride nobody asked about carries none")
}

// The ride page's strip is one row per step, on the endpoint the page already
// fetches for the track.
func TestGetActivityTrackCarriesTheStepsTheRideWasRiddenThrough(t *testing.T) {
	state := trackState("rider-a")
	state.activityWeather = map[string]map[int64][]activities.WeatherStep{
		"rider-a": {1: {{
			At: activityClock(), Step: time.Hour, TemperatureCelsius: 12, ApparentTemperatureCelsius: 10,
			PrecipitationMillimetres: 0.4, WindSpeedKMH: 10, WindDirectionDegrees: 240,
			CloudCoverPercent: 55, WeatherCode: 61,
		}}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Properties.Weather, 1)
	assert.InDelta(t, 12.0, view.Properties.Weather[0].TemperatureCelsius, 1e-9)
	assert.Nil(t, view.Properties.Weather[0].PrecipitationProbabilityPercent,
		"the reanalysis carries none, and none is invented")
}

// A recent ride is answered by the quarter hour rather than the hour, and
// carries a chance of rain the reanalysis does not.
func TestGetActivityTrackCarriesAQuarterHourStepWithItsChanceOfRain(t *testing.T) {
	state := trackState("rider-a")
	state.activityWeather = map[string]map[int64][]activities.WeatherStep{
		"rider-a": {1: {{
			At: activityClock(), Step: 15 * time.Minute, TemperatureCelsius: 12,
			PrecipitationProbabilityPercent: 40, HasPrecipitationProbability: true,
		}}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Properties.Weather, 1)
	assert.Equal(t, 900, view.Properties.Weather[0].StepSeconds)
	require.NotNil(t, view.Properties.Weather[0].PrecipitationProbabilityPercent)
	assert.InDelta(t, 40.0, *view.Properties.Weather[0].PrecipitationProbabilityPercent, 1e-9)
}

func TestGetActivityTrackOmitsTheWeatherOfARideNobodyAskedAbout(t *testing.T) {
	handler := activityHandler(t, trackState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Nil(t, view.Properties.Weather)
}

func TestGetActivitiesReportsAnUnreadableWeatherStore(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activityWeatherErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities")
	assert.Equal(t, http.StatusServiceUnavailable, code)

	// The track is served from a ride this target does hold, so the failure it
	// reports is the weather read rather than a missing ride.
	tracked := trackState("rider-a")
	tracked.activityWeatherErr = errors.New("unreadable")
	trackCode, _ := getTrack(t, activityHandler(t, tracked, nonAdminSessions("rider-a")),
		"/v1/activities/1/track")
	assert.Equal(t, http.StatusServiceUnavailable, trackCode, "and the track alongside it")
}

func TestGetActivitiesReportsAnUnreadableMetricsStore(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activityMetricsErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities")
	assert.Equal(t, http.StatusServiceUnavailable, code)
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

// The estimate is served under its own name, indexed with the coordinates and
// null where none was made. A chart must be able to draw it as an estimate and
// never mistake it for a measurement.
func TestGetActivityTrackServesTheEstimatedPowerUnderItsOwnName(t *testing.T) {
	state := trackState("rider-a")
	track := state.tracks["rider-a/1"]
	track[1].EstimatedPowerWatts, track[1].HasEstimatedPower = 214, true
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Properties.EstimatedPowerWatts, 2, "indexed with the coordinates")
	assert.Nil(t, view.Properties.EstimatedPowerWatts[0], "no estimate for the first sample")
	require.NotNil(t, view.Properties.EstimatedPowerWatts[1])
	assert.InDelta(t, 214.0, *view.Properties.EstimatedPowerWatts[1], 1e-9)
}

// A ride nothing estimated carries no such array at all, rather than one of
// nulls the chart would have to look through to find nothing.
func TestGetActivityTrackOmitsTheEstimateEntirelyWhenNoneWasMade(t *testing.T) {
	handler := activityHandler(t, trackState("rider-a"), nonAdminSessions("rider-a"))

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Nil(t, view.Properties.EstimatedPowerWatts)
	assert.NotNil(t, view.Properties.AltitudeMetres, "the altitudes are still there")
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
