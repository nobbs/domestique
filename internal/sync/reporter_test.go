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
		{title: "Domestique sync", message: "source succeeded: source_stages=3"},
		{title: "Domestique sync", message: "targets succeeded: source_stages=3 created=2 updated=1 deleted=0"},
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
		{title: "Domestique sync failed", message: "targets failed: destination"},
		{title: "Domestique sync failed", message: "targets failed: destination"},
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
		{title: "Domestique sync failed", message: "source failed: state"},
		{title: "Domestique sync failed", message: "targets failed: state"},
	}, notifier.messages)
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

type fakeRunState struct {
	scheduleErr  error
	targetRunErr error
	lastFailure  map[string]time.Time
	phases       []string
	recordedRuns []recordedTargetRun
	runs         int
	source       bool
	targets      bool
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
	_, _ time.Time,
	_, _ string,
	_, _, _, _ int,
) error {
	s.runs++
	s.phases = append(s.phases, phase)

	return nil
}

func (s *fakeRunState) SyncSchedule(context.Context) (source, targets bool, err error) {
	if s.scheduleErr != nil {
		return false, false, s.scheduleErr
	}

	return s.source, s.targets, nil
}

func (s *fakeRunState) LastFailureNotification(_ context.Context, category string) (time.Time, bool, error) {
	sentAt, found := s.lastFailure[category]

	return sentAt, found, nil
}

func (s *fakeRunState) RecordFailureNotification(_ context.Context, category string, sentAt time.Time) error {
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
	messages []notification
}

func (n *fakeNotifier) Send(_ context.Context, title, message string) error {
	n.messages = append(n.messages, notification{title: title, message: message})

	return nil
}

func newReporter(t *testing.T, runner Runner, state RunState, notifier Notifier) *Reporter {
	t.Helper()
	reporter, err := NewReporter(runner, state, notifier)
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
