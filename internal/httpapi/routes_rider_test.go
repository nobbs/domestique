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
)

const riderPath = "/v1/settings/rider"

// riderSubmission differs from every default, so a test that sends it and reads
// the profile back cannot pass on what was already there.
const riderSubmission = `{
	"maxHeartRateBpm": 188,
	"restingHeartRateBpm": 46,
	"thresholdHeartRateBpm": 172,
	"functionalThresholdPowerWatts": 268,
	"riderMassKg": 74.5,
	"bikeMassKg": 8.4
}`

// riderState is two riders with a target each, so a scoping test has another
// rider's profile and rides to fail to read.
func riderState() *fakeState {
	return &fakeState{
		targets: []fakeTarget{
			{id: "rider-a", authorization: "authorized", owner: "rider-a"},
			{id: "rider-b", authorization: "authorized", owner: "rider-b"},
		},
		riderProfiles: map[string]rider.Profile{
			"rider-b": {MaxHeartRateBPM: rider.Set(199)},
		},
		riderSuggestions: map[string]rider.Suggestions{
			"rider-a": {MaxHeartRateBPM: rider.Set(183)},
			"rider-b": {MaxHeartRateBPM: rider.Set(201)},
		},
	}
}

func riderHandler(t *testing.T, state State, subject string) *Handler {
	t.Helper()
	handler := handlerFor(t, nonAdminSessions(subject), &fakeOAuth{}, state, nil)
	handler.now = activityClock

	return handler
}

func riderProfileOf(t *testing.T, handler *Handler, request *http.Request) openapi.RiderProfile {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var view openapi.RiderProfile
	require.NoError(t, json.NewDecoder(response.Body).Decode(&view), "decoding the rider profile")

	return view
}

func TestSetRiderProfileStoresItAndReadsItBack(t *testing.T) {
	state := riderState()
	handler := riderHandler(t, state, "rider-a")

	saved := riderProfileOf(t, handler, authenticatedRequestWithBody(http.MethodPut, riderPath, riderSubmission))
	require.NotNil(t, saved.Profile.MaxHeartRateBpm, "the maximum heart rate")
	assert.InDelta(t, 188.0, *saved.Profile.MaxHeartRateBpm, 1e-9)
	require.NotNil(t, saved.Profile.RiderMassKg, "the rider mass")
	assert.InDelta(t, 74.5, *saved.Profile.RiderMassKg, 1e-9)

	read := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.Equal(t, saved.Profile, read.Profile, "a read answers what the write stored")
}

// A rider who has entered nothing reads an empty profile, and reads it over
// their own subject: another rider's parameters are not on this path.
func TestGetRiderProfileIsEmptyForASubjectThatHasEnteredNothing(t *testing.T) {
	handler := riderHandler(t, riderState(), "rider-a")

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.Nil(t, view.Profile.MaxHeartRateBpm, "no parameter of this rider's own")
	assert.Nil(t, view.Profile.FunctionalThresholdPowerWatts, "nor any other")
}

// An admin is a rider here like any other: this section is the caller's own,
// and the administrator claim widens nothing about it.
func TestRiderProfileIsTheCallersOwnEvenForAnAdmin(t *testing.T) {
	state := riderState()
	state.riderProfiles[testSubject] = rider.Profile{MaxHeartRateBPM: rider.Set(175)}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil)
	handler.now = activityClock

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	require.NotNil(t, view.Profile.MaxHeartRateBpm)
	assert.InDelta(t, 175.0, *view.Profile.MaxHeartRateBpm, 1e-9, "the admin's own, not rider-b's 199")
}

// Suggestions are read over the caller's own targets and over the window the
// domain fixes, so a rider is never offered another rider's best effort.
func TestGetRiderProfileSuggestsFromTheCallersOwnRidesOnly(t *testing.T) {
	state := riderState()
	handler := riderHandler(t, state, "rider-a")

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	require.NotNil(t, view.Suggestions.MaxHeartRateBpm, "the rider's rides carry a strap")
	assert.InDelta(t, 183.0, *view.Suggestions.MaxHeartRateBpm, 1e-9)
	assert.Nil(t, view.Suggestions.FunctionalThresholdPowerWatts, "no ride carried a meter")
	assert.Equal(t, []string{"rider-a"}, state.riderSuggestionFor, "only the caller's own target")
	assert.Equal(t, activityClock().Add(-rider.SuggestionWindow), state.riderSuggestionSince)
}

// A rider with no target yet still reads their profile: the parameters are
// theirs whether or not an account is connected.
func TestGetRiderProfileAnswersARiderWithNoTarget(t *testing.T) {
	handler := riderHandler(t, riderState(), "rider-c")

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.Nil(t, view.Suggestions.MaxHeartRateBpm, "no rides, no suggestion")
}

// The profile is replaced whole, the way every settings section is: a parameter
// left out of the body is cleared rather than kept.
func TestSetRiderProfileReplacesTheWholeProfile(t *testing.T) {
	state := riderState()
	handler := riderHandler(t, state, "rider-a")

	riderProfileOf(t, handler, authenticatedRequestWithBody(http.MethodPut, riderPath, riderSubmission))
	view := riderProfileOf(t, handler,
		authenticatedRequestWithBody(http.MethodPut, riderPath, `{"maxHeartRateBpm": 190}`))

	require.NotNil(t, view.Profile.MaxHeartRateBpm)
	assert.InDelta(t, 190.0, *view.Profile.MaxHeartRateBpm, 1e-9)
	assert.Nil(t, view.Profile.RiderMassKg, "a parameter left out of the second write is cleared")
}

func TestSetRiderProfileRefusesAValueOutsideItsRange(t *testing.T) {
	handler := riderHandler(t, riderState(), "rider-a")

	for name, body := range map[string]string{
		"a heart rate no heart reaches": `{"maxHeartRateBpm": 400}`,
		"a rider of no mass":            `{"riderMassKg": 0}`,
		"a field this section has not":  `{"vo2Max": 60}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderPath, body))
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

func TestRiderProfileReportsAnUnreadableStore(t *testing.T) {
	for name, broken := range map[string]func(*fakeState){
		"the profile cannot be read":    func(s *fakeState) { s.riderProfileErr = errors.New("unreadable") },
		"the profile cannot be written": func(s *fakeState) { s.riderProfileWriteErr = errors.New("unwritable") },
		"the targets cannot be listed":  func(s *fakeState) { s.targetErr = errors.New("unreadable") },
		"the rides cannot be read":      func(s *fakeState) { s.riderSuggestionErr = errors.New("unreadable") },
	} {
		t.Run(name, func(t *testing.T) {
			state := riderState()
			broken(state)
			handler := riderHandler(t, state, "rider-a")

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderPath, riderSubmission))
			assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
		})
	}
}

// The suggestion window is the domain's, not this surface's: ninety days, so a
// best effort from years ago is not offered as a number about this rider now.
func TestRiderSuggestionWindowIsNinetyDays(t *testing.T) {
	assert.Equal(t, 90*24*time.Hour, rider.SuggestionWindow)
}
