package httpapi

import (
	"net/http"
	"slices"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// TaskSyncSource is the registered name of the task that reads the source
// libraries, also used by the reprocess route in routes_library.go.
const TaskSyncSource = "sync:source"

const (
	// codeTaskInProgress is what a refused attempt is told.
	codeTaskInProgress = "task_in_progress"
	// taskInProgress is why. A task refuses for one of two reasons — this exact
	// work is already happening, or something else holds what it needs — and
	// neither is a fault, so both read the same from here.
	taskInProgress = "the task is already running, or something it needs is held by another run"
)

// accepted reports what an attempt came to: 202 when the layer took the work,
// 409 when it refused.
func (h *Handler) accepted(writer http.ResponseWriter, start func() bool) {
	if !start() {
		h.error(writer, http.StatusConflict, codeTaskInProgress, taskInProgress)

		return
	}
	h.writeJSON(writer, http.StatusAccepted, openapi.Accepted{Status: "accepted"})
}

// ListTasks reports every background activity this build registers.
func (h *Handler) ListTasks(writer http.ResponseWriter, _ *http.Request) {
	h.writeJSON(writer, http.StatusOK, h.taskList())
}

// taskList is what this build registers, as the surface reports it.
func (h *Handler) taskList() openapi.TaskList {
	registered := h.tasks.Registered()
	tasks := make([]openapi.Task, 0, len(registered))
	for _, task := range registered {
		tasks = append(tasks, openapi.Task{
			Name:            task.Name,
			Scheduled:       task.Scheduled,
			Enabled:         task.Enabled,
			Running:         task.Running,
			IntervalSeconds: optionalIntervalSeconds(task.Interval),
			NextRunAt:       optionalTime(task.NextRunAt),
		})
	}
	return openapi.TaskList{Tasks: tasks}
}

// SetTaskSchedule sets whether the schedule may start one task, and answers
// with the list as it now stands.
func (h *Handler) SetTaskSchedule(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	if !h.registers(name) {
		h.notFound(writer)

		return
	}
	body, ok := settingsBody[openapi.TaskScheduleUpdate](h, writer, request)
	if !ok {
		return
	}
	if err := h.tasks.Schedule(request.Context(), name, body.Enabled); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, h.taskList())
}

// registers reports whether this build has a task of that name. A name it does
// not is refused, so a page built against another build asks for nothing that
// silently does nothing.
func (h *Handler) registers(name string) bool {
	return slices.ContainsFunc(h.tasks.Registered(), func(task RegisteredTask) bool {
		return task.Name == name
	})
}

// RunTask starts one attempt of a named task, over an argument when the path
// carries one.
func (h *Handler) RunTask(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	if !h.registers(name) {
		h.notFound(writer)

		return
	}
	argument := request.PathValue("argument")
	h.accepted(writer, func() bool { return h.tasks.Run(name, argument) })
}

// GetTaskRuns serves one page of what the background activities have been doing,
// newest first, with the cursor for the next. Local records only: the aggregate
// each attempt was recorded as, and nothing about what it touched.
func (h *Handler) GetTaskRuns(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	// A name this build does not register is refused rather than answered with an
	// empty page, which reads as a task that has never run.
	if task := query.Get("task"); task != "" && !h.registers(task) {
		h.error(writer, http.StatusBadRequest, "invalid_request", "no task of that name is registered")

		return
	}
	limit, ok := h.pageLimit(writer, query)
	if !ok {
		return
	}
	// An empty history is an empty list rather than a null one: the page is the
	// answer either way.
	view := openapi.TaskRunPage{Runs: []openapi.TaskRun{}}
	next, usable, err := h.state.ForEachTaskRunPage(
		request.Context(), query.Get("task"), query.Get("after"), limit,
		func(
			task, argument, trigger string, startedAt, finishedAt time.Time, outcome, detail, reference string,
		) error {
			view.Runs = append(view.Runs, openapi.TaskRun{
				Task:       task,
				Argument:   optionalString(argument),
				Trigger:    optionalString(trigger),
				StartedAt:  wireTime(startedAt),
				FinishedAt: wireTime(finishedAt),
				Outcome:    outcome,
				Detail:     optionalString(detail),
				Reference:  reference,
			})

			return nil
		})
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !usable {
		h.error(writer, http.StatusBadRequest, "invalid_request",
			"the history cursor is not one this service issued")

		return
	}
	view.Next = optionalString(next)
	h.writeJSON(writer, http.StatusOK, view)
}
