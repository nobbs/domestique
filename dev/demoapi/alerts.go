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
	decided map[demoAlert]bool
	mutex   sync.RWMutex
}

// demoAlert identifies one alert. It is what an alert is rather than what was
// decided about it, so a lookup cannot turn on a decision.
type demoAlert struct {
	task  string
	alert string
}

// demoAlertCatalogue is what the shipped binary's tasks declare, which is what
// makes the settings section look like the one an operator meets.
var demoAlertCatalogue = []demoAlert{ //nolint:gochecknoglobals // a fixture for development tooling
	{task: "sync", alert: "state"},
	{task: "sync", alert: "source"},
	{task: "sync", alert: "authorization"},
	{task: "sync", alert: "destination"},
	{task: "sync", alert: "course"},
	{task: "sync", alert: "empty_source"},
	{task: "sync", alert: "deletion_limit"},
	{task: "surface:index", alert: "build"},
	{task: "surface:index", alert: "no_regions"},
}

func newDemoAlerts() *demoAlerts {
	return &demoAlerts{decided: make(map[demoAlert]bool)}
}

// Catalogue lists the demo's alerts and what has been decided about each.
func (a *demoAlerts) Catalogue() []httpapi.AlertSetting {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	catalogue := make([]httpapi.AlertSetting, 0, len(demoAlertCatalogue))
	for _, alert := range demoAlertCatalogue {
		enabled, decided := a.decided[alert]
		catalogue = append(catalogue, httpapi.AlertSetting{
			Task:    alert.task,
			Alert:   alert.alert,
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
		a.decided[demoAlert{task: decision.Task, alert: decision.Alert}] = decision.Enabled
	}

	return nil
}
