package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

const failureNotificationSuppression = 6 * time.Hour

// staleCategory is the suppression bucket for a stale trusted inventory. It
// shares the window and store of a phase failure, keyed apart from any real pair.
const staleCategory = "source:stale"

// RunState records terminal run data and failure-notification delivery state.
type RunState interface {
	// RecordSyncRun writes one terminal run down and returns the reference it
	// is recorded under, which is the name a notification about it can carry.
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) (string, error)
	RecordTargetRun(ctx context.Context, targetID string, finishedAt time.Time, outcome, detail string) error
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	// RecordFailureNotification records a delivered notification at sentAt, or
	// clears the category's suppression record when sentAt is the zero value.
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
	// LastSuccessfulPhaseCompletion returns when a phase last recorded a
	// success, which is what its trusted inventory age is measured against.
	LastSuccessfulPhaseCompletion(ctx context.Context, phase string) (completedAt time.Time, found bool, err error)
	// LastPhaseOutcome returns the outcome of the phase's most recent recorded run.
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
	phase atomic.Pointer[Phase]
	// notifications is read at the moment a message would go out rather than
	// captured, because an operator edits these while the service runs.
	notifications func() Notifications
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

// Notifications is everything the reporter reads before it reports a run. It is
// supplied as a function and read at each decision, never held.
type Notifications struct {
	Success SuccessNotification

	// StaleAfter bounds how long the trusted source inventory may go without a
	// successful refresh before it is reported and notified as stale.
	StaleAfter time.Duration

	// Enabled is the switch for the whole channel. Off suppresses a failure and a
	// stale inventory as surely as it suppresses a routine success.
	Enabled bool
}

// NewReporter creates a reporting runner with explicit dependencies. The
// settings it will read are checked once here, against what they say now.
func NewReporter(
	runner Runner, state RunState, notifier Notifier, notifications func() Notifications,
) (*Reporter, error) {
	if runner == nil || state == nil || notifier == nil || notifications == nil {
		return nil, errors.New("sync runner, run state, notifier, and notification settings are required")
	}
	current := notifications()
	if err := current.Success.validate(); err != nil {
		return nil, err
	}
	if current.StaleAfter <= 0 {
		return nil, errors.New("a stale-inventory bound must be positive")
	}

	return &Reporter{
		runner: runner, state: state, notifier: notifier, notifications: notifications, now: time.Now,
	}, nil
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
	// One instant for everything this pass settles, so a tick on a second boundary
	// cannot make the digest window and the staleness comparison disagree.
	now := r.now().UTC()
	// The digest is considered once the pass has recorded everything it did. Both
	// messages below are held back entirely when notifications are off.
	if notifications := r.notifications(); notifications.Enabled {
		if notifications.Success.Policy == SuccessDigest {
			r.notifyDigest(ctx, now)
		}
		// Checked every pass, whether or not the source phase ran: the inventory can
		// go stale while the schedule has it switched off.
		r.checkStaleness(ctx, now, sourceStored)
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

	// Nothing is sent, and nothing is written down as sent, while the channel is
	// switched off: turning it back on must not find a suppression window it
	// never heard the alert behind.
	if r.notifications().Enabled {
		switch result.Outcome {
		case OutcomeSucceeded:
			r.notifySuccess(ctx, result, reference, recovered)
		case OutcomeFailed, OutcomeBlocked:
			r.notifyFailure(ctx, result, reference, finishedAt)
		}
	}

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

// notifyFailure delivers one failure notification per phase and category, no more
// often than the suppression interval. The phase is part of the key: a failing
// library and a target that cannot be written to are separate problems.
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

// checkStaleness reports and notifies on the age of the trusted source inventory,
// independently of this tick's phases: a source that stopped succeeding leaves no
// new failure to notify on once its category is suppressed. sourceStored is this
// tick's outcome; a source that just succeeded ends a stale alert unconditionally.
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
			// Cleared only once the recovery has actually gone out: a clear that
			// ran regardless would end the incident on a message the operator
			// never received. Left uncleared on a write failure, the next success
			// tries the same recovery again rather than losing it.
			if clearErr := r.state.RecordFailureNotification(ctx, staleCategory, time.Time{}); clearErr != nil {
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
	// Compared in whole seconds, the same precision GET /v1/status reports
	// age_seconds and max_age_seconds in: a sub-second remainder must not let
	// this alert and that response disagree on whether the inventory is stale.
	if age/time.Second < r.notifications().StaleAfter/time.Second || (notified && now.Sub(lastSentAt) < failureNotificationSuppression) {
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
// message padded with the other phase's zeroes reads as though work was skipped.
// Every message names its run: the reference is random and means nothing on its
// own, which is what makes it safe to send.
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
