package main

import (
	"context"
	"sync"
	"time"

	"github.com/nobbs/domestique/internal/httpapi"
	syncservice "github.com/nobbs/domestique/internal/sync"
)

// targetBackstopInterval mirrors cmd/domestique/tasks.go's constant of the
// same name: the demo's task list is read by the same page.
const targetBackstopInterval = 6 * time.Hour

// demoTasks stands in for a running task layer: no schedule, and running a
// synchronization task just reseeds the library.
type demoTasks struct {
	reseed func() bool
	// switched is kept in memory, not the database: the page must see its own
	// edit come back, and nothing in a demo is scheduled for it to govern anyway.
	switched map[string]bool
	mutex    *sync.RWMutex
}

func newDemoTasks(reseed func() bool) demoTasks {
	return demoTasks{reseed: reseed, switched: make(map[string]bool), mutex: &sync.RWMutex{}}
}

// demoTaskNames mirrors what the shipped binary registers, in order, with
// whether each has a schedule there — the two without one are ask-only.
var demoTaskNames = []struct { //nolint:gochecknoglobals // a fixture for development tooling
	name      string
	scheduled bool
	interval  time.Duration
}{
	{name: httpapi.TaskSyncSource, scheduled: true, interval: syncservice.Interval},
	{name: "sync:target", scheduled: true, interval: targetBackstopInterval},
	{name: "sync:clear"},
	{name: "surface:annotate"},
	{name: httpapi.TaskRideModelPredict},
	{name: httpapi.TaskSurfaceIndex, scheduled: true},
}

// Registered lists the demo's tasks, so the page has a switch to draw per task
// instead of looking broken and empty.
func (t demoTasks) Registered() []httpapi.RegisteredTask {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	tasks := make([]httpapi.RegisteredTask, 0, len(demoTaskNames))
	for _, task := range demoTaskNames {
		enabled, ruled := t.switched[task.name]
		tasks = append(tasks, httpapi.RegisteredTask{
			Name: task.name, Scheduled: task.scheduled, Enabled: !ruled || enabled, Interval: task.interval,
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
