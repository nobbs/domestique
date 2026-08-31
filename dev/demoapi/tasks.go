package main

import (
	"context"
	"sync"

	"github.com/nobbs/domestique/internal/httpapi"
)

// demoTasks is the task list a demo shows. The shipped binary builds one from a
// running task layer; a demo has no layer, so this answers with the names an
// operator meets and nothing about a schedule. Running one of the
// synchronization tasks reseeds the library, which is the only work a demo does.
type demoTasks struct {
	reseed func() bool
	// switched is what an operator has turned off here. A demo keeps it in
	// memory rather than in the database: the page has to see its own edit come
	// back, or the switch reads as broken, and nothing in a demo is scheduled
	// for it to govern anyway.
	switched map[string]bool
	mutex    *sync.RWMutex
}

func newDemoTasks(reseed func() bool) demoTasks {
	return demoTasks{reseed: reseed, switched: make(map[string]bool), mutex: &sync.RWMutex{}}
}

// demoTaskNames are what the shipped binary registers, in the order it does,
// with whether each has a schedule there. A demo runs none of them on a clock,
// but reporting them all as scheduled would draw a switch for the two that an
// operator can only ask for.
var demoTaskNames = []struct { //nolint:gochecknoglobals // a fixture for development tooling
	name      string
	scheduled bool
}{
	{name: httpapi.TaskSyncSource, scheduled: true},
	{name: "sync:target", scheduled: true},
	{name: "sync:clear"},
	{name: "surface:annotate"},
	{name: "surface:index", scheduled: true},
}

// Registered lists the demo's tasks. They read as scheduled and enabled, which
// is what the shipped binary's are: the page draws a switch per task, and one
// with nothing to draw is a page that looks broken rather than empty.
func (t demoTasks) Registered() []httpapi.RegisteredTask {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	tasks := make([]httpapi.RegisteredTask, 0, len(demoTaskNames))
	for _, task := range demoTaskNames {
		enabled, ruled := t.switched[task.name]
		tasks = append(tasks, httpapi.RegisteredTask{
			Name: task.name, Scheduled: task.scheduled, Enabled: !ruled || enabled,
		})
	}

	return tasks
}

// Schedule remembers a switch for as long as the demo runs, so the page sees
// its own edit come back.
func (t demoTasks) Schedule(_ context.Context, name string, enabled bool) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.switched[name] = enabled

	return nil
}

// Run reseeds for the synchronization tasks and accepts the rest without work:
// a demo has no upstream to reach, no target to write, and no map to index.
func (t demoTasks) Run(name, _ string) bool {
	switch name {
	case httpapi.TaskSyncSource, "sync:target", "sync:clear":
		return t.reseed()
	default:
		return true
	}
}
