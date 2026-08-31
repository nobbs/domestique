package httpapi

import (
	"net/http"
	"slices"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// taskInProgress is what a refused attempt is told. A task refuses for one of
// two reasons — this exact work is already happening, or something else holds
// what it needs — and neither is a fault, so both read the same from here.
const taskInProgress = "the task is already running, or something it needs is held by another run"

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
	h.accepted(writer, taskInProgress, func() bool {
		return h.tasks.Run(request.Context(), name, argument)
	})
}
