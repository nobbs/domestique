package main

import (
	"context"
	"errors"
	"maps"
	"sync"

	"github.com/nobbs/domestique/internal/sqlite"
)

// taskSwitches answers whether the schedule may start a task, from a snapshot
// it refreshes when a switch is written. Reading them is part of starting: a
// service that could not read them would run what an operator had deliberately
// switched off.
type taskSwitches struct {
	store   *sqlite.Store
	enabled map[string]bool
	mutex   sync.RWMutex
}

func newTaskSwitches(ctx context.Context, store *sqlite.Store) (*taskSwitches, error) {
	switches := &taskSwitches{store: store}
	if err := switches.reload(ctx); err != nil {
		return nil, err
	}

	return switches, nil
}

// enabledFor is what a definition reads at each tick. A task nobody has ruled
// on runs, so one added to a build reaches its schedule without being turned on.
func (s *taskSwitches) enabledFor(task string) func() bool {
	return func() bool {
		if s == nil {
			return true
		}
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		enabled, decided := s.enabled[task]

		return !decided || enabled
	}
}

// snapshot is what has been decided, for the surface that offers the switches.
// A nil set is a service where nothing has been ruled on, which is what every
// task starts as.
func (s *taskSwitches) snapshot() map[string]bool {
	if s == nil {
		return nil
	}
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return maps.Clone(s.enabled)
}

func (s *taskSwitches) reload(ctx context.Context) error {
	enabled, err := s.store.TaskSchedule(ctx)
	if err != nil {
		return err //nolint:wrapcheck // the store already names what it was reading
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.enabled = enabled

	return nil
}

// Set records a switch and refreshes the snapshot behind it. A decision the
// running service has not read is one an operator believes they made.
func (s *taskSwitches) Set(ctx context.Context, task string, enabled bool) error {
	if s == nil {
		return errors.New("this service holds no task schedule to write")
	}
	if err := s.store.SetTaskSchedule(ctx, task, enabled); err != nil {
		return err //nolint:wrapcheck // the store already names what it was writing
	}

	return s.reload(ctx)
}
