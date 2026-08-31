package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	handler, err := New(
		&Options{
			Settings:         settingsWith(testBasemaps()),
			Alerts:           &fakeAlerts{},
			Tasks:            tasks,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{},
	)
	require.NoError(t, err, "New()")

	return handler, tasks
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
	request.Header.Set(assertionHeader, testAssertion)

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
