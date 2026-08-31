package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/route"
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

// Everything reading or writing the trusted inventory takes resourceInventory
// so none overlap; a surface index build takes neither and runs beside them.
const (
	resourceInventory    = "inventory"
	resourceSurfaceIndex = "surface-index"
)

// A failing library is worth hearing about the same morning; a failing weekly
// rebuild is not worth hearing about more than once between builds.
const (
	syncAlertTitle        = "Domestique sync"
	syncAlertSuppression  = 6 * time.Hour
	indexAlertTitle       = "Domestique surface index"
	indexAlertSuppression = 7 * 24 * time.Hour
)

// Reaching an upstream and reading the ground under a stage fail for different
// reasons and recover on different timescales, hence different bases; both cap
// at six hours, a morning's quiet rather than giving up.
const (
	syncBackoffBase     = 30 * time.Second
	targetBackoffBase   = time.Hour
	annotateBackoffBase = 5 * time.Minute
	backoffCap          = 6 * time.Hour
)

// targetBackstopInterval is how often targets are reconciled unasked; a
// successful source read already asks for them at once, so this only catches a
// slot that failed alone or an operator with the read switched off.
const targetBackstopInterval = 6 * time.Hour

// The reasons a surface index rebuild reports. Both are stable words a status
// page may show; neither carries an upstream URL or a local path.
const (
	detailBuild     task.Detail = "build"
	detailNoRegions task.Detail = "no_regions"
)

// detailIncomplete is why a classification pass that left stages unclassified
// is recorded as failed rather than succeeded.
const detailIncomplete task.Detail = "incomplete"

// synchronizer is the sync work the task layer starts; indexBuilder is the
// surface index rebuild. Both live here so definitions can be read without a
// reporter or builder behind them.
type synchronizer interface {
	RunPhase(ctx context.Context, phase syncservice.Phase) syncservice.Result
	RunSourceProvider(ctx context.Context, provider route.Provider) syncservice.Result
	ReconcileTarget(ctx context.Context, targetID string) syncservice.Result
	ClearTarget(ctx context.Context, targetID string) syncservice.Result
	Annotate(ctx context.Context) (failed int)
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
	// The graph is settled before anything runs: an edge naming a task this
	// build lacks, or closing a cycle, refuses the start instead of surfacing at
	// four in the morning.
	if err := manager.Resolve(); err != nil {
		return nil, fmt.Errorf("resolving what follows what: %w", err)
	}

	return manager, nil
}

// syncAlerts covers every failure category the sync package reports, so an
// operator can rule on each in advance. stale is included only when the task
// has a StaleAfter bound — a switch for an alert that can never fire is a
// decoration, not a decision.
func syncAlerts(stale bool) *task.Notify {
	alerts := []task.Detail{
		task.DetailSucceeded,
		task.DetailRecovered,
		task.Detail(syncservice.FailureState),
		task.Detail(syncservice.FailureSource),
		task.Detail(syncservice.FailureAuthorization),
		task.Detail(syncservice.FailureDestination),
		task.Detail(syncservice.FailureCourse),
		task.Detail(syncservice.FailureEmptySource),
		task.Detail(syncservice.FailureDeletionLimit),
	}
	if stale {
		alerts = append(alerts, task.DetailStale)
	}

	return &task.Notify{Title: syncAlertTitle, Suppress: syncAlertSuppression, Alerts: alerts}
}

// inventoryTasks are the activities that reconcile the library, in the order a
// status page reads best.
func inventoryTasks(
	reporter synchronizer, settings *runtimeconfig.Current, enabled func(string) func() bool,
	targetIDs func() []string,
) []task.Definition {
	inventory := func(string) []task.Resource {
		return []task.Resource{{Name: resourceInventory, Exclusive: true}}
	}

	return []task.Definition{
		{
			Name:      taskSyncSource,
			Resources: inventory,
			Notify:    syncAlerts(true),
			Schedule:  task.Every(func() time.Duration { return syncservice.Interval }),
			InitialDelay: func() time.Duration {
				return settings.Values().Sync.InitialDelay
			},
			Enabled: enabled(taskSyncSource),
			// Only the scheduled read is expected on a clock. A library an
			// operator reads by hand is stale the moment they stop asking.
			StaleAfter: func() time.Duration {
				return settings.Values().Sync.StaleAfter
			},
			Backoff: task.Backoff{Base: syncBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				// No argument is every configured library. One names the library
				// alone, the same shape the targets take a slot in.
				if invocation.Argument == "" {
					result := reporter.RunPhase(ctx, syncservice.PhaseSource)

					return syncResult(&result)
				}
				result := reporter.RunSourceProvider(ctx, route.Provider(invocation.Argument))

				return syncResult(&result)
			}),
		},
		{
			Name:      taskSyncTarget,
			Resources: inventory,
			Notify:    syncAlerts(false),
			// A backstop, not the timely path: a successful source read already
			// asks for this immediately.
			Schedule:     task.Every(func() time.Duration { return targetBackstopInterval }),
			InitialDelay: func() time.Duration { return targetBackstopInterval },
			Enabled:      enabled(taskSyncTarget),
			Follows:      []string{taskSyncSource},
			// Backoff runs per slot, not for all at once, so one slot's fault
			// doesn't hold the rest back for as long as the backoff cap.
			FanOut:  targetIDs,
			Backoff: task.Backoff{Base: targetBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
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
			Notify:    syncAlerts(false),
			Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
				result := reporter.ClearTarget(ctx, invocation.Argument)

				return syncResult(&result)
			}),
		},
		{
			Name:      taskSurfaceAnnotate,
			Resources: inventory,
			Follows:   []string{taskSyncSource, taskSurfaceIndex},
			Backoff:   task.Backoff{Base: annotateBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
				if failed := reporter.Annotate(ctx); failed > 0 {
					return task.Result{Outcome: task.Failed, Detail: detailIncomplete}
				}

				return task.Result{Outcome: task.Succeeded}
			}),
		},
	}
}

// surfaceIndexTask rebuilds the surface index; its initial delay counts from
// the last build, not from process start, so a restart doesn't restart it.
func surfaceIndexTask(
	runner indexBuilder,
	settings *runtimeconfig.Current,
	enabled func(string) func() bool,
	lastBuiltAt time.Time,
) task.Definition {
	return task.Definition{
		Name:    taskSurfaceIndex,
		Enabled: enabled(taskSurfaceIndex),
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

// indexResult carries a rebuild's outcome into the task layer's vocabulary. A
// build that found nothing new still reached its upstream, so it reports
// unchanged rather than failed.
func indexResult(outcome osmindex.Outcome, err error) task.Result {
	if err != nil {
		return task.Result{Outcome: task.Failed, Detail: detailBuild}
	}
	switch outcome {
	case osmindex.Rebuilt:
		// A new generation makes every stored classification stale, and nothing
		// else notices that.
		return task.Result{Outcome: task.Succeeded}
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

	return task.Result{
		Outcome: outcome,
		Detail:  task.Detail(result.Failure),
		// Worth reconciling and classifying once any provider stored inventory,
		// even if another provider failed and dragged the aggregate outcome down.
		Advances: result.AnySourceStored(),
	}
}

// taskSurface adapts the manager to what the HTTP surface reads, starts and
// switches.
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
			Interval:  entry.Interval,
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

// Run starts one attempt under the service's own context, not the request's —
// so it ends when the service does, not when the request that asked for it does.
func (s taskSurface) Run(name, argument string) bool {
	return s.manager.Trigger(s.ctx, name, argument)
}

// taskStarter is the task layer as the HTTP boundary needs it; syncReporter is
// what only the reporter can answer.
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
