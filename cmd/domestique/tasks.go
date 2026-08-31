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
	taskSyncSource      = httpapi.TaskSyncSource
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
	syncAlertTitle        = "Domestique sync"
	syncAlertSuppression  = 6 * time.Hour
	indexAlertTitle       = "Domestique surface index"
	indexAlertSuppression = 7 * 24 * time.Hour
)

// How long a failing activity waits before its schedule may start it again.
// Reaching an upstream and reading the ground under a stage fail for different
// reasons and recover on different timescales, so they wait differently; both
// stop doubling at six hours, which is a morning's worth of quiet rather than a
// task that has given up.
const (
	syncBackoffBase     = 30 * time.Second
	targetBackoffBase   = time.Hour
	annotateBackoffBase = 5 * time.Minute
	backoffCap          = 6 * time.Hour
)

// targetBackstopInterval is how often the targets are reconciled unasked. A
// source read that stored something asks for them at once, so this catches only
// the slot that failed alone and the operator who has the read switched off.
const targetBackstopInterval = 6 * time.Hour

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
			task.DetailSucceeded,
			task.DetailRecovered,
			task.DetailStale,
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
func inventoryTasks(
	reporter synchronizer, settings *runtimeconfig.Current, enabled func(string) func() bool,
) []task.Definition {
	inventory := func(string) []task.Resource {
		return []task.Resource{{Name: resourceInventory, Exclusive: true}}
	}

	return []task.Definition{
		{
			Name:      taskSyncSource,
			Resources: inventory,
			Notify:    syncAlerts(),
			Schedule:  task.Every(func() time.Duration { return syncservice.Interval }),
			InitialDelay: func() time.Duration {
				return settings.Values().Sync.InitialDelay
			},
			Enabled: enabled(taskSyncSource),
			// Only the scheduled read is expected on a clock. What the targets
			// hold follows from it, and a slot an operator reconciles by hand is
			// stale the moment they stop asking.
			StaleAfter: func() time.Duration {
				return settings.Values().Sync.StaleAfter
			},
			Backoff: task.Backoff{Base: syncBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
				result := reporter.RunPhase(ctx, syncservice.PhaseSource)

				return sourceResult(&result)
			}),
		},
		{
			Name:      taskSyncTarget,
			Resources: inventory,
			Notify:    syncAlerts(),
			// A backstop rather than the timely path: a read that stored
			// something asks for this straight away. What the schedule is for is
			// the slot that failed on its own, and the operator who has the
			// source read switched off entirely.
			Schedule:     task.Every(func() time.Duration { return targetBackstopInterval }),
			InitialDelay: func() time.Duration { return targetBackstopInterval },
			Enabled:      enabled(taskSyncTarget),
			Backoff:      task.Backoff{Base: targetBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				// No argument is every configured slot, which is what the schedule
				// and a source read both ask for. One names the slot alone.
				if invocation.Argument == "" {
					result := reporter.RunPhase(ctx, syncservice.PhaseTargets)

					return syncResult(&result)
				}
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
			Backoff:   task.Backoff{Base: annotateBackoffBase, Cap: backoffCap},
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

// sourceResult is what a source read came to, and what follows from it. A read
// that stored a library asks for the targets to be written and for the ground
// under the stages to be read again; neither notices on its own.
func sourceResult(result *syncservice.Result) task.Result {
	outcome := syncResult(result)
	if result.SourceStored {
		outcome.Next = []task.Link{{Task: taskSyncTarget}, {Task: taskSurfaceAnnotate}}
	}

	return outcome
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

// taskSurface adapts the manager to what the HTTP surface reads, starts and
// switches. It carries the service's own context rather than taking each
// request's: a request context is cancelled the moment its handler returns,
// which would end every attempt started over HTTP just after it was accepted.
type taskSurface struct {
	ctx      context.Context
	manager  *task.Manager
	switches *taskSwitches
}

// Registered lists what this build registers, in registration order.
func (s taskSurface) Registered() []httpapi.RegisteredTask {
	decided := s.switches.snapshot()
	registered := s.manager.Tasks()
	tasks := make([]httpapi.RegisteredTask, 0, len(registered))
	for _, entry := range registered {
		tasks = append(tasks, httpapi.RegisteredTask{
			Name:      entry.Name,
			Scheduled: entry.Scheduled,
			Enabled:   enabledOf(decided, entry.Name),
			Running:   entry.Running,
			NextRunAt: entry.NextRunAt,
		})
	}

	return tasks
}

// Schedule records whether the schedule may start one task.
func (s taskSurface) Schedule(ctx context.Context, name string, enabled bool) error {
	return s.switches.Set(ctx, name, enabled)
}

// enabledOf is what has been decided about one task, defaulting to on: a task
// nobody has ruled on runs.
func enabledOf(decided map[string]bool, name string) bool {
	enabled, ruled := decided[name]

	return !ruled || enabled
}

// Run starts one attempt, on exactly the terms the schedule starts one, and
// under the same context — so it ends when the service does rather than when
// the request that asked for it does.
func (s taskSurface) Run(name, argument string) bool {
	return s.manager.Trigger(s.ctx, name, argument)
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

// syncSurface says what is under way. Starting work is the task layer's, asked
// for by name; what is left here is the part only the reporter can answer.
func syncSurface(
	tasks taskStarter,
	reporter syncReporter,
	rateLimit func() (int, time.Time, bool),
) httpapi.SyncFuncs {
	return httpapi.SyncFuncs{
		// Two halves of one answer: the reporter knows which half is in flight,
		// and the task layer knows whether anything holds the inventory at all.
		ActivityFunc: func() httpapi.SyncActivityState {
			phase, _ := reporter.Running()
			startsAt, _ := tasks.NextRunAt(taskSyncSource)

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
