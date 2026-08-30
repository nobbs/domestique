package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Manager owns every registered task: it runs them on their schedules, keeps
// them off each other's resources, and waits for them at shutdown.
type Manager struct {
	now   func() time.Time
	after func(time.Duration) <-chan time.Time

	shared    map[string]int
	exclusive map[string]struct{}
	tasks     map[string]*registered
	order     []string

	// mutex guards admission alone: a slot and a resource set are taken
	// together or not at all, so an attempt can never hold one and want the
	// other.
	mutex     sync.Mutex
	triggered sync.WaitGroup
}

// registered is one task and what the manager knows about it right now.
type registered struct {
	// startsAt holds the instant the first scheduled run is due, and only while
	// it is still due.
	startsAt   startsAt
	definition Definition
	inFlight   int
}

// NewManager creates an empty manager. Nothing runs until Run is called.
func NewManager() *Manager {
	return &Manager{
		now:       time.Now,
		after:     time.After,
		shared:    make(map[string]int),
		exclusive: make(map[string]struct{}),
		tasks:     make(map[string]*registered),
	}
}

// Register adds a task. Registration order is the order Run starts schedules in.
func (m *Manager) Register(definition Definition) error {
	if definition.Name == "" || definition.Run == nil {
		return errors.New("task: a name and a runner are required")
	}
	if definition.Schedule != nil && definition.InitialDelay == nil {
		return fmt.Errorf("task %q: a scheduled task needs an initial delay", definition.Name)
	}
	if _, exists := m.tasks[definition.Name]; exists {
		return fmt.Errorf("task %q is already registered", definition.Name)
	}
	m.tasks[definition.Name] = &registered{definition: definition}
	m.order = append(m.order, definition.Name)

	return nil
}

// Run starts every scheduled task and returns once ctx is done and each has
// stopped. Tasks without a schedule wait for a trigger instead.
func (m *Manager) Run(ctx context.Context) {
	var scheduled sync.WaitGroup
	for _, name := range m.order {
		entry := m.tasks[name]
		if entry.definition.Schedule == nil {
			continue
		}
		scheduled.Go(func() { m.follow(ctx, entry) })
	}
	scheduled.Wait()
}

// Trigger starts one attempt in the background, reporting whether it was
// accepted. An accepted attempt outlives whatever asked for it, and Wait is
// what waits for it.
func (m *Manager) Trigger(ctx context.Context, name, argument string) bool {
	entry, known := m.tasks[name]
	if !known {
		return false
	}
	release, admitted := m.admit(entry, argument)
	if !admitted {
		return false
	}
	m.triggered.Go(func() {
		defer release()
		m.attempt(ctx, entry, Invocation{Task: name, Argument: argument, Trigger: TriggerManual})
	})

	return true
}

// Wait waits for every accepted trigger to finish.
func (m *Manager) Wait() {
	m.triggered.Wait()
}

// NextRunAt reports when a task's first scheduled run is being held until,
// while it is still being held. Only the initial delay answers: the gap between
// runs is the task's cadence rather than work held back.
func (m *Manager) NextRunAt(name string) (time.Time, bool) {
	entry, known := m.tasks[name]
	if !known {
		return time.Time{}, false
	}

	return entry.startsAt.load()
}

// Holding reports whether any attempt currently holds the named resource, which
// is what makes "something is working on this state" answerable from outside.
func (m *Manager) Holding(resource string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, taken := m.exclusive[resource]; taken {
		return true
	}

	return m.shared[resource] > 0
}

// follow waits out the initial delay and then runs the task on its schedule
// until ctx is done. It never starts concurrent work of its own.
func (m *Manager) follow(ctx context.Context, entry *registered) {
	delay := entry.definition.InitialDelay()
	due := m.now().UTC().Add(delay)
	entry.startsAt.store(due)
	started := wait(ctx, m.after(delay))
	entry.startsAt.clear()
	if !started {
		return
	}
	m.scheduled(ctx, entry)

	for {
		next, scheduled := nextDue(entry.definition.Schedule, due, m.now().UTC())
		if !scheduled {
			return
		}
		if !wait(ctx, m.after(next.Sub(m.now().UTC()))) {
			return
		}
		due = next
		m.scheduled(ctx, entry)
	}
}

// scheduled performs one attempt from the schedule, which is refused on exactly
// the terms a trigger is.
func (m *Manager) scheduled(ctx context.Context, entry *registered) {
	release, admitted := m.admit(entry, "")
	if !admitted {
		return
	}
	defer release()
	m.attempt(ctx, entry, Invocation{Task: entry.definition.Name, Trigger: TriggerSchedule})
}

// attempt runs the task and reports what it came to. Shutdown outranks whatever
// the runner returned: an attempt it ended did not finish.
func (m *Manager) attempt(ctx context.Context, entry *registered, invocation Invocation) Result {
	result := entry.definition.Run.Run(ctx, invocation)
	if ctx.Err() != nil {
		return Result{Outcome: Cancelled}
	}

	return result
}

// admit takes a concurrency slot and the whole resource set together, returning
// what releases them. Nothing is taken unless all of it is available, so an
// attempt can never wait while holding part of what another one needs.
func (m *Manager) admit(entry *registered, argument string) (func(), bool) {
	wanted := merge(entry.definition.resources(argument))

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if entry.inFlight >= entry.definition.limit() || !m.available(wanted) {
		return nil, false
	}
	for _, resource := range wanted {
		if resource.Exclusive {
			m.exclusive[resource.Name] = struct{}{}

			continue
		}
		m.shared[resource.Name]++
	}
	entry.inFlight++

	return func() { m.release(entry, wanted) }, true
}

// available reports whether every wanted resource is free to take.
func (m *Manager) available(wanted []Resource) bool {
	for _, resource := range wanted {
		if _, taken := m.exclusive[resource.Name]; taken {
			return false
		}
		if resource.Exclusive && m.shared[resource.Name] > 0 {
			return false
		}
	}

	return true
}

func (m *Manager) release(entry *registered, wanted []Resource) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, resource := range wanted {
		if resource.Exclusive {
			delete(m.exclusive, resource.Name)

			continue
		}
		m.shared[resource.Name]--
		if m.shared[resource.Name] <= 0 {
			delete(m.shared, resource.Name)
		}
	}
	entry.inFlight--
}

// merge folds a wanted set so each name appears once and exclusive wins, which
// keeps a task that names one resource twice from taking it twice.
func merge(wanted []Resource) []Resource {
	if len(wanted) < 2 {
		return wanted
	}

	merged := make([]Resource, 0, len(wanted))
	at := make(map[string]int, len(wanted))
	for _, resource := range wanted {
		index, seen := at[resource.Name]
		if !seen {
			at[resource.Name] = len(merged)
			merged = append(merged, resource)

			continue
		}
		merged[index].Exclusive = merged[index].Exclusive || resource.Exclusive
	}

	return merged
}

func wait(ctx context.Context, signal <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-signal:
		return true
	}
}

// startsAt is when a first scheduled run is due, and holds a value only while
// it is still due. Each store keeps its own copy rather than rewriting a value
// a reader may already hold.
type startsAt struct {
	due atomic.Pointer[time.Time]
}

func (s *startsAt) store(due time.Time) { s.due.Store(&due) }
func (s *startsAt) clear()              { s.due.Store(nil) }

func (s *startsAt) load() (time.Time, bool) {
	due := s.due.Load()
	if due == nil {
		return time.Time{}, false
	}

	return *due, true
}
