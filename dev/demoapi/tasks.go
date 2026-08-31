package main

import "github.com/nobbs/domestique/internal/httpapi"

// demoTasks is the task list a demo shows. The shipped binary builds one from
// what it registers; a demo registers none, so this stands in the names an
// operator meets and accepts a run of any of them without doing work — the
// reseeder behind the sync page is what a demo actually runs.
type demoTasks struct{}

// demoTaskNames are what the shipped binary registers, in the order it does.
var demoTaskNames = []string{ //nolint:gochecknoglobals // a fixture for development tooling
	"sync", "sync:target", "sync:clear", "surface:annotate", "surface:index",
}

// Registered lists the demo's tasks, none of them scheduled: a demo has no
// clock worth showing and nothing that runs unasked.
func (demoTasks) Registered() []httpapi.RegisteredTask {
	tasks := make([]httpapi.RegisteredTask, 0, len(demoTaskNames))
	for _, name := range demoTaskNames {
		tasks = append(tasks, httpapi.RegisteredTask{Name: name})
	}

	return tasks
}

// Run accepts any registered name. Nothing happens: a demo has no upstream to
// reach and no target to write.
func (demoTasks) Run(string, string) bool { return true }
