package sync

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// RunState records terminal run data and failure-notification delivery state.
type RunState interface {
	// RecordSyncRun writes one terminal run down and returns the reference it
	// is recorded under, which is the name a notification about it can carry.
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) (string, error)
	RecordTargetRun(ctx context.Context, targetID string, finishedAt time.Time, outcome, detail string) error
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
	// RunSourceProvider reads exactly one configured source library.
	RunSourceProvider(ctx context.Context, provider route.Provider) Result
	// ClearTarget deletes every owned route from exactly one configured
	// target and forgets its stage mappings. Only an operator asks for it.
	ClearTarget(ctx context.Context, targetID string) Result
	// AnnotateStored enriches the stored inventory and reports how much of it it
	// could not classify; a non-nil err means the pass stopped before finishing.
	AnnotateStored(ctx context.Context) (classified, failed int, err error)
	// PredictStored predicts the stored inventory's moving time and reports how
	// much of it it could not predict; a non-nil err means the pass stopped
	// before finishing.
	PredictStored(ctx context.Context) (predicted, failed int, err error)
}

// NewReporter creates a reporting runner with explicit dependencies.
func NewReporter(runner Runner, state RunState) (*Reporter, error) {
	if runner == nil || state == nil {
		return nil, errors.New("a sync runner and run state are required")
	}

	return &Reporter{runner: runner, state: state, now: time.Now}, nil
}

// RunPhase runs one half of a synchronization, whether or not the schedule has
// that half switched on.
func (r *Reporter) RunPhase(ctx context.Context, phase Phase) Result {
	return r.runPhases(ctx, phase == PhaseSource, phase == PhaseTargets)
}

// RunSourceProvider reads exactly one library, on the same recording terms as
// the whole source phase.
func (r *Reporter) RunSourceProvider(ctx context.Context, provider route.Provider) Result {
	return r.runPhasesWith(ctx, true, false, func(ctx context.Context) Result {
		return r.runner.RunSourceProvider(ctx, provider)
	}, nil)
}

// ReconcileTarget reconciles exactly one configured target, on the same
// recording and reporting terms as a scheduled target phase.
func (r *Reporter) ReconcileTarget(ctx context.Context, targetID string) Result {
	return r.runPhasesWith(ctx, false, true, nil, func(ctx context.Context) Result {
		return r.runner.RunTarget(ctx, targetID)
	})
}

// ClearTarget deletes every route this service owns from one target and forgets
// its stage mappings.
func (r *Reporter) ClearTarget(ctx context.Context, targetID string) Result {
	return r.runPhasesWith(ctx, false, true, nil, func(ctx context.Context) Result {
		return r.runner.ClearTarget(ctx, targetID)
	})
}

// Annotate runs one classification pass, touching only the local index and
// cache, and reports how many stages it could not classify. A non-nil err
// means the pass stopped before it could finish walking the inventory.
func (r *Reporter) Annotate(ctx context.Context) (failed int, err error) {
	return r.annotate(ctx)
}

// Predict runs one ride-model prediction pass, touching only the local index
// and cache, and reports how many stages it could not predict. A non-nil err
// means the pass stopped before it could finish walking the inventory.
func (r *Reporter) Predict(ctx context.Context) (failed int, err error) {
	_, failed, err = r.runner.PredictStored(ctx)

	return failed, err //nolint:wrapcheck // the runner already names what it was doing
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
	return r.runPhasesWith(ctx, source, targets, nil, nil)
}

// runPhasesWith is runPhases parameterized over what runs each half; a nil
// half falls back to the service's own RunSource/RunTargets.
func (r *Reporter) runPhasesWith(
	ctx context.Context,
	source, targets bool,
	runSource, runTargets func(context.Context) Result,
) Result {
	defer r.phase.Store(nil)

	result := Result{Outcome: OutcomeSkipped}
	sourceStored := false
	if source {
		if runSource == nil {
			runSource = r.runner.RunSource
		}
		r.enter(PhaseSource)
		result = r.run(ctx, runSource)
		sourceStored = result.AnySourceStored()
	}
	if targets {
		if runTargets == nil {
			runTargets = r.runner.RunTargets
		}
		r.enter(PhaseTargets)
		result = r.run(ctx, runTargets)
	}
	// Enrichment is decided by whoever started this pass, not by the reporter.
	result.SourceStored = sourceStored

	return result
}

// annotate runs one classification pass and records what it could not finish,
// for SurfaceIncomplete to read back.
func (r *Reporter) annotate(ctx context.Context) (failed int, err error) {
	_, failed, err = r.runner.AnnotateStored(ctx)
	r.surfaceIncomplete.Store(int64(failed))

	return failed, err //nolint:wrapcheck // the runner already names what it was doing
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
