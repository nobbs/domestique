package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Store records what attempts came to. It is described here rather than taken
// as a concrete store so this package carries no storage concern, and it deals
// in strings so the store carries none of this package's vocabulary.
type Store interface {
	// RecordTaskRun writes one attempt down under the name a message about it
	// may carry, and prunes that task's history back to retain, keeping the most
	// recent attempt over each argument whatever its age.
	RecordTaskRun(
		ctx context.Context,
		task, argument string,
		startedAt, finishedAt time.Time,
		outcome, detail, reference string,
		retain int,
	) error
	// LastTaskOutcome reports what this task's most recent recorded attempt over
	// one argument came to.
	LastTaskOutcome(ctx context.Context, task, argument string) (outcome string, found bool, err error)
	// LastTaskSuccess reports when this task last succeeded over one argument.
	LastTaskSuccess(ctx context.Context, task, argument string) (finishedAt time.Time, found bool, err error)
	// TaskFaultStreak reports how many of this task's most recent attempts over
	// one argument ended in a fault, and when the last of them finished.
	TaskFaultStreak(ctx context.Context, task, argument string) (faults int, lastAt time.Time, err error)
	// LastFailureNotification reports when an alert of this kind last went out.
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	// RecordFailureNotification remembers that one did, or forgets that one ever
	// had when sentAt is the zero value. Forgetting is what ends an incident, so
	// the next alert of that kind is not held back by a window opened for it.
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
}

// Alerts says which alerts an operator has ruled on. An alert nobody has ruled
// on is not in the map, which is not the same as one switched off.
type Alerts interface {
	Wanted(ctx context.Context, task string, alert Detail) (enabled, decided bool)
}

// Notifier delivers already-safe alert text.
type Notifier interface {
	Send(ctx context.Context, title, message string) error
}

// Manager owns every registered task: it runs them on their schedules, keeps
// them off each other's resources, and waits for them at shutdown.
type Manager struct {
	now      func() time.Time
	after    func(time.Duration) <-chan time.Time
	store    Store
	notifier Notifier
	alerts   Alerts
	enabled  func() bool

	shared     map[string]int
	exclusive  map[string]struct{}
	running    map[invocationKey]struct{}
	tasks      map[string]*registered
	undeclared map[declarationKey]struct{}
	order      []string

	// mutex guards admission alone: a slot and a resource set are taken
	// together or not at all, so an attempt can never hold one and want the
	// other. undeclaredMutex is separate so that invariant stays easy to
	// reason about.
	mutex           sync.Mutex
	undeclaredMutex sync.Mutex
	triggered       sync.WaitGroup
}

// registered is one task and what the manager knows about it right now.
type registered struct {
	// startsAt holds the instant the first scheduled run is due, and only while
	// it is still due.
	// successors are the tasks whose Follows name this one, resolved once when
	// the last task is registered.
	successors []string
	startsAt   startsAt
	definition Definition
	inFlight   int
}

// NewManager creates an empty manager over the store its attempts are recorded
// in and the channel its alerts go out on. Nothing runs until Run is called.
// enabled is read at the moment a message would go out rather than captured,
// because an operator edits it while the service runs.
func NewManager(store Store, notifier Notifier, alerts Alerts, enabled func() bool) (*Manager, error) {
	if store == nil || notifier == nil || alerts == nil || enabled == nil {
		return nil, errors.New("task: a run store, a notifier, alert decisions and a switch are required")
	}

	return &Manager{
		now:        time.Now,
		after:      time.After,
		store:      store,
		notifier:   notifier,
		alerts:     alerts,
		enabled:    enabled,
		shared:     make(map[string]int),
		exclusive:  make(map[string]struct{}),
		running:    make(map[invocationKey]struct{}),
		tasks:      make(map[string]*registered),
		undeclared: make(map[declarationKey]struct{}),
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
	// A window left at zero would either silence the task or say the same thing
	// every tick, and neither is what leaving it out meant.
	if definition.Notify != nil && (definition.Notify.Title == "" || definition.Notify.Suppress <= 0) {
		return fmt.Errorf("task %q: an announced task needs a title and a suppression window", definition.Name)
	}
	// Uncapped doubling reaches days within a morning, which is a task that has
	// stopped rather than one waiting longer.
	if definition.Backoff.Base > 0 && definition.Backoff.Cap < definition.Backoff.Base {
		return fmt.Errorf("task %q: a backoff needs a cap no shorter than its base", definition.Name)
	}
	if _, exists := m.tasks[definition.Name]; exists {
		return fmt.Errorf("task %q is already registered", definition.Name)
	}
	m.tasks[definition.Name] = &registered{definition: *definition}
	m.order = append(m.order, definition.Name)

	return nil
}

// Resolve settles the graph the registrations declare: every edge names a task
// this build has, and no cycle closes. It is what makes the depth cap a
// backstop rather than the only protection, and must be called before Run.
func (m *Manager) Resolve() error {
	// Settled from the declarations each time, so resolving twice describes the
	// same graph rather than one with every edge doubled.
	for _, name := range m.order {
		m.tasks[name].successors = nil
	}
	for _, name := range m.order {
		entry := m.tasks[name]
		for _, follows := range entry.definition.Follows {
			predecessor, known := m.tasks[follows]
			if !known {
				return fmt.Errorf("task %q follows %q, which nothing registers", name, follows)
			}
			predecessor.successors = append(predecessor.successors, name)
		}
	}

	return m.refuseCycles()
}

// refuseCycles walks every task's successors and refuses a graph that comes
// back to where it started. A cycle would otherwise be found by the depth cap,
// once, at four in the morning.
func (m *Manager) refuseCycles() error {
	const (
		visiting = 1
		settled  = 2
	)
	state := make(map[string]int, len(m.order))

	var walk func(name string, path []string) error
	walk = func(name string, path []string) error {
		switch state[name] {
		case settled:
			return nil
		case visiting:
			return fmt.Errorf("task %q follows itself, through %s", name, strings.Join(path, " then "))
		}
		state[name] = visiting
		for _, successor := range m.tasks[name].successors {
			if err := walk(successor, append(path, successor)); err != nil {
				return err
			}
		}
		state[name] = settled

		return nil
	}
	for _, name := range m.order {
		if err := walk(name, []string{name}); err != nil {
			return err
		}
	}

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
		m.refused(ctx, entry, invocation, outcome.detail())

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

// Registered is one task as a surface outside this package reads it.
type Registered struct {
	// NextRunAt is when the first scheduled run is due, and is zero once it has
	// started or for a task nothing schedules.
	NextRunAt time.Time
	// Name is what a trigger asks for.
	Name string
	// Running is how many attempts of this task are in flight.
	Running int
	// Scheduled reports whether the task runs unasked.
	Scheduled bool
}

// Tasks lists every registered task in registration order, with what the
// manager knows about each right now.
func (m *Manager) Tasks() []Registered {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	tasks := make([]Registered, 0, len(m.order))
	for _, name := range m.order {
		entry := m.tasks[name]
		nextRunAt, _ := entry.startsAt.load()
		tasks = append(tasks, Registered{
			Name:      name,
			Scheduled: entry.definition.Schedule != nil,
			Running:   entry.inFlight,
			NextRunAt: nextRunAt,
		})
	}

	return tasks
}

// Declarations lists every alert the registered tasks can raise, in
// registration order. It is what an operator is offered a decision about:
// anything not here is something no task ever announces.
func (m *Manager) Declarations() []Declaration {
	declarations := make([]Declaration, 0, len(m.order))
	for _, name := range m.order {
		entry := m.tasks[name]
		if !entry.definition.alerts() {
			continue
		}
		for _, alert := range entry.definition.Notify.Alerts {
			declarations = append(declarations, Declaration{Task: name, Alert: alert})
		}
	}

	return declarations
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
	// A fixed gap runs and then counts. A calendar schedule waits for the time
	// it names: a restart at three on a Wednesday afternoon is not "every Monday
	// at two".
	if _, atStart := entry.definition.Schedule.(firesAtStart); atStart {
		m.scheduled(ctx, entry)
	}

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
	// A switched-off task waits out its tick rather than leaving the loop, so
	// switching it back on needs no restart. Nothing is recorded: an operator
	// who turned it off is not waiting to be told it did not run.
	if !entry.definition.enabled() {
		return
	}
	invocation := Invocation{Task: entry.definition.Name, Trigger: TriggerSchedule}
	if m.backingOff(ctx, entry, invocation) {
		return
	}
	release, outcome := m.admit(entry, invocation)
	if outcome != admitStarted {
		m.refused(ctx, entry, invocation, outcome.detail())

		return
	}
	m.perform(ctx, entry, invocation, release, nil, 0)
}

// backingOff reports whether a failing task is still being held back from its
// own schedule. It is read from the recorded history rather than kept in
// memory, so a restart neither forgets a backoff nor needs to rebuild one.
//
// Nothing is recorded about an attempt that was held back. The task is already
// in its history as failing, and a row per suppressed tick would bury that
// under the waiting.
func (m *Manager) backingOff(ctx context.Context, entry *registered, invocation Invocation) bool {
	if entry.definition.Backoff.Base <= 0 {
		return false
	}
	faults, lastAt, err := m.store.TaskFaultStreak(ctx, invocation.Task, invocation.Argument)
	if err != nil || faults == 0 {
		// A history that cannot be read must not hold a task back: not running is
		// the more expensive of the two ways to be wrong here.
		return false
	}
	wait := entry.definition.Backoff.delay(faults)
	if m.now().UTC().Sub(lastAt) >= wait {
		return false
	}
	slog.Info("task held back after repeated faults",
		"task", invocation.Task, "argument", invocation.Argument, "faults", faults, "wait", wait)

	return true
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
	visited map[invocationKey]struct{},
	depth int,
) {
	if visited == nil {
		visited = make(map[invocationKey]struct{}, 1)
	}
	visited[keyOf(invocation)] = struct{}{}

	// What follows an attempt follows a successful one. A read that failed
	// stored nothing to write or classify, and a rebuild that found nothing new
	// left every classification standing.
	if result := m.attemptAndRelease(ctx, entry, invocation, release); result.Outcome != Succeeded {
		return
	}
	m.chain(ctx, entry, visited, depth+1)
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

// chain runs what follows this task, which its registration declared. The
// depth cap and the set of what this chain has run stay as belt and braces
// behind registration refusing a cycle.
func (m *Manager) chain(
	ctx context.Context, entry *registered, visited map[invocationKey]struct{}, depth int,
) {
	if len(entry.successors) == 0 {
		return
	}
	if depth >= maxChainDepth {
		slog.Warn("task chain truncated", "depth", depth, "dropped", len(entry.successors))

		return
	}
	for _, name := range entry.successors {
		m.linked(ctx, name, visited, depth)
	}
}

// linked runs one chain link. Work already under way is left to finish rather
// than refused: a link asking for what is happening anyway has its answer, and
// admission is what decides that, so nothing can change between asking and
// starting.
func (m *Manager) linked(ctx context.Context, name string, visited map[invocationKey]struct{}, depth int) {
	entry, known := m.tasks[name]
	if !known || ctx.Err() != nil {
		return
	}
	invocation := Invocation{Task: name, Trigger: TriggerChain}
	if _, seen := visited[keyOf(invocation)]; seen {
		slog.Warn("task chain asked again for what it had already run", "task", name, "depth", depth)

		return
	}
	// A link is asked for by something that succeeded, but a task that keeps
	// faulting is hammering whatever it cannot reach whether the asking came
	// from a schedule or from a chain.
	if m.backingOff(ctx, entry, invocation) {
		visited[keyOf(invocation)] = struct{}{}

		return
	}
	release, outcome := m.admit(entry, invocation)
	switch outcome {
	case admitWorking:
		// The work is happening, which is what the link asked for, so the rest
		// of the chain treats it as run rather than asking again once it ends.
		visited[keyOf(invocation)] = struct{}{}

		return
	case admitHeld:
		m.refused(ctx, entry, invocation, DetailHeld)

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

// refused records an attempt that never started, saying which kind of busy
// stopped it. Recording it is what makes a task that keeps losing to something
// visible after the contention has passed.
func (m *Manager) refused(ctx context.Context, entry *registered, invocation Invocation, detail Detail) {
	m.record(ctx, entry, invocation, m.now().UTC(), Result{Outcome: Skipped, Detail: detail})
}

// record writes down what an attempt came to. A history that cannot be written
// is one stale line on a status page, so it is logged rather than raised.
func (m *Manager) record(
	ctx context.Context, entry *registered, invocation Invocation, startedAt time.Time, result Result,
) {
	if !result.Outcome.recorded() {
		return
	}
	// Asked before recording, because the question is what this task did last
	// and this attempt is about to become the answer to it.
	alert := m.alertFor(ctx, entry, invocation, result)
	// A wall clock that stepped backwards mid-attempt would leave the store
	// refusing a run that finished before it started, costing the row entirely.
	// No measurable time is the lesser wrong.
	finishedAt := m.now().UTC()
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	reference := newRunReference()
	if err := m.store.RecordTaskRun(
		ctx,
		invocation.Task,
		invocation.Argument,
		startedAt,
		finishedAt,
		string(result.Outcome),
		string(result.Detail),
		reference,
		entry.definition.retain(),
	); err != nil {
		slog.Warn("task run not recorded", "task", invocation.Task, "error", err)

		return
	}
	m.announce(ctx, entry, invocation, alert, alertMessage(invocation, result.Outcome, alert, reference), finishedAt)
	m.checkStale(ctx, entry, invocation, result, reference, finishedAt)
}

// alertFor is what this attempt is announced as, empty when it is not announced
// at all. A fault is announced as the reason it gave. A success is a recovery
// when it ends a run of anything else, and routine otherwise — two alerts
// rather than one, because an operator who silences every routine pass still
// wants to hear that the thing came back.
func (m *Manager) alertFor(
	ctx context.Context, entry *registered, invocation Invocation, result Result,
) Detail {
	if result.Outcome.alerts() {
		return result.Detail
	}
	if result.Outcome != Succeeded {
		return ""
	}
	// A success is opt-in where a fault is not. A fault nobody declared is
	// still worth hearing about, but a routine success nobody asked for is
	// noise an operator could not have switched off in advance.
	if m.endsAnIncident(ctx, invocation) && entry.definition.declares(DetailRecovered) {
		return DetailRecovered
	}
	if entry.definition.declares(DetailSucceeded) {
		return DetailSucceeded
	}

	return ""
}

// endsAnIncident reports whether this success follows something that was not
// one. Anything else counts, including the not_ready a target awaiting its
// authorization records: what an operator wants to hear is that the task is
// doing its work again.
func (m *Manager) endsAnIncident(ctx context.Context, invocation Invocation) bool {
	outcome, found, err := m.store.LastTaskOutcome(ctx, invocation.Task, invocation.Argument)
	if err != nil {
		// An unreadable history must not silence what may be the recovery: one
		// message too many costs a line, a withheld recovery costs an alert.
		return true
	}

	return found && outcome != string(Succeeded)
}

// checkStale announces a task that has gone too long without succeeding. A task
// that stopped succeeding raises no new fault once its first one is suppressed,
// so this is asked after every attempt rather than only after a failed one.
func (m *Manager) checkStale(
	ctx context.Context,
	entry *registered,
	invocation Invocation,
	result Result,
	reference string,
	now time.Time,
) {
	// Declared before it can be ruled on, for the same reason a success is: an
	// age nobody asked to hear about is not a fault anyone is waiting for.
	bound := entry.definition.staleAfter()
	if bound <= 0 || !entry.definition.declares(DetailStale) {
		return
	}
	// A success is what freshness is, so it ends the incident rather than being
	// measured against it.
	if result.Outcome == Succeeded {
		m.clearAlert(ctx, invocation.Task, DetailStale)

		return
	}
	lastSuccess, found, err := m.store.LastTaskSuccess(ctx, invocation.Task, invocation.Argument)
	if err != nil || !found {
		return
	}
	// Compared in whole seconds, the same precision the status surface reports
	// an age in: a sub-second remainder must not let this alert and that
	// response disagree on whether the task is stale.
	if now.Sub(lastSuccess)/time.Second < bound/time.Second {
		return
	}
	// Named without an outcome: what is being announced is how long the task has
	// gone without succeeding, not what the attempt that noticed came to.
	m.announce(ctx, entry, invocation, DetailStale, alertMessage(invocation, "", DetailStale, reference), now)
}

// clearAlert forgets that an alert went out, so the next one is not held back
// by a window opened for an incident that is over.
func (m *Manager) clearAlert(ctx context.Context, task string, alert Detail) {
	if err := m.store.RecordFailureNotification(ctx, task+":"+string(alert), time.Time{}); err != nil {
		slog.Warn("alert suppression not cleared", "task", task, "alert", alert, "error", err)
	}
}

// announce sends one alert, no more often than the task's own suppression
// window allows. The window is keyed by the reason as well as the task: a
// library that cannot be read and a target that needs reauthorising are
// separate problems and are worth saying separately.
func (m *Manager) announce(
	ctx context.Context,
	entry *registered,
	invocation Invocation,
	alert Detail,
	message string,
	now time.Time,
) {
	if alert == "" || !entry.definition.alerts() || !m.enabled() {
		return
	}
	m.reportUndeclared(entry, invocation.Task, alert)
	if !m.wanted(ctx, invocation.Task, alert) {
		return
	}
	category := invocation.Task + ":" + string(alert)
	lastSentAt, found, err := m.store.LastFailureNotification(ctx, category)
	if err != nil || (found && now.Sub(lastSentAt) < entry.definition.Notify.Suppress) {
		return
	}
	if err := m.notifier.Send(ctx, entry.definition.Notify.Title, message); err != nil {
		return
	}
	// Nothing is written down as sent until it has been, so a delivery that
	// failed is tried again rather than silenced by its own suppression.
	if err := m.store.RecordFailureNotification(ctx, category, now); err != nil {
		slog.Warn("alert suppression not recorded", "task", invocation.Task, "error", err)
	}
}

// newRunReference names one attempt, in the same twelve hex characters every
// other run in this service is named by. It is random and means nothing on its
// own, which is what makes it safe to send.
func newRunReference() string {
	reference := make([]byte, runReferenceBytes)
	// crypto/rand.Read never returns an error and always fills its argument
	// entirely: it crashes the program rather than answering short.
	_, _ = rand.Read(reference)

	return hex.EncodeToString(reference)
}

// reportUndeclared says once that a task raised something it never declared.
// Whether the declaration is complete is a fact about this build rather than
// about this run, so it is said whatever an operator has decided and whatever
// the window allows — and said once, because repeating it every tick would
// bury it.
func (m *Manager) reportUndeclared(entry *registered, task string, alert Detail) {
	if entry.definition.declares(alert) {
		return
	}

	key := declarationKey{task: task, alert: alert}
	m.undeclaredMutex.Lock()
	_, said := m.undeclared[key]
	m.undeclared[key] = struct{}{}
	m.undeclaredMutex.Unlock()

	if !said {
		slog.Warn("task announced something it had not declared", "task", task, "alert", alert)
	}
}

// wanted reports whether an operator wants this alert delivered. One they have
// ruled on is their decision; one nobody has ruled on is announced, because a
// fault nobody has heard of is the one worth hearing about.
func (m *Manager) wanted(ctx context.Context, task string, alert Detail) bool {
	enabled, decided := m.alerts.Wanted(ctx, task, alert)
	if !decided {
		return true
	}

	return enabled
}

// alertMessage says which task is being announced, over what, and why. The
// reason is left off when it only repeats the outcome, and the outcome is left
// off when what is announced is not one attempt's. Every message names its run:
// the reference is random and means nothing on its own, which is what makes it
// safe to send.
func alertMessage(invocation Invocation, outcome Outcome, alert Detail, reference string) string {
	message := invocation.Task
	if invocation.Argument != "" {
		message += " " + invocation.Argument
	}
	if outcome == "" {
		return message + " " + string(alert) + " run=" + reference
	}
	message += " " + string(outcome)
	if alert != "" && string(alert) != string(outcome) {
		message += ": " + string(alert)
	}

	return message + " run=" + reference
}

// admit takes a concurrency slot and the whole resource set together, returning
// what releases them. Nothing is taken unless all of it is available, so an
// attempt can never wait while holding part of what another one needs.
func (m *Manager) admit(entry *registered, invocation Invocation) (func(), admission) {
	wanted := merge(entry.definition.resources(invocation.Argument))
	key := keyOf(invocation)

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

// declarationKey names one alert of one task, for the once-per-build report
// that its declaration does not mention it.
type declarationKey struct {
	task  string
	alert Detail
}

// invocationKey names one task's work over one argument. It is a pair rather
// than a joined string so that no argument can be spelled to collide with
// another task's.
type invocationKey struct {
	task     string
	argument string
}

func keyOf(invocation Invocation) invocationKey {
	return invocationKey{task: invocation.Task, argument: invocation.Argument}
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

func (m *Manager) release(entry *registered, key invocationKey, wanted []Resource) {
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
