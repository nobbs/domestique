package task

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
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
		"no alert window": {
			Name: "a", Run: succeeds(), Notify: &Notify{Title: "Domestique sync failed"},
		},
		"no alert title": {
			Name: "a", Run: succeeds(), Notify: &Notify{Suppress: time.Hour},
		},
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

	cancel()
	<-stopped
	assert.Zero(t, runner.runs(), "a run happened before the initial delay fired")
}

func TestNextRunAtReportsTheNextTickOnceTheFirstRunHasCompleted(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, _ := newTestManager(t)
	manager.now = reference
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

	// The loop asking for the gap from the first run's due instant is proof the
	// first run has already completed and the next tick has been published.
	assert.Equal(t, 61*time.Minute, <-waits, "the wait after the first run")
	require.Equal(t, 1, runner.runs(), "the first run")
	due, holding := manager.NextRunAt("a")
	assert.True(t, holding, "NextRunAt() after the first run")
	assert.Equal(t, reference().Add(61*time.Minute), due, "NextRunAt() reports the next tick, not the first")

	cancel()
	<-stopped
}

func TestNextRunAtIsZeroWhileTheScheduledAttemptIsInFlight(t *testing.T) {
	t.Parallel()

	runner := blockOn()
	manager, _ := newTestManager(t)
	fired := make(chan time.Time)
	manager.after = func(time.Duration) <-chan time.Time { return fired }
	require.NoError(t, manager.Register(&Definition{
		Name:         "a",
		Run:          runner,
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return time.Minute },
	}), "Register()")

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); manager.Run(ctx) }()

	require.Eventually(t, func() bool {
		_, holding := manager.NextRunAt("a")

		return holding
	}, time.Second, time.Millisecond, "the initial delay was never reported")

	fired <- reference()
	<-runner.started

	_, holding := manager.NextRunAt("a")
	assert.False(t, holding, "NextRunAt() while the run is in flight")

	close(runner.release)
	cancel()
	<-stopped
}

// An operator asking for a task does not reset its cadence: the schedule is
// still waiting out the gap it was already waiting out.
func TestNextRunAtSurvivesAnAttemptNobodyScheduled(t *testing.T) {
	t.Parallel()

	runner := blockOn()
	manager, _ := newTestManager(t)
	manager.now = reference
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

	require.Eventually(t, func() bool {
		_, holding := manager.NextRunAt("a")

		return holding
	}, time.Second, time.Millisecond, "the initial delay was never reported")

	require.True(t, manager.Trigger(ctx, "a", ""), "Trigger()")
	<-runner.started

	due, holding := manager.NextRunAt("a")
	assert.True(t, holding, "NextRunAt() while a triggered attempt is in flight")
	assert.Equal(t, reference().Add(time.Minute), due, "NextRunAt() during a triggered attempt")

	close(runner.release)
	cancel()
	<-stopped
	manager.Wait()
}

func TestNextRunAtIsZeroForATaskNothingSchedules(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: succeeds()}), "Register()")

	_, holding := manager.NextRunAt("a")
	assert.False(t, holding, "NextRunAt()")
}

func TestNextRunAtIsSilentAboutATaskNobodyRegistered(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	_, holding := manager.NextRunAt("absent")
	assert.False(t, holding, "NextRunAt()")
}

func TestTasksReportsTheFixedIntervalOnlyForAnEveryScheduledTask(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "fixed", Run: succeeds(),
		Schedule:     Every(func() time.Duration { return time.Hour }),
		InitialDelay: func() time.Duration { return 0 },
	}), "Register(fixed)")
	require.NoError(t, manager.Register(&Definition{
		Name: "calendar", Run: succeeds(),
		Schedule:     Daily{Zone: func() *time.Location { return time.UTC }, Hour: 3},
		InitialDelay: func() time.Duration { return 0 },
	}), "Register(calendar)")
	require.NoError(t, manager.Register(&Definition{Name: "unscheduled", Run: succeeds()}), "Register(unscheduled)")

	intervals := map[string]time.Duration{}
	for _, entry := range manager.Tasks() {
		intervals[entry.Name] = entry.Interval
	}
	assert.Equal(t, time.Hour, intervals["fixed"], "fixed")
	assert.Zero(t, intervals["calendar"], "calendar")
	assert.Zero(t, intervals["unscheduled"], "unscheduled")
}

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

// arguments records the argument each attempt was given, in the order seen.
type arguments struct {
	given []string
	mutex sync.Mutex
}

func argumentRecorder() *arguments { return &arguments{} }

func (a *arguments) Run(_ context.Context, invocation Invocation) Result {
	a.mutex.Lock()
	a.given = append(a.given, invocation.Argument)
	a.mutex.Unlock()

	return Result{Outcome: Succeeded}
}

func (a *arguments) arguments() []string {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return slices.Clone(a.given)
}

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

	for name, build := range map[string]func() (*Manager, error){
		"no store": func() (*Manager, error) {
			return NewManager(nil, &fakeNotifier{}, &fakeAlerts{}, alwaysOn)
		},
		"no notifier": func() (*Manager, error) {
			return NewManager(&recordingStore{}, nil, &fakeAlerts{}, alwaysOn)
		},
		"no decisions": func() (*Manager, error) {
			return NewManager(&recordingStore{}, &fakeNotifier{}, nil, alwaysOn)
		},
		"no switch": func() (*Manager, error) {
			return NewManager(&recordingStore{}, &fakeNotifier{}, &fakeAlerts{}, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := build()
			assert.Error(t, err, "NewManager()")
		})
	}
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
		{
			task: "a", argument: "slot", trigger: string(TriggerManual),
			outcome: string(Blocked), detail: "deletion_limit", retain: 12,
		},
	}, store.recorded(), "recorded runs")
}

func TestACancelledAttemptIsNotRecorded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager, store := newTestManager(t)
	entry := &registered{definition: Definition{Name: "a", Run: succeeds()}}

	manager.attempt(ctx, entry, Invocation{Task: "a"})
	assert.Empty(t, store.recorded(), "a cancelled attempt was recorded")
}

func TestARefusedAttemptIsRecordedAsSkipped(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "holder", Run: held, Resources: exclusive("inventory")}), "Register(holder)")
	require.NoError(t, manager.Register(&Definition{Name: "other", Run: succeeds(), Resources: exclusive("inventory")}), "Register(other)")

	require.True(t, manager.Trigger(t.Context(), "holder", ""), "Trigger(holder)")
	<-held.started
	require.False(t, manager.Trigger(t.Context(), "other", "slot"), "Trigger(other)")

	// The refusal is recorded off the caller's goroutine, so a "false" from
	// Trigger does not yet mean the row is written.
	require.Eventually(t, func() bool {
		return len(store.recorded()) > 0
	}, time.Second, time.Millisecond, "the refusal was never recorded")
	assert.Equal(t, []recordedRun{
		{
			task: "other", argument: "slot", trigger: string(TriggerManual), outcome: string(Skipped),
			detail: string(DetailHeld), retain: defaultRetainedRuns,
		},
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
		recordedRun{
			task: "scheduled", trigger: string(TriggerSchedule), outcome: string(Skipped),
			detail: string(DetailHeld), retain: defaultRetainedRuns,
		},
		"a refused scheduled run was not recorded")

	close(blocker.release)
	manager.Wait()
	cancel()
	<-stopped
}

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

	manager, store, _ := newAlertingManager(t)

	return manager, store
}

func newAlertingManager(t *testing.T) (*Manager, *recordingStore, *fakeNotifier) {
	t.Helper()

	manager, store, notifier, _ := newDecidingManager(t)

	return manager, store, notifier
}

func newDecidingManager(t *testing.T) (*Manager, *recordingStore, *fakeNotifier, *fakeAlerts) {
	t.Helper()

	store, notifier, decisions := &recordingStore{}, &fakeNotifier{}, &fakeAlerts{decided: map[Detail]bool{}}
	manager, err := NewManager(store, notifier, decisions, alwaysOn)
	require.NoError(t, err, "NewManager()")

	return manager, store, notifier, decisions
}

// fakeAlerts stands in for what an operator has ruled on.
type fakeAlerts struct {
	decided map[Detail]bool
}

func (a *fakeAlerts) Wanted(_ context.Context, _ string, alert Detail) (enabled, decided bool) {
	enabled, decided = a.decided[alert]

	return enabled, decided
}

func alwaysOn() bool { return true }

type sentAlert struct {
	title   string
	message string
}

type fakeNotifier struct {
	err   error
	sent  []sentAlert
	mutex sync.Mutex
}

func (n *fakeNotifier) Send(_ context.Context, title, message string) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.sent = append(n.sent, sentAlert{title: title, message: message})

	return n.err
}

func (n *fakeNotifier) messages() []sentAlert {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return slices.Clone(n.sent)
}

type recordedRun struct {
	task     string
	argument string
	trigger  string
	outcome  string
	detail   string
	retain   int
}

// faultStreak is what a task's recent history came to, as a backoff reads it.
type faultStreak struct {
	lastAt time.Time
	faults int
}

type recordingStore struct {
	err         error
	historyErr  error
	outcomeErr  error
	suppressErr error
	notifiedAt  map[string]time.Time
	succeededAt map[invocationKey]time.Time
	faultStreak map[invocationKey]faultStreak
	reference   string
	runs        []recordedRun
	mutex       sync.Mutex
}

func (s *recordingStore) RecordTaskRun(
	_ context.Context, task, argument, trigger string, startedAt, finishedAt time.Time,
	outcome, detail, reference string, retain int,
) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if finishedAt.Before(startedAt) {
		return errors.New("an attempt finished before it started")
	}
	s.reference = reference
	s.runs = append(s.runs, recordedRun{
		task: task, argument: argument, trigger: trigger, outcome: outcome, detail: detail, retain: retain,
	})

	return s.err
}

// LastTaskOutcome answers from what has been recorded here, so a test that
// records a failure and then a success gets the recovery it set up.
func (s *recordingStore) LastTaskOutcome(
	_ context.Context, task, argument string,
) (outcome string, found bool, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.outcomeErr != nil {
		return "", false, s.outcomeErr
	}
	for _, run := range slices.Backward(s.runs) {
		if run.task == task && run.argument == argument {
			return run.outcome, true, nil
		}
	}

	return "", false, nil
}

func (s *recordingStore) LastTaskSuccess(
	_ context.Context, task, argument string,
) (finishedAt time.Time, found bool, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.outcomeErr != nil {
		return time.Time{}, false, s.outcomeErr
	}
	at, found := s.succeededAt[invocationKey{task: task, argument: argument}]

	return at, found, nil
}

func (s *recordingStore) TaskFaultStreak(
	_ context.Context, task, argument string,
) (faults int, lastAt time.Time, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.outcomeErr != nil {
		return 0, time.Time{}, s.outcomeErr
	}
	if streak, held := s.faultStreak[invocationKey{task: task, argument: argument}]; held {
		return streak.faults, streak.lastAt, nil
	}

	return 0, time.Time{}, nil
}

func (s *recordingStore) LastFailureNotification(_ context.Context, category string) (time.Time, bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sentAt, found := s.notifiedAt[category]

	return sentAt, found, s.historyErr
}

func (s *recordingStore) RecordFailureNotification(_ context.Context, category string, sentAt time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.notifiedAt == nil {
		s.notifiedAt = make(map[string]time.Time)
	}
	// The zero time clears the record, the same as the store this stands in for.
	if sentAt.IsZero() {
		delete(s.notifiedAt, category)
	} else {
		s.notifiedAt[category] = sentAt
	}

	return s.suppressErr
}

func (s *recordingStore) recorded() []recordedRun {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return slices.Clone(s.runs)
}

func TestASuccessorTakesTheResourceItsPredecessorHeld(t *testing.T) {
	t.Parallel()

	followed := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name:      "parent",
		Resources: exclusive("inventory"),
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: followed, Resources: exclusive("inventory"), Follows: []string{"parent"},
	}), "Register(child)")
	require.NoError(t, manager.Resolve(), "Resolve()")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, followed.runs(), "the chained task never ran")
}

func TestASuccessorForWorkAlreadyUnderWayIsDroppedQuietly(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: held, Follows: []string{"parent"},
	}), "Register(child)")
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Resolve(), "Resolve()")

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
		assert.NotEqual(t, string(Skipped), run.outcome, "a coalesced successor was recorded as a refusal")
	}
	close(held.release)
	manager.Wait()
}

func TestASuccessorRefusedByAnotherHolderIsRecorded(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "holder", Run: held, Resources: exclusive("index")}), "Register(holder)")
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: succeeds(), Resources: exclusive("index"), Follows: []string{"parent"},
	}), "Register(child)")
	require.NoError(t, manager.Register(&Definition{Name: "parent", Run: succeeds()}), "Register(parent)")
	require.NoError(t, manager.Resolve(), "Resolve()")

	require.True(t, manager.Trigger(t.Context(), "holder", ""), "Trigger(holder)")
	<-held.started
	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger(parent)")

	require.Eventually(t, func() bool {
		return slices.Contains(store.recorded(), recordedRun{
			task: "child", trigger: string(TriggerChain), outcome: string(Skipped),
			detail: string(DetailHeld), retain: defaultRetainedRuns,
		})
	}, time.Second, time.Millisecond, "a refused successor was not recorded")

	close(held.release)
	manager.Wait()
}

func TestASuccessorRunsWhenItsPredecessorOnlyAdvanced(t *testing.T) {
	t.Parallel()

	followed := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Advances: true}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: followed, Follows: []string{"parent"},
	}), "Register(child)")
	require.NoError(t, manager.Resolve(), "Resolve()")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, followed.runs(), "a partial predecessor's successor never ran")
}

func TestAFanOutSuccessorBacksOffOnlyTheFaultingArgument(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
	seen := argumentRecorder()
	manager, store := newTestManager(t)
	manager.now = func() time.Time { return now }
	store.faultStreak = map[invocationKey]faultStreak{
		{task: "child", argument: "faulting"}: {faults: 5, lastAt: now},
	}
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded}
		}),
	}), "Register(parent)")
	require.NoError(t, manager.Register(&Definition{
		Name:    "child",
		Run:     seen,
		Follows: []string{"parent"},
		FanOut:  func() []string { return []string{"faulting", "healthy"} },
		Backoff: Backoff{Base: 30 * time.Second, Cap: 6 * time.Hour},
	}), "Register(child)")
	require.NoError(t, manager.Resolve(), "Resolve()")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, []string{"healthy"}, seen.arguments(), "backoff on one argument held back the other")
}

func TestAChainWillNotRunTheSameInvocationTwice(t *testing.T) {
	t.Parallel()

	runs := 0
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "loop",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			runs++

			return Result{Outcome: Succeeded}
		}),
	}), "Register()")
	// The graph a correct Resolve refuses, reached behind its back: the set of
	// what this chain has run is what has to stop it.
	manager.tasks["loop"].successors = []string{"loop"}

	require.True(t, manager.Trigger(t.Context(), "loop", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, 1, runs, "a task chained to itself ran again")
}

func TestResolveRefusesAGraphThatFollowsItself(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "first", Run: succeeds(), Follows: []string{"second"},
	}), "Register(first)")
	require.NoError(t, manager.Register(&Definition{
		Name: "second", Run: succeeds(), Follows: []string{"first"},
	}), "Register(second)")

	require.ErrorContains(t, manager.Resolve(), "follows itself", "Resolve()")
}

func TestResolveRefusesAnEdgeToATaskNobodyRegistered(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: succeeds(), Follows: []string{"absent"},
	}), "Register(child)")

	require.ErrorContains(t, manager.Resolve(), "nothing registers", "Resolve()")
}

func TestATaskWithNothingFollowingItRecordsOnlyItself(t *testing.T) {
	t.Parallel()

	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "parent",
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Succeeded}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
	manager.Wait()
	assert.Equal(t, []recordedRun{
		{task: "parent", trigger: string(TriggerManual), outcome: string(Succeeded), retain: defaultRetainedRuns},
	}, store.recorded(), "recorded runs")
}

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

func TestATaskFollowingSeveralRunsAfterEachOfThem(t *testing.T) {
	t.Parallel()

	shared := countingRunner()
	manager, _ := newTestManager(t)
	for _, name := range []string{"first", "second"} {
		require.NoError(t, manager.Register(&Definition{Name: name, Run: succeeds()}), "Register()")
	}
	require.NoError(t, manager.Register(&Definition{
		Name: "shared", Run: shared, Follows: []string{"first", "second"},
	}), "Register(shared)")
	require.NoError(t, manager.Resolve(), "Resolve()")

	require.True(t, manager.Trigger(t.Context(), "first", ""), "Trigger(first)")
	manager.Wait()
	require.True(t, manager.Trigger(t.Context(), "second", ""), "Trigger(second)")
	manager.Wait()

	assert.Equal(t, 2, shared.runs(), "a task following two predecessors did not run after each")
}

func TestResolvingTwiceDescribesTheSameGraph(t *testing.T) {
	t.Parallel()

	following := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "first", Run: succeeds()}), "Register(first)")
	require.NoError(t, manager.Register(&Definition{
		Name: "following", Run: following, Follows: []string{"first"},
	}), "Register(following)")
	require.NoError(t, manager.Resolve(), "Resolve()")
	require.NoError(t, manager.Resolve(), "Resolve() a second time")

	require.True(t, manager.Trigger(t.Context(), "first", ""), "Trigger(first)")
	manager.Wait()

	assert.Equal(t, 1, following.runs(), "a successor ran once per resolution")
}

func TestNothingFollowsAnAttemptThatDidNotSucceed(t *testing.T) {
	t.Parallel()

	tests := map[string]Outcome{"failed": Failed, "found nothing new": Unchanged, "not ready": NotReady}
	for name, outcome := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			followed := countingRunner()
			manager, _ := newTestManager(t)
			came := outcome
			require.NoError(t, manager.Register(&Definition{
				Name: "parent",
				Run: RunnerFunc(func(context.Context, Invocation) Result {
					return Result{Outcome: came}
				}),
			}), "Register(parent)")
			require.NoError(t, manager.Register(&Definition{
				Name: "child", Run: followed, Follows: []string{"parent"},
			}), "Register(child)")
			require.NoError(t, manager.Resolve(), "Resolve()")

			require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger()")
			manager.Wait()

			assert.Zero(t, followed.runs(), "something followed an attempt that did not succeed")
		})
	}
}

func TestARefusalSaysWhichKindOfBusyStoppedIt(t *testing.T) {
	t.Parallel()

	held := blockOn()
	manager, store := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: held, Resources: exclusive("inventory")}), "Register(a)")
	require.NoError(t, manager.Register(&Definition{Name: "b", Run: succeeds(), Resources: exclusive("inventory")}), "Register(b)")

	require.True(t, manager.Trigger(t.Context(), "a", "slot"), "Trigger(a)")
	<-held.started

	assert.False(t, manager.Trigger(t.Context(), "a", "slot"), "the same work was accepted twice")
	assert.False(t, manager.Trigger(t.Context(), "b", ""), "an unrelated task took a held resource")

	// Both refusals are recorded off the caller's goroutine and in no
	// particular order relative to each other, so "false" from Trigger does
	// not yet mean either row is written.
	require.Eventually(t, func() bool {
		return len(store.recorded()) >= 2
	}, time.Second, time.Millisecond, "both refusals were never recorded")
	assert.ElementsMatch(t, []recordedRun{
		{
			task: "a", argument: "slot", trigger: string(TriggerManual), outcome: string(Skipped),
			detail: string(DetailWorking), retain: defaultRetainedRuns,
		},
		{
			task: "b", trigger: string(TriggerManual), outcome: string(Skipped),
			detail: string(DetailHeld), retain: defaultRetainedRuns,
		},
	}, store.recorded(), "recorded refusals")

	close(held.release)
	manager.Wait()
}

func TestACoalescedSuccessorCountsAsRunForTheRestOfTheChain(t *testing.T) {
	t.Parallel()

	var childRuns sync.Mutex
	runs := 0
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	child := RunnerFunc(func(context.Context, Invocation) Result {
		childRuns.Lock()
		runs++
		first := runs == 1
		childRuns.Unlock()

		if first {
			close(started)
			<-release
			close(finished)
		}

		return Result{Outcome: Succeeded}
	})

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "child", Run: child, Follows: []string{"parent"},
	}), "Register(child)")
	require.NoError(t, manager.Register(&Definition{
		Name:    "sibling",
		Follows: []string{"parent"},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			// Let the held run finish, so the successor below is asked after it ends.
			close(release)
			<-finished

			return Result{Outcome: Succeeded}
		}),
	}), "Register(sibling)")
	require.NoError(t, manager.Register(&Definition{Name: "parent", Run: succeeds()}), "Register(parent)")
	require.NoError(t, manager.Resolve(), "Resolve()")

	require.True(t, manager.Trigger(t.Context(), "child", ""), "Trigger(child)")
	<-started
	require.True(t, manager.Trigger(t.Context(), "parent", ""), "Trigger(parent)")
	manager.Wait()

	childRuns.Lock()
	defer childRuns.Unlock()
	assert.Equal(t, 1, runs, "a coalesced successor was asked for again once its work had finished")
}

func TestAFailingTaskIsAnnouncedOncePerWindow(t *testing.T) {
	t.Parallel()

	manager, _, notifier := newAlertingManager(t)
	now := reference()
	manager.now = func() time.Time { return now }
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: 6 * time.Hour,
			Alerts: []Detail{"destination", "source"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "first Trigger()")
	manager.Wait()
	now = now.Add(5 * time.Hour)
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "second Trigger()")
	manager.Wait()
	now = now.Add(2 * time.Hour)
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "third Trigger()")
	manager.Wait()

	sent := notifier.messages()
	require.Len(t, sent, 2, "sent alerts")
	for _, alert := range sent {
		assert.Equal(t, "Domestique sync failed", alert.title, "alert title")
		assert.Regexp(t, `^sync failed: destination run=[0-9a-f]{12}$`, alert.message, "alert message")
	}
}

func TestTwoReasonsAreAnnouncedSeparately(t *testing.T) {
	t.Parallel()

	manager, _, notifier := newAlertingManager(t)
	detail := Detail("source")
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: 6 * time.Hour,
			Alerts: []Detail{"destination", "source"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: detail}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "first Trigger()")
	manager.Wait()
	detail = "destination"
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "second Trigger()")
	manager.Wait()

	assert.Len(t, notifier.messages(), 2, "a second reason was silenced by the first")
}

func TestTwoArgumentsFailingForTheSameReasonAreAnnouncedSeparately(t *testing.T) {
	t.Parallel()

	manager, _, notifier := newAlertingManager(t)
	argument := "rider-a"
	require.NoError(t, manager.Register(&Definition{
		Name: "sync:target",
		Notify: &Notify{
			Title: "Domestique target failed", Suppress: 6 * time.Hour,
			Alerts: []Detail{"destination"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync:target", argument), "first Trigger()")
	manager.Wait()
	argument = "rider-b"
	require.True(t, manager.Trigger(t.Context(), "sync:target", argument), "second Trigger()")
	manager.Wait()

	assert.Len(t, notifier.messages(), 2, "one slot's failure silenced another slot's")
}

func TestASuccessClearsTheStaleWindowOnlyForItsOwnArgument(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	store.notifiedAt = map[string]time.Time{
		"sync:target:rider-a:stale": time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC),
		"sync:target:rider-b:stale": time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC),
	}
	require.NoError(t, manager.Register(&Definition{
		Name:       "sync:target",
		Notify:     &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailStale}},
		StaleAfter: func() time.Duration { return 24 * time.Hour },
		Run:        succeeds(),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync:target", "rider-a"), "Trigger()")
	manager.Wait()

	_, riderAHeld := store.notifiedAt["sync:target:rider-a:stale"]
	_, riderBHeld := store.notifiedAt["sync:target:rider-b:stale"]
	assert.False(t, riderAHeld, "a success left its own stale window open")
	assert.True(t, riderBHeld, "a success on one argument cleared another argument's stale window")
	assert.Empty(t, notifier.messages(), "a success was announced as stale")
}

func TestAnAlertNamesTheArgumentItIsAbout(t *testing.T) {
	t.Parallel()

	manager, _, notifier := newAlertingManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "sync:target",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: time.Hour,
			Alerts: []Detail{"destination"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Blocked, Detail: "deletion_limit"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync:target", "rider-a"), "Trigger()")
	manager.Wait()

	sent := notifier.messages()
	require.Len(t, sent, 1, "sent alerts")
	assert.Equal(t, "Domestique sync failed", sent[0].title, "alert title")
	assert.Regexp(t, `^sync:target rider-a blocked: deletion_limit run=[0-9a-f]{12}$`,
		sent[0].message, "alert message")
}

func TestAnAlertThatCouldNotBeSentIsTriedAgain(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	notifier.err = errors.New("channel unreachable")
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: 6 * time.Hour,
			Alerts: []Detail{"destination", "source"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Empty(t, store.notifiedAt, "a message that never went out was recorded as sent")
}

func TestOnlyATaskThatDeclaredAlertsIsAnnouncedAbout(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition Definition
		want       int
	}{
		"a refusal did no work": {
			definition: Definition{
				Name:   "a",
				Notify: &Notify{Title: "t", Suppress: time.Hour},
				Run: RunnerFunc(func(context.Context, Invocation) Result {
					return Result{Outcome: Skipped, Detail: DetailHeld}
				}),
			},
		},
		"a success nobody asked to hear about": {
			definition: Definition{
				Name:   "a",
				Notify: &Notify{Title: "t", Suppress: time.Hour},
				Run:    succeeds(),
			},
		},
		"a success that was declared": {
			definition: Definition{
				Name:   "a",
				Notify: &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailSucceeded}},
				Run:    succeeds(),
			},
			want: 1,
		},
		"a task that declared nothing": {
			definition: Definition{
				Name: "a",
				Run: RunnerFunc(func(context.Context, Invocation) Result {
					return Result{Outcome: Failed, Detail: "destination"}
				}),
			},
		},
		"a fault that did declare them": {
			definition: Definition{
				Name:   "a",
				Notify: &Notify{Title: "t", Suppress: time.Hour},
				Run: RunnerFunc(func(context.Context, Invocation) Result {
					return Result{Outcome: Failed, Detail: "destination"}
				}),
			},
			want: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			manager, _, notifier := newAlertingManager(t)
			require.NoError(t, manager.Register(&test.definition), "Register()")
			require.True(t, manager.Trigger(t.Context(), "a", ""), "Trigger()")
			manager.Wait()

			assert.Len(t, notifier.messages(), test.want, "sent alerts")
		})
	}
}

func TestNothingIsAnnouncedOrRecordedWhileTheChannelIsOff(t *testing.T) {
	t.Parallel()

	store, notifier := &recordingStore{}, &fakeNotifier{}
	manager, err := NewManager(store, notifier, &fakeAlerts{}, func() bool { return false })
	require.NoError(t, err, "NewManager()")
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: 6 * time.Hour,
			Alerts: []Detail{"destination", "source"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Empty(t, notifier.messages(), "a message went out while the channel was off")
	assert.Empty(t, store.notifiedAt, "a suppression window was opened while the channel was off")
}

func TestAnAlertIsHeldBackWhenItsHistoryCannotBeRead(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	store.historyErr = errors.New("state unavailable")
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: time.Hour,
			Alerts: []Detail{"destination"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Empty(t, notifier.messages(), "an alert went out on an unreadable history")
}

func TestAnAlertStillGoesOutWhenItsSuppressionCannotBeRecorded(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	store.suppressErr = errors.New("state unavailable")
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: time.Hour,
			Alerts: []Detail{"destination"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Len(t, notifier.messages(), 1, "the alert was lost with its suppression record")
}

func TestEachAttemptIsNamedSomethingOfItsOwn(t *testing.T) {
	t.Parallel()

	manager, store, _ := newAlertingManager(t)
	require.NoError(t, manager.Register(&Definition{Name: "a", Run: succeeds()}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "a", ""), "first Trigger()")
	manager.Wait()
	first := store.reference

	require.True(t, manager.Trigger(t.Context(), "a", ""), "second Trigger()")
	manager.Wait()

	assert.Regexp(t, `^[0-9a-f]{12}$`, first, "a run was named something unexpected")
	assert.NotEqual(t, first, store.reference, "two runs shared a name")
}

func TestAnOperatorsDecisionDecidesWhetherAnAlertGoesOut(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		decided map[Detail]bool
		want    int
	}{
		"nobody has ruled on it": {decided: map[Detail]bool{}, want: 1},
		"switched on":            {decided: map[Detail]bool{"destination": true}, want: 1},
		"switched off":           {decided: map[Detail]bool{"destination": false}, want: 0},
		"another alert switched off": {
			decided: map[Detail]bool{"source": false}, want: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			manager, _, notifier, decisions := newDecidingManager(t)
			decisions.decided = test.decided
			require.NoError(t, manager.Register(&Definition{
				Name: "sync",
				Notify: &Notify{
					Title: "Domestique sync failed", Suppress: time.Hour,
					Alerts: []Detail{"destination"},
				},
				Run: RunnerFunc(func(context.Context, Invocation) Result {
					return Result{Outcome: Failed, Detail: "destination"}
				}),
			}), "Register()")

			require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
			manager.Wait()

			assert.Len(t, notifier.messages(), test.want, "sent alerts")
		})
	}
}

func TestAnAlertSwitchedOffOpensNoWindow(t *testing.T) {
	t.Parallel()

	manager, store, _, decisions := newDecidingManager(t)
	decisions.decided = map[Detail]bool{"destination": false}
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: time.Hour,
			Alerts: []Detail{"destination"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Empty(t, store.notifiedAt, "a suppression window was opened for an alert nobody wanted")
}

func TestAnUndeclaredAlertStillGoesOut(t *testing.T) {
	t.Parallel()

	manager, _, notifier := newAlertingManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: time.Hour,
			Alerts: []Detail{"source"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "never_declared"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Len(t, notifier.messages(), 1, "an undeclared alert was swallowed")
}

func TestAnUndeclaredAlertIsReportedEvenWhenNobodyWantsIt(t *testing.T) {
	t.Parallel()

	manager, _, notifier, decisions := newDecidingManager(t)
	decisions.decided = map[Detail]bool{"never_declared": false}
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title: "Domestique sync failed", Suppress: time.Hour,
			Alerts: []Detail{"source"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "never_declared"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "first Trigger()")
	manager.Wait()
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "second Trigger()")
	manager.Wait()

	assert.Empty(t, notifier.messages(), "an alert nobody wanted was sent")
	assert.Len(t, manager.undeclared, 1, "the missing declaration was not noticed")
}

func TestDeclarationsListEveryAlertInRegistrationOrder(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Run:  countingRunner(),
		Notify: &Notify{
			Title:    "Sync failed",
			Alerts:   []Detail{"source", "destination"},
			Suppress: time.Hour,
		},
	}), "Register(sync)")
	require.NoError(t, manager.Register(&Definition{Name: "quiet", Run: countingRunner()}), "Register(quiet)")
	require.NoError(t, manager.Register(&Definition{
		Name:   "surface:index",
		Run:    countingRunner(),
		Notify: &Notify{Title: "Index failed", Alerts: []Detail{"build"}, Suppress: time.Hour},
	}), "Register(surface:index)")

	assert.Equal(t, []Declaration{
		{Task: "sync", Alert: "source"},
		{Task: "sync", Alert: "destination"},
		{Task: "surface:index", Alert: "build"},
	}, manager.Declarations(), "Declarations()")
}

func TestASuccessAfterAFaultIsAnnouncedAsARecovery(t *testing.T) {
	t.Parallel()

	manager, _, notifier := newAlertingManager(t)
	outcome := Failed
	require.NoError(t, manager.Register(&Definition{
		Name: "sync",
		Notify: &Notify{
			Title:    "t",
			Suppress: time.Hour,
			Alerts:   []Detail{DetailSucceeded, DetailRecovered, "destination"},
		},
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: outcome, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger(failing)")
	manager.Wait()
	outcome = Succeeded
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger(recovering)")
	manager.Wait()
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger(routine)")
	manager.Wait()

	messages := notifier.messages()
	require.Len(t, messages, 3, "sent alerts")
	assert.Regexp(t, `^sync failed: destination run=[0-9a-f]{12}$`, messages[0].message, "the fault")
	assert.Regexp(t, `^sync succeeded: recovered run=[0-9a-f]{12}$`, messages[1].message, "the recovery")
	assert.Regexp(t, `^sync succeeded run=[0-9a-f]{12}$`, messages[2].message, "the routine success")
}

func TestSilencingRoutineSuccessesLeavesTheRecovery(t *testing.T) {
	t.Parallel()

	manager, store, notifier, decisions := newDecidingManager(t)
	decisions.decided[DetailSucceeded] = false
	store.runs = []recordedRun{{task: "sync", outcome: string(Failed)}}
	require.NoError(t, manager.Register(&Definition{
		Name:   "sync",
		Notify: &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailSucceeded, DetailRecovered}},
		Run:    succeeds(),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger(recovering)")
	manager.Wait()
	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger(routine)")
	manager.Wait()

	messages := notifier.messages()
	require.Len(t, messages, 1, "the routine success was announced or the recovery was not")
	assert.Regexp(t, `^sync succeeded: recovered run=[0-9a-f]{12}$`, messages[0].message, "the recovery")
}

func TestAnUnreadableHistoryAnnouncesTheSuccessAsARecovery(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	store.outcomeErr = errors.New("state unavailable")
	require.NoError(t, manager.Register(&Definition{
		Name:   "sync",
		Notify: &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailSucceeded, DetailRecovered}},
		Run:    succeeds(),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	messages := notifier.messages()
	require.Len(t, messages, 1, "sent alerts")
	assert.Contains(t, messages[0].message, "recovered", "an unreadable history withheld the recovery")
}

func TestATaskThatHasNotSucceededInTooLongIsAnnouncedAsStale(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	now := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	store.succeededAt = map[invocationKey]time.Time{{task: "sync"}: now.Add(-25 * time.Hour)}
	require.NoError(t, manager.Register(&Definition{
		Name:       "sync",
		Notify:     &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailStale, "destination"}},
		StaleAfter: func() time.Duration { return 24 * time.Hour },
		Run: RunnerFunc(func(context.Context, Invocation) Result {
			return Result{Outcome: Failed, Detail: "destination"}
		}),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	messages := notifier.messages()
	require.Len(t, messages, 2, "sent alerts")
	assert.Regexp(t, `^sync failed: destination run=[0-9a-f]{12}$`, messages[0].message, "the fault")
	assert.Regexp(t, `^sync stale run=[0-9a-f]{12}$`, messages[1].message, "the staleness")
}

func TestATaskWithinItsBoundIsNotAnnouncedAsStale(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		succeeded map[invocationKey]time.Time
		bound     time.Duration
	}{
		"inside the bound": {
			bound:     24 * time.Hour,
			succeeded: map[invocationKey]time.Time{{task: "sync"}: time.Date(2026, time.August, 29, 10, 0, 0, 0, time.UTC)},
		},
		// Nothing is stale before it has ever been fresh: a task nobody has run
		// yet is waiting, not overdue.
		"never succeeded": {bound: 24 * time.Hour},
		"no bound at all": {
			succeeded: map[invocationKey]time.Time{{task: "sync"}: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			manager, store, notifier := newAlertingManager(t)
			manager.now = func() time.Time { return time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC) }
			store.succeededAt = test.succeeded
			bound := test.bound
			require.NoError(t, manager.Register(&Definition{
				Name:       "sync",
				Notify:     &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailStale}},
				StaleAfter: func() time.Duration { return bound },
				Run: RunnerFunc(func(context.Context, Invocation) Result {
					return Result{Outcome: Failed, Detail: "destination"}
				}),
			}), "Register()")

			require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
			manager.Wait()

			assert.NotContains(t, sentAlerts(notifier), "stale", "the task was announced as stale")
		})
	}
}

func TestASuccessClearsTheStaleWindow(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	store.notifiedAt = map[string]time.Time{"sync::stale": time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)}
	require.NoError(t, manager.Register(&Definition{
		Name:       "sync",
		Notify:     &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{DetailStale}},
		StaleAfter: func() time.Duration { return 24 * time.Hour },
		Run:        succeeds(),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	_, held := store.notifiedAt["sync::stale"]
	assert.False(t, held, "a success left the stale window open")
	assert.NotContains(t, sentAlerts(notifier), "stale", "a success was announced as stale")
}

// sentAlerts is every message that went out, joined, for the assertions that
// care only whether one reason was among them.
func sentAlerts(notifier *fakeNotifier) string {
	sent := ""
	for _, message := range notifier.messages() {
		sent += message.message + "\n"
	}

	return sent
}

func TestATaskThatDeclaredOnlyItsFaultsAnnouncesNoSuccess(t *testing.T) {
	t.Parallel()

	manager, store, notifier := newAlertingManager(t)
	store.succeededAt = map[invocationKey]time.Time{
		{task: "surface:index"}: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, manager.Register(&Definition{
		Name:   "surface:index",
		Notify: &Notify{Title: "t", Suppress: time.Hour, Alerts: []Detail{"build"}},
		// A bound with nothing declared against it says nothing either.
		StaleAfter: func() time.Duration { return time.Hour },
		Run:        succeeds(),
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "surface:index", ""), "Trigger()")
	manager.Wait()

	assert.Empty(t, notifier.messages(), "sent alerts")
}

func TestOnlyAFixedGapRunsAsSoonAsItStarts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		schedule Schedule
		want     int
	}{
		"a fixed gap":       {schedule: Every(func() time.Duration { return time.Hour }), want: 1},
		"a daily calendar":  {schedule: Daily{Hour: 2}},
		"a weekly calendar": {schedule: Weekly{Weekday: time.Monday, Hour: 2}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			runner := countingRunner()
			manager, _ := newTestManager(t)
			// The initial delay elapses at once; the wait after it never does.
			// Reaching that second wait is what says the schedule has decided
			// whether to run at start, so the count is read then rather than
			// after a delay chosen by guesswork.
			var waits atomic.Int64
			waiting := make(chan struct{}, 1)
			manager.after = func(time.Duration) <-chan time.Time {
				if waits.Add(1) == 1 {
					elapsed := make(chan time.Time, 1)
					elapsed <- time.Time{}

					return elapsed
				}
				select {
				case waiting <- struct{}{}:
				default:
				}

				return make(chan time.Time)
			}
			require.NoError(t, manager.Register(&Definition{
				Name:         "a",
				Run:          runner,
				Schedule:     test.schedule,
				InitialDelay: func() time.Duration { return 0 },
			}), "Register()")

			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); manager.Run(ctx) }()
			select {
			case <-waiting:
			case <-time.After(time.Second):
				t.Fatal("the schedule never reached the wait after its initial delay")
			}
			runs := runner.runs()
			cancel()
			<-done

			assert.Equal(t, test.want, runs, "runs at start")
		})
	}
}

func TestAFailingTaskIsHeldBackFromItsSchedule(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		streak  faultStreak
		wantRun bool
	}{
		"nothing has faulted":         {wantRun: true},
		"one fault, still waiting":    {streak: faultStreak{faults: 1, lastAt: now.Add(-10 * time.Second)}},
		"one fault, wait served":      {streak: faultStreak{faults: 1, lastAt: now.Add(-40 * time.Second)}, wantRun: true},
		"three faults, still waiting": {streak: faultStreak{faults: 3, lastAt: now.Add(-90 * time.Second)}},
		"three faults, wait served":   {streak: faultStreak{faults: 3, lastAt: now.Add(-3 * time.Minute)}, wantRun: true},
		// The doubling stops at the cap rather than reaching days by morning.
		"far past the cap": {streak: faultStreak{faults: 40, lastAt: now.Add(-7 * time.Hour)}, wantRun: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			runner := countingRunner()
			manager, store := newTestManager(t)
			manager.now = func() time.Time { return now }
			store.faultStreak = map[invocationKey]faultStreak{{task: "sync"}: test.streak}
			entry := &registered{definition: Definition{
				Name:    "sync",
				Run:     runner,
				Backoff: Backoff{Base: 30 * time.Second, Cap: 6 * time.Hour},
			}}
			manager.tasks["sync"] = entry
			manager.order = append(manager.order, "sync")

			manager.scheduled(t.Context(), entry)

			want := 0
			if test.wantRun {
				want = 1
			}
			assert.Equal(t, want, runner.runs(), "attempts")
		})
	}
}

func TestABackoffNeverRefusesAnOperator(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 31, 9, 0, 0, 0, time.UTC)
	runner := countingRunner()
	manager, store := newTestManager(t)
	manager.now = func() time.Time { return now }
	store.faultStreak = map[invocationKey]faultStreak{{task: "sync"}: {faults: 5, lastAt: now}}
	require.NoError(t, manager.Register(&Definition{
		Name:    "sync",
		Run:     runner,
		Backoff: Backoff{Base: 30 * time.Second, Cap: 6 * time.Hour},
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Equal(t, 1, runner.runs(), "a backoff refused the operator who asked")
}

func TestAnUnreadableHistoryDoesNotHoldATaskBack(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, store := newTestManager(t)
	store.outcomeErr = errors.New("state unavailable")
	entry := &registered{definition: Definition{
		Name: "sync", Run: runner, Backoff: Backoff{Base: time.Hour, Cap: 6 * time.Hour},
	}}
	manager.tasks["sync"] = entry

	manager.scheduled(t.Context(), entry)

	assert.Equal(t, 1, runner.runs(), "an unreadable history held the task back")
}

func TestRegisterRefusesABackoffWithNoCap(t *testing.T) {
	t.Parallel()

	manager, _ := newTestManager(t)

	require.Error(t, manager.Register(&Definition{
		Name: "sync", Run: succeeds(), Backoff: Backoff{Base: time.Minute},
	}), "Register() accepted an uncapped backoff")
}

func TestBackoffDoublesToItsCap(t *testing.T) {
	t.Parallel()

	backoff := Backoff{Base: 30 * time.Second, Cap: 2 * time.Minute}
	for faults, want := range map[int]time.Duration{
		0: 0,
		1: 30 * time.Second,
		2: time.Minute,
		3: 2 * time.Minute,
		4: 2 * time.Minute,
		9: 2 * time.Minute,
	} {
		assert.Equalf(t, want, backoff.delay(faults), "delay(%d)", faults)
	}
	assert.Zero(t, Backoff{}.delay(3), "a task with no backoff waited")
}

func TestASwitchedOffTaskSkipsItsTickAndResumes(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, store := newTestManager(t)
	on := false
	entry := &registered{definition: Definition{
		Name:    "sync",
		Run:     runner,
		Enabled: func() bool { return on },
	}}

	manager.scheduled(t.Context(), entry)
	assert.Zero(t, runner.runs(), "a switched-off task ran")
	assert.Empty(t, store.recorded(), "a switched-off tick was recorded")

	on = true
	manager.scheduled(t.Context(), entry)
	assert.Equal(t, 1, runner.runs(), "switching a task back on did not resume it")
}

func TestASwitchedOffTaskStillRunsWhenAskedFor(t *testing.T) {
	t.Parallel()

	runner := countingRunner()
	manager, _ := newTestManager(t)
	require.NoError(t, manager.Register(&Definition{
		Name:    "sync",
		Run:     runner,
		Enabled: func() bool { return false },
	}), "Register()")

	require.True(t, manager.Trigger(t.Context(), "sync", ""), "Trigger()")
	manager.Wait()

	assert.Equal(t, 1, runner.runs(), "a switched-off task refused the operator who asked")
}
