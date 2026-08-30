package sync

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"
)

// RunState records terminal run data and failure-notification delivery state.
type RunState interface {
	// RecordSyncRun writes one terminal run down and returns the reference it
	// is recorded under, which is the name a notification about it can carry.
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) (string, error)
	RecordTargetRun(ctx context.Context, targetID string, finishedAt time.Time, outcome, detail string) error
	SyncSchedule(ctx context.Context) (source, targets bool, err error)
}

// Reporter adds durable run recording around a synchronization service. It does
// not expose provider errors or route names.
type Reporter struct {
	runner Runner
	state  RunState
	now    func() time.Time
	// phase names the half being run right now, and is nil between the moment a
	// run is accepted and the moment its first half starts.
	phase atomic.Pointer[Phase]
	// surfaceIncomplete is how many stages the most recently completed annotation
	// pass could not classify. Read back by SurfaceIncomplete.
	surfaceIncomplete atomic.Int64
}

// Runner is the application service seam consumed by the reporter and
// scheduler. Each half of a synchronization is its own call.
type Runner interface {
	RunSource(ctx context.Context) Result
	RunTargets(ctx context.Context) Result
	// RunTarget reconciles exactly one configured target, on the same terms as
	// RunTargets scoped to that slot alone.
	RunTarget(ctx context.Context, targetID string) Result
	// ClearTarget deletes every owned route from exactly one configured
	// target and forgets its stage mappings. Only an operator asks for it.
	ClearTarget(ctx context.Context, targetID string) Result
	// AnnotateStored enriches the stored inventory and reports how much of it it
	// could not classify. The count never changes a run's outcome.
	AnnotateStored(ctx context.Context) (classified, failed int)
}

// NewReporter creates a reporting runner with explicit dependencies.
func NewReporter(runner Runner, state RunState) (*Reporter, error) {
	if runner == nil || state == nil {
		return nil, errors.New("a sync runner and run state are required")
	}

	return &Reporter{runner: runner, state: state, now: time.Now}, nil
}

// Run performs the scheduled synchronization, each switched-on phase recorded
// and reported on its own. An unreadable schedule is a failed source run.
func (r *Reporter) Run(ctx context.Context) Result {
	source, targets, err := r.state.SyncSchedule(ctx)
	if err != nil {
		return r.record(ctx, r.now().UTC(), &Result{
			Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureState,
		})
	}

	return r.runPhases(ctx, source, targets)
}

// RunBoth runs both halves whether or not the schedule has either switched on,
// which is what an operator asking for a synchronization means by it.
func (r *Reporter) RunBoth(ctx context.Context) Result {
	return r.runPhases(ctx, true, true)
}

// RunPhase runs one half of a synchronization, whether or not the schedule has
// that half switched on.
func (r *Reporter) RunPhase(ctx context.Context, phase Phase) Result {
	return r.runPhases(ctx, phase == PhaseSource, phase == PhaseTargets)
}

// ReconcileTarget reconciles exactly one configured target, on the same
// recording and reporting terms as a scheduled target phase.
func (r *Reporter) ReconcileTarget(ctx context.Context, targetID string) Result {
	return r.runPhasesWith(ctx, false, true, func(ctx context.Context) Result {
		return r.runner.RunTarget(ctx, targetID)
	})
}

// ClearTarget deletes every route this service owns from one target and forgets
// its stage mappings.
func (r *Reporter) ClearTarget(ctx context.Context, targetID string) Result {
	return r.runPhasesWith(ctx, false, true, func(ctx context.Context) Result {
		return r.runner.ClearTarget(ctx, targetID)
	})
}

// Annotate runs one classification pass, touching only the local index and cache.
func (r *Reporter) Annotate(ctx context.Context) {
	r.annotate(ctx)
}

// SurfaceIncomplete reports how many stages the most recently completed
// annotation pass could not classify. Zero before any pass has run.
func (r *Reporter) SurfaceIncomplete() int {
	return int(r.surfaceIncomplete.Load())
}

// Running reports which half is in flight, if any. Whether a run is under way
// at all is the task layer's answer, not this one's.
func (r *Reporter) Running() (Phase, bool) {
	phase := r.phase.Load()
	if phase == nil {
		return "", false
	}

	return *phase, true
}

// enter records which half is being run. The phase is a parameter so each call
// stores a copy of its own, rather than rewriting a value a reader may hold.
func (r *Reporter) enter(phase Phase) {
	r.phase.Store(&phase)
}

// runPhases runs the requested phases in order and returns the last result.
// Source before targets, so one tick carries a change all the way through.
func (r *Reporter) runPhases(ctx context.Context, source, targets bool) Result {
	return r.runPhasesWith(ctx, source, targets, r.runner.RunTargets)
}

// runPhasesWith is runPhases parameterized over what reconciles the target half,
// so a single-target trigger shares every recording and reporting rule.
func (r *Reporter) runPhasesWith(ctx context.Context, source, targets bool, runTargets func(context.Context) Result) Result {
	defer r.phase.Store(nil)

	result := Result{Outcome: OutcomeSkipped}
	sourceStored := false
	if source {
		r.enter(PhaseSource)
		result = r.run(ctx, r.runner.RunSource)
		sourceStored = result.Outcome == OutcomeSucceeded
	}
	if targets {
		r.enter(PhaseTargets)
		result = r.run(ctx, runTargets)
	}
	// Enrichment follows any successful source refresh, changed or not: an
	// unchanged library can still hold stages an earlier pass never got to.
	// Whoever started this pass is what starts that.
	result.SourceStored = sourceStored

	return result
}

// annotate runs one classification pass and records what it could not finish,
// for SurfaceIncomplete to read back.
func (r *Reporter) annotate(ctx context.Context) {
	_, failed := r.runner.AnnotateStored(ctx)
	r.surfaceIncomplete.Store(int64(failed))
}

func (r *Reporter) run(ctx context.Context, phase func(context.Context) Result) Result {
	startedAt := r.now().UTC()
	result := phase(ctx)
	if result.Outcome == OutcomeSkipped {
		return result
	}

	return r.record(ctx, startedAt, &result)
}

// record writes the run down. What is announced about it is the task layer's,
// which sees every activity rather than this one.
func (r *Reporter) record(ctx context.Context, startedAt time.Time, result *Result) Result {
	finishedAt := r.now().UTC()
	_, err := r.state.RecordSyncRun(
		ctx,
		string(result.Phase),
		startedAt,
		finishedAt,
		string(result.Outcome),
		string(result.Failure),
		result.SourceStages,
		result.Created,
		result.Updated,
		result.Deleted,
	)
	if err != nil {
		return *result
	}
	r.recordTargetRuns(ctx, finishedAt, result.Targets)

	return *result
}

// recordTargetRuns writes down what each slot's own reconciliation came to. A
// slot that cannot be recorded is passed over rather than stopping the rest:
// losing one row costs a stale line on a status page.
func (r *Reporter) recordTargetRuns(ctx context.Context, finishedAt time.Time, targets []TargetResult) {
	for _, target := range targets {
		if err := r.state.RecordTargetRun(
			ctx,
			target.ID,
			finishedAt,
			string(target.Outcome),
			string(target.Failure),
		); err != nil {
			slog.Warn("target run not recorded", "target", target.ID, "reason", "state")
		}
	}
}
