// Package task owns the service's background activities: what they are, what
// they may not run beside, and when they run.
package task

import (
	"context"
	"time"
)

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
	// Skipped means the attempt did no work, because something else held what
	// it needed.
	Skipped Outcome = "skipped"
	// Cancelled means shutdown ended the attempt. It is never a fault.
	Cancelled Outcome = "cancelled"
)

// Detail is a stable, safe-to-display reason for an outcome. It never carries
// provider response text, a route name, or an upstream identifier.
type Detail string

// Result is what one attempt came to.
type Result struct {
	Outcome Outcome
	Detail  Detail
}

// Trigger names what started an attempt. A task whose scheduled behaviour
// differs from what an operator asked for reads it; most do not.
type Trigger string

const (
	// TriggerSchedule is the task's own schedule.
	TriggerSchedule Trigger = "schedule"
	// TriggerManual is an operator asking for the task directly.
	TriggerManual Trigger = "manual"
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
}

// resources is the set this argument needs, empty when the task named none.
func (d *Definition) resources(argument string) []Resource {
	if d.Resources == nil {
		return nil
	}

	return d.Resources(argument)
}

// limit is how many attempts may run at once.
func (d *Definition) limit() int {
	if d.Concurrency < 1 {
		return 1
	}

	return d.Concurrency
}
