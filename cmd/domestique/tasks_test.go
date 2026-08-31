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

	definitions := inventoryTasks(&fakeSynchronizer{}, liveSettings(t))

	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
		assert.Equal(t,
			[]task.Resource{{Name: resourceInventory, Exclusive: true}},
			definition.Resources(""),
			definition.Name+" resources",
		)
	}
	assert.Equal(t, []string{taskSync, taskSyncTarget, taskSyncClear, taskSurfaceAnnotate}, names, "registered tasks")
}

// Only the full synchronization runs unasked. Reconciling one slot, clearing
// one, and classifying are things an operator asks for.
func TestOnlyTheSyncTaskIsScheduled(t *testing.T) {
	t.Parallel()

	for _, definition := range inventoryTasks(&fakeSynchronizer{}, liveSettings(t)) {
		if definition.Name == taskSync {
			assert.NotNil(t, definition.Schedule, "the synchronization is not scheduled")

			continue
		}
		assert.Nil(t, definition.Schedule, definition.Name+" runs unasked")
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
			definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t)), test.task)

			result := definition.Run.Run(t.Context(), task.Invocation{Task: test.task, Argument: test.argument})
			assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
			test.expect(t, synchronizer)
		})
	}
}

// A scheduled synchronization honours the schedule switches. An operator asking
// for one has already decided, so it runs both halves whatever they say.
func TestRunSyncSeparatesTheScheduleFromWhatWasAskedFor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		expect     func(*testing.T, *fakeSynchronizer)
		invocation task.Invocation
	}{
		"scheduled": {
			invocation: task.Invocation{Trigger: task.TriggerSchedule},
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, 1, s.scheduled, "scheduled runs")
				assert.Zero(t, s.both, "a scheduled run ignored the schedule switches")
			},
		},
		"asked for outright": {
			invocation: task.Invocation{Trigger: task.TriggerManual},
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, 1, s.both, "runs of both halves")
				assert.Zero(t, s.scheduled, "a manual run honoured the schedule switches")
			},
		},
		"one half asked for": {
			invocation: task.Invocation{Trigger: task.TriggerManual, Argument: string(syncservice.PhaseSource)},
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, []syncservice.Phase{syncservice.PhaseSource}, s.phases, "phases run")
			},
		},
		"an argument naming no half": {
			invocation: task.Invocation{Trigger: task.TriggerManual, Argument: "nonsense"},
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, 1, s.both, "an unrecognised argument ran something other than both halves")
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			synchronizer := &fakeSynchronizer{}
			runSync(t.Context(), synchronizer, test.invocation)
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
	definition := definitionNamed(t, inventoryTasks(&fakeSynchronizer{}, settings), taskSync)

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
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t)), taskSync)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskSync, Trigger: task.TriggerSchedule})
	assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
	assert.Equal(t, 1, synchronizer.scheduled, "scheduled runs")
}

// The HTTP boundary names a phase; each one has to reach the task that performs
// it, and a phase this binary does not serve reaches nothing.
func TestSyncSurfaceCarriesEveryPhaseToTheTaskLayer(t *testing.T) {
	t.Parallel()

	tests := map[httpapi.SyncPhase]string{
		httpapi.SyncPhaseAll:     "",
		httpapi.SyncPhaseSource:  string(syncservice.PhaseSource),
		httpapi.SyncPhaseTargets: string(syncservice.PhaseTargets),
	}
	for phase, argument := range tests {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()

			starter := &fakeStarter{accept: true}
			surface := syncSurface(t.Context(), starter, &fakeSyncReporter{}, nil)

			assert.True(t, surface.Trigger(phase), "Trigger()")
			assert.Equal(t, []started{{name: taskSync, argument: argument}}, starter.starts, "started tasks")
		})
	}
}

func TestSyncSurfaceRefusesAPhaseItDoesNotServe(t *testing.T) {
	t.Parallel()

	starter := &fakeStarter{accept: true}
	surface := syncSurface(t.Context(), starter, &fakeSyncReporter{}, nil)

	assert.False(t, surface.Trigger("invented"), "Trigger()")
	assert.Empty(t, starter.starts, "a phase this service does not serve started a task")
}

func TestSyncSurfaceStartsTheTargetAndClassificationTasks(t *testing.T) {
	t.Parallel()

	starter := &fakeStarter{accept: true}
	surface := syncSurface(t.Context(), starter, &fakeSyncReporter{}, nil)

	require.True(t, surface.TriggerTarget("rider-a"), "TriggerTarget()")
	require.True(t, surface.TriggerClear("rider-b"), "TriggerClear()")
	require.True(t, surface.TriggerAnnotate(), "TriggerAnnotate()")
	assert.Equal(t, []started{
		{name: taskSyncTarget, argument: "rider-a"},
		{name: taskSyncClear, argument: "rider-b"},
		{name: taskSurfaceAnnotate},
	}, starter.starts, "started tasks")
}

func TestSyncSurfaceReportsARefusedTrigger(t *testing.T) {
	t.Parallel()

	surface := syncSurface(t.Context(), &fakeStarter{}, &fakeSyncReporter{}, nil)

	assert.False(t, surface.Trigger(httpapi.SyncPhaseAll), "Trigger()")
	assert.False(t, surface.TriggerTarget("rider-a"), "TriggerTarget()")
	assert.False(t, surface.TriggerClear("rider-a"), "TriggerClear()")
	assert.False(t, surface.TriggerAnnotate(), "TriggerAnnotate()")
}

// Activity is assembled from two places at once, and the status response is
// built from whatever both of them say at that moment.
func TestSyncSurfaceReportsActivityFromBothHalves(t *testing.T) {
	t.Parallel()

	due := time.Date(2026, time.August, 30, 9, 5, 0, 0, time.UTC)
	starter := &fakeStarter{holding: true, nextRunAt: due, held: true}
	reporter := &fakeSyncReporter{phase: syncservice.PhaseTargets, incomplete: 3}
	surface := syncSurface(t.Context(), starter, reporter, nil)

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

	definitions := append(inventoryTasks(&fakeSynchronizer{}, liveSettings(t)),
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
func TestSyncResultAsksForClassificationOnlyAfterStoringAnInventory(t *testing.T) {
	t.Parallel()

	stored := syncResult(&syncservice.Result{Outcome: syncservice.OutcomeSucceeded, SourceStored: true})
	assert.Equal(t, []task.Link{{Task: taskSurfaceAnnotate}}, stored.Next, "a stored inventory asked for no classification")

	untouched := syncResult(&syncservice.Result{Outcome: syncservice.OutcomeSucceeded})
	assert.Empty(t, untouched.Next, "a pass that stored nothing asked for classification anyway")
}

func alwaysOn() bool { return true }

type silentNotifier struct{}

func (*silentNotifier) Send(context.Context, string, string) error { return nil }

// undecided stands in for an operator who has ruled on nothing.
type undecided struct{}

func (undecided) Wanted(context.Context, string, task.Detail) (enabled, decided bool) {
	return false, false
}
