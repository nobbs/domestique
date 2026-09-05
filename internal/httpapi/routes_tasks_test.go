package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

const tasksPath = "/v1/tasks"

// tasksHandler builds a handler over a task list a test can read back.
func tasksHandler(t *testing.T, registered ...RegisteredTask) (*Handler, *fakeTasks) {
	t.Helper()
	tasks := &fakeTasks{registered: registered}

	return handlerOverTasks(t, &fakeState{}, tasks), tasks
}

// handlerOverTasks builds a handler over one state and one task list.
func handlerOverTasks(t *testing.T, state State, tasks Tasks) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			Settings:         settingsWith(testBasemaps()),
			Alerts:           &fakeAlerts{},
			Tasks:            tasks,
			Sessions:         newFakeSessions(),
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// taskListOf sends a request and decodes the task list it answers with.
func taskListOf(t *testing.T, handler *Handler, request *http.Request) openapi.TaskList {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var view openapi.TaskList
	require.NoError(t, json.NewDecoder(response.Body).Decode(&view), "decoding the task list")

	return view
}

func TestListTasksReportsWhatThisBuildRegisters(t *testing.T) {
	due := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
	handler, _ := tasksHandler(t,
		RegisteredTask{Name: "sync:source", Scheduled: true, Running: 1, NextRunAt: due, Interval: time.Hour},
		RegisteredTask{Name: "sync:clear"},
	)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, tasksPath))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var view openapi.TaskList
	require.NoError(t, json.NewDecoder(response.Body).Decode(&view), "decoding the task list")
	require.Len(t, view.Tasks, 2, "tasks")
	assert.Equal(t, "sync:source", view.Tasks[0].Name, "the first task")
	assert.True(t, view.Tasks[0].Scheduled, "scheduled")
	assert.Equal(t, 1, view.Tasks[0].Running, "running")
	require.NotNil(t, view.Tasks[0].NextRunAt, "the due time")
	assert.Equal(t, due, view.Tasks[0].NextRunAt.UTC(), "the due time")
	require.NotNil(t, view.Tasks[0].IntervalSeconds, "the fixed gap between runs")
	assert.Equal(t, 3600, *view.Tasks[0].IntervalSeconds, "interval seconds")
	// A task nothing schedules is due at no particular time, and that has to
	// read as absent rather than as the zero instant. The same task has no
	// fixed gap either.
	assert.Nil(t, view.Tasks[1].NextRunAt, "a task nothing schedules reported a due time")
	assert.Nil(t, view.Tasks[1].IntervalSeconds, "a task nothing schedules reported an interval")
}

// A sub-second interval must not truncate to a zero that reads as absent's
// opposite: a task running every instant rather than one with no schedule.
func TestListTasksOmitsAnIntervalThatRoundsToZero(t *testing.T) {
	handler, _ := tasksHandler(t, RegisteredTask{Name: "sync:source", Interval: 400 * time.Millisecond})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, tasksPath))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var view openapi.TaskList
	require.NoError(t, json.NewDecoder(response.Body).Decode(&view), "decoding the task list")
	require.Len(t, view.Tasks, 1, "tasks")
	assert.Nil(t, view.Tasks[0].IntervalSeconds, "a sub-second interval was served as intervalSeconds: 0")
}

// A service with nothing registered still answers with a list, an empty one.
func TestListTasksSendsAnEmptyListAsAList(t *testing.T) {
	handler, _ := tasksHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, tasksPath))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var view openapi.TaskList
	require.NoError(t, json.NewDecoder(response.Body).Decode(&view), "decoding the task list")
	assert.NotNil(t, view.Tasks, "an empty list was sent as null")
	assert.Empty(t, view.Tasks, "tasks")
}

func TestRunTaskStartsTheNamedTask(t *testing.T) {
	tests := map[string]struct {
		target   string
		task     string
		argument string
	}{
		"with no argument": {target: "/v1/tasks/sync%3Asource/run", task: "sync:source"},
		"over an argument": {
			target: "/v1/tasks/sync%3Atarget/run/rider-a", task: "sync:target", argument: "rider-a",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler, tasks := tasksHandler(t,
				RegisteredTask{Name: "sync:source"}, RegisteredTask{Name: "sync:target"})

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, test.target))

			require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
			assert.Equal(t, []startedTask{{name: test.task, argument: test.argument}}, tasks.started, "started")
		})
	}
}

func TestRunTaskRefusesANameThisBuildDoesNotRegister(t *testing.T) {
	handler, tasks := tasksHandler(t, RegisteredTask{Name: "sync:source"})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/tasks/invented/run"))

	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	assert.Empty(t, tasks.asked, "a task this build does not register was asked for")
}

// A refusal is not a fault: the work is already happening, or something it
// needs is held. Either way the caller is told rather than left waiting.
func TestRunTaskReportsARefusal(t *testing.T) {
	handler, tasks := tasksHandler(t, RegisteredTask{Name: "sync:source"})
	tasks.refuse = true

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/tasks/sync%3Asource/run"))

	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "task_in_progress",
		"a task conflict was reported under another surface's code")
	assert.Empty(t, tasks.started, "a refused attempt started work")
	assert.Len(t, tasks.asked, 1, "attempts the handler asked for")
}

// The list is read-only, so a browser sends no Origin on it and requiring one
// would refuse the whole page.
func TestListTasksNeedsNoOrigin(t *testing.T) {
	handler, _ := tasksHandler(t, RegisteredTask{Name: "sync:source"})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tasksPath, http.NoBody)
	withSession(request)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
}

// A task name carries a colon, which a browser client percent-encodes. The
// route has to see the name the operator asked for rather than the escape.
func TestRunTaskAcceptsAPercentEncodedName(t *testing.T) {
	handler, tasks := tasksHandler(t, RegisteredTask{Name: "sync:target"})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/tasks/sync%3Atarget/run/rider-a"))

	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	assert.Equal(t, []startedTask{{name: "sync:target", argument: "rider-a"}}, tasks.started, "started")
}

const taskRunsPath = "/v1/tasks/runs"

// taskHistoryHandler builds a handler over three recorded attempts, newest
// first, of the two tasks it registers.
func taskHistoryHandler(t *testing.T) *Handler {
	t.Helper()

	return handlerOverTasks(t, taskHistoryFixture(), &fakeTasks{registered: []RegisteredTask{
		{Name: "sync:source"}, {Name: "sync:target"},
	}})
}

func taskHistoryFixture() *fakeState {
	endedAt := time.Date(2026, time.August, 31, 6, 30, 0, 0, time.UTC)

	return &fakeState{taskHistory: []recordedTaskRun{
		{
			task: "sync:source", trigger: "manual", reference: "aaaaaaaaaaaa",
			startedAt: endedAt.Add(-time.Minute), finishedAt: endedAt, outcome: "succeeded",
		},
		{
			task: "sync:target", argument: "rider-a", trigger: "schedule", reference: "bbbbbbbbbbbb",
			startedAt: endedAt.Add(-time.Hour), finishedAt: endedAt.Add(-time.Hour).Add(time.Minute),
			outcome: "failed", detail: "destination",
		},
		{
			task: "sync:source", trigger: "schedule", reference: "cccccccccccc",
			startedAt: endedAt.Add(-2 * time.Hour), finishedAt: endedAt.Add(-2 * time.Hour).Add(time.Minute),
			outcome: "unchanged",
		},
	}}
}

func taskRunPage(t *testing.T, handler http.Handler, query string) openapi.TaskRunPage {
	t.Helper()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, taskRunsPath+query))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var view openapi.TaskRunPage
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view), "decoding the task history")

	return view
}

// The history is read a page at a time, following the cursor the page before it
// ended with.
func TestGetTaskRunsServesTheHistoryOnePageAtATime(t *testing.T) {
	handler := taskHistoryHandler(t)

	first := taskRunPage(t, handler, "?limit=2")
	require.Len(t, first.Runs, 2, "the first page")
	assert.Equal(t, "sync:source", first.Runs[0].Task, "the newest attempt")
	assert.Equal(t, "manual", value(first.Runs[0].Trigger), "what started it")
	assert.Equal(t, "succeeded", first.Runs[0].Outcome, "outcome")
	assert.Equal(t, "aaaaaaaaaaaa", first.Runs[0].Reference, "reference")
	assert.Equal(t, "2026-08-31T06:30:00Z", wireInstant(first.Runs[0].FinishedAt), "finish")
	assert.Equal(t, "rider-a", value(first.Runs[1].Argument), "what the attempt was over")
	assert.Equal(t, "destination", value(first.Runs[1].Detail), "the reason it failed")
	require.NotEmpty(t, first.Next, "a cursor for the page after the first")

	second := taskRunPage(t, handler, "?limit=2&after="+*first.Next)
	require.Len(t, second.Runs, 1, "the page after the first")
	assert.Equal(t, "cccccccccccc", second.Runs[0].Reference, "the oldest attempt")
	assert.Empty(t, second.Next, "a cursor past the oldest recorded attempt")
}

// A filter narrows the feed to one task, which is how a page about one activity
// reads only its own attempts.
func TestGetTaskRunsNarrowsToOneTask(t *testing.T) {
	page := taskRunPage(t, taskHistoryHandler(t), "?task=sync%3Asource")

	require.Len(t, page.Runs, 2, "the named task's attempts")
	for _, run := range page.Runs {
		assert.Equal(t, "sync:source", run.Task, "an attempt of another task was served")
	}
}

// A name this build does not register is refused, so a page built against
// another build is told rather than shown a history that reads as empty.
func TestGetTaskRunsRefusesATaskThisBuildDoesNotRegister(t *testing.T) {
	response := httptest.NewRecorder()
	taskHistoryHandler(t).ServeHTTP(response, authenticatedRequest(http.MethodGet, taskRunsPath+"?task=invented"))

	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "invalid_request", "the error code")
}

// State this service cannot read is its own fault, and it says so rather than
// serving an empty history that would read as "nothing has run".
func TestGetTaskRunsReportsAHistoryItCannotRead(t *testing.T) {
	state := taskHistoryFixture()
	state.taskHistoryErr = errors.New("state is unavailable")
	handler := handlerOverTasks(t, state, &fakeTasks{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, taskRunsPath))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

// A cursor this service did not issue is the caller's mistake, and answering it
// with the newest page would silently restart the walk.
func TestGetTaskRunsRefusesACursorItDidNotIssue(t *testing.T) {
	response := httptest.NewRecorder()
	taskHistoryHandler(t).ServeHTTP(response, authenticatedRequest(http.MethodGet, taskRunsPath+"?after=the-newest-one"))

	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "invalid_request", "the error code")
}

// An attempt is served as the aggregate it was recorded as. Anything it touched
// — a route name, geometry, whatever an upstream said — stays out of the page.
func TestGetTaskRunsServesNothingAboutWhatAnAttemptTouched(t *testing.T) {
	response := httptest.NewRecorder()
	taskHistoryHandler(t).ServeHTTP(response, authenticatedRequest(http.MethodGet, taskRunsPath+"?limit=2"))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var page struct {
		Runs []map[string]any `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page), "decoding the task history")
	require.Len(t, page.Runs, 2, "the page")
	fields := make([]string, 0, len(page.Runs[1]))
	for field := range page.Runs[1] {
		fields = append(fields, field)
	}
	slices.Sort(fields)
	assert.Equal(t, []string{
		"argument", "detail", "finishedAt", "outcome", "reference", "startedAt", "task", "trigger",
	}, fields, "the fields a recorded attempt is served as")
}

// An empty history is a page with nothing in it, not a missing list.
func TestGetTaskRunsServesAnEmptyHistoryAsAnEmptyPage(t *testing.T) {
	page := taskRunPage(t, handlerOverTasks(t, &fakeState{}, &fakeTasks{}), "")

	assert.NotNil(t, page.Runs, "an empty page was sent as null")
	assert.Empty(t, page.Runs, "runs")
	assert.Empty(t, page.Next, "a cursor for a history with nothing in it")
}

// The page size is bounded so one request cannot read the whole retained window.
func TestGetTaskRunsRefusesAPageSizeItWillNotServe(t *testing.T) {
	handler := taskHistoryHandler(t)

	for _, limit := range []string{"0", "-1", "1000", "all"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, taskRunsPath+"?limit="+limit))
		assert.Equal(t, http.StatusBadRequest, response.Code, "status for limit="+limit)
	}
}

// Task administration is the whole service's: what is registered, what it has
// been doing, whether the schedule may start it, and starting anything other
// than a rider's own target sync.
func TestTaskAdministrationRefusesANonAdminSession(t *testing.T) {
	tests := map[string]struct {
		method, target, body string
	}{
		"list":     {http.MethodGet, tasksPath, ""},
		"runs":     {http.MethodGet, taskRunsPath, ""},
		"schedule": {http.MethodPut, "/v1/tasks/sync%3Asource/schedule", `{"enabled": false}`},
		"run":      {http.MethodPost, "/v1/tasks/sync%3Asource/run", ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncSource}}}
			handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

			request := authenticatedRequest(test.method, test.target)
			if test.body != "" {
				request = authenticatedRequestWithBody(test.method, test.target, test.body)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "forbidden")
			assert.Empty(t, tasks.asked, "the task layer was reached")
			assert.Empty(t, tasks.scheduled, "the schedule was reached")
		})
	}
}

func TestTaskAdministrationStillAnswersAnAdminSession(t *testing.T) {
	tests := map[string]struct {
		method, target, body string
		wantStatus           int
	}{
		"list":     {http.MethodGet, tasksPath, "", http.StatusOK},
		"runs":     {http.MethodGet, taskRunsPath, "", http.StatusOK},
		"schedule": {http.MethodPut, "/v1/tasks/sync%3Asource/schedule", `{"enabled": false}`, http.StatusOK},
		"run":      {http.MethodPost, "/v1/tasks/sync%3Asource/run", "", http.StatusAccepted},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler, _ := tasksHandler(t, RegisteredTask{Name: TaskSyncSource})

			request := authenticatedRequest(test.method, test.target)
			if test.body != "" {
				request = authenticatedRequestWithBody(test.method, test.target, test.body)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, test.wantStatus, response.Code, response.Body.String())
		})
	}
}

// The gate does not change what a rider's own target sync answers: a refused
// start is still a conflict rather than a refusal of the caller.
func TestRunTaskStillReportsAConflictToANonAdminOverTheirOwnTarget(t *testing.T) {
	tasks := &fakeTasks{registered: []RegisteredTask{{Name: TaskSyncTarget}}, refuse: true}
	handler := handlerFor(t, nonAdminSessions("rider-a"), &fakeOAuth{}, &fakeState{}, tasks)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		authenticatedRequest(http.MethodPost, "/v1/tasks/"+encodedTaskName(TaskSyncTarget)+"/run/rider-a"))

	assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
}
