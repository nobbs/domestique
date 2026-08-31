package main

import (
	"context"
	"sync"

	"github.com/nobbs/domestique/internal/httpapi"
)

// demoAlerts is an alert matrix held in memory, standing in for the catalogue
// the shipped binary builds from its registered tasks.
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

// syncFailureAlerts is what a synchronization can fail with, shared by every
// sync task's slice of the catalogue below.
var syncFailureAlerts = []string{ //nolint:gochecknoglobals // a fixture for development tooling
	"state", "source", "authorization", "destination", "course", "empty_source", "deletion_limit",
}

// demoAlertCatalogue mirrors what the shipped binary's tasks declare, so the
// settings section looks like the one an operator meets.
var demoAlertCatalogue = buildDemoAlertCatalogue() //nolint:gochecknoglobals // a fixture for development tooling

func buildDemoAlertCatalogue() []demoAlert {
	catalogue := make([]demoAlert, 0, 3*(len(syncFailureAlerts)+2)+1+2)
	for _, task := range []string{"sync:source", "sync:target", "sync:clear"} {
		catalogue = append(catalogue, demoAlert{task: task, alert: "succeeded"}, demoAlert{task: task, alert: "recovered"})
		if task == "sync:source" {
			catalogue = append(catalogue, demoAlert{task: task, alert: "stale"})
		}
		for _, alert := range syncFailureAlerts {
			catalogue = append(catalogue, demoAlert{task: task, alert: alert})
		}
	}
	catalogue = append(catalogue,
		demoAlert{task: "surface:index", alert: "build"}, demoAlert{task: "surface:index", alert: "no_regions"})

	return catalogue
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
