package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	stdsync "sync"
	"sync/atomic"
	"time"
)

const failureNotificationSuppression = 6 * time.Hour

// RunState records terminal run data and failure-notification delivery state.
type RunState interface {
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error
	RecordTargetRun(ctx context.Context, targetID string, finishedAt time.Time, outcome, detail string) error
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
	SyncSchedule(ctx context.Context) (source, targets bool, err error)
}

// Notifier delivers already-safe notification text.
type Notifier interface {
	Send(ctx context.Context, title, message string) error
}

// Reporter adds durable run recording and notification policy around a
// synchronization service. It does not expose provider errors or route names.
type Reporter struct {
	runner   Runner
	state    RunState
	notifier Notifier
	now      func() time.Time
	// phase names the half being run right now, and is nil between the moment a
	// run is accepted and the moment its first half starts.
	phase     atomic.Pointer[Phase]
	running   atomic.Bool
	triggered stdsync.WaitGroup
}

// Runner is the application service seam consumed by the reporter and
// scheduler. Each half of a synchronization is its own call, because each is
// separately switched and separately triggered.
type Runner interface {
	RunSource(ctx context.Context) Result
	RunTargets(ctx context.Context) Result
	// AnnotateStored enriches the stored inventory. It reports nothing because
	// nothing it does may change a run's outcome.
	AnnotateStored(ctx context.Context)
}

// NewReporter creates a reporting runner with explicit dependencies.
func NewReporter(runner Runner, state RunState, notifier Notifier) (*Reporter, error) {
	if runner == nil || state == nil || notifier == nil {
		return nil, errors.New("sync runner, run state, and notifier are required")
	}

	return &Reporter{runner: runner, state: state, notifier: notifier, now: time.Now}, nil
}

// Run performs the scheduled synchronization: whichever phases the operator has
// left switched on, in order, each recorded and reported on its own.
//
// A schedule that cannot be read runs nothing. The alternative is contacting a
// provider the operator may have switched off, and the difference between "off"
// and "unreadable" is not one a timer should guess at. The unreadable schedule
// is recorded as a failed source run so it reaches the same notification path as
// any other state failure rather than passing as a quiet tick.
func (r *Reporter) Run(ctx context.Context) Result {
	if !r.running.CompareAndSwap(false, true) {
		return Result{Outcome: OutcomeSkipped}
	}
	defer r.running.Store(false)

	source, targets, err := r.state.SyncSchedule(ctx)
	if err != nil {
		return r.record(ctx, r.now().UTC(), &Result{
			Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureState,
		})
	}

	return r.runPhases(ctx, source, targets)
}

// Trigger starts a manual synchronization of both phases in the background. It
// returns false when a scheduled or another manual synchronization is already
// running.
//
// A manual trigger runs the phase whether or not the timer is allowed to: the
// switches govern what happens unattended, and an operator asking for a run now
// has already decided.
func (r *Reporter) Trigger(ctx context.Context) bool {
	return r.trigger(ctx, true, true)
}

// TriggerPhase starts one manual phase in the background, on the same terms as
// Trigger.
func (r *Reporter) TriggerPhase(ctx context.Context, phase Phase) bool {
	return r.trigger(ctx, phase == PhaseSource, phase == PhaseTargets)
}

func (r *Reporter) trigger(ctx context.Context, source, targets bool) bool {
	if !source && !targets {
		return false
	}
	if !r.running.CompareAndSwap(false, true) {
		return false
	}
	r.triggered.Go(func() {
		defer r.running.Store(false)
		_ = r.runPhases(ctx, source, targets)
	})

	return true
}

// Running reports what this process has under way: the half in flight, and
// whether a run is under way at all.
//
// The two answers are separate because a run is accepted before its first half
// starts. Reporting nothing in that window would leave a status response
// falling back to the last finished run, which would claim a terminal result
// for work that has not begun.
func (r *Reporter) Running() (Phase, bool) {
	// Read the phase first: a finishing run clears its phase before it clears
	// running, so this order can report a run without the half it is in, but
	// never a half of a run that has already finished.
	phase := r.phase.Load()
	running := r.running.Load()
	if phase == nil || !running {
		return "", running
	}

	return *phase, true
}

// enter records which half is being run. The phase is a parameter so each call
// stores a copy of its own, rather than rewriting a value a reader may hold.
func (r *Reporter) enter(phase Phase) {
	r.phase.Store(&phase)
}

// runPhases runs the requested phases in order and returns the last result.
//
// Reading the source before writing to the targets is what makes one tick carry
// a change all the way through; the order also means a failed read leaves the
// targets reconciling the last inventory known to be whole rather than nothing.
func (r *Reporter) runPhases(ctx context.Context, source, targets bool) Result {
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
		result = r.run(ctx, r.runner.RunTargets)
	}
	// Enrichment comes after everything a rider is waiting for, and only when
	// this pass stored something new to enrich.
	if sourceStored {
		r.runner.AnnotateStored(ctx)
	}

	return result
}

// Wait waits for any manual synchronization accepted by Trigger to finish.
func (r *Reporter) Wait() {
	r.triggered.Wait()
}

func (r *Reporter) run(ctx context.Context, phase func(context.Context) Result) Result {
	startedAt := r.now().UTC()
	result := phase(ctx)
	if result.Outcome == OutcomeSkipped {
		return result
	}

	return r.record(ctx, startedAt, &result)
}

// record writes the run down and notifies on its outcome. Notification or
// state-delivery failures never rewrite the outcome they describe.
func (r *Reporter) record(ctx context.Context, startedAt time.Time, result *Result) Result {
	finishedAt := r.now().UTC()
	if err := r.state.RecordSyncRun(
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
	); err != nil {
		return *result
	}
	r.recordTargetRuns(ctx, finishedAt, result.Targets)

	switch result.Outcome {
	case OutcomeSucceeded:
		if err := r.notifier.Send(ctx, "Domestique sync", successMessage(result)); err != nil {
			return *result
		}
	case OutcomeFailed, OutcomeBlocked:
		r.notifyFailure(ctx, result, finishedAt)
	}

	return *result
}

// recordTargetRuns writes down what each slot's own reconciliation came to.
//
// A slot that cannot be recorded is passed over rather than allowed to stop the
// rest: these rows report convergence, and losing one costs an operator a stale
// line on a status page, whereas abandoning the loop would cost them every line
// after it.
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

// notifyFailure delivers one failure notification per phase and category, no
// more often than the suppression interval.
//
// The phase is part of the key. A library that has been failing to load all
// morning must not be the reason a target stops reporting that it can no longer
// be written to: they are separate problems with separate remedies, and each is
// worth one alert.
func (r *Reporter) notifyFailure(ctx context.Context, result *Result, now time.Time) {
	if result.Failure == FailureNone {
		return
	}
	category := string(result.Phase) + ":" + string(result.Failure)
	lastSentAt, found, err := r.state.LastFailureNotification(ctx, category)
	if err != nil || (found && now.Sub(lastSentAt) < failureNotificationSuppression) {
		return
	}
	if err := r.notifier.Send(ctx, "Domestique sync failed", failureMessage(result)); err != nil {
		return
	}
	if err := r.state.RecordFailureNotification(ctx, category, now); err != nil {
		return
	}
}

// successMessage reports the counts the finished phase actually produced. A
// source run that listed forty stages and a target run that changed none are
// different events, and a message padded with the other phase's zeroes reads as
// though work was skipped.
func successMessage(result *Result) string {
	if result.Phase == PhaseSource {
		return fmt.Sprintf("source succeeded: source_stages=%d", result.SourceStages)
	}

	return fmt.Sprintf(
		"targets succeeded: source_stages=%d created=%d updated=%d deleted=%d",
		result.SourceStages,
		result.Created,
		result.Updated,
		result.Deleted,
	)
}

func failureMessage(result *Result) string {
	return string(result.Phase) + " failed: " + string(result.Failure)
}
