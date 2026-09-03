package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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

// An admin's status view marks exactly their own target as `own`, and carries
// no `own` field on anyone else's; a non-admin's view carries no `own` field
// at all, since a status view scoped to one target has nothing to compare.
func TestGetStatusMarksOnlyTheAdminsOwnTargetAsOwn(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{
		{id: "target-1", authorization: "authorized", owner: "rider-b"},
		{id: "target-2", authorization: "authorized", owner: testSubject},
	}}

	adminView := statusOf(t, handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil))
	require.Len(t, adminView.Targets, 2)
	own := map[string]bool{}
	for _, target := range adminView.Targets {
		own[target.ID] = target.Own != nil && *target.Own
	}
	assert.Equal(t, map[string]bool{"target-1": false, "target-2": true}, own)

	response := httptest.NewRecorder()
	handlerFor(t, nonAdminSessions("rider-b"), &fakeOAuth{}, state, nil).
		ServeHTTP(response, signedInRequest(http.MethodGet, "/v1/status"))
	require.Equal(t, http.StatusOK, response.Code)

	var nonAdminView struct {
		Targets []struct {
			Own *bool `json:"own"`
		} `json:"targets"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &nonAdminView))
	require.Len(t, nonAdminView.Targets, 1)
	assert.Nil(t, nonAdminView.Targets[0].Own, "own is admin-only")
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

// A target whose slot matches the caller's own subject but whose recorded
// owner is someone else — reachable only by an operator editing state by
// hand — must not let self-service quietly start writing to it anyway.
func TestStartOAuthRefusesTheCallersOwnSlotWhenItIsOwnedBySomeoneElse(t *testing.T) {
	state := &fakeState{targets: []fakeTarget{{id: "rider-a", authorization: "authorized", owner: "rider-b"}}}
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := handlerFor(t, nonAdminSessions("rider-a"), oauthService, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/oauth/wahoo/start"))
	assert.Equal(t, http.StatusNotFound, response.Code, "start status")
	assert.Empty(t, oauthService.targetID, "a misassigned target must not have been started")
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

// A non-admin's own sync:target run is accepted: it is the one task they may
// start at all.
func TestRunTaskAllowsNonAdminToRunTheirOwnTarget(t *testing.T) {
	tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncTarget}}}
	handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncTarget)+"/run/rider-a"))
	assert.Equal(t, http.StatusAccepted, response.Code, "run status")
	assert.Equal(t, []startedTask{{name: TaskSyncTarget, argument: "rider-a"}}, tasks.started)
}

// Clearing a target is service administration, so a non-admin is refused even
// over their own subject.
func TestRunTaskRefusesSyncClearOverTheOwnTargetForNonAdmin(t *testing.T) {
	tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncClear}}}
	handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncClear)+"/run/rider-a"))
	assert.Equal(t, http.StatusForbidden, response.Code, "run status")
	assert.Empty(t, tasks.asked, "the task must not have been reached")
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

// sync:clear has no "every target" meaning the way sync:target's empty
// argument does: it always deletes one named target's routes, so an empty
// argument is refused as invalid rather than run — even for an admin, who
// would otherwise be the one caller this doesn't already refuse as not found.
func TestRunTaskRefusesAnEmptyArgumentForSyncClearEvenForAdmin(t *testing.T) {
	tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncClear}}}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncClear)+"/run"))
	assert.Equal(t, http.StatusBadRequest, response.Code, "run status with no argument")
	assert.Empty(t, tasks.asked, "the task must not have been reached")
}

// encodedTaskName percent-encodes a task name's colon the way a browser's own
// request does, so a path built from it matches what the mux registers.
func encodedTaskName(name string) string {
	return strings.ReplaceAll(name, ":", "%3A")
}

// A status request that cannot read who owns what is unavailable rather than
// silently answering as though nobody owned anything.
func TestGetStatusReportsUnavailableWhenTargetIDsFails(t *testing.T) {
	state := &fakeState{targetErr: errors.New("state unavailable")}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/v1/status"))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}

// targetRouteCounts and targetRuns each read ownership scoping on their own,
// independently of GetStatus's own read, and each must fail the same way.
func TestTargetRouteCountsAndTargetRunsReturnTheTargetIDsError(t *testing.T) {
	state := &fakeState{targetErr: errors.New("state unavailable")}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil)

	_, err := handler.targetRouteCounts(context.Background())
	require.Error(t, err)

	_, err = handler.targetRuns(context.Background())
	require.Error(t, err)
}

// A failed self-service creation is reported as unavailable, not silently
// treated as though the caller had no target to connect.
func TestStartOAuthReportsUnavailableWhenEnsureTargetOwnerFails(t *testing.T) {
	state := &fakeState{ensureTargetOwnerErr: errors.New("state unavailable")}
	handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/oauth/wahoo/start/rider-a"))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}

// An admin naming another subject's target still needs the existing-target
// list to check it against; a failed read is unavailable, not a silent 404.
func TestStartOAuthReportsUnavailableWhenTargetIDsFailsForAdmin(t *testing.T) {
	state := &fakeState{targetErr: errors.New("state unavailable")}
	handler := handlerFor(t, newFakeSessions(), &fakeOAuth{}, state, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/oauth/wahoo/start/rider-b"))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}
