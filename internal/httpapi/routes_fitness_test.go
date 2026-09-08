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

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

// fitnessState is one rider's own target carrying the given rides, plus another
// rider's target so a scoping test has something to fail to read.
func fitnessState(loads ...trainingload.RideLoad) *fakeState {
	return &fakeState{
		targets: []fakeTarget{
			{id: "rider-a", authorization: "authorized", owner: "rider-a"},
			{id: "rider-b", authorization: "authorized", owner: "rider-b"},
		},
		rideLoads: map[string][]trainingload.RideLoad{
			"rider-a": loads,
			"rider-b": {{At: activityClock().Add(-24 * time.Hour), TSS: 999}},
		},
	}
}

func getFitness(t *testing.T, handler *Handler, target string) (int, openapi.Fitness) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, target))
	var view openapi.Fitness
	if response.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the timeline")
	}

	return response.Code, view
}

func TestGetFitnessServesOneRowPerDayOfTheWindow(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{At: activityClock().Add(-72 * time.Hour), TSS: 100, TRIMP: 80},
		trainingload.RideLoad{At: activityClock().Add(-24 * time.Hour), TSS: 60, TRIMP: 40},
	)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Days, 4, "the day of the first ride through to today")
	assert.Positive(t, view.Days[0].TssLoad, "the first ride's own day")
	// A day nobody rode still moves the averages, which is what makes rest visible.
	assert.Zero(t, view.Days[1].TssLoad)
	assert.Less(t, view.Days[1].TssFatigue, view.Days[0].TssFatigue)
	assert.InDelta(t, view.Days[0].TssFitness-view.Days[0].TssFatigue, view.Days[0].TssForm, 1e-9)
}

// The acceptance criterion: a subject with no derived metrics gets an empty
// series, not an error.
func TestGetFitnessIsEmptyForARiderWithNoDerivedMetrics(t *testing.T) {
	handler := activityHandler(t, fitnessState(), nonAdminSessions("rider-a"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, view.Days)
	assert.Empty(t, view.Weeks)
}

// A rider with no target of their own has an empty timeline, not a missing page.
func TestGetFitnessIsEmptyForARiderWithNoTarget(t *testing.T) {
	handler := activityHandler(t, fitnessState(), nonAdminSessions("rider-c"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, view.Days)
}

// Scoped exactly as the activity list is: naming another rider's target is not
// found rather than forbidden, so the surface never confirms which exist.
func TestGetFitnessRefusesAnotherRidersTarget(t *testing.T) {
	handler := activityHandler(t, fitnessState(), nonAdminSessions("rider-a"))

	code, _ := getFitness(t, handler, "/v1/activities/fitness?target=rider-b")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestGetFitnessServesAnyTargetForAnAdmin(t *testing.T) {
	handler := activityHandler(t, fitnessState(), newFakeSessions())

	code, view := getFitness(t, handler, "/v1/activities/fitness?target=rider-b")
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, view.Days, "an admin may read another rider's timeline")
}

// The window cuts what is returned, never what is folded: the averages at the
// window's first day are the ones the rider had actually built by then.
func TestGetFitnessFoldsFromTheFirstRideAndCutsToTheWindow(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{At: activityClock().Add(-30 * 24 * time.Hour), TSS: 200},
		trainingload.RideLoad{At: activityClock().Add(-24 * time.Hour), TSS: 50},
	)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	from := activityClock().Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)
	code, view := getFitness(t, handler, "/v1/activities/fitness?from="+from)
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, view.Days)
	assert.Positive(t, view.Days[0].TssFitness,
		"the fitness the month before it had built, not nothing")
	assert.Len(t, view.Days, 3, "the days the window touches, and no more")
}

// A window starts at whatever moment the reader opened the page, not at
// midnight. A day it half covers is a day it covers, and so is the week around
// that day.
func TestGetFitnessKeepsTheDayAndWeekAWindowOnlyPartlyCovers(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{
			At:    activityClock().Add(-36 * time.Hour),
			TSS:   50,
			Zones: trainingload.Zones{600, 0, 0, 0, 0},
		},
	)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	// Midday on the ride's own day: the morning of it is outside the window and
	// the afternoon inside.
	from := activityClock().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	code, view := getFitness(t, handler, "/v1/activities/fitness?from="+from)
	require.Equal(t, http.StatusOK, code)

	require.NotEmpty(t, view.Days, "the half-covered day is still a day of the window")
	assert.Len(t, view.Weeks, 1, "and its week is still a week of it")
}

func TestGetFitnessSumsTimeInZoneByWeek(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{
			At:    activityClock().Add(-24 * time.Hour),
			Zones: trainingload.Zones{600, 1200, 0, 0, 0},
		},
	)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.Weeks, 1)
	assert.Equal(t, []float64{600, 1200, 0, 0, 0}, view.Weeks[0].ZoneSeconds)
	assert.NotEmpty(t, view.Weeks[0].WeekStart)
}

func TestGetFitnessRefusesAWindowThatEndsBeforeItStarts(t *testing.T) {
	handler := activityHandler(t, fitnessState(), nonAdminSessions("rider-a"))

	code, _ := getFitness(t, handler,
		"/v1/activities/fitness?from=2026-08-24T00:00:00Z&to=2026-08-01T00:00:00Z")
	assert.Equal(t, http.StatusBadRequest, code)
}

// A zone this build cannot load leaves the days in UTC rather than failing the
// read: a timeline in the wrong zone is a smaller problem than no timeline.
func TestGetFitnessFallsBackToUTCForAZoneItCannotLoad(t *testing.T) {
	state := fitnessState(trainingload.RideLoad{At: activityClock().Add(-24 * time.Hour), TSS: 50})
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))
	// Written past the rules rather than through them: the settings refuse a
	// zone they cannot load, so the only way to reach this is the tzdata
	// database changing under a running process, which is what it defends.
	settings, ok := handler.settings.(*staticSettings)
	require.True(t, ok, "the test handler's own settings")
	settings.values.Timezone = "Nowhere/Invented"

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, view.Days, "the timeline is still served")
}

// A target this caller may not read is refused before the store is asked.
func TestGetFitnessReportsAnUnreadableTargetList(t *testing.T) {
	state := fitnessState()
	state.targetErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getFitness(t, handler, "/v1/activities/fitness")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

func TestGetFitnessReportsAnUnreadableStore(t *testing.T) {
	state := fitnessState()
	state.rideLoadsErr = errors.New("unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, _ := getFitness(t, handler, "/v1/activities/fitness")
	assert.Equal(t, http.StatusServiceUnavailable, code)
}

// The curve is served beside the timeline, shortest duration first, carrying
// only the durations the window's rides actually reached.
func TestGetFitnessCarriesTheWindowsPowerCurve(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{At: activityClock().Add(-24 * time.Hour), TSS: 60, TRIMP: 40},
	)
	curve := rider.PowerCurve{}
	curve.Watts[0], curve.Held[0] = 900, true
	curve.Watts[rider.ThresholdPowerPoint], curve.Held[rider.ThresholdPowerPoint] = 268, true
	state.powerCurves = map[string]rider.PowerCurve{"rider-a": curve}
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, view.PowerCurve, 2, "only the durations the rides reached")
	assert.Equal(t, 5, view.PowerCurve[0].Seconds)
	assert.InDelta(t, 900.0, view.PowerCurve[0].Watts, 1e-9)
	assert.Equal(t, 1200, view.PowerCurve[1].Seconds, "the threshold window")
	assert.InDelta(t, 268.0, view.PowerCurve[1].Watts, 1e-9)
	assert.Equal(t, []string{"rider-a"}, state.powerCurveFor, "the caller's own target alone")
}

// A window whose rides carried no meter has no curve at all, rather than a
// curve of noughts.
func TestGetFitnessOmitsAPowerCurveNoRideCarried(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{At: activityClock().Add(-24 * time.Hour), TSS: 60, TRIMP: 40},
	)
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, view.PowerCurve)
	assert.NotEmpty(t, view.Days, "the timeline is still served")
}

// A curve that cannot be read costs the page its curve, not its timeline.
func TestGetFitnessServesTheTimelineWhenTheCurveCannotBeRead(t *testing.T) {
	state := fitnessState(
		trainingload.RideLoad{At: activityClock().Add(-24 * time.Hour), TSS: 60, TRIMP: 40},
	)
	state.powerCurveErr = errors.New("the state is unreadable")
	handler := activityHandler(t, state, nonAdminSessions("rider-a"))

	code, view := getFitness(t, handler, "/v1/activities/fitness")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, view.PowerCurve)
	assert.NotEmpty(t, view.Days)
}
