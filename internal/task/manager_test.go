package task

import (
	"context"
	"errors"
	"slices"
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

			manager, _ := newTestManager(t)
			assert.Error(t, manager.Register(&definition), "Register()")
		})
	}
}

func TestRegisterRefusesNoDefinitionAtAll(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	assert.Error(t, manager.Register(nil), "Register(nil)")
}

func TestRegisterRefusesTheSameNameTwice(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: succeeds()}), "first Register()")
	assert.Error(t, manager.Register(&Definition{Name: "a", Run: succeeds()}), "second Register()")
}

func TestTriggerRefusesATaskNobodyRegistered(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	assert.False(t, manager.Trigger(t.Context(), "absent", ""), "Trigger()")
}

// A trigger reports whether the work was taken on, and the work outlives the
// call. Wait is what makes that safe at shutdown.
func TestTriggerRunsInTheBackgroundAndWaitWaitsForIt(t *testing.T) {
	t.Parallel()

	runner := blockOn()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: runner}), "Register()")

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
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "holder", Run: held, Resources: exclusive("inventory")}), "Register(holder)")
	require.NoError(t, manager.Register(&Definition{Name: "other", Run: succeeds(), Resources: exclusive("inventory")}), "Register(other)")

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
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "first", Run: first, Resources: shared("inventory")}), "Register(first)")
	require.NoError(t, manager.Register(&Definition{Name: "second", Run: second, Resources: shared("inventory")}), "Register(second)")
	require.NoError(t, manager.Register(&Definition{Name: "writer", Run: succeeds(), Resources: exclusive("inventory")}), "Register(writer)")

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
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "holder", Run: held, Resources: exclusive("inventory")}), "Register(holder)")
	require.NoError(t, manager.Register(&Definition{Name: "other", Run: succeeds(), Resources: exclusive("surface-index")}), "Register(other)")

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
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: held, Concurrency: 2}), "Register()")

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
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: held}), "Register()")

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

	manager, _ := newTestManager(t)
	result := manager.attempt(ctx, entry, Invocation{Task: "a"})
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

	manager, _ := newTestManager(t)
	result := manager.attempt(t.Context(), entry, Invocation{Task: "a"})
	assert.Equal(t, Result{Outcome: Blocked, Detail: "deletion_limit"}, result, "attempt()")
}

// A schedule holds its first run back, says so while it is holding it, and
// stops saying so once the run has started.
func TestRunHoldsTheFirstRunAndReportsWhenItIsDue(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, _ := newTestManager(t)
	fired := make(chan time.Time)
	manager.now = reference
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	require.NoError(t, manager.Register(&Definition{
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

	manager, _ := newTestManager(t)
	_, holding := manager.NextRunAt("absent")
	assert.False(t, holding, "NextRunAt()")
}

// Cancelling has to stop a schedule that is still waiting out its initial
// delay, or shutdown would wait for a run that is not coming.
func TestRunStopsAScheduleStillWaitingToStart(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, _ := newTestManager(t)
	manager.after = func(time.Duration) <-chan time.Time { return make(chan time.Time) }
	require.NoError(t, manager.Register(&Definition{
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
	manager, _ := newTestManager(t)
	fired := make(chan time.Time)
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	for name, runner := range map[string]*blockingRunner{"first": first, "second": second} {
		require.NoError(t, manager.Register(&Definition{
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
	manager, _ := newTestManager(t)
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }
	require.NoError(t, manager.Register(&Definition{Name: "blocker", Run: blocker, Resources: exclusive("inventory")}), "Register(blocker)")
	require.NoError(t, manager.Register(&Definition{
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
	manager, _ := newTestManager(t)
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }
	require.NoError(t, manager.Register(&Definition{
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
	manager, _ := newTestManager(t)
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }

	var gaps sync.Mutex
	gap := time.Hour
	require.NoError(t, manager.Register(&Definition{
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
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: runner}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.False(t, manager.Trigger(ctx, "a", ""), "Trigger()")
	manager.Wait()
	assert.Zero(t, runner.runs(), "an attempt started after cancellation")
}

// A wait watches the clock and the context at once and may report that it
// fired even though both were ready, so the guard is asked directly here rather
// than through a wait that would answer before reaching it.
func TestScheduledStartsNothingOnceTheContextIsDone(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: runner}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager.scheduled(ctx, manager.tasks["a"])

	assert.Zero(t, runner.runs(), "a scheduled attempt started after cancellation")
	assert.Empty(t, store.recorded(), "a refusal was recorded during shutdown")
}

// A wait watches the clock and the context at once, and may report that it
// fired even though both were ready. Starting work on that report would run one
// more attempt into a shutdown.
func TestAScheduledRunDoesNotStartOnceTheContextIsDone(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, _ := newTestManager(t)
	fired := make(chan time.Time, 1)
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	require.NoError(t, manager.Register(&Definition{
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

func TestNewManagerNeedsSomewhereToRecord(t *testing.T) {
	t.Parallel()

	_, err := NewManager(nil)
	assert.Error(t, err, "NewManager()")
}

func TestAnAttemptIsRecordedWithWhatItCameTo(t *testing.T) {
	t.Parallel()

	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name:   "a",
		Retain: 12,
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Blocked, Detail: "deletion_limit"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "a", "slot"), "Trigger()")
	manager.Wait()
	assert.Equal(t, []recordedRun{
		{task: "a", argument: "slot", outcome: string(Blocked), detail: "deletion_limit", retain: 12},
	}, store.recorded(), "recorded runs")
}

// An attempt that found its work already current did nothing, and a history of
// nothing is what the fan-out over a whole library would otherwise write every
// tick.
func TestAnAttemptThatFoundItsWorkCurrentIsNotRecorded(t *testing.T) {
	t.Parallel()

	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "a",
		Run:  RunnerFunc(func(context.Context, Invocation) Result { return Result{Outcome: Current} }),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "a", ""), "Trigger()")
	manager.Wait()
	assert.Empty(t, store.recorded(), "an attempt that did nothing was recorded")
}

// Shutdown cancels the context every write would need, so a cancelled attempt
// is not recorded rather than failing to be.
func TestACancelledAttemptIsNotRecorded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager, store := newTestManager(t)
	entry := &registered{definition: Definition{Name: "a", Run: succeeds()}}

	manager.attempt(ctx, entry, Invocation{Task: "a"})
	assert.Empty(t, store.recorded(), "a cancelled attempt was recorded")
}

// A refusal is the answer to why something did not run, and it is only
// answerable afterwards if it was written down.
func TestARefusedAttemptIsRecordedAsSkipped(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "holder", Run: held, Resources: exclusive("inventory")}), "Register(holder)")
	require.NoError(t, manager.Register(&Definition{Name: "other", Run: succeeds(), Resources: exclusive("inventory")}), "Register(other)")

	require.True(t, manager.Trigger(t.Context(), "holder", ""), "Trigger(holder)")
	<-held.started
	require.False(t, manager.Trigger(t.Context(), "other", "slot"), "Trigger(other)")

	assert.Equal(t, []recordedRun{
		{task: "other", argument: "slot", outcome: string(Skipped), retain: defaultRetainedRuns},
	}, store.recorded(), "recorded runs")

	close(held.release)
	manager.Wait()
}

func TestARefusedScheduledRunIsRecorded(t *testing.T) {
	t.Parallel()

	blocker := blockOn()
	manager, store := newTestManager(t)
	fired, waits := make(chan time.Time), make(chan time.Duration, 4)
	manager.after = func(delay time.Duration) <-chan time.Time { waits <- delay; return fired }
	require.NoError(t, manager.Register(&Definition{Name: "blocker", Run: blocker, Resources: exclusive("inventory")}), "Register(blocker)")
	require.NoError(t, manager.Register(&Definition{
		Name:         "scheduled",
		Run:          countingRunner(),
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
	<-waits

	assert.Contains(t, store.recorded(),
		recordedRun{task: "scheduled", outcome: string(Skipped), retain: defaultRetainedRuns},
		"a refused scheduled run was not recorded")

	close(blocker.release)
	manager.Wait()
	cancel()
	<-stopped
}

// A clock that steps backwards mid-attempt must not cost the row: the store
// refuses a run that finished before it started, and losing the history is
// worse than recording no measurable time.
func TestAnAttemptSurvivesAClockThatStepsBackwards(t *testing.T) {
	t.Parallel()

	manager, store := newTestManager(t)
	readings := []time.Time{reference(), reference().Add(-time.Hour)}
	manager.now = func() time.Time {
		at := readings[0]
		if len(readings) > 1 {
			readings = readings[1:]
		}

		return at
	}
	entry := &registered{definition: Definition{Name: "a", Run: succeeds()}}

	manager.attempt(t.Context(), entry, Invocation{Task: "a"})
	assert.Equal(t, []recordedRun{
		{task: "a", outcome: string(Succeeded), retain: defaultRetainedRuns},
	}, store.recorded(), "an attempt was lost to a clock that stepped backwards")
}

// Losing a history row costs a stale line on a status page, which must not be
// allowed to rewrite what the attempt actually came to.
func TestAHistoryThatCannotBeWrittenDoesNotChangeTheOutcome(t *testing.T) {
	t.Parallel()

	manager, store := newTestManager(t)
	store.err = errors.New("state unavailable")
	entry := &registered{definition: Definition{Name: "a", Run: succeeds()}}

	result := manager.attempt(t.Context(), entry, Invocation{Task: "a"})
	assert.Equal(t, Succeeded, result.Outcome, "outcome")
}

func newTestManager(t *testing.T) (*Manager, *recordingStore) {
	t.Helper()

	store := &recordingStore{}
	manager, err := NewManager(store)
	require.NoError(t, err, "NewManager()")

	return manager, store
}

type recordedRun struct {
	task     string
	argument string
	outcome  string
	detail   string
	retain   int
}

type recordingStore struct {
	err   error
	runs  []recordedRun
	mutex sync.Mutex
}

func (s *recordingStore) RecordTaskRun(
	_ context.Context, task, argument string, startedAt, finishedAt time.Time, outcome, detail string, retain int,
) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if finishedAt.Before(startedAt) {
		return errors.New("an attempt finished before it started")
	}
	s.runs = append(s.runs, recordedRun{
		task: task, argument: argument, outcome: outcome, detail: detail, retain: retain,
	})

	return s.err
}

func (s *recordingStore) recorded() []recordedRun {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return slices.Clone(s.runs)
}

// A link wanting what its parent held has to be able to take it, or every chain
// between two tasks over the same state would refuse itself.
func TestAChainLinkTakesTheResourceItsParentHeld(t *testing.T) {
	t.Parallel()

	followed := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name:      "parent",
		Resources: exclusive("inventory"),
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "child"}}}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: followed, Resources: exclusive("inventory"),
	}), "Register(child)")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, followed.runs(), "the chained task never ran")
}

func TestAChainCarriesTheArgumentItNames(t *testing.T) {
	t.Parallel()

	followed := blockOn()
	close(followed.release)
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "child", Argument: "stage-7"}}}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{Name: "child", Run: followed}), "Register(child)")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, "stage-7", followed.argument(), "the argument the chained task was given")
}

// Work already under way has the link's answer, so asking again is dropped
// rather than recorded as a refusal that means something.
func TestAChainLinkForWorkAlreadyUnderWayIsDroppedQuietly(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "child", Run: held}), "Register(child)")
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "child"}}}
		}),
	}), "Register(parent)")

	require.True(t, manager.Trigger(t.Context(), "child", ""), "Trigger(child)")
	<-held.started
	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger(parent)")

	require.Eventually(t, func() bool {
		for _, run := range store.recorded() {
			if run.task == "parent" {
				return true
			}
		}

		return false
	}, time.Second, time.Millisecond, "the parent never finished")

	for _, run := range store.recorded() {
		assert.NotEqual(t, string(Skipped), run.outcome, "a coalesced link was recorded as a refusal")
	}
	close(held.release)
	manager.Wait()
}

// A link losing a resource to something unrelated is a refusal, and refusals
// are what answer why a task did not run.
func TestAChainLinkRefusedByAnotherHolderIsRecorded(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "holder", Run: held, Resources: exclusive("index")}), "Register(holder)")
	require.NoError(t, manager.Register(&Definition{Name: "child", Run: succeeds(), Resources: exclusive("index")}), "Register(child)")
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "child"}}}
		}),
	}), "Register(parent)")

	require.True(t, manager.Trigger(t.Context(), "holder", ""), "Trigger(holder)")
	<-held.started
	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger(parent)")

	require.Eventually(t, func() bool {
		return slices.Contains(store.recorded(),
			recordedRun{task: "child", outcome: string(Skipped), retain: defaultRetainedRuns})
	}, time.Second, time.Millisecond, "a refused chain link was not recorded")

	close(held.release)
	manager.Wait()
}

// Links are chosen while a task runs, so nothing can reject a cycle when it is
// registered. What a chain has already run is carried down it instead.
func TestAChainWillNotRunTheSameInvocationTwice(t *testing.T) {
	t.Parallel()

	runs := 0
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "loop",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			runs++

			return Result{Outcome: Succeeded, Next: []Link{{Task: "loop"}}}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "loop", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, runs, "a task chained to itself ran again")
}

// Two tasks each asking for the other cannot be caught by the set alone once
// the arguments differ, so the depth behind it is what ends the chain.
func TestAChainStopsAtItsDepthLimit(t *testing.T) {
	t.Parallel()

	runs := 0
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name:        "endless",
		Concurrency: maxChainDepth + 2,
		Run: RunnerFunc(func(_ context.Context, invocation Invocation) Result {
			runs++

			return Result{
				Outcome: Succeeded,
				Next:    []Link{{Task: "endless", Argument: invocation.Argument + "x"}},
			}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "endless", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, maxChainDepth, runs, "the chain did not stop at its depth limit")
}

func TestAChainIgnoresATaskNobodyRegistered(t *testing.T) {
	t.Parallel()

	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "absent"}}}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, []recordedRun{
		{task: "parent", outcome: string(Succeeded), retain: defaultRetainedRuns},
	}, store.recorded(), "recorded runs")
}

// A runner that gives up must not take the resource with it. Nothing here
// recovers a panic, so this is about the shape of the code rather than a state
// the service reaches: what is released on the way out stays released.
func TestAnAttemptGivesBackWhatItHeldWhenTheRunnerPanics(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name:      "a",
		Resources: exclusive("inventory"),
		Run:       RunnerFunc(func(context.Context, Invocation) Result { panic("the runner gave up") }),
	}), "Register()")

	entry := manager.tasks["a"]
	invocation := Invocation{Task: "a"}
	release, outcome := manager.admit(entry, invocation)
	require.Equal(t, admitStarted, outcome, "admit()")

	func() {
		defer func() {
			assert.NotNil(t, recover(), "the runner did not give up after all")
		}()
		manager.perform(t.Context(), entry, invocation, release, nil, 0)
	}()

	assert.False(t, manager.Holding("inventory"), "a runner that gave up kept the resource")
	_, again := manager.admit(entry, invocation)
	assert.Equal(t, admitStarted, again, "a runner that gave up stayed listed as working")
}

// One chain shares one set of what it has run, so a task named twice in the
// same list runs once rather than once per naming.
func TestAChainRunsAnInvocationOnceHoweverManyLinksNameIt(t *testing.T) {
	t.Parallel()

	followed := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "child"}, {Task: "child"}}}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{Name: "child", Run: followed}), "Register(child)")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, followed.runs(), "a task named twice in one chain ran more than once")
}

// A link one branch reaches is a link every later branch has already had, so a
// sibling naming it again finds it run.
func TestAChainSharesWhatItHasRunAcrossBranches(t *testing.T) {
	t.Parallel()

	shared := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "first"}, {Task: "shared"}}}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{
		Name: "first",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded, Next: []Link{{Task: "shared"}}}
		}),
	}), "Register(first)")
	require.NoError(t, manager.Register(&Definition{Name: "shared", Run: shared}), "Register(shared)")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, shared.runs(), "a link two branches reached ran twice")
}
