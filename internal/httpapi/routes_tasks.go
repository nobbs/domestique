package httpapi

import (
	"net/http"
	"slices"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// TaskSync is the registered name of one whole synchronization. It is named
// here because a task name is part of what this surface publishes: the page
// asks for a task rather than for a route, and so does the reprocess request
// below, which is a synchronization asked for on a stage's behalf.
const TaskSync = "sync"

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
	registered := h.tasks.Registered()
	tasks := make([]openapi.Task, 0, len(registered))
	for _, task := range registered {
		tasks = append(tasks, openapi.Task{
			Name:      task.Name,
			Scheduled: task.Scheduled,
			Running:   task.Running,
			NextRunAt: optionalTime(task.NextRunAt),
		})
	}
	h.writeJSON(writer, http.StatusOK, openapi.TaskList{Tasks: tasks})
}

// RunTask starts one attempt of a named task, over an argument when the path
// carries one. A name this build does not register is not found, so a page
// built against another build asks for nothing that silently does nothing.
func (h *Handler) RunTask(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	if !slices.ContainsFunc(h.tasks.Registered(), func(task RegisteredTask) bool { return task.Name == name }) {
		h.notFound(writer)

		return
	}
	argument := request.PathValue("argument")
	h.accepted(writer, func() bool { return h.tasks.Run(name, argument) })
}
