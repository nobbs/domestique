package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReporterRecordsAndNotifiesEverySuccess(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 3},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3, Created: 2, Updated: 1},
	}
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	assert.Equal(t, 2, state.runs, "recorded runs")
	assert.Equal(t, []string{"source", "targets"}, state.phases, "recorded phases")
	assert.Equal(t, []notification{
		{title: "Domestique sync", message: "source succeeded: source_stages=3 run=" + recordedRunReference},
		{title: "Domestique sync", message: "targets succeeded: source_stages=3 created=2 updated=1 deleted=0 run=" + recordedRunReference},
	}, notifier.messages)
}

// The switches govern the timer, so a scheduled tick performs only what is still
// switched on and leaves no record of the half it did not run.
func TestReporterRunsOnlyTheScheduledPhases(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 4},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded},
	}
	state := &fakeRunState{source: true, targets: false}
	reporter := newReporter(t, runner, state, &fakeNotifier{})

	reporter.Run(t.Context())
	assert.Zero(t, runner.targetRuns, "the target phase ran while its schedule was off")
	assert.Equal(t, []string{"source"}, state.phases, "recorded phases")
}

func TestReporterRunsNothingWhenBothPhasesAreSwitchedOff(t *testing.T) {
	runner := &reportingRunner{}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state, &fakeNotifier{})

	assert.Equal(t, OutcomeSkipped, reporter.Run(t.Context()).Outcome, "Run() outcome")
	assert.Zero(t, runner.sourceRuns+runner.targetRuns, "a phase ran with both schedules off")
	assert.Zero(t, state.runs, "recorded runs")
}

// "Off" and "unreadable" are different answers, and a timer must not act on the
// second as though it were the first.
func TestReporterRunsNothingAndReportsAnUnreadableSchedule(t *testing.T) {
	runner := &reportingRunner{}
	state := &fakeRunState{scheduleErr: errors.New("state unavailable")}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)

	result := reporter.Run(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "Run() outcome")
	assert.Equal(t, FailureState, result.Failure, "Run() failure")
	assert.Zero(t, runner.sourceRuns+runner.targetRuns, "a phase ran on an unreadable schedule")
	assert.Equal(t, 1, state.runs, "recorded runs")
	assert.Len(t, notifier.messages, 1, "want one failure alert")
}

// An operator asking for a phase has already decided; the switch only ever
// governed what happens unattended.
func TestReporterTriggersAPhaseTheScheduleHasSwitchedOff(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state, &fakeNotifier{})

	require.True(t, reporter.TriggerPhase(t.Context(), PhaseTargets), "TriggerPhase() rejected the run")
	reporter.Wait()
	assert.Equal(t, 1, runner.targetRuns, "target runs")
	assert.Zero(t, runner.sourceRuns, "the source phase ran for a target trigger")
}

func TestReporterSuppressesMatchingFailureForSixHours(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureDestination}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	now = now.Add(5 * time.Hour)
	reporter.Run(t.Context())
	now = now.Add(time.Hour)
	reporter.Run(t.Context())
	assert.Equal(t, []notification{
		{title: "Domestique sync failed", message: "targets failed: destination run=" + recordedRunReference},
		{title: "Domestique sync failed", message: "targets failed: destination run=" + recordedRunReference},
	}, notifier.messages)
}

// A library that has been failing to load all morning must not be the reason a
// target stops reporting that it can no longer be written to.
func TestReporterAlertsOnEachPhaseFailingTheSameWay(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureState},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState},
	}
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	assert.Equal(t, []notification{
		{title: "Domestique sync failed", message: "source failed: state run=" + recordedRunReference},
		{title: "Domestique sync failed", message: "targets failed: state run=" + recordedRunReference},
	}, notifier.messages)
}

// A source that never succeeded has no trusted inventory to be stale, and
// staleness must never be the reason one is claimed to exist.
func TestReporterReportsNoStalenessBeforeAnySuccessfulSourceRun(t *testing.T) {
	state := &fakeRunState{}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, &reportingRunner{}, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	assert.Empty(t, notifier.messages, "a service with no successful source run was called stale")
}

// A source phase can stop succeeding without a newly visible incident once its
// own failure category is suppressed; this is the deterministic, provider-free
// check that still catches it.
func TestReporterAlertsWhenTheTrustedInventoryGoesStale(t *testing.T) {
	state := &fakeRunState{lastSuccessAt: map[string]time.Time{
		"source": time.Date(2026, time.August, 16, 8, 0, 0, 0, time.UTC),
	}}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, &reportingRunner{}, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want one stale alert")
	assert.Equal(t, "Domestique sync failed", notifier.messages[0].title)
	assert.Contains(t, notifier.messages[0].message, "source stale:")
}

// The alert threshold is compared in whole seconds, the same precision
// GET /v1/status reports age against, so a sub-second remainder cannot leave
// the two disagreeing about whether the inventory is stale.
func TestReporterComparesStalenessInWholeSecondsLikeStatusDoes(t *testing.T) {
	lastSuccess := time.Date(2026, time.August, 17, 8, 29, 58, 600_000_000, time.UTC)
	now := time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC)
	state := &fakeRunState{lastSuccessAt: map[string]time.Time{"source": lastSuccess}}
	notifier := &fakeNotifier{}
	reporter, err := NewReporter(
		&reportingRunner{}, state, notifier, SuccessNotification{Policy: SuccessEvery}, 1500*time.Millisecond,
	)
	require.NoError(t, err, "NewReporter()")
	reporter.now = func() time.Time { return now }

	// A 1.4s age against a 1.5s bound: age_seconds (1) < max_age_seconds (1) is
	// false, the same "stale" answer GET /v1/status would give — even though
	// the untruncated durations (1.4s < 1.5s) say fresh.
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want the alert the status contract's truncation also calls for")
}

// The stale alert is rate-limited the same way an ordinary failure is, so a
// schedule left off for days does not repeat it every tick.
func TestReporterSuppressesRepeatedStaleAlertsForSixHours(t *testing.T) {
	state := &fakeRunState{lastSuccessAt: map[string]time.Time{
		"source": time.Date(2026, time.August, 16, 8, 0, 0, 0, time.UTC),
	}}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, &reportingRunner{}, state, notifier)
	now := time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	now = now.Add(5 * time.Hour)
	reporter.Run(t.Context())
	now = now.Add(time.Hour + time.Minute)
	reporter.Run(t.Context())
	assert.Len(t, notifier.messages, 2, "want the second alert suppressed and the third let through")
}

// A successful refresh ends an outstanding stale alert unconditionally, the
// same way an ordinary recovery is never held back by policy.
func TestReporterSendsRecoveryWhenSourceSucceedsAfterBeingStale(t *testing.T) {
	state := &fakeRunState{
		source: true,
		lastFailure: map[string]time.Time{
			staleCategory: time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC),
		},
	}
	runner := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 5}}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 18, 9, 0, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "want the routine success and the recovery")
	assert.Equal(t, "source recovered: trusted inventory is fresh again", notifier.messages[1].message)

	// The incident the recovery closed must not still be open on the books: a
	// second successful tick is routine, not a second recovery.
	reporter.Run(t.Context())
	assert.Len(t, notifier.messages, 3, "a second success after recovery sent a second recovery message")
}

// An unreadable suppression record must not be treated as "nothing to
// recover from" or "nothing yet alerted" — it holds the staleness check back
// entirely rather than guessing.
func TestReporterStalenessCheckDeclinesWhenSuppressionStateCannotBeRead(t *testing.T) {
	state := &fakeRunState{
		lastFailureErr: errors.New("state unavailable"),
		lastSuccessAt: map[string]time.Time{
			"source": time.Date(2026, time.August, 16, 8, 0, 0, 0, time.UTC),
		},
	}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, &reportingRunner{}, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	assert.Empty(t, notifier.messages, "a staleness check with no readable suppression state sent something")
}

// A recovery message Pushover refuses is not retried from here; the next
// success carries the same signal again.
func TestReporterStaleRecoveryKeepsGoingWhenTheNotificationCannotBeSent(t *testing.T) {
	state := &fakeRunState{
		source: true,
		lastFailure: map[string]time.Time{
			staleCategory: time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC),
		},
	}
	runner := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeSucceeded}}
	notifier := &fakeNotifier{sendErr: errors.New("pushover unavailable")}
	reporter := newReporter(t, runner, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 18, 9, 0, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	_, notified, err := state.LastFailureNotification(t.Context(), staleCategory)
	require.NoError(t, err, "LastFailureNotification()")
	assert.True(t, notified, "a recovery Pushover refused still closed the incident")
}

// A cleared suppression record that cannot be written leaves the incident
// open on the books rather than silently losing it: the next success tries
// the same recovery again.
func TestReporterKeepsTheStaleRecordWhenItCannotClearAfterRecovery(t *testing.T) {
	state := &fakeRunState{
		source:          true,
		clearFailureErr: errors.New("state unavailable"),
		lastFailure: map[string]time.Time{
			staleCategory: time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC),
		},
	}
	runner := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeSucceeded}}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 18, 9, 0, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "want the routine success and the recovery attempt")
	_, notified, err := state.LastFailureNotification(t.Context(), staleCategory)
	require.NoError(t, err, "LastFailureNotification()")
	assert.True(t, notified, "the stale record was cleared despite the clear failing")
}

// A stale alert Pushover refuses is not recorded as sent, so the next tick
// still has a suppression window to try again.
func TestReporterDoesNotRecordAStaleAlertPushoverRefused(t *testing.T) {
	state := &fakeRunState{lastSuccessAt: map[string]time.Time{
		"source": time.Date(2026, time.August, 16, 8, 0, 0, 0, time.UTC),
	}}
	notifier := &fakeNotifier{sendErr: errors.New("pushover unavailable")}
	reporter := newReporter(t, &reportingRunner{}, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	_, notified, err := state.LastFailureNotification(t.Context(), staleCategory)
	require.NoError(t, err, "LastFailureNotification()")
	assert.False(t, notified, "a stale alert Pushover refused was recorded as delivered")
}

// A stale alert whose suppression record cannot be written must still have
// gone out; losing the record only costs a repeated alert, not a missed one.
func TestReporterSendsAStaleAlertItCannotRecordTheSuppressionOf(t *testing.T) {
	state := &fakeRunState{
		recordFailureErr: errors.New("state unavailable"),
		lastSuccessAt: map[string]time.Time{
			"source": time.Date(2026, time.August, 16, 8, 0, 0, 0, time.UTC),
		},
	}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, &reportingRunner{}, state, notifier)
	reporter.now = func() time.Time { return time.Date(2026, time.August, 17, 8, 30, 0, 0, time.UTC) }

	reporter.Run(t.Context())
	assert.Len(t, notifier.messages, 1, "want one stale alert sent despite the suppression record failing to write")
}

// Enrichment follows the work a rider is waiting for, and only happens when a
// pass stored something new to enrich.
func TestReporterEnrichesOnlyAfterStoringANewInventory(t *testing.T) {
	stored := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded},
	}
	newReporter(t, stored, &fakeRunState{source: true, targets: true}, &fakeNotifier{}).Run(t.Context())
	assert.Equal(t, 1, stored.annotations, "annotation passes")

	failed := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureSource}}
	newReporter(t, failed, &fakeRunState{source: true}, &fakeNotifier{}).Run(t.Context())
	assert.Zero(t, failed.annotations, "a failed read was enriched anyway")

	targetsOnly := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}}
	newReporter(t, targetsOnly, &fakeRunState{targets: true}, &fakeNotifier{}).Run(t.Context())
	assert.Zero(t, targetsOnly.annotations, "a run that stored no inventory was enriched anyway")
}

func TestReporterDoesNotRecordOrNotifySkippedRun(t *testing.T) {
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSkipped},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSkipped},
	}
	reporter := newReporter(t, runner, state, notifier)

	reporter.Run(t.Context())
	assert.Zero(t, state.runs, "a skipped run was recorded")
	assert.Empty(t, notifier.messages, "a skipped run was notified")
}

func TestReporterTriggerRejectsOverlappingRun(t *testing.T) {
	runner := &blockingReportingRunner{started: make(chan struct{}), release: make(chan struct{})}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state, &fakeNotifier{})
	require.True(t, reporter.Trigger(t.Context()), "Trigger() rejected the first run")
	<-runner.started
	assert.False(t, reporter.Trigger(t.Context()), "Trigger() accepted a run while one was active")
	close(runner.release)
	reporter.Wait()
	assert.Equal(t, 2, state.runs, "recorded runs")
}

// A status response is built while a run is in flight, so whatever it says
// about one has to be true at the moment it is asked.
func TestReporterReportsThePhaseInFlight(t *testing.T) {
	runner := &blockingReportingRunner{started: make(chan struct{}), release: make(chan struct{})}
	reporter := newReporter(t, runner, &fakeRunState{}, &fakeNotifier{})
	phase, running := reporter.Running()
	assert.False(t, running, "Running() reported a run before one started")
	assert.Empty(t, phase, "Running() named a phase before a run started")

	require.True(t, reporter.Trigger(t.Context()), "Trigger() rejected the run")
	<-runner.started
	phase, running = reporter.Running()
	assert.True(t, running, "Running() reported no run while one was in flight")
	assert.Equal(t, PhaseSource, phase, "Running() phase")

	close(runner.release)
	reporter.Wait()
	phase, running = reporter.Running()
	assert.False(t, running, "Running() reported a run after the last phase finished")
	assert.Empty(t, phase, "Running() named a phase after the last phase finished")
}

type reportingRunner struct {
	source      Result
	targets     Result
	sourceRuns  int
	targetRuns  int
	annotations int
}

func (r *reportingRunner) RunSource(context.Context) Result {
	r.sourceRuns++

	return r.source
}

func (r *reportingRunner) RunTargets(context.Context) Result {
	r.targetRuns++

	return r.targets
}

func (r *reportingRunner) AnnotateStored(context.Context) {
	r.annotations++
}

type blockingReportingRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingReportingRunner) RunSource(context.Context) Result {
	close(r.started)
	<-r.release

	return Result{Phase: PhaseSource, Outcome: OutcomeSucceeded}
}

func (r *blockingReportingRunner) RunTargets(context.Context) Result {
	return Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}
}

func (r *blockingReportingRunner) AnnotateStored(context.Context) {}

// recordedRunReference is what this fake names every run it records. The store
// mints a different one per run; what matters here is that whatever it returns
// reaches the notification.
const recordedRunReference = "1a2b3c4d5e6f"

type fakeRunState struct {
	scheduleErr       error
	targetRunErr      error
	phaseOutcomeErr   error
	digestReadErr     error
	digestRecordErr   error
	successfulRunsErr error
	lastFailureErr    error
	recordFailureErr  error
	clearFailureErr   error
	lastFailure       map[string]time.Time
	lastPhase         map[string]string
	lastSuccessAt     map[string]time.Time
	lastSuccessErr    error
	lastDigest        time.Time
	successfulRuns    []successfulRun
	phases            []string
	recordedRuns      []recordedTargetRun
	runs              int
	lastDigestRunID   int64
	digestFound       bool
	source            bool
	targets           bool
}

// successfulRun is one recorded success as a digest reads it back.
type successfulRun struct {
	phase                     string
	id                        int64
	created, updated, deleted int
}

// recordedTargetRun is one slot's result as it reached durable state.
type recordedTargetRun struct {
	finishedAt time.Time
	id         string
	outcome    string
	detail     string
}

func (s *fakeRunState) RecordTargetRun(
	_ context.Context,
	targetID string,
	finishedAt time.Time,
	outcome, detail string,
) error {
	if s.targetRunErr != nil {
		return s.targetRunErr
	}
	s.recordedRuns = append(s.recordedRuns, recordedTargetRun{
		finishedAt: finishedAt,
		id:         targetID,
		outcome:    outcome,
		detail:     detail,
	})

	return nil
}

func (s *fakeRunState) RecordSyncRun(
	_ context.Context,
	phase string,
	_, finishedAt time.Time,
	outcome, _ string,
	_, created, updated, deleted int,
) (string, error) {
	s.runs++
	s.phases = append(s.phases, phase)
	if s.lastPhase == nil {
		s.lastPhase = make(map[string]string)
	}
	s.lastPhase[phase] = outcome
	if outcome == string(OutcomeSucceeded) {
		// The store's run ids are monotonic, which is what the digest window is
		// bounded by; the row count stands in for them here.
		s.successfulRuns = append(s.successfulRuns, successfulRun{
			id: int64(s.runs), phase: phase, created: created, updated: updated, deleted: deleted,
		})
		if s.lastSuccessAt == nil {
			s.lastSuccessAt = make(map[string]time.Time)
		}
		s.lastSuccessAt[phase] = finishedAt
	}

	return recordedRunReference, nil
}

func (s *fakeRunState) LastSuccessfulPhaseCompletion(
	_ context.Context, phase string,
) (completedAt time.Time, found bool, err error) {
	if s.lastSuccessErr != nil {
		return time.Time{}, false, s.lastSuccessErr
	}
	completedAt, found = s.lastSuccessAt[phase]

	return completedAt, found, nil
}

func (s *fakeRunState) LastPhaseOutcome(_ context.Context, phase string) (outcome string, found bool, err error) {
	if s.phaseOutcomeErr != nil {
		return "", false, s.phaseOutcomeErr
	}
	outcome, found = s.lastPhase[phase]

	return outcome, found, nil
}

func (s *fakeRunState) LastDigestNotification(context.Context) (sentAt time.Time, lastRunID int64, found bool, err error) {
	if s.digestReadErr != nil {
		return time.Time{}, 0, false, s.digestReadErr
	}

	return s.lastDigest, s.lastDigestRunID, s.digestFound, nil
}

func (s *fakeRunState) RecordDigestNotification(_ context.Context, sentAt time.Time, lastRunID int64) error {
	if s.digestRecordErr != nil {
		return s.digestRecordErr
	}
	s.lastDigest = sentAt
	s.lastDigestRunID = lastRunID
	s.digestFound = true

	return nil
}

func (s *fakeRunState) ForEachSuccessfulRunAfter(
	_ context.Context,
	runID int64,
	visit func(id int64, phase string, created, updated, deleted int) error,
) error {
	if s.successfulRunsErr != nil {
		return s.successfulRunsErr
	}
	for _, run := range s.successfulRuns {
		if run.id <= runID {
			continue
		}
		if err := visit(run.id, run.phase, run.created, run.updated, run.deleted); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeRunState) SyncSchedule(context.Context) (source, targets bool, err error) {
	if s.scheduleErr != nil {
		return false, false, s.scheduleErr
	}

	return s.source, s.targets, nil
}

func (s *fakeRunState) LastFailureNotification(_ context.Context, category string) (time.Time, bool, error) {
	if s.lastFailureErr != nil {
		return time.Time{}, false, s.lastFailureErr
	}
	sentAt, found := s.lastFailure[category]

	return sentAt, found, nil
}

func (s *fakeRunState) RecordFailureNotification(_ context.Context, category string, sentAt time.Time) error {
	if sentAt.IsZero() {
		if s.clearFailureErr != nil {
			return s.clearFailureErr
		}
		delete(s.lastFailure, category)

		return nil
	}
	if s.recordFailureErr != nil {
		return s.recordFailureErr
	}
	if s.lastFailure == nil {
		s.lastFailure = make(map[string]time.Time)
	}
	s.lastFailure[category] = sentAt

	return nil
}

type notification struct {
	title   string
	message string
}

type fakeNotifier struct {
	sendErr  error
	messages []notification
}

func (n *fakeNotifier) Send(_ context.Context, title, message string) error {
	n.messages = append(n.messages, notification{title: title, message: message})

	return n.sendErr
}

func newReporter(t *testing.T, runner Runner, state RunState, notifier Notifier) *Reporter {
	t.Helper()

	return newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessEvery})
}

func newPolicyReporter(
	t *testing.T,
	runner Runner,
	state RunState,
	notifier Notifier,
	success SuccessNotification,
) *Reporter {
	t.Helper()
	reporter, err := NewReporter(runner, state, notifier, success, 24*time.Hour)
	require.NoError(t, err, "NewReporter()")

	return reporter
}

// The per-slot rows are what a status page reads to answer "is this account
// current", so each one is written with the run's own finish time.
func TestReporterRecordsEachTargetsOwnResult(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase:   PhaseTargets,
		Outcome: OutcomeFailed,
		Failure: FailureDestination,
		Targets: []TargetResult{
			{ID: "rider-a", Outcome: OutcomeSucceeded},
			{ID: "rider-b", Outcome: OutcomeFailed, Failure: FailureDestination},
		},
	}}
	state := &fakeRunState{targets: true}
	reporter := newReporter(t, runner, state, &fakeNotifier{})
	now := time.Date(2026, time.August, 18, 9, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	assert.Equal(t, []recordedTargetRun{
		{finishedAt: now, id: "rider-a", outcome: "succeeded"},
		{finishedAt: now, id: "rider-b", outcome: "failed", detail: "destination"},
	}, state.recordedRuns)
}

// A slot whose row cannot be written costs an operator one stale line. Stopping
// there would cost them every line after it, and the run itself is already
// recorded and reported by then.
func TestReporterStillNotifiesWhenATargetRowCannotBeWritten(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase:   PhaseTargets,
		Outcome: OutcomeSucceeded,
		Targets: []TargetResult{{ID: "rider-a", Outcome: OutcomeSucceeded}},
	}}
	state := &fakeRunState{targets: true, targetRunErr: errors.New("state unavailable")}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)

	result := reporter.Run(t.Context())
	require.Equal(t, OutcomeSucceeded, result.Outcome)
	assert.Empty(t, state.recordedRuns)
	assert.Len(t, notifier.messages, 1)
}

// The source phase writes to no target, so it records nothing about one.
func TestReporterRecordsNoTargetRunsForASourceRun(t *testing.T) {
	runner := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 2}}
	state := &fakeRunState{source: true}
	reporter := newReporter(t, runner, state, &fakeNotifier{})

	reporter.Run(t.Context())
	assert.Empty(t, state.recordedRuns)
}

// The whole point of a quiet policy is that a healthy service says nothing.
func TestReporterQuietPolicySendsNothingForARoutineSuccess(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 3},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3, Created: 1},
	}
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

	reporter.Run(t.Context())
	assert.Equal(t, 2, state.runs, "recorded runs")
	assert.Empty(t, notifier.messages, "a quiet policy pushed a routine success")
}

// A quiet policy governs routine success and nothing else. The failure an
// operator installed notifications for still arrives.
func TestReporterQuietPolicyStillReportsFailureAndBlockedRuns(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		outcome Outcome
		failure FailureCategory
	}{
		{name: "failed", outcome: OutcomeFailed, failure: FailureDestination},
		{name: "blocked", outcome: OutcomeBlocked, failure: FailureDeletionLimit},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := &reportingRunner{
				targets: Result{Phase: PhaseTargets, Outcome: testCase.outcome, Failure: testCase.failure},
			}
			state := &fakeRunState{targets: true}
			notifier := &fakeNotifier{}
			reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

			reporter.Run(t.Context())
			require.Len(t, notifier.messages, 1, "a quiet policy suppressed a %s run", testCase.name)
			assert.Equal(t, "Domestique sync failed", notifier.messages[0].title, "notification title")
		})
	}
}

// The first success after a failure is the recovery signal, and it is the one
// success no policy may hold back: it is what tells the operator the alert they
// were sent is over.
func TestReporterQuietPolicySendsTheRecoverySuccess(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureDestination}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want the failure alert")

	runner.targets = Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 2, Updated: 1}
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "the recovery success was suppressed")
	assert.Equal(t, notification{
		title:   "Domestique sync",
		message: "targets succeeded: source_stages=2 created=0 updated=1 deleted=0 run=" + recordedRunReference,
	}, notifier.messages[1], "recovery notification")

	// The recovery is the first following success and only the first: once the
	// phase is healthy again its successes are routine.
	reporter.Run(t.Context())
	assert.Len(t, notifier.messages, 2, "a routine success after recovery was pushed")
}

// Recovery is asked per phase, because a failure in one half says nothing about
// the other and each is separately alerted on.
func TestReporterRecoveryIsPerPhase(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 5},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureDestination},
	}
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want the target failure alert alone")

	// The source half was healthy throughout, so its success stays routine even
	// though the targets half is recovering.
	runner.targets = Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 5}
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "want only the targets recovery")
	assert.Contains(t, notifier.messages[1].message, "targets succeeded", "recovered phase")
}

// An unreadable history must not be the reason a recovery goes unsent. Erring
// towards one extra message is the safe direction.
func TestReporterSendsSuccessWhenTheRecoveryQuestionCannotBeAnswered(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 1}}
	state := &fakeRunState{targets: true, phaseOutcomeErr: errors.New("state unavailable")}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

	reporter.Run(t.Context())
	assert.Len(t, notifier.messages, 1, "an unreadable history silenced a possible recovery")
}

// The first digest starts the clock rather than reporting whatever history the
// database happens to hold.
func TestReporterDigestStartsItsClockWithoutSending(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 4}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: 24 * time.Hour,
	})
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	assert.Empty(t, notifier.messages, "the first digest reported history it was not asked for")
	assert.True(t, state.digestFound, "the digest clock was not started")
}

// A digest replaces the per-run push with one aggregate message per interval,
// and totals only the counts.
func TestReporterDigestAggregatesOneIntervalOfSuccesses(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 6},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 6, Created: 2, Updated: 1, Deleted: 1},
	}
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: 24 * time.Hour,
	})
	start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	now := start
	reporter.now = func() time.Time { return now }

	// The opening run starts the clock; the next stays inside the interval.
	reporter.Run(t.Context())
	now = start.Add(time.Hour)
	reporter.Run(t.Context())
	require.Empty(t, notifier.messages, "a digest was sent inside its own interval")

	now = start.Add(25 * time.Hour)
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want one digest once the interval elapsed")
	assert.Equal(t, notification{
		title:   "Domestique sync digest",
		message: "since 2026-08-17T08:00:00Z: source_runs=2 target_runs=2 created=4 updated=2 deleted=2",
	}, notifier.messages[0], "digest notification")
}

// A digest is aggregate, so the run reference that names one run has no place
// in it — and neither does anything a run touched.
func TestReporterDigestCarriesCountsAlone(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 2, Created: 1,
		Targets: []TargetResult{{ID: "rider-a", Outcome: OutcomeSucceeded}},
	}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: time.Hour,
	})
	start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	now := start
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	now = start.Add(2 * time.Hour)
	reporter.Run(t.Context())

	require.Len(t, notifier.messages, 1, "want one digest")
	message := notifier.messages[0].message
	assert.NotContains(t, message, recordedRunReference, "the digest named a single run")
	assert.NotContains(t, message, "rider-a", "the digest named a target")
}

// A digest holds back routine success, not the end of an alert.
func TestReporterDigestPolicySendsTheRecoverySuccess(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureDestination}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: 24 * time.Hour,
	})

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want the failure alert")

	runner.targets = Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3}
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "the recovery success was held for a digest")
	assert.Equal(t, "Domestique sync", notifier.messages[1].title, "recovery notification title")
}

// A policy the composition root failed to supply must stop startup rather than
// quietly deciding how much the operator hears.
func TestNewReporterRejectsAnUnusablePolicy(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		success SuccessNotification
	}{
		{name: "unset", success: SuccessNotification{}},
		{name: "unknown", success: SuccessNotification{Policy: SuccessPolicy("silent")}},
		{name: "digest without an interval", success: SuccessNotification{Policy: SuccessDigest}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewReporter(&reportingRunner{}, &fakeRunState{}, &fakeNotifier{}, testCase.success, 24*time.Hour)
			require.Error(t, err, "NewReporter() accepted %s", testCase.name)
		})
	}
}

// A stale-inventory bound of zero would flag every service as stale on its
// very first tick, so startup must refuse it rather than silently notifying.
func TestNewReporterRejectsANonPositiveStaleAfter(t *testing.T) {
	for _, staleAfter := range []time.Duration{0, -time.Hour} {
		_, err := NewReporter(
			&reportingRunner{}, &fakeRunState{}, &fakeNotifier{}, SuccessNotification{Policy: SuccessEvery}, staleAfter,
		)
		require.Error(t, err, "NewReporter() accepted a stale-after bound of %s", staleAfter)
	}
}

// An interval nothing succeeded in is silent, and it still moves the window.
//
// Leaving the window where it was would make the next digest report two
// intervals of work under one interval's heading; sending an all-zero message
// would defeat the policy an operator chose it for.
func TestReporterDigestPassesOverAnIntervalWithNothingToReport(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3, Created: 1,
	}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: 24 * time.Hour,
	})
	start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	now := start
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())

	// Both halves switched off: the interval elapses over a service that ran
	// nothing at all.
	state.targets = false
	now = start.Add(25 * time.Hour)
	reporter.Run(t.Context())
	require.Empty(t, notifier.messages, "an interval with nothing in it still sent a digest")
	require.Equal(t, now, state.lastDigest, "the empty interval did not move the window")

	state.targets = true
	now = start.Add(26 * time.Hour)
	reporter.Run(t.Context())
	now = start.Add(50 * time.Hour)
	reporter.Run(t.Context())

	require.Len(t, notifier.messages, 1, "want one digest for the interval that had runs in it")
	assert.Equal(t, notification{
		title:   "Domestique sync digest",
		message: "since 2026-08-18T09:00:00Z: source_runs=0 target_runs=2 created=2 updated=0 deleted=0",
	}, notifier.messages[0], "digest notification")
}

// A digest is a convenience over history that is already recorded. State that
// cannot be read holds the message back rather than sending a total that is
// wrong, and the window stays where it was so the next pass reports the period
// in full.
func TestReporterDigestStaysSilentWhenItsHistoryCannotBeRead(t *testing.T) {
	for _, testCase := range []struct {
		apply func(state *fakeRunState)
		name  string
	}{
		{
			name:  "the window cannot be read",
			apply: func(state *fakeRunState) { state.digestReadErr = errors.New("state unavailable") },
		},
		{
			name:  "the runs in it cannot be totalled",
			apply: func(state *fakeRunState) { state.successfulRunsErr = errors.New("state unavailable") },
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 2}}
			state := &fakeRunState{targets: true}
			notifier := &fakeNotifier{}
			reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
				Policy: SuccessDigest, Interval: time.Hour,
			})
			start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
			now := start
			reporter.now = func() time.Time { return now }

			reporter.Run(t.Context())
			testCase.apply(state)
			now = start.Add(2 * time.Hour)
			reporter.Run(t.Context())

			assert.Empty(t, notifier.messages, "a digest was sent over unreadable history")
			assert.Equal(t, start, state.lastDigest, "the window moved without a digest being sent")
		})
	}
}

// The window is anchored after Pushover accepts the digest, so an anchor that
// cannot be written leaves the period open: the next pass says it again rather
// than the service losing it. Neither outcome may stop the run.
func TestReporterDigestSurvivesAnAnchorItCannotWrite(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 2, Created: 1,
	}}
	state := &fakeRunState{targets: true, digestRecordErr: errors.New("state unavailable")}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: time.Hour,
	})
	start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	now := start
	reporter.now = func() time.Time { return now }

	// The opening pass cannot anchor either, so the digest never leaves its
	// unstarted state and every later pass is still the first one.
	result := reporter.Run(t.Context())
	require.Equal(t, OutcomeSucceeded, result.Outcome, "an unwritable anchor changed the run's outcome")
	now = start.Add(2 * time.Hour)
	result = reporter.Run(t.Context())
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "an unwritable anchor changed the run's outcome")
	assert.Empty(t, notifier.messages, "a digest was sent from a window that was never anchored")
}

// A notification Pushover refuses is one an operator does not have, and the run
// it describes still happened. Reporting is never allowed to rewrite it.
func TestReporterKeepsTheOutcomeWhenANotificationIsRefused(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 2}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{sendErr: errors.New("pushover unavailable")}
	reporter := newReporter(t, runner, state, notifier)

	result := reporter.Run(t.Context())
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "a refused notification changed the run's outcome")
	assert.Len(t, notifier.messages, 1, "the notification was not attempted")
}

// A digest Pushover refused was never delivered, so the window it covered stays
// open and the next pass offers the same period again.
func TestReporterHoldsTheDigestWindowOpenWhenTheMessageIsRefused(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 2, Created: 1,
	}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: time.Hour,
	})
	start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	now := start
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	notifier.sendErr = errors.New("pushover unavailable")
	now = start.Add(2 * time.Hour)
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want the refused digest to have been attempted")
	require.Equal(t, start, state.lastDigest, "a refused digest moved the window past its own period")

	// The period is offered again, now also carrying the run that finished while
	// it was being retried. The opening run is behind the anchor and in neither.
	notifier.sendErr = nil
	now = start.Add(3 * time.Hour)
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "the refused period was never offered again")
	assert.Equal(t, notification{
		title:   "Domestique sync digest",
		message: "since 2026-08-17T08:00:00Z: source_runs=0 target_runs=2 created=2 updated=0 deleted=0",
	}, notifier.messages[1], "digest notification")
}

// A target left awaiting OAuth records `not_ready` and notifies nothing, so the
// failure alert before it is still open. The success that follows is what closes
// it, whatever was recorded in between.
func TestReporterRecoveryOutlastsAPhaseLeftAwaitingOnboarding(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		alert  Result
		alerts int
	}{
		{
			name:   "after a failure",
			alert:  Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureDestination},
			alerts: 1,
		},
		{
			name:   "after a blocked run",
			alert:  Result{Phase: PhaseTargets, Outcome: OutcomeBlocked, Failure: FailureDeletionLimit},
			alerts: 1,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := &reportingRunner{targets: testCase.alert}
			state := &fakeRunState{targets: true}
			notifier := &fakeNotifier{}
			reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

			reporter.Run(t.Context())
			require.Len(t, notifier.messages, testCase.alerts, "want the alert")

			runner.targets = Result{Phase: PhaseTargets, Outcome: OutcomeNotReady}
			reporter.Run(t.Context())
			require.Len(t, notifier.messages, testCase.alerts, "a run awaiting onboarding notified")

			runner.targets = Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3}
			reporter.Run(t.Context())
			require.Len(t, notifier.messages, testCase.alerts+1,
				"the alert was left open with nothing to close it")
			assert.Equal(t, "Domestique sync", notifier.messages[testCase.alerts].title,
				"recovery notification title")
		})
	}
}

// A blocked run is an alert like any other, so the success that follows it
// closes that alert even with no failure in between.
func TestReporterQuietPolicySendsTheRecoveryAfterABlockedRun(t *testing.T) {
	runner := &reportingRunner{targets: Result{
		Phase: PhaseTargets, Outcome: OutcomeBlocked, Failure: FailureDeletionLimit,
	}}
	state := &fakeRunState{targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{Policy: SuccessQuiet})

	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 1, "want the blocked alert")

	runner.targets = Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3}
	reporter.Run(t.Context())
	require.Len(t, notifier.messages, 2, "the success after a blocked run was suppressed")
	assert.Equal(t, "Domestique sync", notifier.messages[1].title, "recovery notification title")

	// And only the first: the phase is healthy now, so its next success is
	// routine and a quiet policy holds it back.
	reporter.Run(t.Context())
	assert.Len(t, notifier.messages, 2, "a routine success followed the recovery out")
}

// pacedRunner takes time to run each half, so a pass occupies a span rather than
// an instant and a digest interval can elapse partway through one.
type pacedRunner struct {
	clock   *time.Time
	source  Result
	targets Result
	step    time.Duration
}

func (r *pacedRunner) RunSource(context.Context) Result {
	*r.clock = r.clock.Add(r.step)

	return r.source
}

func (r *pacedRunner) RunTargets(context.Context) Result {
	*r.clock = r.clock.Add(r.step)

	return r.targets
}

func (r *pacedRunner) AnnotateStored(context.Context) {}

// Both halves of the pass that carries the window past the interval are counted
// in the same digest.
//
// A window closing between them would pair each pass's targets half with the
// next pass's source half instead — no run is lost, but every digest then
// reports a period that never happened. The counts below differ per pass so
// that regrouping is visible: pairing them the wrong way reports the previous
// pass's writes under this pass's heading.
func TestReporterDigestCountsAWholePassInOneInterval(t *testing.T) {
	start := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	now := start
	runner := &pacedRunner{
		clock:   &now,
		step:    time.Hour,
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 6},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 6, Created: 1},
	}
	state := &fakeRunState{source: true, targets: true}
	notifier := &fakeNotifier{}
	reporter := newPolicyReporter(t, runner, state, notifier, SuccessNotification{
		Policy: SuccessDigest, Interval: 2 * time.Hour,
	})
	reporter.now = func() time.Time { return now }

	// The opening pass ends at 10:00 and anchors the window there.
	reporter.Run(t.Context())
	require.Empty(t, notifier.messages, "the first digest reported history it was not asked for")

	runner.targets.Created = 2
	reporter.Run(t.Context())
	runner.targets.Created = 7
	reporter.Run(t.Context())

	require.Len(t, notifier.messages, 2, "want one digest per elapsed interval")
	assert.Equal(t, []notification{
		{
			title:   "Domestique sync digest",
			message: "since 2026-08-17T10:00:00Z: source_runs=1 target_runs=1 created=2 updated=0 deleted=0",
		},
		{
			title:   "Domestique sync digest",
			message: "since 2026-08-17T12:00:00Z: source_runs=1 target_runs=1 created=7 updated=0 deleted=0",
		},
	}, notifier.messages, "digest notifications")
}
