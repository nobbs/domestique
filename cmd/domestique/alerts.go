package main

import (
	"context"
	"sync"

	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/task"
)

// alertDecisions is an in-memory snapshot of what an operator has ruled about
// each alert, refreshed whenever a decision is written.
type alertDecisions struct {
	store   *sqlite.Store
	decided map[alertKey]bool
	mutex   sync.RWMutex
}

type alertKey struct {
	task  string
	alert string
}

func newAlertDecisions(ctx context.Context, store *sqlite.Store) (*alertDecisions, error) {
	decisions := &alertDecisions{store: store}
	if err := decisions.reload(ctx); err != nil {
		return nil, err
	}

	return decisions, nil
}

// Wanted reports what was decided about one alert, and whether anything was.
func (d *alertDecisions) Wanted(
	_ context.Context, taskName string, alert task.Detail,
) (enabled, decided bool) {
	return d.lookup(taskName, string(alert))
}

// lookup answers the same question without a context, for the callers that
// have none.
func (d *alertDecisions) lookup(taskName, alert string) (enabled, decided bool) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	enabled, decided = d.decided[alertKey{task: taskName, alert: alert}]

	return enabled, decided
}

// reload replaces the snapshot with what is stored.
func (d *alertDecisions) reload(ctx context.Context) error {
	toggles, err := d.store.AlertToggles(ctx)
	if err != nil {
		return err //nolint:wrapcheck // the store already names what it was reading
	}

	decided := make(map[alertKey]bool, len(toggles))
	for _, toggle := range toggles {
		// A decision about one scope is not a decision about the task. Nothing
		// scopes its alerts yet, so a scoped row is somebody else's to read.
		if toggle.Scope != "" {
			continue
		}
		decided[alertKey{task: toggle.Task, alert: toggle.Alert}] = toggle.Enabled
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.decided = decided

	return nil
}

// Set records what an operator decided and refreshes the snapshot behind it.
func (d *alertDecisions) Set(ctx context.Context, toggles []sqlite.AlertToggle) error {
	if err := d.store.SetAlertToggles(ctx, toggles); err != nil {
		return err //nolint:wrapcheck // the store already names what it was writing
	}
	// Reload before reporting success, or the running service would keep
	// sending the alert an operator just switched off.
	return d.reload(ctx)
}

// alertMatrix crosses what the registered tasks declare against what an
// operator has decided about each alert.
type alertMatrix struct {
	decisions    *alertDecisions
	declarations []task.Declaration
}

// Catalogue lists every alert the registered tasks can raise. An alert nobody
// has ruled on reads as enabled.
func (m alertMatrix) Catalogue() []httpapi.AlertSetting {
	catalogue := make([]httpapi.AlertSetting, 0, len(m.declarations))
	for _, declaration := range m.declarations {
		enabled, decided := m.decisions.lookup(declaration.Task, string(declaration.Alert))
		catalogue = append(catalogue, httpapi.AlertSetting{
			Task:    declaration.Task,
			Alert:   string(declaration.Alert),
			Enabled: !decided || enabled,
			Decided: decided,
		})
	}

	return catalogue
}

// Decide records what an operator decided about the alerts they ruled on.
func (m alertMatrix) Decide(ctx context.Context, decisions []httpapi.AlertDecision) error {
	toggles := make([]sqlite.AlertToggle, 0, len(decisions))
	for _, decision := range decisions {
		toggles = append(toggles, sqlite.AlertToggle{
			Task:    decision.Task,
			Alert:   decision.Alert,
			Enabled: decision.Enabled,
		})
	}

	return m.decisions.Set(ctx, toggles)
}
