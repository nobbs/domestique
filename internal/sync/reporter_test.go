package sync

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestReporterRecordsAndNotifiesEverySuccess(t *testing.T) {
	runner := &reportingRunner{result: Result{Outcome: OutcomeSucceeded, SourceStages: 3, Created: 2, Updated: 1, Deleted: 0}}
	state := &fakeRunState{}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, runner, state, notifier)
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.Run(t.Context())
	reporter.Run(t.Context())
	if got, want := state.runs, 2; got != want {
		t.Errorf("recorded runs = %d, want %d", got, want)
	}
	if got, want := notifier.messages, []notification{
		{title: "Domestique sync", message: "succeeded: source_stages=3 created=2 updated=1 deleted=0"},
		{title: "Domestique sync", message: "succeeded: source_stages=3 created=2 updated=1 deleted=0"},
	}; !equalNotifications(got, want) {
		t.Errorf("notifications = %#v, want %#v", got, want)
	}
}

func TestReporterSuppressesMatchingFailureForSixHours(t *testing.T) {
	runner := &reportingRunner{result: Result{Outcome: OutcomeFailed, Failure: FailureDestination}}
	state := &fakeRunState{}
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
		{title: "Domestique sync failed", message: "failed: destination"},
		{title: "Domestique sync failed", message: "failed: destination"},
	}; !equalNotifications(got, want) {
		t.Errorf("notifications = %#v, want %#v", got, want)
	}
}

func TestReporterDoesNotRecordOrNotifySkippedRun(t *testing.T) {
	state := &fakeRunState{}
	notifier := &fakeNotifier{}
	reporter := newReporter(t, &reportingRunner{result: Result{Outcome: OutcomeSkipped}}, state, notifier)

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
	if got, want := state.runs, 1; got != want {
		t.Errorf("recorded runs = %d, want %d", got, want)
	}
}

type reportingRunner struct {
	result Result
}

func (r *reportingRunner) Run(context.Context) Result {
	return r.result
}

type blockingReportingRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingReportingRunner) Run(context.Context) Result {
	close(r.started)
	<-r.release

	return Result{Outcome: OutcomeSucceeded}
}

type fakeRunState struct {
	lastFailure map[string]time.Time
	runs        int
}

func (s *fakeRunState) RecordSyncRun(
	context.Context,
	time.Time,
	time.Time,
	string,
	string,
	int,
	int,
	int,
	int,
) error {
	s.runs++

	return nil
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
