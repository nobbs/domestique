package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/task"
)

func TestInventoryTasksAllHoldTheInventoryExclusively(t *testing.T) {
	t.Parallel()

	definitions := inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled, twoTargets)

	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
		assert.Equal(t,
			[]task.Resource{{Name: resourceInventory, Exclusive: true}},
			definition.Resources(""),
			definition.Name+" resources",
		)
	}
	assert.Equal(t,
		[]string{taskSyncSource, taskSyncTarget, taskSyncClear, taskSurfaceAnnotate, taskRideModelPredict},
		names, "registered tasks")
}

func TestOnlyTheReadDeclaresTheStaleAlert(t *testing.T) {
	t.Parallel()

	stale := map[string]bool{taskSyncSource: true}
	for _, definition := range inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled, twoTargets) {
		if definition.Notify == nil {
			continue
		}
		assert.Equalf(t, stale[definition.Name],
			slices.Contains(definition.Notify.Alerts, task.DetailStale),
			"%s stale alert declared", definition.Name)
	}
}

func TestOnlyTheReadAndTheTargetsAreScheduled(t *testing.T) {
	t.Parallel()

	scheduled := map[string]bool{taskSyncSource: true, taskSyncTarget: true}
	for _, definition := range inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled, twoTargets) {
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
		"prediction": {
			task: taskRideModelPredict,
			expect: func(t *testing.T, s *fakeSynchronizer) {
				assert.Equal(t, 1, s.predictions, "prediction passes")
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			synchronizer := &fakeSynchronizer{result: syncservice.Result{Outcome: syncservice.OutcomeSucceeded}}
			definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), test.task)

			result := definition.Run.Run(t.Context(), task.Invocation{Task: test.task, Argument: test.argument})
			assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
			test.expect(t, synchronizer)
		})
	}
}

func TestClassificationRecordsFailureWhenStagesAreLeftUnclassified(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{annotationFailed: 2}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), taskSurfaceAnnotate)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskSurfaceAnnotate})
	assert.Equal(t, task.Failed, result.Outcome, "outcome")
	assert.Equal(t, detailIncomplete, result.Detail, "detail")
	// A partial classification still advances to prediction, rather than
	// silencing it over stages that did classify.
	assert.True(t, result.Advances, "advances")
}

// A pass that stops before it can name a single failed stage — a state read
// error, say — is still a failure the history and backoff have to see, not a
// silent success that happens to have found nothing wrong.
func TestClassificationRecordsFailureWhenThePassStopsEarly(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{annotationErr: errors.New("index unavailable")}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), taskSurfaceAnnotate)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskSurfaceAnnotate})
	assert.Equal(t, task.Failed, result.Outcome, "outcome")
	assert.Equal(t, detailStoppedEarly, result.Detail, "detail")
	// Whatever the pass did manage is still worth predicting over.
	assert.True(t, result.Advances, "advances")
}

func TestPredictionRecordsFailureWhenStagesAreLeftUnpredicted(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{predictionFailed: 2}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), taskRideModelPredict)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskRideModelPredict})
	assert.Equal(t, task.Failed, result.Outcome, "outcome")
	assert.Equal(t, detailIncomplete, result.Detail, "detail")
}

// Same as classification: a stopped-early pass has to fail the run even with
// nothing individually counted as failed.
func TestPredictionRecordsFailureWhenThePassStopsEarly(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{predictionErr: errors.New("coefficients unavailable")}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), taskRideModelPredict)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskRideModelPredict})
	assert.Equal(t, task.Failed, result.Outcome, "outcome")
	assert.Equal(t, detailStoppedEarly, result.Detail, "detail")
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

func TestSyncResultTreatsAnUnknownOutcomeAsAFailure(t *testing.T) {
	t.Parallel()

	assert.Equal(t, task.Failed, syncResult(&syncservice.Result{Outcome: "invented"}).Outcome, "outcome")
}

func TestSurfaceIndexTaskHoldsOnlyItsOwnIndex(t *testing.T) {
	t.Parallel()

	builder := &fakeIndexBuilder{outcome: osmindex.Rebuilt}
	definition := surfaceIndexTask(builder, liveSettings(t), allEnabled, time.Time{})

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

	definition := surfaceIndexTask(&fakeIndexBuilder{}, liveSettings(t), allEnabled, time.Time{})

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

	for index := range definitions {
		if definitions[index].Name == name {
			return definitions[index]
		}
	}
	t.Fatalf("no task named %q was registered", name)

	return task.Definition{}
}

type fakeSynchronizer struct {
	annotationErr    error
	predictionErr    error
	cleared          []string
	providers        []route.Provider
	reconciled       []string
	phases           []syncservice.Phase
	result           syncservice.Result
	scheduled        int
	both             int
	annotations      int
	annotationFailed int
	predictions      int
	predictionFailed int
}

func (s *fakeSynchronizer) Run(context.Context) syncservice.Result {
	s.scheduled++

	return s.result
}

func (s *fakeSynchronizer) RunBoth(context.Context) syncservice.Result {
	s.both++

	return s.result
}

func (s *fakeSynchronizer) RunSourceProvider(_ context.Context, provider route.Provider) syncservice.Result {
	s.providers = append(s.providers, provider)

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

func (s *fakeSynchronizer) Annotate(context.Context) (failed int, err error) {
	s.annotations++

	return s.annotationFailed, s.annotationErr
}

func (s *fakeSynchronizer) Predict(context.Context) (failed int, err error) {
	s.predictions++

	return s.predictionFailed, s.predictionErr
}

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
	definition := definitionNamed(t, inventoryTasks(&fakeSynchronizer{}, settings, allEnabled, twoTargets), taskSyncSource)

	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, at.Add(syncservice.Interval), definition.Schedule.NextFire(at), "NextFire()")
	assert.Equal(t, settings.Values().Sync.InitialDelay, definition.InitialDelay(), "InitialDelay()")
}

func TestSurfaceIndexTaskReadsItsCadenceFromSettings(t *testing.T) {
	t.Parallel()

	settings := liveSettings(t)
	definition := surfaceIndexTask(&fakeIndexBuilder{}, settings, allEnabled, time.Time{})

	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, at.Add(settings.Values().Surface.RebuildInterval), definition.Schedule.NextFire(at), "NextFire()")
}

func TestSyncTaskRunsWhatWasAskedOfIt(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{result: syncservice.Result{Outcome: syncservice.OutcomeSucceeded}}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), taskSyncSource)

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskSyncSource, Trigger: task.TriggerSchedule})
	assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
	assert.Equal(t, []syncservice.Phase{syncservice.PhaseSource}, synchronizer.phases, "phases run")
}

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

func TestIndexResultSeparatesABuildFromAnUpstreamThatHadNothingNew(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		outcome osmindex.Outcome
		err     error
		want    task.Result
	}{
		"rebuilt": {
			outcome: osmindex.Rebuilt,
			want:    task.Result{Outcome: task.Succeeded},
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

	definitions := append(inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled, twoTargets),
		surfaceIndexTask(&fakeIndexBuilder{}, liveSettings(t), allEnabled, time.Time{}))

	manager, err := registerTasks(&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn, definitions)
	require.NoError(t, err, "registerTasks()")
	for _, definition := range definitions {
		_, known := manager.NextRunAt(definition.Name)
		assert.False(t, known, definition.Name+" is holding a first run before anything started")
	}
}

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
	context.Context, string, string, string, time.Time, time.Time, string, string, string, int,
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

// Classification follows the read and the rebuild alike, because each on its
// own leaves stages wanting it.
func TestTheGraphDeclaresWhatFollowsEveryRead(t *testing.T) {
	t.Parallel()

	definitions := append(
		inventoryTasks(&fakeSynchronizer{}, liveSettings(t), allEnabled, twoTargets),
		surfaceIndexTask(&fakeIndexBuilder{}, liveSettings(t), allEnabled, time.Time{}),
	)

	targets := definitionNamed(t, definitions, taskSyncTarget)
	assert.Equal(t, []string{taskSyncSource}, targets.Follows, "what the targets follow")

	annotate := definitionNamed(t, definitions, taskSurfaceAnnotate)
	assert.Equal(t, []string{taskSyncSource, taskSurfaceIndex}, annotate.Follows,
		"what classification follows")

	predict := definitionNamed(t, definitions, taskRideModelPredict)
	assert.Equal(t, []string{taskSurfaceAnnotate}, predict.Follows, "what prediction follows")
}

func alwaysOn() bool { return true }

type silentNotifier struct{}

func (*silentNotifier) Send(context.Context, string, string) error { return nil }

// undecided stands in for an operator who has ruled on nothing.
type undecided struct{}

func (undecided) Wanted(context.Context, string, task.Detail) (enabled, decided bool) {
	return false, false
}

// A task added to the layer appears without anybody writing an endpoint for it.
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

// twoTargets is a service configured with two destination slots.
func twoTargets() []string { return []string{"rider-a", "rider-b"} }

func TestTaskSurfaceSwitchesATaskAndReportsIt(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	switches, err := newTaskSwitches(t.Context(), store)
	require.NoError(t, err, "newTaskSwitches()")
	manager, err := registerTasks(
		&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn,
		[]task.Definition{{Name: "sync:target", Run: task.RunnerFunc(func(context.Context, task.Invocation) task.Result {
			return task.Result{Outcome: task.Succeeded}
		})}},
	)
	require.NoError(t, err, "registerTasks()")
	surface := taskSurface{ctx: t.Context(), manager: manager, switches: switches}

	require.True(t, surface.Registered()[0].Enabled, "a task nobody ruled on read as switched off")
	require.NoError(t, surface.Schedule(t.Context(), "sync:target", false), "Schedule()")
	assert.False(t, surface.Registered()[0].Enabled, "the switch did not reach the list")
}

func TestTaskSurfaceRefusesToSwitchWithoutASchedule(t *testing.T) {
	t.Parallel()

	manager, err := registerTasks(&countingStore{}, &silentNotifier{}, undecided{}, alwaysOn, nil)
	require.NoError(t, err, "registerTasks()")
	surface := taskSurface{ctx: t.Context(), manager: manager}

	assert.NotPanics(t, func() { _ = surface.Registered() }, "listing without a schedule panicked")
	require.Error(t, surface.Schedule(t.Context(), "sync:target", false), "Schedule() without a schedule")
}

func TestTheReadTakesOneLibraryOrEveryOne(t *testing.T) {
	t.Parallel()

	synchronizer := &fakeSynchronizer{result: syncservice.Result{Outcome: syncservice.OutcomeSucceeded}}
	definition := definitionNamed(t, inventoryTasks(synchronizer, liveSettings(t), allEnabled, twoTargets), taskSyncSource)

	definition.Run.Run(t.Context(), task.Invocation{Task: taskSyncSource})
	assert.Equal(t, []syncservice.Phase{syncservice.PhaseSource}, synchronizer.phases, "phases run")
	assert.Empty(t, synchronizer.providers, "reading everything named a library")

	definition.Run.Run(t.Context(), task.Invocation{
		Task: taskSyncSource, Argument: string(route.ProviderKomoot),
	})
	assert.Equal(t, []route.Provider{route.ProviderKomoot}, synchronizer.providers, "the library read")
}

// Polling a rider's activities reads their Wahoo account but writes only
// activity rows, so it holds its own resource and runs beside a reconciliation.
func TestActivityPollTaskHoldsOnlyTheActivities(t *testing.T) {
	t.Parallel()

	poller := &fakePoller{results: map[string]activity.Result{"rider-a": {Outcome: activity.Polled, Stored: 2}}}
	definition := activityPollTask(poller, allEnabled, func() []string { return []string{"rider-a"} })

	assert.Equal(t, taskActivityPoll, definition.Name, "name")
	assert.Equal(t,
		[]task.Resource{{Name: resourceActivities, Exclusive: true}},
		definition.Resources(""),
		"resources",
	)
	assert.Equal(t, []string{"rider-a"}, definition.FanOut(), "fan-out")
	assert.Equal(t, activityPollInterval, definition.InitialDelay(), "InitialDelay()")
	at := time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, at.Add(activityPollInterval), definition.Schedule.NextFire(at), "NextFire()")

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskActivityPoll, Argument: "rider-a"})
	assert.Equal(t, task.Succeeded, result.Outcome, "outcome")
	assert.Equal(t, []string{"rider-a"}, poller.polled, "polled")
}

// No argument is every slot, and one slot's failure neither hides the others nor
// is hidden by them.
func TestActivityPollTaskWithoutAnArgumentPollsEveryTarget(t *testing.T) {
	t.Parallel()

	poller := &fakePoller{results: map[string]activity.Result{
		"rider-a": {Outcome: activity.Polled},
		"rider-b": {Outcome: activity.Failed, Failure: activity.FailureUpstream},
		"rider-c": {Outcome: activity.Unchanged},
	}}
	definition := activityPollTask(poller, allEnabled, func() []string { return []string{"rider-a", "rider-b", "rider-c"} })

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskActivityPoll})
	assert.Equal(t, task.Failed, result.Outcome, "outcome")
	assert.Equal(t, detailActivityUpstream, result.Detail, "detail")
	assert.Equal(t, []string{"rider-a", "rider-b", "rider-c"}, poller.polled, "every slot was polled")
}

// With nothing connected there is nothing to poll, which is not a failure.
func TestActivityPollTaskWithoutATargetIsNotReady(t *testing.T) {
	t.Parallel()

	definition := activityPollTask(&fakePoller{}, allEnabled, func() []string { return nil })

	result := definition.Run.Run(t.Context(), task.Invocation{Task: taskActivityPoll})
	assert.Equal(t, task.NotReady, result.Outcome, "outcome")
}

func TestActivityResultCarriesEveryOutcomeAcross(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want    task.Outcome
		detail  task.Detail
		failure activity.Failure
		outcome activity.Outcome
	}{
		"polled":        {want: task.Succeeded, outcome: activity.Polled},
		"unchanged":     {want: task.Unchanged, outcome: activity.Unchanged},
		"not ready":     {want: task.NotReady, outcome: activity.NotReady},
		"authorization": {want: task.Failed, detail: detailActivityAuthorization, failure: activity.FailureAuthorization},
		"state":         {want: task.Failed, detail: detailActivityState, failure: activity.FailureState},
		"upstream":      {want: task.Failed, detail: detailActivityUpstream, failure: activity.FailureUpstream},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := activityResult(activity.Result{Outcome: test.outcome, Failure: test.failure})
			assert.Equal(t, test.want, result.Outcome, "outcome")
			assert.Equal(t, test.detail, result.Detail, "detail")
		})
	}
}

type fakePoller struct {
	results map[string]activity.Result
	polled  []string
}

func (p *fakePoller) Poll(_ context.Context, targetID string) activity.Result {
	p.polled = append(p.polled, targetID)

	return p.results[targetID]
}
