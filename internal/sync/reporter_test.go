package sync

import (
	"context"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReporterRecordsEverySuccess(t *testing.T) {
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 3},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, SourceStages: 3, Created: 2, Updated: 1},
	}
	state := &fakeRunState{source: true, targets: true}
	reporter := newReporter(t, runner, state)
	now := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.RunPhase(t.Context(), PhaseSource)
	reporter.RunPhase(t.Context(), PhaseTargets)
	assert.Equal(t, 2, state.runs, "recorded runs")
	assert.Equal(t, []string{"source", "targets"}, state.phases, "recorded phases")
}

// A target-specific trigger reconciles exactly the slot asked for, through
// RunTarget rather than RunTargets, and touches the source phase not at all.
func TestReporterReconcilesOneTargetAlone(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, Updated: 1}}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state)

	reporter.ReconcileTarget(t.Context(), "rider-a")
	assert.Equal(t, []string{"rider-a"}, runner.targetRunIDs, "target reconciled")
	assert.Zero(t, runner.sourceRuns, "the source phase ran for a single-target trigger")
	assert.Equal(t, []string{"targets"}, state.phases, "recorded phases")
}

// A clear runs through the same recording and notification path as any other
// target work, so a cleared account appears in history as the deletion it was
// rather than as an unexplained drop in what that account holds.
func TestReporterClearsOneTargetAlone(t *testing.T) {
	runner := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded, Deleted: 4}}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state)

	reporter.ClearTarget(t.Context(), "rider-a")
	assert.Equal(t, []string{"rider-a"}, runner.clearedIDs, "target cleared")
	assert.Zero(t, runner.sourceRuns, "the source phase ran for a clear")
	assert.Equal(t, []string{"targets"}, state.phases, "recorded phases")
}

// Enrichment follows the work a rider is waiting for, so a pass reports whether
// it stored something new to enrich rather than enriching it itself.
func TestReporterReportsWhetherItStoredANewInventory(t *testing.T) {
	stored := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSucceeded},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded},
	}
	result := newReporter(t, stored, &fakeRunState{}).RunPhase(t.Context(), PhaseSource)
	assert.True(t, result.SourceStored, "a pass that stored an inventory did not say so")
	assert.Zero(t, stored.annotations, "the reporter enriched what it stored instead of reporting it")

	failed := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureSource}}
	failedResult := newReporter(t, failed, &fakeRunState{}).RunPhase(t.Context(), PhaseSource)
	assert.False(t, failedResult.SourceStored, "a failed read reported a stored inventory")

	targetsOnly := &reportingRunner{targets: Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}}
	targetsResult := newReporter(t, targetsOnly, &fakeRunState{}).RunPhase(t.Context(), PhaseTargets)
	assert.False(t, targetsResult.SourceStored, "a run that stored no inventory reported one")
}

// SurfaceIncomplete is what tells a stage that keeps failing classification
// apart from one nobody has asked about yet, both otherwise absent alike.
func TestReporterReportsTheLastAnnotationPassesIncompleteCount(t *testing.T) {
	runner := &reportingRunner{
		source:             Result{Phase: PhaseSource, Outcome: OutcomeSucceeded},
		annotateClassified: 3,
		annotateFailed:     2,
	}
	reporter := newReporter(t, runner, &fakeRunState{source: true})
	assert.Zero(t, reporter.SurfaceIncomplete(), "an incomplete count was reported before any pass ran")

	reporter.Annotate(t.Context())
	assert.Equal(t, 2, reporter.SurfaceIncomplete(), "SurfaceIncomplete()")

	// A pass that catches up moves the gauge back down, rather than latching
	// the worst count any pass ever saw.
	runner.annotateFailed = 0
	reporter.Annotate(t.Context())
	assert.Zero(t, reporter.SurfaceIncomplete(), "a recovered pass left the old incomplete count in place")
}

// A manual retry classifies without touching either phase, and reports the
// same way a scheduled pass does.
func TestReporterAnnotateRunsOnlyClassification(t *testing.T) {
	runner := &reportingRunner{annotateFailed: 1}
	reporter := newReporter(t, runner, &fakeRunState{})

	reporter.Annotate(t.Context())
	assert.Equal(t, 1, runner.annotations, "annotation passes")
	assert.Zero(t, runner.sourceRuns, "Annotate read the source")
	assert.Zero(t, runner.targetRuns, "Annotate wrote a target")
	assert.Equal(t, 1, reporter.SurfaceIncomplete(), "SurfaceIncomplete()")
}

func TestReporterDoesNotRecordOrNotifySkippedRun(t *testing.T) {
	state := &fakeRunState{source: true, targets: true}
	runner := &reportingRunner{
		source:  Result{Phase: PhaseSource, Outcome: OutcomeSkipped},
		targets: Result{Phase: PhaseTargets, Outcome: OutcomeSkipped},
	}
	reporter := newReporter(t, runner, state)

	reporter.RunPhase(t.Context(), PhaseSource)
	reporter.RunPhase(t.Context(), PhaseTargets)
	assert.Zero(t, state.runs, "a skipped run was recorded")
}

// A status response is built while a run is in flight, so whatever it says
// about one has to be true at the moment it is asked.
func TestReporterReportsThePhaseInFlight(t *testing.T) {
	runner := &blockingReportingRunner{started: make(chan struct{}), release: make(chan struct{})}
	reporter := newReporter(t, runner, &fakeRunState{})
	phase, running := reporter.Running()
	assert.False(t, running, "Running() named a phase before a run started")
	assert.Empty(t, phase, "Running() named a phase before a run started")

	finished := make(chan struct{})
	go func() { defer close(finished); reporter.RunPhase(t.Context(), PhaseSource) }()
	<-runner.started
	phase, running = reporter.Running()
	assert.True(t, running, "Running() reported no phase while one was in flight")
	assert.Equal(t, PhaseSource, phase, "Running() phase")

	close(runner.release)
	<-finished
	phase, running = reporter.Running()
	assert.False(t, running, "Running() named a phase after the last phase finished")
	assert.Empty(t, phase, "Running() named a phase after the last phase finished")
}

type reportingRunner struct {
	targetRunIDs       []string
	clearedIDs         []string
	source             Result
	targets            Result
	sourceRuns         int
	targetRuns         int
	annotations        int
	annotateClassified int
	annotateFailed     int
}

func (r *reportingRunner) RunSourceProvider(_ context.Context, _ route.Provider) Result {
	r.sourceRuns++

	return r.source
}

func (r *reportingRunner) RunSource(context.Context) Result {
	r.sourceRuns++

	return r.source
}

func (r *reportingRunner) RunTargets(context.Context) Result {
	r.targetRuns++

	return r.targets
}

func (r *reportingRunner) RunTarget(_ context.Context, targetID string) Result {
	r.targetRuns++
	r.targetRunIDs = append(r.targetRunIDs, targetID)

	return r.targets
}

func (r *reportingRunner) ClearTarget(_ context.Context, targetID string) Result {
	r.clearedIDs = append(r.clearedIDs, targetID)

	return r.targets
}

func (r *reportingRunner) AnnotateStored(context.Context) (classified, failed int) {
	r.annotations++

	return r.annotateClassified, r.annotateFailed
}

type blockingReportingRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingReportingRunner) RunSourceProvider(ctx context.Context, _ route.Provider) Result {
	return r.RunSource(ctx)
}

func (r *blockingReportingRunner) RunSource(context.Context) Result {
	close(r.started)
	<-r.release

	return Result{Phase: PhaseSource, Outcome: OutcomeSucceeded}
}

func (r *blockingReportingRunner) RunTargets(context.Context) Result {
	return Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}
}

func (r *blockingReportingRunner) RunTarget(context.Context, string) Result {
	return Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}
}

func (r *blockingReportingRunner) ClearTarget(context.Context, string) Result {
	return Result{Phase: PhaseTargets, Outcome: OutcomeSucceeded}
}

func (r *blockingReportingRunner) AnnotateStored(context.Context) (classified, failed int) {
	return 0, 0
}

// recordedRunReference is what this fake names every run it records. The store
// mints a different one per run; what matters here is that whatever it returns
// reaches the notification.
const recordedRunReference = "1a2b3c4d5e6f"

type fakeRunState struct {
	scheduleErr      error
	targetRunErr     error
	lastFailureErr   error
	recordFailureErr error
	clearFailureErr  error
	lastFailure      map[string]time.Time
	lastPhase        map[string]string
	lastSuccessAt    map[string]time.Time
	lastSuccessErr   error
	successfulRuns   []successfulRun
	phases           []string
	recordedRuns     []recordedTargetRun
	runs             int
	source           bool
	targets          bool
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

func newReporter(t *testing.T, runner Runner, state RunState) *Reporter {
	t.Helper()
	reporter, err := NewReporter(runner, state)
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
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state)
	now := time.Date(2026, time.August, 18, 9, 0, 0, 0, time.UTC)
	reporter.now = func() time.Time { return now }

	reporter.RunPhase(t.Context(), PhaseTargets)
	assert.Equal(t, []recordedTargetRun{
		{finishedAt: now, id: "rider-a", outcome: "succeeded"},
		{finishedAt: now, id: "rider-b", outcome: "failed", detail: "destination"},
	}, state.recordedRuns)
}

// The source phase writes to no target, so it records nothing about one.
func TestReporterRecordsNoTargetRunsForASourceRun(t *testing.T) {
	runner := &reportingRunner{source: Result{Phase: PhaseSource, Outcome: OutcomeSucceeded, SourceStages: 2}}
	state := &fakeRunState{}
	reporter := newReporter(t, runner, state)

	reporter.RunPhase(t.Context(), PhaseSource)
	assert.Empty(t, state.recordedRuns)
}

// The reporter has to hand the named library to the runner rather than falling
// back to reading every one: a task that asks for one library and gets all of
// them looks like it worked.
func TestReporterRunSourceProviderReadsTheLibraryItWasGiven(t *testing.T) {
	t.Parallel()

	runner := &recordingSourceRunner{}
	runner.source = Result{Phase: PhaseSource, Outcome: OutcomeSucceeded}
	reporter := newReporter(t, runner, &fakeRunState{})

	reporter.RunSourceProvider(t.Context(), route.ProviderKomoot)

	assert.Equal(t, []route.Provider{route.ProviderKomoot}, runner.asked, "the library the runner was asked for")
	assert.Zero(t, runner.wholePhaseRuns, "the reporter read every library instead of the one it was given")
}

// recordingSourceRunner tells a read of one library from a read of them all.
type recordingSourceRunner struct {
	asked []route.Provider
	reportingRunner
	wholePhaseRuns int
}

func (r *recordingSourceRunner) RunSourceProvider(_ context.Context, provider route.Provider) Result {
	r.asked = append(r.asked, provider)

	return r.source
}

func (r *recordingSourceRunner) RunSource(context.Context) Result {
	r.wholePhaseRuns++

	return r.source
}
