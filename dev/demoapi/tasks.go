package main

import "github.com/nobbs/domestique/internal/httpapi"

// demoTasks is the task list a demo shows. The shipped binary builds one from a
// running task layer; a demo has no layer, so this answers with the names an
// operator meets and nothing about a schedule. Running one of the
// synchronization tasks reseeds the library, which is the only work a demo does.
type demoTasks struct{ reseed func() bool }

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

// Run reseeds for the synchronization tasks and accepts the rest without work:
// a demo has no upstream to reach, no target to write, and no map to index.
func (t demoTasks) Run(name, _ string) bool {
	switch name {
	case "sync", "sync:target", "sync:clear":
		return t.reseed()
	default:
		return true
	}
}
