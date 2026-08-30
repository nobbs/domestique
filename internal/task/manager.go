package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Store records what attempts came to. It is described here rather than taken
// as a concrete store so this package carries no storage concern, and it deals
// in strings so the store carries none of this package's vocabulary.
type Store interface {
	// RecordTaskRun writes one attempt down and prunes that task's history back
	// to retain, keeping the most recent attempt over each argument whatever
	// its age.
	RecordTaskRun(
		ctx context.Context,
		task, argument string,
		startedAt, finishedAt time.Time,
		outcome, detail string,
		retain int,
	) error
}

// Manager owns every registered task: it runs them on their schedules, keeps
// them off each other's resources, and waits for them at shutdown.
type Manager struct {
	now   func() time.Time
	after func(time.Duration) <-chan time.Time
	store Store

	shared    map[string]int
	exclusive map[string]struct{}
	running   map[string]struct{}
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

// NewManager creates an empty manager over the store its attempts are recorded
// in. Nothing runs until Run is called.
func NewManager(store Store) (*Manager, error) {
	if store == nil {
		return nil, errors.New("task: a run store is required")
	}

	return &Manager{
		now:       time.Now,
		after:     time.After,
		store:     store,
		shared:    make(map[string]int),
		exclusive: make(map[string]struct{}),
		running:   make(map[string]struct{}),
		tasks:     make(map[string]*registered),
	}, nil
}

// Register adds a task. Registration order is the order Run starts schedules in.
func (m *Manager) Register(definition *Definition) error {
	if definition == nil {
		return errors.New("task: a definition is required")
	}
	if definition.Name == "" || definition.Run == nil {
		return errors.New("task: a name and a runner are required")
	}
	if definition.Schedule != nil && definition.InitialDelay == nil {
		return fmt.Errorf("task %q: a scheduled task needs an initial delay", definition.Name)
	}
	if _, exists := m.tasks[definition.Name]; exists {
		return fmt.Errorf("task %q is already registered", definition.Name)
	}
	m.tasks[definition.Name] = &registered{definition: *definition}
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
// accepted. An accepted attempt outlives this call, is bounded by ctx, and is
// what Wait waits for. A service already shutting down accepts nothing.
func (m *Manager) Trigger(ctx context.Context, name, argument string) bool {
	entry, known := m.tasks[name]
	if !known || ctx.Err() != nil {
		return false
	}
	invocation := Invocation{Task: name, Argument: argument, Trigger: TriggerManual}
	release, outcome := m.admit(entry, invocation)
	if outcome != admitStarted {
		m.refused(ctx, entry, invocation)

		return false
	}
	m.triggered.Go(func() { m.perform(ctx, entry, invocation, release, nil, 0) })

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
// the terms a trigger is. A wait that ended as the context was cancelled can
// still report that it fired, so cancellation is checked before starting work
// rather than left to the runner to notice.
func (m *Manager) scheduled(ctx context.Context, entry *registered) {
	if ctx.Err() != nil {
		return
	}
	invocation := Invocation{Task: entry.definition.Name, Trigger: TriggerSchedule}
	release, outcome := m.admit(entry, invocation)
	if outcome != admitStarted {
		m.refused(ctx, entry, invocation)

		return
	}
	m.perform(ctx, entry, invocation, release, nil, 0)
}

// perform runs one attempt and then whatever it asked should follow. The
// resources are released before the chain starts, because a link wanting what
// its parent held would otherwise be refused by it every time.
//
// One chain shares one set of what it has run. A chain is sequential, so
// siblings see each other rather than each starting from its parent's copy.
func (m *Manager) perform(
	ctx context.Context,
	entry *registered,
	invocation Invocation,
	release func(),
	visited map[string]struct{},
	depth int,
) {
	if visited == nil {
		visited = make(map[string]struct{}, 1)
	}
	visited[invocationKey(invocation)] = struct{}{}

	result := m.attemptAndRelease(ctx, entry, invocation, release)
	if len(result.Next) == 0 {
		return
	}
	m.chain(ctx, result.Next, visited, depth+1)
}

// attemptAndRelease runs one attempt and gives back what it held, whatever
// becomes of the runner. Releasing before the chain is what lets a link take
// what its parent was holding.
func (m *Manager) attemptAndRelease(
	ctx context.Context, entry *registered, invocation Invocation, release func(),
) Result {
	defer release()

	return m.attempt(ctx, entry, invocation)
}

// chain runs what one attempt asked should follow it. Links are chosen at run
// time, so nothing can reject a cycle at registration: the set of what this
// chain has already run is carried down it, and the depth is capped behind that.
func (m *Manager) chain(ctx context.Context, links []Link, visited map[string]struct{}, depth int) {
	if depth >= maxChainDepth {
		slog.Warn("task chain truncated", "depth", depth)

		return
	}
	for _, link := range links {
		m.linked(ctx, link, visited, depth)
	}
}

// linked runs one chain link. Work already under way is left to finish rather
// than refused: a link asking for what is happening anyway has its answer, and
// admission is what decides that, so nothing can change between asking and
// starting.
func (m *Manager) linked(ctx context.Context, link Link, visited map[string]struct{}, depth int) {
	entry, known := m.tasks[link.Task]
	if !known || ctx.Err() != nil {
		return
	}
	invocation := Invocation{Task: link.Task, Argument: link.Argument, Trigger: TriggerChain}
	if _, seen := visited[invocationKey(invocation)]; seen {
		slog.Warn("task chain asked again for what it had already run", "task", link.Task)

		return
	}
	release, outcome := m.admit(entry, invocation)
	switch outcome {
	case admitWorking:
		return
	case admitHeld:
		m.refused(ctx, entry, invocation)

		return
	case admitStarted:
	}
	m.perform(ctx, entry, invocation, release, visited, depth)
}

// attempt runs the task, records what it came to, and reports it. Shutdown
// outranks whatever the runner returned: an attempt it ended did not finish.
func (m *Manager) attempt(ctx context.Context, entry *registered, invocation Invocation) Result {
	startedAt := m.now().UTC()
	result := entry.definition.Run.Run(ctx, invocation)
	if ctx.Err() != nil {
		return Result{Outcome: Cancelled}
	}
	m.record(ctx, entry, invocation, startedAt, result)

	return result
}

// refused records an attempt that never started. Recording it is what makes a
// task that keeps losing a resource visible after the contention has passed.
func (m *Manager) refused(ctx context.Context, entry *registered, invocation Invocation) {
	m.record(ctx, entry, invocation, m.now().UTC(), Result{Outcome: Skipped})
}

// record writes down what an attempt came to. A history that cannot be written
// is one stale line on a status page, so it is logged rather than raised.
func (m *Manager) record(
	ctx context.Context, entry *registered, invocation Invocation, startedAt time.Time, result Result,
) {
	if !result.Outcome.recorded() {
		return
	}
	// A wall clock that stepped backwards mid-attempt would leave the store
	// refusing a run that finished before it started, costing the row entirely.
	// No measurable time is the lesser wrong.
	finishedAt := m.now().UTC()
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	if err := m.store.RecordTaskRun(
		ctx,
		invocation.Task,
		invocation.Argument,
		startedAt,
		finishedAt,
		string(result.Outcome),
		string(result.Detail),
		entry.definition.retain(),
	); err != nil {
		slog.Warn("task run not recorded", "task", invocation.Task, "error", err)
	}
}

// admit takes a concurrency slot and the whole resource set together, returning
// what releases them. Nothing is taken unless all of it is available, so an
// attempt can never wait while holding part of what another one needs.
func (m *Manager) admit(entry *registered, invocation Invocation) (func(), admission) {
	wanted := merge(entry.definition.resources(invocation.Argument))
	key := invocationKey(invocation)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, working := m.running[key]; working {
		return nil, admitWorking
	}
	if entry.inFlight >= entry.definition.limit() || !m.available(wanted) {
		return nil, admitHeld
	}
	m.running[key] = struct{}{}
	for _, resource := range wanted {
		if resource.Exclusive {
			m.exclusive[resource.Name] = struct{}{}

			continue
		}
		m.shared[resource.Name]++
	}
	entry.inFlight++

	return func() { m.release(entry, key, wanted) }, admitStarted
}

// invocationKey names one task's work over one argument. The separator is a
// byte neither a task name nor an argument carries.
func invocationKey(invocation Invocation) string {
	return invocation.Task + "\x00" + invocation.Argument
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

func (m *Manager) release(entry *registered, key string, wanted []Resource) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.running, key)

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
