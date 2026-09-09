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
	"github.com/nobbs/domestique/internal/measure"
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
			"rider-b": {{
				ID: 99, StartedAt: activityClock().Add(-time.Hour), DistanceMetres: 1,
				Provider: activities.ProviderWahoo,
			}},
		},
	}
	for index, age := range ages {
		state.activities[subject] = append(state.activities[subject], activities.Stored{
			ID: int64(index + 1), StartedAt: activityClock().Add(-age),
			DistanceMetres: 1000, MovingSeconds: 60, ElapsedSeconds: 90, AscentMetres: 10,
			TypeID: 15, LocationID: 1, Provider: activities.ProviderWahoo,
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
				MaxSpeedKmh: 54.2, HasSpeed: true,
			},
			HasEstimateQuality: true,
			EstimateQuality: measure.Quality{
				Autocorrelation1: 0.91, MeanAbsDeltaWattsPerSecond: 11.4, ClipBiasWatts: 2.1,
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
	require.NotNil(t, derived.Metrics.EstimateQuality, "the estimate's own quality diagnostics")
	assert.InDelta(t, 0.91, derived.Metrics.EstimateQuality.Autocorrelation, 1e-9)
	assert.InDelta(t, 11.4, derived.Metrics.EstimateQuality.MeanAbsDeltaWattsPerSecond, 1e-9)
	assert.InDelta(t, 2.1, derived.Metrics.EstimateQuality.ClipBiasWatts, 1e-9)
	require.NotNil(t, derived.Metrics.AverageHeartRateBpm)
	assert.InDelta(t, 142.5, *derived.Metrics.AverageHeartRateBpm, 1e-9)
	require.NotNil(t, derived.Metrics.MaxHeartRateBpm)
	assert.InDelta(t, 178.0, *derived.Metrics.MaxHeartRateBpm, 1e-9)
	require.NotNil(t, derived.Metrics.AverageCadenceRpm)
	assert.InDelta(t, 81.5, *derived.Metrics.AverageCadenceRpm, 1e-9)
	require.NotNil(t, derived.Metrics.AveragePowerWatts, "which the average power does not need")
	assert.InDelta(t, 196.25, *derived.Metrics.AveragePowerWatts, 1e-9)
	require.NotNil(t, derived.Metrics.MaxSpeedKmh)
	assert.InDelta(t, 54.2, *derived.Metrics.MaxSpeedKmh, 1e-9)
	assert.Nil(t, plain.Metrics, "and a ride with no row carries none at all")
}

// A ride with a measured average power carries no estimate, and so no
// quality diagnostics about one either.
func TestGetActivitiesCarriesNoEstimateQualityWithoutAnEstimate(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Averages: activities.RideAverages{PowerWatts: 196.25, HasPower: true},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 2)

	derived := list.Activities[0]
	require.NotNil(t, derived.Metrics)
	assert.Nil(t, derived.Metrics.EstimatedPowerWatts)
	assert.Nil(t, derived.Metrics.EstimateQuality)
	assert.Nil(t, derived.Metrics.MaxSpeedKmh, "no speed series behind this ride")
}

// A ride derived before the diagnostics existed carries an estimate and no
// quality until it is derived again; nulls are not a quality of nought.
func TestGetActivitiesCarriesNoEstimateQualityForAnEstimateDerivedBeforeItExisted(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Load: trainingload.Metrics{EstimatedPowerWatts: 150, HasEstimatedPower: true},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)

	derived := list.Activities[0]
	require.NotNil(t, derived.Metrics)
	require.NotNil(t, derived.Metrics.EstimatedPowerWatts)
	assert.Nil(t, derived.Metrics.EstimateQuality)
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
	assert.Equal(t, openapi.Activity_ProviderWahoo, list.Activities[0].Provider, "which upstream recorded the ride")
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

// An indoor ride is served no geometry regardless of what it stored, because
// its coordinates, if any, belong to a virtual world and a map of them would
// be false.
func TestGetActivityTrackServesNoGeometryForAnIndoorRide(t *testing.T) {
	state := trackState("rider-a")
	state.recordsStateTypes = map[string]int{"rider-a/1": 68}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))
	handler.indoorTypes = []int{68}

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Nil(t, view.Geometry, "an indoor ride has no line to draw")
	assert.Equal(t, trackStateIndoor, view.Properties.State)
}

// A ride in a virtual world this service knows keeps saying it was indoors —
// it was ridden over no ground — but carries its line and the world to draw it
// over.
func TestGetActivityTrackServesTheWorldOfAZwiftRide(t *testing.T) {
	state := trackState("rider-a")
	state.recordsStateTypes = map[string]int{"rider-a/1": 68}
	state.providerSummaries = map[string]fakeProviderSummary{
		"rider-a/1": {provider: activities.ProviderZwift, summary: []byte(`{"worldId":9}`)},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))
	handler.indoorTypes = []int{68}
	handler.zwiftWorldOf = testWorldOf

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, trackStateIndoor, view.Properties.State, "a virtual world is still no ground")
	require.NotNil(t, view.Geometry, "a world map needs the line to draw on it")
	assert.NotEmpty(t, view.BBox)
	require.NotNil(t, view.Properties.World)
	assert.Equal(t, int64(9), view.Properties.World.ID)
	assert.Equal(t, "Makuri Islands", view.Properties.World.Name)
	assert.Equal(t, "/v1/zwift/worlds/9/map", view.Properties.World.MapURL)
	assert.InDelta(t, -10.73746, view.Properties.World.Bounds.North, 1e-9)
}

// A world ride whose samples are not stored yet has no line, so it names no
// world either: the page would have nothing to draw over the artwork.
func TestGetActivityTrackNamesNoWorldWithoutALine(t *testing.T) {
	state := trackState("rider-a")
	state.tracks["rider-a/1"] = state.tracks["rider-a/1"][:1]
	state.recordsStateTypes = map[string]int{"rider-a/1": 68}
	state.providerSummaries = map[string]fakeProviderSummary{
		"rider-a/1": {provider: activities.ProviderZwift, summary: []byte(`{"worldId":9}`)},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))
	handler.indoorTypes = []int{68}
	handler.zwiftWorldOf = testWorldOf

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, trackStateIndoor, view.Properties.State)
	assert.Nil(t, view.Geometry)
	assert.Nil(t, view.Properties.World, "no line, so no world to draw it over")
}

// An indoor ride in no world this service knows is served as it was before
// there were any: no line, and nothing to draw it over.
func TestGetActivityTrackServesNoWorldForAnUnknownOne(t *testing.T) {
	for name, summary := range map[string]fakeProviderSummary{
		"another provider": {provider: "wahoo", summary: []byte(`{"worldId":9}`)},
		"unknown world":    {provider: activities.ProviderZwift, summary: []byte(`{"worldId":99}`)},
	} {
		t.Run(name, func(t *testing.T) {
			state := trackState("rider-a")
			state.recordsStateTypes = map[string]int{"rider-a/1": 68}
			state.providerSummaries = map[string]fakeProviderSummary{"rider-a/1": summary}
			handler := activityHandler(t, state, nonAdminSessions("rider-a"))
			handler.indoorTypes = []int{68}
			handler.zwiftWorldOf = testWorldOf

			code, view := getTrack(t, handler, "/v1/activities/1/track")
			require.Equal(t, http.StatusOK, code)
			assert.Equal(t, trackStateIndoor, view.Properties.State)
			assert.Nil(t, view.Geometry, "an indoor ride in no known world has no line")
			assert.Nil(t, view.Properties.World)
		})
	}
}

// A store that cannot say which provider recorded the ride is a failure, not a
// ride without a world.
func TestGetActivityTrackReportsAFailedWorldRead(t *testing.T) {
	state := trackState("rider-a")
	state.recordsStateTypes = map[string]int{"rider-a/1": 68}
	state.providerSummaryErr = assert.AnError
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))
	handler.indoorTypes = []int{68}
	handler.zwiftWorldOf = testWorldOf

	code, _ := getTrack(t, handler, "/v1/activities/1/track")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// testWorldOf stands in for the composition root's own adaptation of the Zwift
// world table, over the one world these tests name.
func testWorldOf(summary []byte) (ZwiftWorld, bool) {
	if string(summary) != `{"worldId":9}` {
		return ZwiftWorld{}, false
	}

	return ZwiftWorld{
		ID: 9, Name: "Makuri Islands",
		North: -10.73746, West: 165.76591, South: -10.85234, East: 165.88222,
	}, true
}

// An outdoor ride of the same shape is unaffected by the indoor type list.
func TestGetActivityTrackDrawsAnOutdoorRideDespiteAnIndoorTypeList(t *testing.T) {
	state := trackState("rider-a")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))
	handler.indoorTypes = []int{68}

	code, view := getTrack(t, handler, "/v1/activities/1/track")
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, view.Geometry, "an outdoor ride still draws its line")
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

// The two drift readings are served as the ride carries them: decoupling as a
// bare percentage, the heat-drift reading as the pair it only means anything as.
func TestGetActivitiesCarriesTheDriftReadingsOfEachRide(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Load:       trainingload.Metrics{Inputs: trainingload.Inputs{ThresholdHeartRateBPM: 170}},
			Decoupling: activities.Decoupling{Percent: 4.25, Known: true},
			HeatDrift: activities.HeatDrift{
				HeartRateBPM: 141.5, TemperatureCelsius: 29.5, Samples: 1800, Known: true,
			},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list.Activities, 2)

	derived, plain := list.Activities[0], list.Activities[1]
	require.NotNil(t, derived.Metrics)
	require.NotNil(t, derived.Metrics.DecouplingPercent)
	assert.InDelta(t, 4.25, *derived.Metrics.DecouplingPercent, 1e-9)
	require.NotNil(t, derived.Metrics.HeatDrift, "the pair, or nothing")
	assert.InDelta(t, 141.5, derived.Metrics.HeatDrift.HeartRateBpm, 1e-9)
	assert.InDelta(t, 29.5, derived.Metrics.HeatDrift.TemperatureCelsius, 1e-9)
	assert.Equal(t, 1800, derived.Metrics.HeatDrift.Samples)
	assert.Nil(t, plain.Metrics, "a ride nothing was derived for carries no metrics at all")
}

// A ride the derivation found neither reading for sends neither, rather than
// sending a decoupling of nought and a reading at nought degrees.
func TestGetActivitiesOmitTheDriftReadingsARideDidNotYield(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Load: trainingload.Metrics{TRIMP: 42.5, HasTRIMP: true},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	require.NotNil(t, list.Activities[0].Metrics)
	assert.Nil(t, list.Activities[0].Metrics.DecouplingPercent)
	assert.Nil(t, list.Activities[0].Metrics.HeatDrift)
}

// aKnownReading is a Reading the file declared, at the given value.
func aKnownReading(value float64) activities.Reading {
	return activities.Reading{Value: value, Known: true}
}

// deviceSession is the device's own session figures for one ride, over the
// five fields the derived averages also carry plus the ones only it does.
func deviceSession() activities.Session {
	return activities.Session{
		MaxSpeedKmh:          aKnownReading(61.0),
		AverageSpeedKmh:      aKnownReading(28.4),
		AverageHeartRateBPM:  aKnownReading(145.0),
		MaxHeartRateBPM:      aKnownReading(181.0),
		MinHeartRateBPM:      aKnownReading(88.0),
		AverageCadenceRPM:    aKnownReading(84.0),
		MaxCadenceRPM:        aKnownReading(112.0),
		AveragePowerWatts:    aKnownReading(201.0),
		MaxPowerWatts:        aKnownReading(742.0),
		ThresholdPowerWatts:  aKnownReading(255.0),
		DescentMetres:        aKnownReading(950.0),
		CaloriesKcal:         aKnownReading(1800.0),
		Sport:                "cycling",
		HeartRateZoneSeconds: []float64{60, 120, 180, 240, 300},
		HeartRateZoneHighBPM: []float64{120, 140, 160, 180, 254},
	}
}

// A ride with both a derived row and a device session serves the device's own
// figure for the five fields both carry, the new fields the derived row has
// no equivalent for, and the device's own zone bounds without the sentinel —
// all beside the profile-cut zoneSeconds, which the session leaves untouched.
func TestGetActivitiesPrefersTheDevicesOwnSessionFigures(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Load: trainingload.Metrics{
				Inputs: trainingload.Inputs{ThresholdHeartRateBPM: 170},
				Zones:  trainingload.Zones{10, 20, 30, 40, 50}, HasZones: true,
			},
			Averages: activities.RideAverages{
				HeartRateBPM: 130, MaxHeartRateBPM: 160, HasHeartRate: true,
				CadenceRPM: 70, HasCadence: true,
				PowerWatts: 150, HasPower: true,
				MaxSpeedKmh: 45, HasSpeed: true,
			},
		}},
	}
	state.activitySessions = map[string]map[int64]activities.Session{
		"rider-a": {1: deviceSession()},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	metrics := list.Activities[0].Metrics
	require.NotNil(t, metrics)

	// The profile-cut zones are untouched by the session.
	assert.Equal(t, []float64{10, 20, 30, 40, 50}, metrics.ZoneSeconds)

	require.NotNil(t, metrics.MaxSpeedKmh)
	assert.InDelta(t, 61.0, *metrics.MaxSpeedKmh, 1e-9, "the session wins over the derived average")
	require.NotNil(t, metrics.AverageHeartRateBpm)
	assert.InDelta(t, 145.0, *metrics.AverageHeartRateBpm, 1e-9)
	require.NotNil(t, metrics.MaxHeartRateBpm)
	assert.InDelta(t, 181.0, *metrics.MaxHeartRateBpm, 1e-9)
	require.NotNil(t, metrics.AverageCadenceRpm)
	assert.InDelta(t, 84.0, *metrics.AverageCadenceRpm, 1e-9)
	require.NotNil(t, metrics.AveragePowerWatts)
	assert.InDelta(t, 201.0, *metrics.AveragePowerWatts, 1e-9)

	require.NotNil(t, metrics.AverageSpeedKmh)
	assert.InDelta(t, 28.4, *metrics.AverageSpeedKmh, 1e-9)
	require.NotNil(t, metrics.MinHeartRateBpm)
	assert.InDelta(t, 88.0, *metrics.MinHeartRateBpm, 1e-9)
	require.NotNil(t, metrics.MaxCadenceRpm)
	assert.InDelta(t, 112.0, *metrics.MaxCadenceRpm, 1e-9)
	require.NotNil(t, metrics.MaxPowerWatts)
	assert.InDelta(t, 742.0, *metrics.MaxPowerWatts, 1e-9)
	require.NotNil(t, metrics.ThresholdPowerWatts)
	assert.InDelta(t, 255.0, *metrics.ThresholdPowerWatts, 1e-9)
	require.NotNil(t, metrics.Sport)
	assert.Equal(t, "cycling", *metrics.Sport)

	assert.Equal(t, []float64{60, 120, 180, 240, 300}, metrics.DeviceZoneSeconds)
	// The last high is the device's sentinel top, not a real cut.
	assert.Equal(t, []float64{120, 140, 160, 180}, metrics.DeviceZoneBoundsBpm)

	require.NotNil(t, list.Activities[0].DescentMetres)
	assert.InDelta(t, 950.0, *list.Activities[0].DescentMetres, 1e-9)
	require.NotNil(t, list.Activities[0].CaloriesKcal)
	assert.InDelta(t, 1800.0, *list.Activities[0].CaloriesKcal, 1e-9)
}

// A ride with a derived row and no session serves exactly what it did before
// this session data existed.
func TestGetActivitiesLeavesARideWithNoSessionUnchanged(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activityMetrics = map[string]map[int64]activities.RideMetrics{
		"rider-a": {1: {
			Averages: activities.RideAverages{HeartRateBPM: 130, MaxHeartRateBPM: 160, HasHeartRate: true},
		}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	metrics := list.Activities[0].Metrics
	require.NotNil(t, metrics)
	require.NotNil(t, metrics.AverageHeartRateBpm)
	assert.InDelta(t, 130.0, *metrics.AverageHeartRateBpm, 1e-9)
	assert.Nil(t, metrics.AverageSpeedKmh)
	assert.Nil(t, metrics.Sport)
	assert.Nil(t, metrics.DeviceZoneSeconds)
	assert.Nil(t, list.Activities[0].DescentMetres)
	assert.Nil(t, list.Activities[0].CaloriesKcal)
}

// A ride with a session but no derived row still carries metrics, holding
// only what the session declared.
func TestGetActivitiesServesASessionWithNoDerivedRow(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activitySessions = map[string]map[int64]activities.Session{
		"rider-a": {1: deviceSession()},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	metrics := list.Activities[0].Metrics
	require.NotNil(t, metrics, "a session with no metrics row still carries metrics")
	assert.Nil(t, metrics.ZoneSeconds, "no profile zones were derived for this ride")
	require.NotNil(t, metrics.AverageSpeedKmh)
	assert.InDelta(t, 28.4, *metrics.AverageSpeedKmh, 1e-9)
	require.NotNil(t, metrics.MaxSpeedKmh)
	assert.InDelta(t, 61.0, *metrics.MaxSpeedKmh, 1e-9)
}

// descentMetres and caloriesKcal ride with the activity, not the metrics, and
// only appear where the session declared them.
func TestGetActivitiesCarriesDescentAndCaloriesOnlyWhenKnown(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activitySessions = map[string]map[int64]activities.Session{
		"rider-a": {1: {DescentMetres: aKnownReading(400)}},
	}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	withDescent, withoutSession := list.Activities[0], list.Activities[1]
	require.NotNil(t, withDescent.DescentMetres)
	assert.InDelta(t, 400.0, *withDescent.DescentMetres, 1e-9)
	assert.Nil(t, withDescent.CaloriesKcal, "the session declared no calories for this ride")
	assert.Nil(t, withoutSession.DescentMetres)
	assert.Nil(t, withoutSession.CaloriesKcal)
}

// What Zwift lists a ride as is served alongside it; a ride with none stored
// carries no workout fields at all.
func TestGetActivitiesCarriesTheWorkoutOnlyWhenPresent(t *testing.T) {
	state := activityState("rider-a", time.Hour, 2*time.Hour)
	state.activities["rider-a"][0].HasWorkout = true
	state.activities["rider-a"][0].WorkoutName = "Sweet Spot Progression"
	state.activities["rider-a"][0].WorkoutHash = 998877
	state.activities["rider-a"][0].WorkoutCompletion = 0.87
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	withWorkout, withoutWorkout := list.Activities[0], list.Activities[1]
	require.NotNil(t, withWorkout.WorkoutName)
	assert.Equal(t, "Sweet Spot Progression", *withWorkout.WorkoutName)
	require.NotNil(t, withWorkout.WorkoutHash)
	assert.Equal(t, int64(998877), *withWorkout.WorkoutHash)
	require.NotNil(t, withWorkout.WorkoutCompletion)
	assert.InDelta(t, 0.87, *withWorkout.WorkoutCompletion, 1e-9)
	assert.Nil(t, withoutWorkout.WorkoutName, "nothing stored, nothing served")
	assert.Nil(t, withoutWorkout.WorkoutHash)
	assert.Nil(t, withoutWorkout.WorkoutCompletion)
}

// A store that cannot read sessions fails the whole list, the same as one
// that cannot read metrics.
func TestGetActivitiesReportsAnUnreadableSessionsStore(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	state.activitySessionsErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getActivities(t, handler, "/v1/activities")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGetActivitiesWithholdsDeviceZoneBoundsWithoutTheirTimes(t *testing.T) {
	state := activityState("rider-a", time.Hour)
	session := deviceSession()
	session.HeartRateZoneSeconds = nil
	state.activitySessions = map[string]map[int64]activities.Session{"rider-a": {1: session}}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, list := getActivities(t, handler, "/v1/activities")
	require.Equal(t, http.StatusOK, code)
	metrics := list.Activities[0].Metrics
	require.NotNil(t, metrics)
	assert.Nil(t, metrics.DeviceZoneSeconds)
	assert.Nil(t, metrics.DeviceZoneBoundsBpm)
}
