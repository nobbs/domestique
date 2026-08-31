package httpapi

import (
	"net/http"
	"slices"

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
