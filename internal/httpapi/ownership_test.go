package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/session"
)

// nonAdminSessions is a session service that admits testSessionToken as the
// given, non-admin subject — the identity every ownership-scoping test in
// this file runs its request as.
func nonAdminSessions(subject string) *fakeSessions {
	sessions := newFakeSessions()
	sessions.identity = session.Identity{Subject: subject, Display: subject, Admin: false}

	return sessions
}

// handlerFor builds a handler over the given state, task list, and OAuth
// service, gated by sessions. Ownership tests each need a different
// combination of these, unlike the rest of the suite which mostly varies one.
func handlerFor(t *testing.T, sessions Sessions, oauthService OAuth, state State, tasks Tasks) *Handler {
	t.Helper()
	if tasks == nil {
		tasks = &fakeTasks{}
	}
	handler, err := New(
		&Options{
			Alerts:           &fakeAlerts{},
			Tasks:            tasks,
			Settings:         settingsWith(testBasemaps()),
			Sessions:         sessions,
			BrowserOriginURL: testBrowserOriginURL,
		},
		oauthService, state, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// A non-admin's own status view names only their own target: another rider's
// is neither counted nor named, and owner is never sent to a non-admin at all.
func TestGetStatusScopesTargetsToTheCallersOwnSubjectForNonAdmin(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{
		{id: "rider-a", authorization: "authorized", owner: "rider-a"},
		{id: "rider-b", authorization: "authorized", owner: "rider-b"},
	}}
	handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/v1/status"))
	require.Equal(t, http.StatusOK, response.Code)

	var view struct {
		Targets []struct {
			Owner *string `json:"owner"`
			ID    string  `json:"id"`
		} `json:"targets"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view))
	require.Len(t, view.Targets, 1, "a non-admin sees only their own target")
	assert.Equal(t, "rider-a", view.Targets[0].ID)
	assert.Nil(t, view.Targets[0].Owner, "owner is admin-only")
}

// An admin's status view names every target, each carrying who owns it.
func TestGetStatusNamesEveryTargetsOwnerForAdmin(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{
		{id: "rider-a", authorization: "authorized", owner: "rider-a"},
		{id: "rider-b", authorization: "authorized", owner: "rider-b"},
	}}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil)

	view := statusOf(t, handler)

	require.Len(t, view.Targets, 2, "an admin sees every target")
	owners := map[string]string{}
	for _, target := range view.Targets {
		require.NotNil(t, target.Owner, "%s: owner", target.ID)
		owners[target.ID] = *target.Owner
	}
	assert.Equal(t, map[string]string{"rider-a": "rider-a", "rider-b": "rider-b"}, owners)
}

// A rider's own "Connect" click is always allowed, and creates their target
// the first time — the one self-service creation point.
func TestStartOAuthCreatesAndAllowsTheCallersOwnTarget(t *testing.T) {
	state := &fakeState{}
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := handlerFor(t, nonAdminSessions("rider-a"), oauthService, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/oauth/wahoo/start/rider-a"))
	assert.Equal(t, http.StatusFound, response.Code, "start status")
	assert.Equal(t, "rider-a", oauthService.targetID, "the OAuth flow was started for the caller's own target")
	assert.Contains(t, state.ensuredOwners, "rider-a", "the target was created on first connect")
}

// A non-admin cannot start a Wahoo authorization for another subject's
// target: refused as not found, the same as a target that does not exist,
// so a rider cannot probe which other targets are configured.
func TestStartOAuthRefusesAnotherSubjectsTargetForNonAdmin(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{{id: "rider-b", authorization: "authorized", owner: "rider-b"}}}
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := handlerFor(t, nonAdminSessions("rider-a"), oauthService, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/oauth/wahoo/start/rider-b"))
	assert.Equal(t, http.StatusNotFound, response.Code, "start status")
	assert.Empty(t, oauthService.targetID, "another subject's target must not have been started")
}

// An admin may start an existing target that is not their own — the shared
// operator right this build otherwise scopes away.
func TestStartOAuthAllowsAdminToStartAnotherExistingTarget(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{{id: "rider-b", authorization: "not_authorized", owner: "rider-b"}}}
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := handlerFor(t, newFakeSessions(), oauthService, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/oauth/wahoo/start/rider-b"))
	assert.Equal(t, http.StatusFound, response.Code, "start status")
	assert.Equal(t, "rider-b", oauthService.targetID)
}

// A non-admin's own sync:target/sync:clear run is accepted.
func TestRunTaskAllowsNonAdminToRunTheirOwnTarget(t *testing.T) {
	for _, name := range []string{TaskSyncTarget, TaskSyncClear} {
		t.Run(name, func(t *testing.T) {
			tasks := &fakeTasks{registered: []RegisteredTask{{Name: name}}}
			handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(name)+"/run/rider-a"))
			assert.Equal(t, http.StatusAccepted, response.Code, "run status")
			assert.Equal(t, []startedTask{{name: name, argument: "rider-a"}}, tasks.started)
		})
	}
}

// A non-admin cannot start sync:target/sync:clear against another subject's
// target, nor run it over every target by leaving the argument empty.
func TestRunTaskRefusesAnotherSubjectsTargetForNonAdmin(t *testing.T) {
	for _, name := range []string{TaskSyncTarget, TaskSyncClear} {
		t.Run(name, func(t *testing.T) {
			tasks := &fakeTasks{registered: []RegisteredTask{{Name: name}}}
			handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(name)+"/run/rider-b"))
			assert.Equal(t, http.StatusNotFound, response.Code, "run status")
			assert.Empty(t, tasks.asked, "the task must not have been reached")
		})
	}
}

// Leaving the argument empty runs sync:target/sync:clear over every target,
// which only an admin may ask for.
func TestRunTaskRefusesAnEmptyArgumentForNonAdmin(t *testing.T) {
	tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncTarget}}}
	handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncTarget)+"/run"))
	assert.Equal(t, http.StatusNotFound, response.Code, "run status")
	assert.Empty(t, tasks.asked, "the task must not have been reached")
}

// An admin may run sync:target/sync:clear over any target, including every
// one at once.
func TestRunTaskAllowsAdminAnyTargetOrEveryTarget(t *testing.T) {
	tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncTarget}}}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncTarget)+"/run/rider-b"))
	assert.Equal(t, http.StatusAccepted, response.Code, "run status against another subject's target")

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncTarget)+"/run"))
	assert.Equal(t, http.StatusAccepted, response.Code, "run status with no argument")
}

// encodedTaskName percent-encodes a task name's colon the way a browser's own
// request does, so a path built from it matches what the mux registers.
func encodedTaskName(name string) string {
	return strings.ReplaceAll(name, ":", "%3A")
}
