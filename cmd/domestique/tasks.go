package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/task"
)

// The background activities this service runs.
const (
	taskSyncSource         = httpapi.TaskSyncSource
	taskSyncTarget         = httpapi.TaskSyncTarget
	taskSyncClear          = httpapi.TaskSyncClear
	taskSurfaceAnnotate    = "surface:annotate"
	taskSurfaceIndex       = httpapi.TaskSurfaceIndex
	taskRideModelPredict   = httpapi.TaskRideModelPredict
	taskRideModelCalibrate = httpapi.TaskRideModelCalibrate
	taskActivityPoll       = httpapi.TaskActivityPoll
)

// Everything reading or writing the trusted inventory takes resourceInventory
// so none overlap; a surface index build takes neither and runs beside them.
// Recorded activities are neither, so polling them runs beside both.
const (
	resourceInventory    = "inventory"
	resourceSurfaceIndex = "surface-index"
	resourceActivities   = "activities"
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
	syncBackoffBase       = 30 * time.Second
	targetBackoffBase     = time.Hour
	enrichmentBackoffBase = 5 * time.Minute
	backoffCap            = 6 * time.Hour
)

// targetBackstopInterval is how often targets are reconciled unasked; a
// successful source read already asks for them at once, so this only catches a
// slot that failed alone or an operator with the read switched off.
const targetBackstopInterval = 6 * time.Hour

// activityPollInterval is how often a target's recorded activities are read. A
// ride is finished long before anybody asks about it, and each poll spends from
// the same daily Wahoo budget the reconciliation does.
const activityPollInterval = 12 * time.Hour

// A rider's form moves over months, so the pair is refitted weekly; the first
// fit waits an hour rather than spending a restart on regression.
const (
	calibrationInterval     = 7 * 24 * time.Hour
	calibrationInitialDelay = time.Hour
)

// The reasons a surface index rebuild reports. Both are stable words a status
// page may show; neither carries an upstream URL or a local path.
const (
	detailBuild     task.Detail = "build"
	detailNoRegions task.Detail = "no_regions"
)

// The reasons a calibration declines, and the one it fails on. Each is a stable
// word a status page may show, and none names a ride.
const (
	detailTooFewRides task.Detail = "too_few_rides"
	detailDegenerate  task.Detail = "degenerate"
	detailModelState  task.Detail = "state"
)

// detailIncomplete is why a classification or prediction pass that left stages
// unfinished is recorded as failed rather than succeeded. detailStoppedEarly is
// why one is failed even with nothing named unfinished: it never got far enough
// to say, the same word the pass's own log line already uses for this.
const (
	detailIncomplete   task.Detail = "incomplete"
	detailStoppedEarly task.Detail = "stopped_early"
)

// The reasons an activity poll reports. Each is a stable word a status page may
// show; none carries a ride, a name or a credential.
const (
	detailActivityAuthorization task.Detail = "authorization"
	detailActivityUpstream      task.Detail = "upstream"
	detailActivityState         task.Detail = "state"
	// detailActivitySkipped marks a poll that set an unreadable activity aside,
	// so an account quietly missing rides is visible in its run record.
	detailActivitySkipped task.Detail = "skipped"
)

// synchronizer is the sync work the task layer starts; indexBuilder is the
// surface index rebuild. Both live here so definitions can be read without a
// reporter or builder behind them.
type synchronizer interface {
	RunPhase(ctx context.Context, phase syncservice.Phase) syncservice.Result
	RunSourceProvider(ctx context.Context, provider route.Provider) syncservice.Result
	ReconcileTarget(ctx context.Context, targetID string) syncservice.Result
	ClearTarget(ctx context.Context, targetID string) syncservice.Result
	Annotate(ctx context.Context) (failed int, err error)
	Predict(ctx context.Context) (failed int, err error)
}

// activityPoller is the activity work the task layer starts.
type activityPoller interface {
	Poll(ctx context.Context, targetID string) activity.Result
}

// rideCorpus is the stored corpus a calibration fits and where the pair it
// finds is kept; coefficientHolder is what predicts with the pair in force.
type rideCorpus interface {
	ActivityRides(ctx context.Context) ([]ridemodel.Ride, error)
	StoreRideModelCoefficients(ctx context.Context, coefficients ridemodel.Coefficients, now time.Time) error
}

type coefficientHolder interface {
	fingerprint() string
	reload(ctx context.Context) error
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
			Backoff:   task.Backoff{Base: enrichmentBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
				failed, err := reporter.Annotate(ctx)
				if err != nil {
					return task.Result{Outcome: task.Failed, Detail: detailStoppedEarly, Advances: true}
				}
				if failed > 0 {
					return task.Result{Outcome: task.Failed, Detail: detailIncomplete, Advances: true}
				}

				return task.Result{Outcome: task.Succeeded}
			}),
		},
		{
			Name:      taskRideModelPredict,
			Resources: inventory,
			Follows:   []string{taskSyncSource, taskRideModelCalibrate},
			Backoff:   task.Backoff{Base: enrichmentBackoffBase, Cap: backoffCap},
			Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
				failed, err := reporter.Predict(ctx)
				if err != nil {
					return task.Result{Outcome: task.Failed, Detail: detailStoppedEarly}
				}
				if failed > 0 {
					return task.Result{Outcome: task.Failed, Detail: detailIncomplete}
				}

				return task.Result{Outcome: task.Succeeded}
			}),
		},
	}
}

// activityPollTask reads each target's recorded activities into the store. It
// takes its own resource rather than the inventory: it reads a rider's Wahoo
// account and writes only activity rows, so it runs beside a reconciliation.
func activityPollTask(
	poller activityPoller, enabled func(string) func() bool, targetIDs func() []string,
) task.Definition {
	return task.Definition{
		Name:    taskActivityPoll,
		Enabled: enabled(taskActivityPoll),
		Resources: func(string) []task.Resource {
			return []task.Resource{{Name: resourceActivities, Exclusive: true}}
		},
		Schedule:     task.Every(func() time.Duration { return activityPollInterval }),
		InitialDelay: func() time.Duration { return activityPollInterval },
		FanOut:       targetIDs,
		Backoff:      task.Backoff{Base: targetBackoffBase, Cap: backoffCap},
		Run: task.RunnerFunc(func(ctx context.Context, invocation task.Invocation) task.Result {
			if invocation.Argument != "" {
				return activityResult(poller.Poll(ctx, invocation.Argument))
			}

			return pollEveryTarget(ctx, poller, targetIDs())
		}),
	}
}

// pollEveryTarget polls every slot, reporting the most serious thing that
// happened: one rider's dead token must not stop another's rides being read.
func pollEveryTarget(ctx context.Context, poller activityPoller, targetIDs []string) task.Result {
	aggregate := task.Result{Outcome: task.NotReady}
	for _, targetID := range targetIDs {
		result := activityResult(poller.Poll(ctx, targetID))
		// At equal severity only a result with a detail displaces the aggregate,
		// so one slot's skip is not hidden behind another's clean success.
		if severity(result.Outcome) > severity(aggregate.Outcome) ||
			(severity(result.Outcome) == severity(aggregate.Outcome) && result.Detail != "") {
			aggregate = result
		}
	}

	return aggregate
}

// severity orders what one slot came to, worst highest, so an aggregate over
// every slot reports the outcome an operator would act on first.
func severity(outcome task.Outcome) int {
	switch outcome {
	case task.Failed:
		return 3
	case task.Succeeded:
		return 2
	case task.Unchanged:
		return 1
	default:
		return 0
	}
}

// activityResult carries a poll's outcome into the task layer's vocabulary.
func activityResult(result activity.Result) task.Result {
	switch result.Outcome {
	case activity.Polled:
		if result.Skipped > 0 {
			return task.Result{Outcome: task.Succeeded, Detail: detailActivitySkipped}
		}

		return task.Result{Outcome: task.Succeeded}
	case activity.Unchanged:
		return task.Result{Outcome: task.Unchanged}
	case activity.NotReady:
		return task.Result{Outcome: task.NotReady}
	case activity.Failed:
	}

	return task.Result{Outcome: task.Failed, Detail: activityDetail(result.Failure)}
}

func activityDetail(failure activity.Failure) task.Detail {
	switch failure {
	case activity.FailureAuthorization:
		return detailActivityAuthorization
	case activity.FailureState:
		return detailActivityState
	case activity.FailureUpstream, activity.FailureNone:
	}

	return detailActivityUpstream
}

// rideModelCalibrateTask refits the coefficient pair from every target's
// recorded activities. It holds the activities so a poll cannot write into the
// corpus mid-fit, and leaves the pair in force alone when the fit is refused.
func rideModelCalibrateTask(
	corpus rideCorpus, model coefficientHolder, enabled func(string) func() bool, now func() time.Time,
) task.Definition {
	return task.Definition{
		Name:    taskRideModelCalibrate,
		Enabled: enabled(taskRideModelCalibrate),
		Resources: func(string) []task.Resource {
			return []task.Resource{{Name: resourceActivities, Exclusive: true}}
		},
		Schedule:     task.Every(func() time.Duration { return calibrationInterval }),
		InitialDelay: func() time.Duration { return calibrationInitialDelay },
		Backoff:      task.Backoff{Base: enrichmentBackoffBase, Cap: backoffCap},
		Run: task.RunnerFunc(func(ctx context.Context, _ task.Invocation) task.Result {
			return calibrate(ctx, corpus, model, now())
		}),
	}
}

func calibrate(ctx context.Context, corpus rideCorpus, model coefficientHolder, now time.Time) task.Result {
	rides, err := corpus.ActivityRides(ctx)
	if err != nil {
		return task.Result{Outcome: task.Failed, Detail: detailModelState}
	}
	fitted, err := ridemodel.Fit(rides, now)
	switch {
	case errors.Is(err, ridemodel.ErrTooFewRides):
		// Both branches are the corpus's own shape rather than a fault: nothing
		// is wrong, there is just not enough of it yet, or not ever for a rider
		// whose rides cannot separate distance from ascent.
		return task.Result{Outcome: task.NotReady, Detail: detailTooFewRides}
	case err != nil:
		return task.Result{Outcome: task.NotReady, Detail: detailDegenerate}
	}
	// Storing the same pair again would drop every cached prediction for nothing.
	if fitted.Fingerprint == model.fingerprint() {
		return task.Result{Outcome: task.Unchanged}
	}
	if err := corpus.StoreRideModelCoefficients(ctx, fitted, now); err != nil {
		return task.Result{Outcome: task.Failed, Detail: detailModelState}
	}
	if err := model.reload(ctx); err != nil {
		return task.Result{Outcome: task.Failed, Detail: detailModelState}
	}

	return task.Result{Outcome: task.Succeeded}
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
