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

// staleCategory names the notification suppression bucket for a stale trusted
// source inventory. It shares the same suppression window and store as an
// ordinary phase failure, keyed apart from any real phase-and-failure pair.
const staleCategory = "source:stale"

// RunState records terminal run data and failure-notification delivery state.
type RunState interface {
	// RecordSyncRun writes one terminal run down and returns the reference it
	// is recorded under, which is the name a notification about it can carry.
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) (string, error)
	RecordTargetRun(ctx context.Context, targetID string, finishedAt time.Time, outcome, detail string) error
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
	// LastSuccessfulPhaseCompletion returns when a phase last recorded a
	// success, which is what its trusted inventory age is measured against.
	LastSuccessfulPhaseCompletion(ctx context.Context, phase string) (completedAt time.Time, found bool, err error)
	// LastPhaseOutcome returns the outcome of the phase's most recent recorded
	// run, which is what tells a success that ends a failure apart from a
	// routine one.
	LastPhaseOutcome(ctx context.Context, phase string) (outcome string, found bool, err error)
	// LastDigestNotification returns when the last digest was sent and the
	// highest run it covered, which together bound the next one.
	LastDigestNotification(ctx context.Context) (sentAt time.Time, lastRunID int64, found bool, err error)
	RecordDigestNotification(ctx context.Context, sentAt time.Time, lastRunID int64) error
	// ForEachSuccessfulRunAfter visits every successful run recorded after the
	// given one, carrying the counts a digest totals and nothing else.
	ForEachSuccessfulRunAfter(ctx context.Context, runID int64, visit func(id int64, phase string, created, updated, deleted int) error) error
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
	phase      atomic.Pointer[Phase]
	success    SuccessNotification
	running    atomic.Bool
	triggered  stdsync.WaitGroup
	staleAfter time.Duration
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

// NewReporter creates a reporting runner with explicit dependencies. staleAfter
// bounds how long the trusted source inventory may go without a successful
// refresh before it is reported and notified as stale.
func NewReporter(
	runner Runner, state RunState, notifier Notifier, success SuccessNotification, staleAfter time.Duration,
) (*Reporter, error) {
	if runner == nil || state == nil || notifier == nil {
		return nil, errors.New("sync runner, run state, and notifier are required")
	}
	if err := success.validate(); err != nil {
		return nil, err
	}
	if staleAfter <= 0 {
		return nil, errors.New("a stale-inventory bound must be positive")
	}

	return &Reporter{
		runner: runner, state: state, notifier: notifier, success: success, staleAfter: staleAfter, now: time.Now,
	}, nil
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
	// The digest is considered once the pass has recorded everything it did, so
	// its window closes on a whole pass rather than between two halves.
	if r.success.Policy == SuccessDigest {
		r.notifyDigest(ctx, r.now().UTC())
	}
	// Checked every pass, whether or not the source phase ran this tick: the
	// inventory can go stale while the schedule has it switched off, and this
	// reads only local state, so it costs no provider call either way.
	r.checkStaleness(ctx, r.now().UTC(), sourceStored)
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
	// Ask before recording. The question is what the phase did last, and this
	// run is about to become the answer to it.
	recovered := result.Outcome == OutcomeSucceeded && r.recovered(ctx, result.Phase)
	reference, err := r.state.RecordSyncRun(
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

	switch result.Outcome {
	case OutcomeSucceeded:
		r.notifySuccess(ctx, result, reference, recovered)
	case OutcomeFailed, OutcomeBlocked:
		r.notifyFailure(ctx, result, reference, finishedAt)
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
func (r *Reporter) notifyFailure(ctx context.Context, result *Result, reference string, now time.Time) {
	if result.Failure == FailureNone {
		return
	}
	category := string(result.Phase) + ":" + string(result.Failure)
	lastSentAt, found, err := r.state.LastFailureNotification(ctx, category)
	if err != nil || (found && now.Sub(lastSentAt) < failureNotificationSuppression) {
		return
	}
	if err := r.notifier.Send(ctx, "Domestique sync failed", failureMessage(result, reference)); err != nil {
		return
	}
	if err := r.state.RecordFailureNotification(ctx, category, now); err != nil {
		return
	}
}

// checkStaleness reports and notifies on the age of the trusted source
// inventory, independently of whatever this tick's phases did — a source that
// has stopped succeeding leaves no new failure to notify on once its failure
// category is already suppressed, and this is what still catches that.
//
// sourceStored is this tick's own source-phase outcome. A source that just
// succeeded ends any outstanding stale alert unconditionally, the same way an
// ordinary recovery is never held back by policy; a source that did not
// succeed this tick is checked against how long it has been since one did.
func (r *Reporter) checkStaleness(ctx context.Context, now time.Time, sourceStored bool) {
	lastSentAt, notified, err := r.state.LastFailureNotification(ctx, staleCategory)
	if err != nil {
		return
	}
	if sourceStored {
		if notified {
			if sendErr := r.notifier.Send(
				ctx, "Domestique sync", "source recovered: trusted inventory is fresh again",
			); sendErr != nil {
				return
			}
		}

		return
	}

	lastSuccess, found, err := r.state.LastSuccessfulPhaseCompletion(ctx, string(PhaseSource))
	if err != nil || !found {
		return
	}
	age := now.Sub(lastSuccess)
	if age < r.staleAfter || (notified && now.Sub(lastSentAt) < failureNotificationSuppression) {
		return
	}
	if err := r.notifier.Send(ctx, "Domestique sync failed", staleMessage(age)); err != nil {
		return
	}
	if err := r.state.RecordFailureNotification(ctx, staleCategory, now); err != nil {
		return
	}
}

func staleMessage(age time.Duration) string {
	return fmt.Sprintf("source stale: trusted inventory age=%s", age.Round(time.Minute))
}

// successMessage reports the counts the finished phase actually produced. A
// source run that listed forty stages and a target run that changed none are
// different events, and a message padded with the other phase's zeroes reads as
// though work was skipped.
//
// Every message names the run it is about, so an operator reading it on a phone
// can find that run and nothing else in the history. The reference is random and
// means nothing on its own, which is what makes it safe to send: it says which
// run without saying anything about it.
func successMessage(result *Result, reference string) string {
	if result.Phase == PhaseSource {
		return fmt.Sprintf("source succeeded: source_stages=%d run=%s", result.SourceStages, reference)
	}

	return fmt.Sprintf(
		"targets succeeded: source_stages=%d created=%d updated=%d deleted=%d run=%s",
		result.SourceStages,
		result.Created,
		result.Updated,
		result.Deleted,
		reference,
	)
}

func failureMessage(result *Result, reference string) string {
	return string(result.Phase) + " failed: " + string(result.Failure) + " run=" + reference
}
