package activity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFakeAnalyseStore = errors.New("fake analyse store")

type fakeAnalyseStore struct {
	metrics        map[int64]RideMetrics
	stored         map[int64]Analysis
	ownerErr       error
	pendingErr     error
	storeErr       error
	owner          string
	email          string
	password       string
	pending        []PendingAnalysis
	heldTypes      []int
	earlier        []Analysis
	loads          []trainingload.RideLoad
	heldSince      time.Time
	since          time.Time
	earlierBefore  []time.Time
	pendingLimit   int
	contextReads   int
	credentialsErr bool
}

func newFakeAnalyseStore(pending ...PendingAnalysis) *fakeAnalyseStore {
	metrics := make(map[int64]RideMetrics, len(pending))
	for _, ride := range pending {
		metrics[ride.ID] = RideMetrics{Load: trainingload.Metrics{TRIMP: 80, HasTRIMP: true}}
	}

	return &fakeAnalyseStore{owner: "subject-a", pending: pending, metrics: metrics, stored: map[int64]Analysis{}}
}

func (s *fakeAnalyseStore) TargetOwner(context.Context, string) (string, error) {
	return s.owner, s.ownerErr
}

func (s *fakeAnalyseStore) RiderProfile(context.Context, string) (rider.Profile, error) {
	s.contextReads++

	return rider.Profile{}, nil
}

func (s *fakeAnalyseStore) RiderZwiftCredentials(context.Context, string) (email, password []byte, err error) {
	if s.credentialsErr {
		return nil, nil, errFakeAnalyseStore
	}

	return []byte(s.email), []byte(s.password), nil
}

func (s *fakeAnalyseStore) ActivitiesAwaitingAnalysis(
	_ context.Context, _ string, since, heldSince time.Time, heldTypeIDs []int, limit int,
) ([]PendingAnalysis, error) {
	s.since, s.heldSince, s.heldTypes, s.pendingLimit = since, heldSince, heldTypeIDs, limit

	return s.pending, s.pendingErr
}

func (s *fakeAnalyseStore) ActivityMetrics(context.Context, string) (map[int64]RideMetrics, error) {
	return s.metrics, nil
}

func (s *fakeAnalyseStore) ActivityRideLoads(context.Context, string) ([]trainingload.RideLoad, error) {
	return s.loads, nil
}

func (s *fakeAnalyseStore) AnalysesBefore(_ context.Context, _ string, before time.Time, _ int) ([]Analysis, error) {
	s.earlierBefore = append(s.earlierBefore, before)

	return s.earlier, nil
}

func (s *fakeAnalyseStore) StoreActivityAnalysis(_ context.Context, _ string, id int64, analysis Analysis) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored[id] = analysis

	return nil
}

var errFakeAsk = errors.New("fake ask")

// fakeAsker answers each prompt in turn from answers; an empty entry fails with failure.
type fakeAsker struct {
	failure Failure
	answers []string
	prompts []string
}

func (a *fakeAsker) Ask(_ context.Context, prompt string) (text, model string, err error) {
	a.prompts = append(a.prompts, prompt)
	answer := a.answers[len(a.prompts)-1]
	if answer == "" {
		return "", "", errFakeAsk
	}

	return answer, "model-a", nil
}

func (a *fakeAsker) FailureOf(error) Failure { return a.failure }

func analyseNow() time.Time { return time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC) }

func newTestAnalyser(t *testing.T, store *fakeAnalyseStore, asker *fakeAsker) *Analyser {
	t.Helper()

	analyser, err := NewAnalyser(store, asker, analyseNow().Add(-24*time.Hour), []int{12},
		func() string { return "Europe/Berlin" }, analyseNow)
	require.NoError(t, err)

	return analyser
}

func TestAnalyseStoresAnAnswerForEachOwedRide(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(
		PendingAnalysis{ID: 1, StartedAt: analyseNow().Add(-3 * time.Hour)},
		PendingAnalysis{ID: 2, StartedAt: analyseNow().Add(-2 * time.Hour)},
	)
	asker := &fakeAsker{answers: []string{"first", "second"}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Polled, Analysed: 2}, result)
	assert.Equal(t, map[int64]Analysis{
		1: {AnalysedAt: analyseNow(), Text: "first", Model: "model-a", PromptRevision: PromptRevision},
		2: {AnalysedAt: analyseNow(), Text: "second", Model: "model-a", PromptRevision: PromptRevision},
	}, store.stored)
	assert.Equal(t, analyseNow().Add(-24*time.Hour), store.since, "owed since the enabled instant")
	assert.Equal(t, analysisRidesPerRun, store.pendingLimit)
	assert.Equal(t, []time.Time{store.pending[0].StartedAt, store.pending[1].StartedAt}, store.earlierBefore,
		"each ride's context is the analyses before it")
	assert.Equal(t, 1, store.contextReads, "the profile is read once per run")
}

// No backfill and no request: a target with nothing owed reads nothing else.
func TestAnalyseAsksNothingWhenNothingIsOwed(t *testing.T) {
	t.Parallel()
	store, asker := newFakeAnalyseStore(), &fakeAsker{}

	assert.Equal(t, Result{Outcome: Unchanged}, newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a"))
	assert.Empty(t, asker.prompts)
	assert.Zero(t, store.contextReads)
}

func TestAnalyseLeavesASlotNobodyOwnsAlone(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	store.owner = ""

	assert.Equal(t, Result{Outcome: Unchanged}, newTestAnalyser(t, store, &fakeAsker{}).Analyse(t.Context(), "rider-a"))
}

// A rider with Zwift credentials has a recent head unit indoor ride held back;
// a rider without them has nothing held.
func TestAnalyseHoldsIndoorRidesOnlyForARiderWithZwift(t *testing.T) {
	t.Parallel()
	without := newFakeAnalyseStore()
	newTestAnalyser(t, without, &fakeAsker{}).Analyse(t.Context(), "rider-a")
	assert.Empty(t, without.heldTypes)

	with := newFakeAnalyseStore()
	with.email, with.password = "rider@example.test", "hunter2"
	newTestAnalyser(t, with, &fakeAsker{}).Analyse(t.Context(), "rider-a")
	assert.Equal(t, []int{12}, with.heldTypes)
	assert.Equal(t, analyseNow().Add(-trainerCopyHold), with.heldSince)
}

// The first failed request ends the run; the rides after it stay owed, and the
// log carries a category and counts, never the prompt or an answer.
func TestAnalyseStopsAtTheFirstFailure(t *testing.T) {
	logged := captureLogs(t)
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1}, PendingAnalysis{ID: 2}, PendingAnalysis{ID: 3})
	store.earlier = []Analysis{{Text: "an earlier private answer"}}
	asker := &fakeAsker{failure: FailureAllowance, answers: []string{"a private answer", "", "never asked"}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureAllowance, Analysed: 1}, result)
	assert.Len(t, asker.prompts, 2, "nothing is asked after the failure")
	assert.NotContains(t, store.stored, int64(2))
	assert.Contains(t, logged.String(), "failure=allowance")
	assert.NotContains(t, logged.String(), "private answer")
	assert.NotContains(t, logged.String(), "TRIMP")
}

func TestAnalyseRefusesAnAnswerOverTheBound(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	asker := &fakeAsker{answers: []string{strings.Repeat("é", MaximumAnalysisCharacters+1)}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureUnusable}, result)
	assert.Empty(t, store.stored)
}

func TestAnalyseReportsStoreFailuresAsState(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*fakeAnalyseStore){
		"the owner":       func(s *fakeAnalyseStore) { s.ownerErr = errFakeAnalyseStore },
		"the credentials": func(s *fakeAnalyseStore) { s.credentialsErr = true },
		"the owed rides":  func(s *fakeAnalyseStore) { s.pendingErr = errFakeAnalyseStore },
		"the write":       func(s *fakeAnalyseStore) { s.storeErr = errFakeAnalyseStore },
		"a missing row":   func(s *fakeAnalyseStore) { s.metrics = map[int64]RideMetrics{} },
	}
	for name, breakStore := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
			breakStore(store)

			result := newTestAnalyser(t, store, &fakeAsker{answers: []string{"answer"}}).Analyse(t.Context(), "rider-a")
			assert.Equal(t, Result{Outcome: Failed, Failure: FailureState}, result)
		})
	}
}

func TestNewAnalyserRefusesIncompleteOptions(t *testing.T) {
	t.Parallel()
	store, asker, since, zone := newFakeAnalyseStore(), &fakeAsker{}, analyseNow(), func() string { return "UTC" }

	_, err := NewAnalyser(nil, asker, since, []int{12}, zone, analyseNow)
	require.Error(t, err, "no store")
	_, err = NewAnalyser(store, asker, time.Time{}, []int{12}, zone, analyseNow)
	require.Error(t, err, "no enabled instant")
	_, err = NewAnalyser(store, asker, since, nil, zone, analyseNow)
	assert.Error(t, err, "no indoor types")
}
