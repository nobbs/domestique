package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/task"
)

func TestInventoryTasksAllHoldTheInventoryExclusively(t *testing.T) {
	t.Parallel()

	definitions := inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled)

	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
		assert.Equal(t,
			[]task.Resource{{Name: resourceInventory, Exclusive: true}},
			definition.Resources(""),
			definition.Name+" resources",
		)
	}
	assert.Equal(t, []string{taskSyncSource, taskSyncTarget, taskSyncClear, taskSurfaceAnnotate}, names, "registered tasks")
}

// The read runs unasked, and so do the targets — the second as a backstop
// behind the read that asks for them. Clearing a slot and classifying are
// things an operator asks for.
func TestOnlyTheReadAndTheTargetsAreScheduled(t *testing.T) {
	t.Parallel()

	scheduled := map[string]bool{taskSyncSource: true, taskSyncTarget: true}
	for _, definition := range inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled) {
		if scheduled[definition.Name] {
			assert.NotNilf(t, definition.Schedule, "%s is not scheduled", definition.Name)

			continue
		}
		assert.Nilf(t, definition.Schedule, "%s runs unasked", definition.Name)
	}
}

func TestEachInventoryTaskRunsItsOwnWork(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		expect   func(*testing.T, *fakeSynchronizer)
		task     string
		argument string
	}{
		"one target": {
			task: taskSyncTarget, argument: "rider-a",
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, []string{"rider-a"}, s.reconciled, "reconciled targets")
			},
		},
		"one clear": {
			task: taskSyncClear, argument: "rider-b",
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, []string{"rider-b"}, s.cleared, "cleared targets")
			},
		},
		"classification": {
			task: taskSurfaceAnnotate,
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, 1, s.annotations, "annotation passes")
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			synchronizer := &fakeSynchronizer{result: syncservice.Result{Outcome: syncservice.OutcomeSucceeded}}
			definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled), test.task)

			result := definition.Run.Run(t.Context(), task.Invocation{Task: test.task, Argument: test.argument})
			assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
			test.expect(t, synchronizer)
		})
	}
}

func TestSyncResultCarriesEveryOutcomeAcross(t *testing.T) {
	t.Parallel()

	tests := map[syncservice.Outcome]task.Outcome{
		syncservice.OutcomeSucceeded: task.Succeeded,
		syncservice.OutcomeFailed:    task.Failed,
		syncservice.OutcomeBlocked:   task.Blocked,
		syncservice.OutcomeNotReady:  task.NotReady,
		syncservice.OutcomeSkipped:   task.Skipped,
	}
	for outcome, want := range tests {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()

			result := syncResult(&syncservice.Result{Outcome: outcome, Failure: syncservice.FailureState})
			assert.Equal(t, want, result.Outcome, "outcome")
			assert.Equal(t, task.Detail(syncservice.FailureState), result.Detail, "detail")
		})
	}
}

// An outcome this binary has never heard of is a failure rather than a success:
// nothing may read as done because it could not be understood.
func TestSyncResultTreatsAnUnknownOutcomeAsAFailure(t *testing.T) {
	t.Parallel()

	assert.Equal(t, task.Failed, syncResult(&syncservice.Result{Outcome: "invented"}).Outcome, "outcome")
}

func TestSurfaceIndexTaskHoldsOnlyItsOwnIndex(t *testing.T) {
	t.Parallel()

	builder := &fakeIndexBuilder{outcome: osmindex.Rebuilt}
	definition := surfaceIndexTask(builder, liveSettings(t), time.Time{})

	assert.Equal(t, taskSurfaceIndex, definition.Name, "name")
	assert.Equal(t,
		[]task.Resource{{Name: resourceSurfaceIndex, Exclusive: true}},
		definition.Resources(""),
		"resources",
	)
	require.NotNil(t, definition.Schedule, "the rebuild is not scheduled")

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskSurfaceIndex})
	assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
	assert.Equal(t, 1, builder.builds, "builds")
}

// A host that has never built waits out the floor rather than starting a
// memory-hungry build behind a process that has only just come up.
func TestSurfaceIndexTaskDelaysTheFirstBuildOfAHostThatHasNeverBuilt(t *testing.T) {
	t.Parallel()

	definition := surfaceIndexTask(&fakeIndexBuilder{}, liveSettings(t), time.Time{})

	assert.Equal(t, osmindex.InitialBuildDelay, definition.InitialDelay(), "InitialDelay()")
}

// liveSettings is what a service that has never been configured starts with,
// which is all these definitions read.
func liveSettings(t *testing.T) *runtimeconfig.Current {
	t.Helper()

	return testSettings(t, testStore(t, t.TempDir()))
}

func definitionNamed(t *testing.T, definitions []task.Definition, name string) task.Definition {
	t.Helper()

	for _, definition := range definitions {
		if definition.Name == name {
			return definition
		}
	}
	t.Fatalf("no task named %q was registered", name)

	return task.Definition{}
}

type fakeSynchronizer struct {
	phases      []syncservice.Phase
	reconciled  []string
	cleared     []string
	result      syncservice.Result
	scheduled   int
	both        int
	annotations int
}

func (s *fakeSynchronizer) Run(context.Context) syncservice.Result {
	s.scheduled++

	return s.result
}

func (s *fakeSynchronizer) RunBoth(context.Context) syncservice.Result {
	s.both++

	return s.result
}

func (s *fakeSynchronizer) RunPhase(_ context.Context, phase syncservice.Phase) syncservice.Result {
	s.phases = append(s.phases, phase)

	return s.result
}

func (s *fakeSynchronizer) ReconcileTarget(_ context.Context, targetID string) syncservice.Result {
	s.reconciled = append(s.reconciled, targetID)

	return s.result
}

func (s *fakeSynchronizer) ClearTarget(_ context.Context, targetID string) syncservice.Result {
	s.cleared = append(s.cleared, targetID)

	return s.result
}

func (s *fakeSynchronizer) Annotate(context.Context) { s.annotations++ }

type fakeIndexBuilder struct {
	err     error
	outcome osmindex.Outcome
	builds  int
}

func (b *fakeIndexBuilder) Run(context.Context) (osmindex.Outcome, error) {
	b.builds++

	return b.outcome, b.err
}

// The synchronization cadence is fixed by the rate limits it has to live
// within; only how long the first run waits after start is an operator's.
func TestSyncTaskRunsOnAFixedCadenceAfterASettingsDrivenFirstDelay(t *testing.T) {
	t.Parallel()

	settings := liveSettings(t)
	definition := definitionNamed(t, inventoryTasks(&fakeSynchronizer{}, settings, allEnabled), taskSyncSource)

	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, at.Add(syncservice.Interval), definition.Schedule.NextFire(at), "NextFire()")
	assert.Equal(t, settings.Values().Sync.InitialDelay, definition.InitialDelay(), "InitialDelay()")
}

func TestSurfaceIndexTaskReadsItsCadenceFromSettings(t *testing.T) {
	t.Parallel()

	settings := liveSettings(t)
	definition := surfaceIndexTask(&fakeIndexBuilder{}, settings, time.Time{})

	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, at.Add(settings.Values().Surface.RebuildInterval), definition.Schedule.NextFire(at), "NextFire()")
}

func TestSyncTaskRunsWhatWasAskedOfIt(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{result: syncservice.Result{Outcome: syncservice.OutcomeSucceeded}}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled), taskSyncSource)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskSyncSource, Trigger: task.TriggerSchedule})
	assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
	assert.Equal(t, []syncservice.Phase{syncservice.PhaseSource}, synchronizer.phases, "phases run")
}

// Activity is assembled from two places at once, and the status response is
// built from whatever both of them say at that moment.
func TestSyncSurfaceReportsActivityFromBothHalves(t *testing.T) {
	t.Parallel()

	due := time.Date(2026, time.August, 30, 9, 5, 0, 0, time.UTC)
	starter := &fakeStarter{holding: true, nextRunAt: due, held: true}
	reporter := &fakeSyncReporter{phase: syncservice.PhaseTargets, incomplete: 3}
	surface := syncSurface(starter, reporter, nil)

	assert.Equal(t, httpapi.SyncActivityState{
		StartsAt: due,
		Phase:    httpapi.SyncPhase(syncservice.PhaseTargets),
		Running:  true,
	}, surface.Activity(), "Activity()")
	assert.Equal(t, resourceInventory, starter.askedAbout, "the resource activity was read from")
	assert.Equal(t, 3, surface.SurfaceIncomplete(), "SurfaceIncomplete()")
}

type started struct {
	name     string
	argument string
}

type fakeStarter struct {
	nextRunAt  time.Time
	askedAbout string
	starts     []started
	accept     bool
	holding    bool
	held       bool
}

func (s *fakeStarter) Trigger(_ context.Context, name, argument string) bool {
	if !s.accept {
		return false
	}
	s.starts = append(s.starts, started{name: name, argument: argument})

	return true
}

func (s *fakeStarter) Holding(resource string) bool {
	s.askedAbout = resource

	return s.holding
}

func (s *fakeStarter) NextRunAt(string) (time.Time, bool) { return s.nextRunAt, s.held }

type fakeSyncReporter struct {
	phase      syncservice.Phase
	incomplete int
}

func (r *fakeSyncReporter) Running() (syncservice.Phase, bool) { return r.phase, r.phase != "" }
func (r *fakeSyncReporter) SurfaceIncomplete() int             { return r.incomplete }

// A rebuild reports what it came to so the history says whether the map moved,
// rather than only that something ran.
func TestIndexResultSeparatesABuildFromAnUpstreamThatHadNothingNew(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		outcome osmindex.Outcome
		err     error
		want    task.Result
	}{
		"rebuilt": {
			outcome: osmindex.Rebuilt,
			want: task.Result{
				Outcome: task.Succeeded,
				Next:    []task.Link{{Task: taskSurfaceAnnotate}},
			},
		},
		"nothing new upstream": {
			outcome: osmindex.Unchanged, want: task.Result{Outcome: task.Unchanged},
		},
		"no region configured": {
			outcome: osmindex.NoRegions, want: task.Result{Outcome: task.NotReady, Detail: detailNoRegions},
		},
		"failed": {
			err: errors.New("extract unreachable"), want: task.Result{Outcome: task.Failed, Detail: detailBuild},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, indexResult(test.outcome, test.err), "indexResult()")
		})
	}
}

func TestRegisterTasksRefusesADefinitionTheLayerCannotRun(t *testing.T) {
	t.Parallel()

	_, err := registerTasks(&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn, []task.Definition{{Name: "", Run: nil}})
	require.Error(t, err, "registerTasks()")
}

func TestRegisterTasksNeedsSomewhereToRecord(t *testing.T) {
	t.Parallel()

	_, err := registerTasks(nil, &silentNotifier{}, undecided{}, alwaysOn, nil)
	require.Error(t, err, "registerTasks()")
}

func TestRegisterTasksTakesEveryDefinitionItIsGiven(t *testing.T) {
	t.Parallel()

	definitions := append(inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled),
		surfaceIndexTask(&fakeIndexBuilder{}, liveSettings(t), time.Time{}))

	manager, err := registerTasks(&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn, definitions)
	require.NoError(t, err, "registerTasks()")
	for _, definition := range definitions {
		_, known := manager.NextRunAt(definition.Name)
		assert.False(t, known, definition.Name+" is holding a first run before anything started")
	}
}

// An outcome this binary has not heard of must not read as a success: the
// history would say the map moved when nothing here knows whether it did.
func TestIndexResultTreatsAnUnknownOutcomeAsAFailure(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		task.Result{Outcome: task.Failed, Detail: detailBuild},
		indexResult("invented", nil),
		"indexResult()",
	)
}

type countingStore struct{ runs int }

func (s *countingStore) RecordTaskRun(
	context.Context, string, string, time.Time, time.Time, string, string, string, int,
) error {
	s.runs++

	return nil
}

func (*countingStore) LastTaskOutcome(context.Context, string, string) (outcome string, found bool, err error) {
	return "", false, nil
}

func (*countingStore) LastTaskSuccess(context.Context, string, string) (finishedAt time.Time, found bool, err error) {
	return time.Time{}, false, nil
}

func (*countingStore) TaskFaultStreak(
	context.Context, string, string,
) (faults int, lastAt time.Time, err error) {
	return 0, time.Time{}, nil
}

func (*countingStore) LastFailureNotification(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (*countingStore) RecordFailureNotification(context.Context, string, time.Time) error { return nil }

// A refreshed inventory is worth reading the ground under again; a pass that
// stored nothing asks for nothing.
// A read that stored a library asks for the targets to be written and for the
// ground under the stages to be read again. One that stored nothing asks for
// neither, and a target reconciliation never asks for anything.
func TestSourceResultAsksForWhatFollowsAStoredInventory(t *testing.T) {
	t.Parallel()

	stored := sourceResult(&syncservice.Result{Outcome: syncservice.OutcomeSucceeded, SourceStored: true})
	assert.Equal(t, []task.Link{
		{Task: taskSyncTarget},
		{Task: taskSurfaceAnnotate},
	}, stored.Next, "what a stored inventory asked for")

	untouched := sourceResult(&syncservice.Result{Outcome: syncservice.OutcomeSucceeded})
	assert.Empty(t, untouched.Next, "a read that stored nothing asked for something anyway")

	reconciled := syncResult(&syncservice.Result{Outcome: syncservice.OutcomeSucceeded, SourceStored: true})
	assert.Empty(t, reconciled.Next, "a target reconciliation asked for something to follow it")
}

func alwaysOn() bool { return true }

type silentNotifier struct{}

func (*silentNotifier) Send(context.Context, string, string) error { return nil }

// undecided stands in for an operator who has ruled on nothing.
type undecided struct{}

func (undecided) Wanted(context.Context, string, task.Detail) (enabled, decided bool) {
	return false, false
}

// The surface a page reads is the manager's own registrations, so a task added
// to the layer appears without anybody writing an endpoint for it.
func TestTaskSurfaceListsWhatTheManagerRegisters(t *testing.T) {
	t.Parallel()

	manager, err := registerTasks(
		&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn,
		[]task.Definition{
			{
				Name:         "scheduled",
				Run:          task.RunnerFunc(func(context.Context, task.Invocation) task.Result { return task.Result{Outcome: task.Succeeded} }),
				Schedule:     task.Every(func() time.Duration { return time.Hour }),
				InitialDelay: func() time.Duration { return time.Minute },
			},
			{
				Name: "asked for",
				Run:  task.RunnerFunc(func(context.Context, task.Invocation) task.Result { return task.Result{Outcome: task.Succeeded} }),
			},
		},
	)
	require.NoError(t, err, "registerTasks()")

	surface := taskSurface{ctx: t.Context(), manager: manager}
	listed := surface.Registered()

	require.Len(t, listed, 2, "listed tasks")
	assert.Equal(t, "scheduled", listed[0].Name, "the first task")
	assert.True(t, listed[0].Scheduled, "a scheduled task did not read as one")
	assert.Equal(t, "asked for", listed[1].Name, "the second task")
	assert.False(t, listed[1].Scheduled, "a task nothing schedules read as scheduled")
}

// Running through the surface is running through the manager, so an attempt
// asked for over HTTP is refused on exactly the terms a schedule is.
func TestTaskSurfaceRunsThroughTheManager(t *testing.T) {
	t.Parallel()

	ran := make(chan task.Invocation, 1)
	manager, err := registerTasks(
		&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn,
		[]task.Definition{{
			Name: "sync:target",
			Run: task.RunnerFunc(func(_ context.Context, invocation task.Invocation) task.Result {
				ran <- invocation

				return task.Result{Outcome: task.Succeeded}
			}),
		}},
	)
	require.NoError(t, err, "registerTasks()")

	surface := taskSurface{ctx: t.Context(), manager: manager}
	require.True(t, surface.Run("sync:target", "rider-a"), "Run()")
	manager.Wait()

	invocation := <-ran
	assert.Equal(t, "rider-a", invocation.Argument, "the argument reached the runner")
	assert.Equal(t, task.TriggerManual, invocation.Trigger, "an operator's attempt was not recorded as one")

	assert.False(t, surface.Run("invented", ""), "a name nothing registers was accepted")
}

// An attempt's life is the service's, not the request's. A request context is
// cancelled the moment its handler returns, so a surface holding one would end
// every attempt just after accepting it; the surface takes no request context
// at all, and what it does hold is what the attempt runs under.
func TestTaskSurfaceRunsUnderTheServiceContext(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		stopService bool
		wantErr     bool
	}{
		"while the service runs": {},
		"once the service stops": {stopService: true, wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			started := make(chan struct{})
			observed := make(chan error, 1)
			manager, err := registerTasks(
				&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn,
				[]task.Definition{{
					Name: "slow",
					Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
						close(started)
						select {
						case <-ctx.Done():
						case <-time.After(50 * time.Millisecond):
						}
						observed <- ctx.Err()

						return task.Result{Outcome: task.Succeeded}
					}),
				}},
			)
			require.NoError(t, err, "registerTasks()")

			service, stopService := context.WithCancel(t.Context())
			defer stopService()
			require.True(t, taskSurface{ctx: service, manager: manager}.Run("slow", ""), "Run()")

			<-started
			if test.stopService {
				stopService()
			}
			manager.Wait()

			if test.wantErr {
				assert.Error(t, <-observed, "the attempt outlived the service that started it")

				return
			}
			assert.NoError(t, <-observed, "the attempt was cancelled while the service was still running")
		})
	}
}

// allEnabled is a service where nobody has switched anything off.
func allEnabled(string) func() bool { return func() bool { return true } }
