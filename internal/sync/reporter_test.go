package sync

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
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
	if got, want := state.runs, 2; got != want {
		t.Errorf("recorded runs = %d, want %d", got, want)
	}
	if got, want := state.phases, []string{"source", "targets"}; !slices.Equal(got, want) {
		t.Errorf("recorded phases = %v, want %v", got, want)
	}
	if got, want := notifier.messages, []notification{
		{title: "Domestique sync", message: "source succeeded: source_stages=3"},
		{title: "Domestique sync", message: "targets succeeded: source_stages=3 created=2 updated=1 deleted=0"},
	}; !equalNotifications(got, want) {
		t.Errorf("notifications = %#v, want %#v", got, want)
	}
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
	if runner.targetRuns != 0 {
		t.Errorf("target runs = %d, want 0 while the target schedule is off", runner.targetRuns)
	}
	if got, want := state.phases, []string{"source"}; !slices.Equal(got, want) {
		t.Errorf("recorded phases = %v, want %v", got, want)
	}
}

func TestReporterRunsNothingWhenBothPhasesAreSwitchedOff(t *testing.T) {
	runner := &reportingRunner{}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state, &fakeNotifier{})

	if got, want := reporter.Run(t.Context()).Outcome, OutcomeSkipped; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if runner.sourceRuns+runner.targetRuns != 0 {
		t.Errorf("runs = %d, want none", runner.sourceRuns+runner.targetRuns)
	}
	if state.runs != 0 {
		t.Errorf("recorded runs = %d, want 0", state.runs)
	}
}

// "Off" and "unreadable" are different answers, and a timer must not act on the
// second as though it were the first.
func TestReporterRunsNothingAndReportsAnUnreadableSchedule(t *testing.T) {
	runner := &reportingRunner{}
	state := &fakeRunState{scheduleErr: errors.New("state unavailable")}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)

	result := reporter.Run(t.Context())
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureState; got != want {
		t.Errorf("Run() failure = %q, want %q", got, want)
	}
	if runner.sourceRuns+runner.targetRuns != 0 {
		t.Errorf("runs = %d, want none", runner.sourceRuns+runner.targetRuns)
	}
	if got, want := state.runs, 1; got != want {
		t.Errorf("recorded runs = %d, want %d", got, want)
	}
	if len(notifier.messages) != 1 {
		t.Errorf("notifications = %#v, want one failure alert", notifier.messages)
	}
}

// An operator asking for a phase has already decided; the switch only ever
// governed what happens unattended.
func TestReporterTriggersAPhaseTheScheduleHasSwitchedOff(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state, &fakeNotifier{})

	if !reporter.TriggerPhase(t.Context(), PhaseTargets) {
		t.Fatal("TriggerPhase() = false, want accepted run")
	}
	reporter.Wait()
	if got, want := runner.targetRuns, 1; got != want {
		t.Errorf("target runs = %d, want %d", got, want)
	}
	if runner.sourceRuns != 0 {
		t.Errorf("source runs = %d, want 0", runner.sourceRuns)
	}
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
	if got, want := notifier.messages, []notification{
		{title: "Domestique sync failed", message: "targets failed: destination"},
		{title: "Domestique sync failed", message: "targets failed: destination"},
	}; !equalNotifications(got, want) {
		t.Errorf("notifications = %#v, want %#v", got, want)
	}
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
	if got, want := notifier.messages, []notification{
		{title: "Domestique sync failed", message: "source failed: state"},
		{title: "Domestique sync failed", message: "targets failed: state"},
	}; !equalNotifications(got, want) {
		t.Errorf("notifications = %#v, want %#v", got, want)
	}
}

// Enrichment follows the work a rider is waiting for, and only happens when a
// pass stored something new to enrich.
func TestReporterEnrichesOnlyAfterStoringANewInventory(t *testing.T) {
	stored := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded},
	}
	newReporter(t, stored, &fakeRunState{source: true, targets: true}, &fakeNotifier{}).Run(t.Context())
	if got, want := stored.annotations, 1; got != want {
		t.Errorf("annotation passes = %d, want %d", got, want)
	}

	failed := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureSource}}
	newReporter(t, failed, &fakeRunState{source: true}, &fakeNotifier{}).Run(t.Context())
	if got, want := failed.annotations, 0; got != want {
		t.Errorf("annotation passes after a failed read = %d, want %d", got, want)
	}

	targetsOnly := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}}
	newReporter(t, targetsOnly, &fakeRunState{targets: true}, &fakeNotifier{}).Run(t.Context())
	if got, want := targetsOnly.annotations, 0; got != want {
		t.Errorf("annotation passes without a source run = %d, want %d", got, want)
	}
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
	if state.runs != 0 {
		t.Errorf("recorded runs = %d, want 0", state.runs)
	}
	if len(notifier.messages) != 0 {
		t.Errorf("notifications = %#v, want none", notifier.messages)
	}
}

func TestReporterTriggerRejectsOverlappingRun(t *testing.T) {
	runner := &blockingReportingRunner{started: make(chan struct{}), release: make(chan struct{})}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state, &fakeNotifier{})
	if !reporter.Trigger(t.Context()) {
		t.Fatal("Trigger() = false, want accepted run")
	}
	<-runner.started
	if reporter.Trigger(t.Context()) {
		t.Error("Trigger() = true, want rejection while run is active")
	}
	close(runner.release)
	reporter.Wait()
	if got, want := state.runs, 2; got != want {
		t.Errorf("recorded runs = %d, want %d", got, want)
	}
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
	scheduleErr error
	lastFailure map[string]time.Time
	phases      []string
	runs        int
	source      bool
	targets     bool
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
	if err != nil {
		t.Fatalf("NewReporter() error = %v", err)
	}

	return reporter
}

func equalNotifications(left, right []notification) bool {
	return slices.Equal(left, right)
}
