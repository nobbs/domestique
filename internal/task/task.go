// Package task owns the service's background activities: what they are, what
// they may not run beside, and when they run.
package task

import (
	"context"
	"slices"
	"time"
)

// admission is why an attempt may or may not start. Telling the two refusals
// apart is what lets a chain coalesce onto work already happening while still
// recording one that genuinely lost a resource.
type admission int

const (
	// admitStarted means the attempt has taken what it needs.
	admitStarted admission = iota
	// admitWorking means this exact invocation is already being worked on.
	admitWorking
	// admitHeld means a resource or a concurrency slot was not free.
	admitHeld
)

// detail is what a refused admission is recorded as.
func (a admission) detail() Detail {
	if a == admitWorking {
		return DetailWorking
	}

	return DetailHeld
}

// runReferenceBytes is how much randomness names one attempt. Twelve hex
// characters are readable aloud and leave a bounded history nowhere near a
// collision.
const runReferenceBytes = 6

// maxChainDepth bounds how far one attempt's consequences may reach. Every
// chain the service registers is one or two links long; this is the backstop
// behind the set of what a chain has already run.
const maxChainDepth = 8

// defaultRetainedRuns bounds a task's history when it names no bound of its
// own. An hourly task keeps roughly a week that way.
const defaultRetainedRuns = 200

// Outcome is the terminal result of one attempt.
type Outcome string

const (
	// Succeeded means the attempt did what it set out to do.
	Succeeded Outcome = "succeeded"
	// Failed means a safe failure stopped it.
	Failed Outcome = "failed"
	// Blocked means a safety gate refused the work.
	Blocked Outcome = "blocked"
	// NotReady means there was nothing it could safely do yet, because a
	// setting is unset or a target still awaits onboarding.
	NotReady Outcome = "not_ready"
	// Skipped means the attempt did no work, because a resource it needs was
	// held or the task was already running as many attempts as it may.
	Skipped Outcome = "skipped"
	// Cancelled means shutdown ended the attempt. It is never a fault.
	Cancelled Outcome = "cancelled"
	// Unchanged means the attempt ran, checked, and found nothing new.
	Unchanged Outcome = "unchanged"
	// Current means what the attempt covers was already up to date, so it did
	// nothing at all.
	Current Outcome = "current"
)

// alerts reports whether an attempt with this outcome is worth announcing. A
// refusal is not: something else was working, which is not a fault.
func (o Outcome) alerts() bool {
	return o == Failed || o == Blocked
}

// recorded reports whether an attempt with this outcome is written down. One
// that found its work already current did nothing worth remembering, and a
// cancelled one cannot write during the shutdown that ended it.
func (o Outcome) recorded() bool {
	return o != Current && o != Cancelled
}

// Detail is a stable, safe-to-display reason for an outcome. It never carries
// provider response text, a route name, or an upstream identifier.
type Detail string

// Why an attempt did no work. They are separate because they mean different
// things to whoever reads the history: one is this service busy with the very
// same work, the other is it busy with something else.
const (
	// DetailWorking means the same task over the same argument was already
	// being worked on.
	DetailWorking Detail = "already_working"
	// DetailHeld means a resource or a concurrency slot was not free.
	DetailHeld Detail = "resource_held"
)

// What an attempt is announced for besides a fault. They are alerts like any
// other, declared by the task and ruled on one at a time, so an operator who
// wants to hear about a library that stopped refreshing but not about every
// routine pass can have exactly that.
const (
	// DetailSucceeded means the attempt did its work and nothing was wrong
	// before it — either the one before also succeeded, or there was none. It is
	// the routine traffic an operator most often switches off.
	DetailSucceeded Detail = "succeeded"
	// DetailRecovered means the attempt succeeded where the one before it did
	// not. It ends an incident, which is worth hearing even from a task whose
	// routine successes are silenced.
	DetailRecovered Detail = "recovered"
	// DetailStale means the task has gone longer than its bound without
	// succeeding. A task that stopped succeeding raises no new fault once its
	// first one is suppressed, so without this it goes quiet exactly when it
	// matters.
	DetailStale Detail = "stale"
)

// Link is one invocation a finished attempt asks for. A task returns links for
// the work its own result made necessary, so what follows what is decided by
// whoever knows, rather than declared where nobody can see the outcome.
type Link struct {
	Task     string
	Argument string
}

// Result is what one attempt came to, and what it asks should happen next.
type Result struct {
	Outcome Outcome
	Detail  Detail
	Next    []Link
}

// Trigger names what started an attempt. A task whose scheduled behaviour
// differs from what an operator asked for reads it; most do not.
type Trigger string

const (
	// TriggerSchedule is the task's own schedule.
	TriggerSchedule Trigger = "schedule"
	// TriggerManual is an operator asking for the task directly.
	TriggerManual Trigger = "manual"
	// TriggerChain is another attempt that finished and asked for this one.
	TriggerChain Trigger = "chain"
)

// Invocation is one attempt: which task, over which argument, and what started
// it. A task with no arguments is invoked with an empty one.
type Invocation struct {
	Task     string
	Argument string
	Trigger  Trigger
}

// Runner performs one attempt.
type Runner interface {
	Run(ctx context.Context, invocation Invocation) Result
}

// RunnerFunc adapts a plain function to Runner.
type RunnerFunc func(ctx context.Context, invocation Invocation) Result

// Run calls f.
func (f RunnerFunc) Run(ctx context.Context, invocation Invocation) Result {
	return f(ctx, invocation)
}

// Notify is what the layer announces about one task. A task declares it once;
// which of its alerts actually reach anyone is a separate question.
type Notify struct {
	// Title is what a message about this task is titled.
	Title string
	// Alerts are the reasons this task can be announced for. Declaring them is
	// what lets an operator rule on each separately, rather than discovering
	// one exists by being woken by it.
	Alerts []Detail
	// Suppress is how long one alert silences the next for the same reason. A
	// failing task is worth one message, and the same message every tick
	// afterwards is noise an operator learns to ignore.
	Suppress time.Duration
}

// Declaration is one alert a registered task can raise.
type Declaration struct {
	Task  string
	Alert Detail
}

// Resource is state an attempt needs before it may start. Two attempts wanting
// the same name serialize unless both want it shared.
type Resource struct {
	Name      string
	Exclusive bool
}

// Definition registers one task.
type Definition struct {
	Run Runner
	// Schedule is when the task runs unasked. Nil is a task only a trigger
	// starts.
	Schedule Schedule
	// Resources are what an attempt over this argument must hold before it may
	// start. Nil needs nothing, so nothing can refuse it.
	Resources func(argument string) []Resource
	// InitialDelay is how long the first scheduled run waits after start. It is
	// read once, when the schedule starts.
	InitialDelay func() time.Duration
	// StaleAfter is how long this task may go without succeeding before it is
	// announced as stale. Nil is a task nothing is expected of on a clock.
	StaleAfter func() time.Duration
	// Notify is what an alert about this task says and how often it may say it.
	// Nil is a task nothing is announced about.
	Notify *Notify
	// Name identifies the task and is what a trigger asks for.
	Name string
	// Concurrency is how many attempts of this task may run at once. Zero means
	// one, so registering a task never introduces parallelism by accident.
	Concurrency int
	// Retain is how many of this task's attempts are kept. Zero means the
	// default; the most recent attempt over each argument is kept regardless.
	Retain int
}

// declares reports whether this task said in advance that it could be
// announced for this reason.
func (d *Definition) declares(alert Detail) bool {
	return d.Notify != nil && slices.Contains(d.Notify.Alerts, alert)
}

// alerts reports whether anything is announced about this task. What it says
// and how often are the same declaration, so a task cannot be announced about
// without a window to announce within.
func (d *Definition) alerts() bool {
	return d.Notify != nil
}

// staleAfter is how long this task may go without succeeding, zero when it is
// not expected to on a clock.
func (d *Definition) staleAfter() time.Duration {
	if d.StaleAfter == nil {
		return 0
	}

	return d.StaleAfter()
}

// resources is the set this argument needs, empty when the task named none.
func (d *Definition) resources(argument string) []Resource {
	if d.Resources == nil {
		return nil
	}

	return d.Resources(argument)
}

// retain is how many of this task's attempts are kept.
func (d *Definition) retain() int {
	if d.Retain < 1 {
		return defaultRetainedRuns
	}

	return d.Retain
}

// limit is how many attempts may run at once.
func (d *Definition) limit() int {
	if d.Concurrency < 1 {
		return 1
	}

	return d.Concurrency
}
