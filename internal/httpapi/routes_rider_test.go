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
	"bikeMassKg": 8.4,
	"dragAreaM2": 0.40,
	"rollingResistance": 0.008
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

	return riderHandlerWithTasks(t, state, subject, nil)
}

func riderHandlerWithTasks(t *testing.T, state State, subject string, tasks Tasks) *Handler {
	t.Helper()
	handler := handlerFor(t, nonAdminSessions(subject), &fakeOAuth{}, state, tasks)
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
	require.NotNil(t, saved.Profile.DragAreaM2, "the bicycle's drag area")
	assert.InDelta(t, 0.40, *saved.Profile.DragAreaM2, 1e-9)
	require.NotNil(t, saved.Profile.RollingResistance, "the bicycle's rolling resistance")
	assert.InDelta(t, 0.008, *saved.Profile.RollingResistance, 1e-9)

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

// A suggestion is the best across every target the caller owns, not the last
// one looked at, so a rider with two accounts is offered their better effort.
func TestGetRiderProfileSuggestsTheBestAcrossTheCallersTargets(t *testing.T) {
	state := riderState()
	state.targets = append(state.targets, fakeTarget{id: "rider-a-2", authorization: "authorized", owner: "rider-a"})
	state.riderSuggestions["rider-a-2"] = rider.Suggestions{
		MaxHeartRateBPM:               rider.Set(179),
		FunctionalThresholdPowerWatts: rider.Set(244),
	}
	handler := riderHandler(t, state, "rider-a")

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.Equal(t, []string{"rider-a", "rider-a-2"}, state.riderSuggestionFor, "both of the caller's own")
	require.NotNil(t, view.Suggestions.MaxHeartRateBpm)
	assert.InDelta(t, 183.0, *view.Suggestions.MaxHeartRateBpm, 1e-9, "the better of the two")
	require.NotNil(t, view.Suggestions.FunctionalThresholdPowerWatts)
	assert.InDelta(t, 244.0, *view.Suggestions.FunctionalThresholdPowerWatts, 1e-9,
		"and the only one of the other")
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

// Everything derived from these numbers is now worked out against values
// nobody holds any more, so a profile write starts the derivation — over this
// rider's own targets and no others.
func TestSetRiderProfileStartsTheDerivationOverTheCallersOwnTargets(t *testing.T) {
	state := riderState()
	state.targets = append(state.targets, fakeTarget{id: "rider-a-2", authorization: "authorized", owner: "rider-a"})
	tasks := &fakeTasks{}
	handler := riderHandlerWithTasks(t, state, "rider-a", tasks)

	riderProfileOf(t, handler, authenticatedRequestWithBody(http.MethodPut, riderPath, riderSubmission))

	assert.Equal(t, []startedTask{
		{name: TaskActivityDerive, argument: "rider-a"},
		{name: TaskActivityDerive, argument: "rider-a-2"},
	}, tasks.asked, "the caller's own targets, and never rider-b's")
}

// A refused start means that work is already happening, and the task recomputes
// against the profile as it stands when it runs — so the write still answers.
func TestSetRiderProfileAnswersWhenTheDerivationIsAlreadyRunning(t *testing.T) {
	state := riderState()
	handler := riderHandlerWithTasks(t, state, "rider-a", &fakeTasks{refuse: true})

	view := riderProfileOf(t, handler, authenticatedRequestWithBody(http.MethodPut, riderPath, riderSubmission))
	require.NotNil(t, view.Profile.MaxHeartRateBpm, "the write still stored and answered")
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
		"the credentials cannot be read": func(s *fakeState) {
			s.riderCredentialsErr = errors.New("unreadable")
		},
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

// The habit is served as an object or not at all, and the eligible types the
// service is configured with are what the store is asked over.
func TestGetRiderProfileServesTheCallersOwnStoppingHabit(t *testing.T) {
	state := riderState()
	state.riderSuggestions["rider-a"] = rider.Suggestions{
		Stopping: rider.Stopping{
			MedianSecondsPerHour:        300,
			LowerQuartileSecondsPerHour: 200,
			UpperQuartileSecondsPerHour: 400,
			Rides:                       9,
			Set:                         true,
		},
	}
	handler := riderHandler(t, state, "rider-a")
	handler.stoppingTypes = []int{15, 16}

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	require.NotNil(t, view.Suggestions.Stopping, "a measured habit is served")
	assert.InDelta(t, 300.0, view.Suggestions.Stopping.MedianSecondsPerHour, 0.001)
	assert.InDelta(t, 200.0, view.Suggestions.Stopping.LowerQuartileSecondsPerHour, 0.001)
	assert.InDelta(t, 400.0, view.Suggestions.Stopping.UpperQuartileSecondsPerHour, 0.001)
	assert.Equal(t, 9, view.Suggestions.Stopping.Rides)
	assert.Equal(t, []int{15, 16}, state.riderSuggestionTypes, "the configured eligible types")
}

func TestGetRiderProfileOmitsAStoppingHabitTheRidesDoNotCarry(t *testing.T) {
	handler := riderHandler(t, riderState(), "rider-a")

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.Nil(t, view.Suggestions.Stopping, "too few rides offer nothing rather than zeroes")
}

const riderZwiftCredentialsPath = "/v1/settings/rider/credentials/zwift" //nolint:gosec // G101: a route path, not a credential

// A rider who has entered nothing reads emailSet/passwordSet both false.
func TestGetRiderProfileReportsNeitherZwiftCredentialSet(t *testing.T) {
	handler := riderHandler(t, riderState(), "rider-a")

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.False(t, view.Zwift.EmailSet)
	assert.False(t, view.Zwift.PasswordSet)
}

// A PUT stores through the fake, and the answer never carries the value: a GET
// afterwards reports only that it is set.
func TestSetRiderZwiftCredentialsStoresAndNeverReturnsAValue(t *testing.T) {
	state := riderState()
	handler := riderHandler(t, state, "rider-a")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath,
		`{"email": "rider@example.test", "password": "opensesame"}`))
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "opensesame")
	assert.NotContains(t, response.Body.String(), "rider@example.test")

	require.Contains(t, state.riderCredentials, "rider-a")
	assert.Equal(t, []byte("rider@example.test"),
		state.riderCredentials["rider-a"][rider.CredentialZwiftEmail].Bytes())
	assert.Equal(t, []byte("opensesame"),
		state.riderCredentials["rider-a"][rider.CredentialZwiftPassword].Bytes())

	view := riderProfileOf(t, handler, authenticatedRequest(http.MethodGet, riderPath))
	assert.True(t, view.Zwift.EmailSet)
	assert.True(t, view.Zwift.PasswordSet)
}

// A field left out of the body keeps whatever is stored: this is a partial
// save, unlike the rider's parameters, which a save replaces whole.
func TestSetRiderZwiftCredentialsLeavesOutAFieldNotTyped(t *testing.T) {
	state := riderState()
	handler := riderHandler(t, state, "rider-a")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath,
		`{"email": "rider@example.test", "password": "opensesame"}`))
	require.Equal(t, http.StatusNoContent, response.Code)

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath,
		`{"password": "newpassword"}`))
	require.Equal(t, http.StatusNoContent, response.Code)

	assert.Equal(t, []byte("rider@example.test"),
		state.riderCredentials["rider-a"][rider.CredentialZwiftEmail].Bytes(),
		"the field left out of the second write")
	assert.Equal(t, []byte("newpassword"),
		state.riderCredentials["rider-a"][rider.CredentialZwiftPassword].Bytes())
}

// DELETE clears both credentials.
func TestDeleteRiderZwiftCredentialsClearsBoth(t *testing.T) {
	state := riderState()
	handler := riderHandler(t, state, "rider-a")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath,
		`{"email": "rider@example.test", "password": "opensesame"}`))
	require.Equal(t, http.StatusNoContent, response.Code)

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodDelete, riderZwiftCredentialsPath))
	require.Equal(t, http.StatusNoContent, response.Code)

	assert.Empty(t, state.riderCredentials["rider-a"])
}

// Both routes act on the caller's own subject, exactly like the rider profile
// they sit beside, and work for a non-admin session.
func TestRiderZwiftCredentialsAreTheCallersOwnEvenForAnAdmin(t *testing.T) {
	state := riderState()
	state.riderCredentials = map[string]map[rider.CredentialName]rider.Credential{
		"rider-b": {rider.CredentialZwiftEmail: rider.NewCredential([]byte("other@example.test"))},
	}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil)
	handler.now = activityClock

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath,
		`{"email": "admin@example.test"}`))
	require.Equal(t, http.StatusNoContent, response.Code)

	assert.Equal(t, []byte("admin@example.test"),
		state.riderCredentials[testSubject][rider.CredentialZwiftEmail].Bytes())
	assert.Equal(t, []byte("other@example.test"),
		state.riderCredentials["rider-b"][rider.CredentialZwiftEmail].Bytes(),
		"another rider's credential is untouched")
}

func TestRiderZwiftCredentialsReportAnUnreadableStore(t *testing.T) {
	state := riderState()
	state.riderCredentialsErr = errors.New("unreadable")
	handler := riderHandler(t, state, "rider-a")

	put := httptest.NewRecorder()
	handler.ServeHTTP(put, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath, `{"email": "x"}`))
	assert.Equal(t, http.StatusServiceUnavailable, put.Code)

	del := httptest.NewRecorder()
	handler.ServeHTTP(del, authenticatedRequest(http.MethodDelete, riderZwiftCredentialsPath))
	assert.Equal(t, http.StatusServiceUnavailable, del.Code)
}

func TestSetRiderZwiftCredentialsRefusesAFieldThisSectionHasNot(t *testing.T) {
	handler := riderHandler(t, riderState(), "rider-a")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, riderZwiftCredentialsPath,
		`{"apiKey": "x"}`))
	assert.Equal(t, http.StatusBadRequest, response.Code)
}
