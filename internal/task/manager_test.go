package task

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterRefusesADefinitionItCannotRun(t *testing.T) {
	t.Parallel()

	tests := map[string]Definition{
		"no name":   {Run: succeeds()},
		"no runner": {Name: "a"},
		"no delay":  {Name: "a", Run: succeeds(), Schedule: Every(func() time.Duration { return time.Hour })},
	}
	for name, definition := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Error(t, NewManager().Register(definition), "Register()")
		})
	}
}

func TestRegisterRefusesTheSameNameTwice(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "a", Run: succeeds()}), "first Register()")
	assert.Error(t, manager.Register(Definition{Name: "a", Run: succeeds()}), "second Register()")
}

func TestTriggerRefusesATaskNobodyRegistered(t *testing.T) {
	t.Parallel()

	assert.False(t, NewManager().Trigger(t.Context(), "absent", ""), "Trigger()")
}

// A trigger reports whether the work was taken on, and the work outlives the
// call. Wait is what makes that safe at shutdown.
func TestTriggerRunsInTheBackgroundAndWaitWaitsForIt(t *testing.T) {
	t.Parallel()

	runner := blockOn()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "a", Run: runner}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "a", "slot"), "Trigger()")
	<-runner.started
	assert.Equal(t, "slot", runner.argument(), "the argument the task was given")
	close(runner.release)
	manager.Wait()
	assert.Equal(t, 1, runner.runs(), "runs")
}

// Everything that touches the same state exclusively runs one at a time, even
// when a different task asked for it.
func TestAnExclusiveResourceRefusesAnyOtherTaskWantingIt(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "holder", Run: held, Resources: exclusive("inventory")}), "Register(holder)")
	require.NoError(t, manager.Register(Definition{Name: "other", Run: succeeds(), Resources: exclusive("inventory")}), "Register(other)")

	require.True(t, manager.Trigger(t.Context(), "holder", ""), "Trigger(holder)")
	<-held.started
	assert.True(t, manager.Holding("inventory"), "Holding()")
	assert.False(t, manager.Trigger(t.Context(), "other", ""), "Trigger(other) ran beside the holder")

	close(held.release)
	manager.Wait()
	assert.False(t, manager.Holding("inventory"), "the resource was still held after the run finished")
	assert.True(t, manager.Trigger(t.Context(), "other", ""), "Trigger(other) was refused after the holder finished")
	manager.Wait()
}

// Readers of the same state run together; a writer waits for all of them.
func TestASharedResourceAdmitsReadersAndRefusesAWriter(t *testing.T) {
	t.Parallel()

	first, second := blockOn(), blockOn()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "first", Run: first, Resources: shared("inventory")}), "Register(first)")
	require.NoError(t, manager.Register(Definition{Name: "second", Run: second, Resources: shared("inventory")}), "Register(second)")
	require.NoError(t, manager.Register(Definition{Name: "writer", Run: succeeds(), Resources: exclusive("inventory")}), "Register(writer)")

	require.True(t, manager.Trigger(t.Context(), "first", ""), "Trigger(first)")
	<-first.started
	require.True(t, manager.Trigger(t.Context(), "second", ""), "Trigger(second) was refused beside another reader")
	<-second.started
	assert.False(t, manager.Trigger(t.Context(), "writer", ""), "Trigger(writer) ran beside two readers")

	close(first.release)
	close(second.release)
	manager.Wait()
}

func TestUnrelatedResourcesDoNotRefuseEachOther(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "holder", Run: held, Resources: exclusive("inventory")}), "Register(holder)")
	require.NoError(t, manager.Register(Definition{Name: "other", Run: succeeds(), Resources: exclusive("surface-index")}), "Register(other)")

	require.True(t, manager.Trigger(t.Context(), "holder", ""), "Trigger(holder)")
	<-held.started
	assert.True(t, manager.Trigger(t.Context(), "other", ""), "an unrelated resource refused a run")

	close(held.release)
	manager.Wait()
}

// A task naming no resource can refuse only itself, which is what the
// concurrency limit is for.
func TestConcurrencyLimitsHowManyAttemptsOfOneTaskRun(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "a", Run: held, Concurrency: 2}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "a", "one"), "first Trigger()")
	<-held.started
	require.True(t, manager.Trigger(t.Context(), "a", "two"), "second Trigger() below the limit")
	assert.False(t, manager.Trigger(t.Context(), "a", "three"), "third Trigger() past the limit")

	close(held.release)
	manager.Wait()
}

func TestAnUnsetConcurrencyAdmitsOneAttempt(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "a", Run: held}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "a", ""), "first Trigger()")
	<-held.started
	assert.False(t, manager.Trigger(t.Context(), "a", ""), "second Trigger() ran beside the first")

	close(held.release)
	manager.Wait()
}

// Shutdown is not a fault, so an attempt it ends says so.
func TestACancelledAttemptIsNotAFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	entry := &registered{definition: Definition{Name: "a", Run: succeeds()}}

	result := NewManager().attempt(ctx, entry, Invocation{Task: "a"})
	assert.Equal(t, Cancelled, result.Outcome, "outcome")
}

func TestAttemptReportsWhatTheRunnerReturned(t *testing.T) {
	t.Parallel()

	entry := &registered{definition: Definition{
		Name: "a",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Blocked, Detail: "deletion_limit"}
		}),
	}}

	result := NewManager().attempt(t.Context(), entry, Invocation{Task: "a"})
	assert.Equal(t, Result{Outcome: Blocked, Detail: "deletion_limit"}, result, "attempt()")
}

// A schedule holds its first run back, says so while it is holding it, and
// stops saying so once the run has started.
func TestRunHoldsTheFirstRunAndReportsWhenItIsDue(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager := NewManager()
	fired := make(chan time.Time)
	manager.now = reference
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	require.NoError(t, manager.Register(Definition{
		Name:         "a",
		Run:          runner,
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return 5 * time.Minute },
	}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(ctx) }()

	require.Eventually(t, func() bool {
		_, holding := manager.NextRunAt("a")

		return holding
	}, time.Second, time.Millisecond, "the schedule never reported a held-back first run")
	due, _ := manager.NextRunAt("a")
	assert.Equal(t, reference().Add(5*time.Minute), due, "NextRunAt()")

	fired <- reference()
	require.Eventually(t, func() bool { return runner.runs() == 1 }, time.Second, time.Millisecond, "the first run never happened")
	_, holding := manager.NextRunAt("a")
	assert.False(t, holding, "a run that has started is still reported as held back")

	cancel()
	<-stopped
}

func TestNextRunAtIsSilentAboutATaskNobodyRegistered(t *testing.T) {
	t.Parallel()

	_, holding := NewManager().NextRunAt("absent")
	assert.False(t, holding, "NextRunAt()")
}

// Cancelling has to stop a schedule that is still waiting out its initial
// delay, or shutdown would wait for a run that is not coming.
func TestRunStopsAScheduleStillWaitingToStart(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager := NewManager()
	manager.after = func(time.Duration) <-chan time.Time { return make(chan time.Time) }
	require.NoError(t, manager.Register(Definition{
		Name:         "a",
		Run:          runner,
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return time.Minute },
	}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(ctx) }()
	cancel()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
	assert.Zero(t, runner.runs(), "a run happened after cancellation")
}

// Run returns only once every scheduled task has stopped, which is what lets
// the caller close the state those tasks are still reading.
func TestRunWaitsForEveryScheduledTask(t *testing.T) {
	t.Parallel()

	first, second := blockOn(), blockOn()
	manager := NewManager()
	fired := make(chan time.Time)
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	for name, runner := range map[string]*blockingRunner{"first": first, "second": second} {
		require.NoError(t, manager.Register(Definition{
			Name:         name,
			Run:          runner,
			Schedule:     Every(func() time.Duration { return time.Hour }),
			InitialDelay: func() time.Duration { return time.Minute },
		}), "Register()")
	}

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(ctx) }()

	fired <- reference()
	fired <- reference()
	<-first.started
	<-second.started
	cancel()

	select {
	case <-stopped:
		t.Fatal("Run returned while a scheduled task was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(first.release)
	close(second.release)
	<-stopped
}

// A schedule is refused on exactly the terms a trigger is: whatever holds the
// resource wins, and the schedule simply comes round again.
func TestAScheduledRunIsRefusedWhileItsResourceIsHeld(t *testing.T) {
	t.Parallel()

	blocker, scheduled := blockOn(), countingRunner()
	manager := NewManager()
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }
	require.NoError(t, manager.Register(Definition{Name: "blocker", Run: blocker, Resources: exclusive("inventory")}), "Register(blocker)")
	require.NoError(t, manager.Register(Definition{
		Name:         "scheduled",
		Run:          scheduled,
		Resources:    exclusive("inventory"),
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return time.Minute },
	}), "Register(scheduled)")

	require.True(t, manager.Trigger(t.Context(), "blocker", ""), "Trigger(blocker)")
	<-blocker.started

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(ctx) }()

	<-waits
	fired <- reference()
	// The schedule asking for its next gap is proof the refused run is behind it.
	<-waits
	assert.Zero(t, scheduled.runs(), "a scheduled run started while the resource was held")

	close(blocker.release)
	manager.Wait()
	cancel()
	<-stopped
}

// The gap between runs is a cadence, not a one-off: the schedule keeps firing
// for as long as its context lives.
func TestRunKeepsFiringOnTheSchedule(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager := NewManager()
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }
	require.NoError(t, manager.Register(Definition{
		Name:         "a",
		Run:          runner,
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return time.Minute },
	}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(ctx) }()

	assert.Equal(t, time.Minute, <-waits, "the first wait is the initial delay")
	fired <- reference()
	<-waits
	fired <- reference()
	<-waits
	assert.Equal(t, 2, runner.runs(), "runs")

	cancel()
	<-stopped
}

// A cadence an operator has emptied stops the schedule rather than spinning on
// a gap that is not a gap.
func TestRunStopsWhenTheCadenceIsEmptied(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager := NewManager()
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }

	var gaps sync.Mutex
	gap := time.Hour
	require.NoError(t, manager.Register(Definition{
		Name: "a",
		Run:  runner,
		Schedule: Every(func() time.Duration {
			gaps.Lock()
			defer gaps.Unlock()

			return gap
		}),
		InitialDelay: func() time.Duration { return time.Minute },
	}), "Register()")

	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(t.Context()) }()

	<-waits
	gaps.Lock()
	gap = 0
	gaps.Unlock()
	fired <- reference()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after the cadence was emptied")
	}
	assert.Equal(t, 1, runner.runs(), "runs")
}

// A task naming one resource twice must not take it twice, or releasing it once
// would leave it held forever.
func TestMergeFoldsARepeatedResourceAndKeepsTheStricterHold(t *testing.T) {
	t.Parallel()

	merged := merge([]Resource{
		{Name: "inventory"},
		{Name: "stage"},
		{Name: "inventory", Exclusive: true},
	})

	assert.Equal(t, []Resource{{Name: "inventory", Exclusive: true}, {Name: "stage"}}, merged, "merge()")
}

func TestMergeLeavesASetItCannotShorten(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []Resource{{Name: "a"}}, merge([]Resource{{Name: "a"}}), "merge()")
}

func exclusive(name string) func(string) []Resource {
	return func(string) []Resource { return []Resource{{Name: name, Exclusive: true}} }
}

func shared(name string) func(string) []Resource {
	return func(string) []Resource { return []Resource{{Name: name}} }
}

func succeeds() Runner {
	return RunnerFunc(func(context.Context, Invocation) Result { return Result{Outcome: Succeeded} })
}

// blockingRunner stops in the middle of an attempt, so a test can ask what the
// manager allows while one is genuinely in flight.
type blockingRunner struct {
	started   chan struct{}
	release   chan struct{}
	lastGiven string

	mutex sync.Mutex
	count int
}

func blockOn() *blockingRunner {
	return &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *blockingRunner) Run(_ context.Context, invocation Invocation) Result {
	r.mutex.Lock()
	r.count++
	r.lastGiven = invocation.Argument
	first := r.count == 1
	r.mutex.Unlock()

	if first {
		close(r.started)
	}
	<-r.release

	return Result{Outcome: Succeeded}
}

func (r *blockingRunner) runs() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.count
}

func (r *blockingRunner) argument() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.lastGiven
}

type counting struct {
	mutex sync.Mutex
	count int
}

func countingRunner() *counting { return &counting{} }

func (c *counting) Run(context.Context, Invocation) Result {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.count++

	return Result{Outcome: Succeeded}
}

func (c *counting) runs() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.count
}

// A service that is shutting down takes on nothing new, rather than accepting
// work whose only outcome can be that it was cancelled.
func TestTriggerRefusesOnceTheContextIsDone(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager := NewManager()
	require.NoError(t, manager.Register(Definition{Name: "a", Run: runner}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.False(t, manager.Trigger(ctx, "a", ""), "Trigger()")
	manager.Wait()
	assert.Zero(t, runner.runs(), "an attempt started after cancellation")
}

// A wait watches the clock and the context at once, and may report that it
// fired even though both were ready. Starting work on that report would run one
// more attempt into a shutdown.
func TestAScheduledRunDoesNotStartOnceTheContextIsDone(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager := NewManager()
	fired := make(chan time.Time, 1)
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	require.NoError(t, manager.Register(Definition{
		Name:         "a",
		Run:          runner,
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return time.Minute },
	}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fired <- reference()

	manager.Run(ctx)
	assert.Zero(t, runner.runs(), "a scheduled attempt started after cancellation")
}
