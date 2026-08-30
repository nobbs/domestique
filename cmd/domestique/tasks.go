package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/task"
)

// The background activities this service runs.
const (
	taskSync            = "sync"
	taskSyncTarget      = "sync:target"
	taskSyncClear       = "sync:clear"
	taskSurfaceAnnotate = "surface:annotate"
	taskSurfaceIndex    = "surface:index"
)

// The state a task holds while it works. Everything that reads or writes the
// trusted inventory takes the same one, so no two of them overlap; a surface
// index build touches neither, and runs beside them.
const (
	resourceInventory    = "inventory"
	resourceSurfaceIndex = "surface-index"
)

// What an alert about each task says, and how long one silences the next. A
// failing library is worth hearing about the same morning; a failing weekly
// rebuild is not worth hearing about more than once between builds.
const (
	syncAlertTitle        = "Domestique sync failed"
	syncAlertSuppression  = 6 * time.Hour
	indexAlertTitle       = "Domestique surface index failed"
	indexAlertSuppression = 7 * 24 * time.Hour
)

// The reasons a surface index rebuild reports. Both are stable words a status
// page may show; neither carries an upstream URL or a local path.
const (
	detailBuild     task.Detail = "build"
	detailNoRegions task.Detail = "no_regions"
)

// synchronizer is the synchronization work the task layer starts, and
// indexBuilder is the surface index rebuild. Both are declared here so the task
// definitions can be read without a reporter or a builder behind them.
type synchronizer interface {
	Run(ctx context.Context) syncservice.Result
	RunBoth(ctx context.Context) syncservice.Result
	RunPhase(ctx context.Context, phase syncservice.Phase) syncservice.Result
	ReconcileTarget(ctx context.Context, targetID string) syncservice.Result
	ClearTarget(ctx context.Context, targetID string) syncservice.Result
	Annotate(ctx context.Context)
}

type indexBuilder interface {
	Run(ctx context.Context) (osmindex.Outcome, error)
}

// registerTasks registers every activity this service runs unasked, over the
// store their attempts are recorded in.
func registerTasks(
	store task.Store, notifier task.Notifier, alerts task.Alerts,
	enabled func() bool, definitions []task.Definition,
) (*task.Manager, error) {
	manager, err := task.NewManager(store, notifier, alerts, enabled)
	if err != nil {
		return nil, fmt.Errorf("creating the task manager: %w", err)
	}
	for index := range definitions {
		if err := manager.Register(&definitions[index]); err != nil {
			return nil, fmt.Errorf("registering background tasks: %w", err)
		}
	}

	return manager, nil
}

// syncAlerts is what a synchronization can be announced for. Every failure
// category the sync package reports is here, so an operator rules on each
// rather than meeting one for the first time at four in the morning.
func syncAlerts() *task.Notify {
	return &task.Notify{
		Title:    syncAlertTitle,
		Suppress: syncAlertSuppression,
		Alerts: []task.Detail{
			task.Detail(syncservice.FailureState),
			task.Detail(syncservice.FailureSource),
			task.Detail(syncservice.FailureAuthorization),
			task.Detail(syncservice.FailureDestination),
			task.Detail(syncservice.FailureCourse),
			task.Detail(syncservice.FailureEmptySource),
			task.Detail(syncservice.FailureDeletionLimit),
		},
	}
}

// inventoryTasks are the activities that reconcile the library, in the order a
// status page reads best.
func inventoryTasks(reporter synchronizer, settings *runtimeconfig.Current) []task.Definition {
	inventory := func(string) []task.Resource {
		return []task.Resource{{Name: resourceInventory, Exclusive: true}}
	}

	return []task.Definition{
		{
			Name:      taskSync,
			Resources: inventory,
			Notify:    syncAlerts(),
			Schedule:  task.Every(func() time.Duration { return syncservice.Interval }),
			InitialDelay: func() time.Duration {
				return settings.Values().Sync.InitialDelay
			},
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				result := runSync(ctx, reporter, invocation)

				return syncResult(&result)
			}),
		},
		{
			Name:      taskSyncTarget,
			Resources: inventory,
			Notify:    syncAlerts(),
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				result := reporter.ReconcileTarget(ctx, invocation.Argument)

				return syncResult(&result)
			}),
		},
		{
			Name:      taskSyncClear,
			Resources: inventory,
			Notify:    syncAlerts(),
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				result := reporter.ClearTarget(ctx, invocation.Argument)

				return syncResult(&result)
			}),
		},
		{
			Name:      taskSurfaceAnnotate,
			Resources: inventory,
			Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
				reporter.Annotate(ctx)

				return task.Result{Outcome: task.Succeeded}
			}),
		},
	}
}

// surfaceIndexTask rebuilds the surface index. Its initial delay counts from the
// last build rather than from this process starting, so restarting the service
// does not restart the interval.
func surfaceIndexTask(
	runner indexBuilder, settings *runtimeconfig.Current, lastBuiltAt time.Time,
) task.Definition {
	return task.Definition{
		Name: taskSurfaceIndex,
		Notify: &task.Notify{
			Title:    indexAlertTitle,
			Suppress: indexAlertSuppression,
			Alerts:   []task.Detail{detailBuild, detailNoRegions},
		},
		Resources: func(string) []task.Resource {
			return []task.Resource{{Name: resourceSurfaceIndex, Exclusive: true}}
		},
		Schedule: task.Every(func() time.Duration {
			return settings.Values().Surface.RebuildInterval
		}),
		InitialDelay: func() time.Duration {
			return osmindex.InitialDelay(
				lastBuiltAt, settings.Values().Surface.RebuildInterval,
				osmindex.InitialBuildDelay, time.Now().UTC(),
			)
		},
		Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
			return indexResult(runner.Run(ctx))
		}),
	}
}

// runSync performs what was asked of the synchronization task. A scheduled run
// honours the schedule switches; an operator asking for one overrides them,
// because asking is the point.
func runSync(ctx context.Context, reporter synchronizer, invocation task.Invocation) syncservice.Result {
	if invocation.Trigger == task.TriggerSchedule {
		return reporter.Run(ctx)
	}
	switch phase := syncservice.Phase(invocation.Argument); phase {
	case syncservice.PhaseSource, syncservice.PhaseTargets:
		return reporter.RunPhase(ctx, phase)
	}

	return reporter.RunBoth(ctx)
}

// indexResult carries a rebuild's outcome into the task layer's vocabulary. A
// build that found nothing new still reached its upstream, which is why it is
// unchanged rather than current.
func indexResult(outcome osmindex.Outcome, err error) task.Result {
	if err != nil {
		return task.Result{Outcome: task.Failed, Detail: detailBuild}
	}
	switch outcome {
	case osmindex.Rebuilt:
		// A new generation makes every stored classification stale, and nothing
		// else notices that.
		return task.Result{
			Outcome: task.Succeeded,
			Next:    []task.Link{{Task: taskSurfaceAnnotate}},
		}
	case osmindex.Unchanged:
		return task.Result{Outcome: task.Unchanged}
	case osmindex.NoRegions:
		return task.Result{Outcome: task.NotReady, Detail: detailNoRegions}
	}

	// An outcome this binary has not heard of must not read as a success: the
	// history would say the map moved when nothing here knows whether it did.
	return task.Result{Outcome: task.Failed, Detail: detailBuild}
}

// syncResult carries a synchronization outcome into the task layer's vocabulary,
// the failure category travelling as the detail.
func syncResult(result *syncservice.Result) task.Result {
	var next []task.Link
	if result.SourceStored {
		// An inventory that has just been replaced is worth reading the ground
		// under again, whether or not any stage in it changed.
		next = []task.Link{{Task: taskSurfaceAnnotate}}
	}
	outcome := task.Failed
	switch result.Outcome {
	case syncservice.OutcomeSucceeded:
		outcome = task.Succeeded
	case syncservice.OutcomeBlocked:
		outcome = task.Blocked
	case syncservice.OutcomeNotReady:
		outcome = task.NotReady
	case syncservice.OutcomeSkipped:
		outcome = task.Skipped
	case syncservice.OutcomeFailed:
		outcome = task.Failed
	}

	return task.Result{Outcome: outcome, Detail: task.Detail(result.Failure), Next: next}
}

// taskStarter is the task layer as the HTTP boundary needs it, and syncReporter
// is what only the reporter can answer. Both are narrow so the adaptation below
// can be read without a manager or a reporter behind it.
type taskStarter interface {
	Trigger(ctx context.Context, name, argument string) bool
	Holding(resource string) bool
	NextRunAt(name string) (time.Time, bool)
}

type syncReporter interface {
	Running() (syncservice.Phase, bool)
	SurfaceIncomplete() int
}

// syncSurface adapts the task layer to what the HTTP boundary asks of it: start
// work, and say what is under way. The HTTP surface names a phase; what running
// one means is the sync package's to decide, and a manual trigger deliberately
// ignores the schedule switches.
func syncSurface(
	ctx context.Context,
	tasks taskStarter,
	reporter syncReporter,
	rateLimit func() (int, time.Time, bool),
) httpapi.SyncFuncs {
	return httpapi.SyncFuncs{
		TriggerFunc: func(phase httpapi.SyncPhase) bool {
			argument := ""
			switch phase {
			case httpapi.SyncPhaseAll:
			case httpapi.SyncPhaseSource:
				argument = string(syncservice.PhaseSource)
			case httpapi.SyncPhaseTargets:
				argument = string(syncservice.PhaseTargets)
			default:
				return false
			}

			return tasks.Trigger(ctx, taskSync, argument)
		},
		TriggerTargetFunc: func(targetID string) bool {
			return tasks.Trigger(ctx, taskSyncTarget, targetID)
		},
		TriggerClearFunc: func(targetID string) bool {
			return tasks.Trigger(ctx, taskSyncClear, targetID)
		},
		TriggerAnnotateFunc: func() bool {
			return tasks.Trigger(ctx, taskSurfaceAnnotate, "")
		},
		// Two halves of one answer: the reporter knows which half is in flight,
		// and the task layer knows whether anything holds the inventory at all.
		ActivityFunc: func() httpapi.SyncActivityState {
			phase, _ := reporter.Running()
			startsAt, _ := tasks.NextRunAt(taskSync)

			return httpapi.SyncActivityState{
				StartsAt: startsAt,
				Phase:    httpapi.SyncPhase(phase),
				Running:  tasks.Holding(resourceInventory),
			}
		},
		SurfaceIncompleteFunc: reporter.SurfaceIncomplete,
		RateLimitFunc:         rateLimit,
	}
}
