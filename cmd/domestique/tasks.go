package main

import (
	"context"
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
	Run(ctx context.Context)
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
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				result := reporter.ReconcileTarget(ctx, invocation.Argument)

				return syncResult(&result)
			}),
		},
		{
			Name:      taskSyncClear,
			Resources: inventory,
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
			runner.Run(ctx)

			return task.Result{Outcome: task.Succeeded}
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

// syncResult carries a synchronization outcome into the task layer's vocabulary,
// the failure category travelling as the detail.
func syncResult(result *syncservice.Result) task.Result {
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

	return task.Result{Outcome: outcome, Detail: task.Detail(result.Failure)}
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
