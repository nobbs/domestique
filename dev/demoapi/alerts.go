package main

import (
	"context"
	"sync"

	"github.com/nobbs/domestique/internal/httpapi"
)

// demoAlerts is an alert matrix held in memory. The shipped binary builds its
// catalogue from the tasks it registers; a demo registers none, so this stands
// in a plausible one and remembers what is decided about it for as long as the
// process runs.
type demoAlerts struct {
	decided map[httpapi.AlertDecision]bool
	mutex   sync.RWMutex
}

// demoAlertCatalogue is what the shipped binary's tasks declare, which is what
// makes the settings section look like the one an operator meets.
var demoAlertCatalogue = []httpapi.AlertDecision{ //nolint:gochecknoglobals // a fixture for development tooling
	{Task: "sync", Alert: "state"},
	{Task: "sync", Alert: "source"},
	{Task: "sync", Alert: "authorization"},
	{Task: "sync", Alert: "destination"},
	{Task: "sync", Alert: "course"},
	{Task: "sync", Alert: "empty_source"},
	{Task: "sync", Alert: "deletion_limit"},
	{Task: "surface:index", Alert: "build"},
	{Task: "surface:index", Alert: "no_regions"},
}

func newDemoAlerts() *demoAlerts {
	return &demoAlerts{decided: make(map[httpapi.AlertDecision]bool)}
}

// Catalogue lists the demo's alerts and what has been decided about each.
func (a *demoAlerts) Catalogue() []httpapi.AlertSetting {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	catalogue := make([]httpapi.AlertSetting, 0, len(demoAlertCatalogue))
	for _, alert := range demoAlertCatalogue {
		enabled, decided := a.decided[alert]
		catalogue = append(catalogue, httpapi.AlertSetting{
			Task:    alert.Task,
			Alert:   alert.Alert,
			Enabled: !decided || enabled,
			Decided: decided,
		})
	}

	return catalogue
}

// Decide remembers what was decided.
func (a *demoAlerts) Decide(_ context.Context, decisions []httpapi.AlertDecision) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	for _, decision := range decisions {
		a.decided[httpapi.AlertDecision{Task: decision.Task, Alert: decision.Alert}] = decision.Enabled
	}

	return nil
}
