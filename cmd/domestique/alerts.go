package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/task"
)

// alertDecisions answers what an operator has ruled about each alert, from a
// snapshot it refreshes when the decisions are written. A run that cannot read
// them announces what it would have announced anyway: a fault nobody has heard
// of is the one worth hearing about.
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
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	enabled, decided = d.decided[alertKey{task: taskName, alert: string(alert)}]

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
	if err := d.reload(ctx); err != nil {
		slog.Warn("alert decisions not reloaded after a write", "error", err)
	}

	return nil
}
