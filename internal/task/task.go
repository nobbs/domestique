// Package task owns the service's background activities: what they are, what
// they may not run beside, and when they run.
package task

import (
	"context"
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

// recorded reports whether an attempt with this outcome is written down. One
// that found its work already current did nothing worth remembering, and a
// cancelled one cannot write during the shutdown that ended it.
func (o Outcome) recorded() bool {
	return o != Current && o != Cancelled
}

// Detail is a stable, safe-to-display reason for an outcome. It never carries
// provider response text, a route name, or an upstream identifier.
type Detail string

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
	// Name identifies the task and is what a trigger asks for.
	Name string
	// Concurrency is how many attempts of this task may run at once. Zero means
	// one, so registering a task never introduces parallelism by accident.
	Concurrency int
	// Retain is how many of this task's attempts are kept. Zero means the
	// default; the most recent attempt over each argument is kept regardless.
	Retain int
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
