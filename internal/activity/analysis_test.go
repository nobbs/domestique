package activity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFakeAnalyseStore = errors.New("fake analyse store")

type fakeAnalyseStore struct {
	metrics         map[int64]RideMetrics
	stored          map[int64]Analysis
	started         map[int64]time.Time
	rides           map[int64]Stored
	recentRides     []Stored
	sessions        map[int64]Session
	matches         map[int64]RouteMatch
	weatherSums     map[int64]WeatherSummary
	routeNames      map[route.Key]string
	stageLine       []measure.Coordinate
	stageElevations []float64
	climbAttempts   []StoredClimbAttempt
	ownerErr        error
	pendingErr      error
	storeErr        error
	contextErr      error
	earlierErr      error
	metricsErr      error
	loadsErr        error
	startedErr      error
	sessionsErr     error
	matchesErr      error
	weatherSumsErr  error
	powerCurveErr   error
	recentErr       error
	ridesErr        error
	weatherStepsErr error
	seriesErr       error
	owner           string
	email           string
	password        string
	pending         []PendingAnalysis
	heldTypes       []int
	earlier         []Analysis
	loads           []trainingload.RideLoad
	heldSince       time.Time
	since           time.Time
	earlierBefore   []time.Time
	pendingLimit    int
	contextReads    int
	credentialsErr  bool
}

func newFakeAnalyseStore(pending ...PendingAnalysis) *fakeAnalyseStore {
	metrics := make(map[int64]RideMetrics, len(pending))
	rides := make(map[int64]Stored, len(pending))
	for _, ride := range pending {
		metrics[ride.ID] = RideMetrics{Load: trainingload.Metrics{TRIMP: 80, HasTRIMP: true}}
		rides[ride.ID] = Stored{ID: ride.ID, StartedAt: ride.StartedAt}
	}

	return &fakeAnalyseStore{
		owner: "subject-a", pending: pending, metrics: metrics, rides: rides, stored: map[int64]Analysis{},
	}
}

func (s *fakeAnalyseStore) TargetOwner(context.Context, string) (string, error) {
	return s.owner, s.ownerErr
}

func (s *fakeAnalyseStore) RiderProfile(context.Context, string) (rider.Profile, error) {
	s.contextReads++

	return rider.Profile{}, s.contextErr
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
	return s.metrics, s.metricsErr
}

func (s *fakeAnalyseStore) ActivityRideLoads(context.Context, string) ([]trainingload.RideLoad, error) {
	return s.loads, s.loadsErr
}

func (s *fakeAnalyseStore) AnalysesBefore(_ context.Context, _ string, before time.Time, _ int) ([]Analysis, error) {
	s.earlierBefore = append(s.earlierBefore, before)

	return s.earlier, s.earlierErr
}

func (s *fakeAnalyseStore) ActivityStartedAt(_ context.Context, _ string, id int64) (time.Time, bool, error) {
	at, held := s.started[id]

	return at, held, s.startedErr
}

//nolint:gocritic // hugeParam: mirrors AnalyseStore's own by-value signature.
func (s *fakeAnalyseStore) StoreActivityAnalysis(_ context.Context, _ string, id int64, analysis Analysis) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored[id] = analysis

	return nil
}

func (s *fakeAnalyseStore) ActivitiesBetween(
	_ context.Context, _ string, from, to time.Time, limit int,
) ([]Stored, error) {
	if limit == sameSecondRides {
		if s.ridesErr != nil {
			return nil, s.ridesErr
		}
		var inWindow []Stored
		for id := range s.rides {
			ride := s.rides[id]
			if !ride.StartedAt.Before(from) && ride.StartedAt.Before(to) {
				inWindow = append(inWindow, ride)
			}
		}

		return inWindow, nil
	}

	if s.recentErr != nil {
		return nil, s.recentErr
	}
	var inWindow []Stored
	for _, ride := range s.recentRides {
		if !ride.StartedAt.Before(from) && ride.StartedAt.Before(to) {
			inWindow = append(inWindow, ride)
		}
	}

	return inWindow, nil
}

func (s *fakeAnalyseStore) ActivitySessions(context.Context, string) (map[int64]Session, error) {
	return s.sessions, s.sessionsErr
}

func (s *fakeAnalyseStore) ActivityRouteMatches(context.Context, string) (map[int64]RouteMatch, error) {
	return s.matches, s.matchesErr
}

func (s *fakeAnalyseStore) RouteName(_ context.Context, key route.Key) (name string, found bool, err error) {
	name, found = s.routeNames[key]

	return name, found, nil
}

func (s *fakeAnalyseStore) ActivityWeatherSummaries(context.Context, string) (map[int64]WeatherSummary, error) {
	return s.weatherSums, s.weatherSumsErr
}

func (s *fakeAnalyseStore) ActivityWeatherSteps(context.Context, string, int64) ([]WeatherStep, error) {
	return nil, s.weatherStepsErr
}

func (s *fakeAnalyseStore) ActivityRecordSeries(context.Context, string, int64) ([]SampleRow, error) {
	return nil, s.seriesErr
}

func (s *fakeAnalyseStore) StageProfile(
	context.Context, route.Key,
) (line []measure.Coordinate, elevations []float64, found bool, err error) {
	return s.stageLine, s.stageElevations, s.stageLine != nil, nil
}

func (s *fakeAnalyseStore) RouteClimbAttempts(context.Context, string, route.Key) ([]StoredClimbAttempt, error) {
	return s.climbAttempts, nil
}

func (s *fakeAnalyseStore) PowerCurve(context.Context, []string, time.Time, time.Time) (rider.PowerCurve, error) {
	return rider.PowerCurve{}, s.powerCurveErr
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

// document is a minimal, valid revision-3 answer carrying summary as its
// summary: every other field the schema requires is filled with a placeholder.
func document(summary string) string {
	return fmt.Sprintf(
		`{"ride_type":"endurance","headline":"h","summary":%q,"load_effect":"l","highlights":[],"concerns":[],"next_session":{"advice":"a","suggested_rest_days":1},"data_gaps":[]}`,
		summary)
}

func analyseNow() time.Time { return time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC) }

func newTestAnalyser(t *testing.T, store *fakeAnalyseStore, asker *fakeAsker) *Analyser {
	t.Helper()

	analyser, err := NewAnalyser(store, asker, analyseNow().Add(-24*time.Hour), []int{12},
		func() string { return "Europe/Berlin" }, analyseNow)
	require.NoError(t, err)

	return analyser
}

// A rich context (a session, a route match, a weather summary and a recent
// ride on the same route) all reach the prompt through buildBundle.
func TestAnalyseBuildsARichBundle(t *testing.T) {
	t.Parallel()
	ride := PendingAnalysis{ID: 1, StartedAt: analyseNow().Add(-time.Hour)}
	store := newFakeAnalyseStore(ride)
	key := route.NewKey(route.ProviderVeloPlanner, 42, 0)
	store.sessions = map[int64]Session{1: {Sport: "cycling", AveragePowerWatts: Reading{Value: 200, Known: true}}}
	store.matches = map[int64]RouteMatch{1: {Key: key, RouteCoverage: 0.9, RideCoverage: 0.9}}
	store.weatherSums = map[int64]WeatherSummary{1: {TemperatureMinCelsius: 10, TemperatureMaxCelsius: 15}}
	store.routeNames = map[route.Key]string{key: "Alpe secrète"}
	store.recentRides = []Stored{{ID: 2, StartedAt: ride.StartedAt.Add(-24 * time.Hour), DistanceMetres: 10000}}
	store.matches[2] = RouteMatch{Key: key}
	asker := &fakeAsker{answers: []string{document("a rich ride")}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Polled, Analysed: 1}, result)
	require.Len(t, asker.prompts, 1)
	assert.Contains(t, asker.prompts[0], "library route: Alpe secrète")
	assert.Contains(t, asker.prompts[0], "weather: 10.0 to 15.0")
	assert.Contains(t, asker.prompts[0], "Alpe secrète") // named in the recent-rides table too
}

func TestAnalyseStoresAnAnswerForEachOwedRide(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(
		PendingAnalysis{ID: 1, StartedAt: analyseNow().Add(-3 * time.Hour)},
		PendingAnalysis{ID: 2, StartedAt: analyseNow().Add(-2 * time.Hour)},
	)
	asker := &fakeAsker{answers: []string{document("first"), document("second")}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Polled, Analysed: 2}, result)
	require.Contains(t, store.stored, int64(1))
	require.Contains(t, store.stored, int64(2))
	assert.Equal(t, "first", store.stored[1].Text)
	assert.Equal(t, "first", store.stored[1].Document.Summary)
	assert.Equal(t, "second", store.stored[2].Text)
	assert.Equal(t, PromptRevision, store.stored[1].PromptRevision)
	assert.Equal(t, "model-a", store.stored[1].Model)
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
	asker := &fakeAsker{failure: FailureAllowance, answers: []string{document("a private answer"), "", "never asked"}}

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
	asker := &fakeAsker{answers: []string{document(strings.Repeat("é", MaximumAnalysisCharacters+1))}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureUnusable}, result)
	assert.Empty(t, store.stored)
}

func TestAnalyseRefusesAnAnswerOverTheDocumentBound(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	asker := &fakeAsker{answers: []string{document(strings.Repeat("a", MaximumDocumentBytes))}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureUnusable}, result)
	assert.Empty(t, store.stored)
}

func TestAnalyseRefusesAnUnparsableAnswer(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	asker := &fakeAsker{answers: []string{"not json"}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureUnusable}, result)
	assert.Empty(t, store.stored)
}

func TestAnalyseRefusesAnEmptySummary(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	asker := &fakeAsker{answers: []string{document("")}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureUnusable}, result)
	assert.Empty(t, store.stored)
}

func TestAnalyseRefusesAnUnknownRideType(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	asker := &fakeAsker{answers: []string{`{"ride_type":"epic","summary":"first"}`}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureUnusable}, result)
	assert.Empty(t, store.stored)
}

func TestAnalyseReportsStoreFailuresAsState(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*fakeAnalyseStore){
		"the owner":                func(s *fakeAnalyseStore) { s.ownerErr = errFakeAnalyseStore },
		"the credentials":          func(s *fakeAnalyseStore) { s.credentialsErr = true },
		"the owed rides":           func(s *fakeAnalyseStore) { s.pendingErr = errFakeAnalyseStore },
		"the write":                func(s *fakeAnalyseStore) { s.storeErr = errFakeAnalyseStore },
		"the profile":              func(s *fakeAnalyseStore) { s.contextErr = errFakeAnalyseStore },
		"the earlier":              func(s *fakeAnalyseStore) { s.earlierErr = errFakeAnalyseStore },
		"the metrics":              func(s *fakeAnalyseStore) { s.metricsErr = errFakeAnalyseStore },
		"the loads":                func(s *fakeAnalyseStore) { s.loadsErr = errFakeAnalyseStore },
		"a missing row":            func(s *fakeAnalyseStore) { s.metrics = map[int64]RideMetrics{} },
		"the sessions":             func(s *fakeAnalyseStore) { s.sessionsErr = errFakeAnalyseStore },
		"the route matches":        func(s *fakeAnalyseStore) { s.matchesErr = errFakeAnalyseStore },
		"the weather summaries":    func(s *fakeAnalyseStore) { s.weatherSumsErr = errFakeAnalyseStore },
		"the power curve":          func(s *fakeAnalyseStore) { s.powerCurveErr = errFakeAnalyseStore },
		"the recent rides":         func(s *fakeAnalyseStore) { s.recentErr = errFakeAnalyseStore },
		"the ride's own read":      func(s *fakeAnalyseStore) { s.ridesErr = errFakeAnalyseStore },
		"the ride's weather steps": func(s *fakeAnalyseStore) { s.weatherStepsErr = errFakeAnalyseStore },
		"the ride's series":        func(s *fakeAnalyseStore) { s.seriesErr = errFakeAnalyseStore },
	}
	for name, breakStore := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
			breakStore(store)

			result := newTestAnalyser(t, store, &fakeAsker{answers: []string{document("answer")}}).Analyse(t.Context(), "rider-a")
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

// The prompt carries the rider's current load, cut in the configured zone, or
// in UTC when that zone cannot be read.
func TestAnalyseTellsTheCurrentLoad(t *testing.T) {
	t.Parallel()
	ride := PendingAnalysis{ID: 1, StartedAt: analyseNow().Add(-time.Hour)}
	store := newFakeAnalyseStore(ride)
	store.loads = []trainingload.RideLoad{{At: ride.StartedAt, TRIMP: 80, TSS: 70}}
	asker := &fakeAsker{answers: []string{document("answer")}}
	analyser, err := NewAnalyser(store, asker, analyseNow().Add(-24*time.Hour), []int{12},
		func() string { return "Not/AZone" }, analyseNow)
	require.NoError(t, err)

	assert.Equal(t, Result{Outcome: Polled, Analysed: 1}, analyser.Analyse(t.Context(), "rider-a"))
	require.Len(t, asker.prompts, 1)
	assert.Contains(t, asker.prompts[0], "The rider's training load now")
}

// A shutdown mid-request is not a failed analysis worth a warning.
func TestAnalyseLogsNoFailureForAShutdown(t *testing.T) {
	logged := captureLogs(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := newFakeAnalyseStore(PendingAnalysis{ID: 1})
	asker := &fakeAsker{failure: FailureExecutable, answers: []string{""}}

	result := newTestAnalyser(t, store, asker).Analyse(ctx, "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.NotContains(t, logged.String(), "ride analysis failed")
}

// An admin's re-analysis asks about the one ride whatever it started and whatever
// stands, and replaces what stood only with an answer.
func TestReanalyseAsksAboutOneDerivedRideWheneverItStarted(t *testing.T) {
	t.Parallel()
	started := analyseNow().Add(-400 * 24 * time.Hour)
	store := newFakeAnalyseStore()
	store.metrics[7] = RideMetrics{Load: trainingload.Metrics{TRIMP: 80, HasTRIMP: true}}
	store.started = map[int64]time.Time{7: started}
	store.rides[7] = Stored{ID: 7, StartedAt: started}
	store.stored[7] = Analysis{Text: "what stood"}
	asker := &fakeAsker{answers: []string{document("said again")}}

	result := newTestAnalyser(t, store, asker).Reanalyse(t.Context(), "rider-a", 7)

	assert.Equal(t, Result{Outcome: Polled, Analysed: 1}, result)
	assert.Equal(t, "said again", store.stored[7].Text)
	assert.Equal(t, []time.Time{started}, store.earlierBefore, "its context is the analyses before it")
}

// A year-old ride is told the load it left behind, not the rider's load today.
func TestReanalyseReadsTheLoadOnTheRidesOwnDay(t *testing.T) {
	t.Parallel()
	started := analyseNow().Add(-400 * 24 * time.Hour)
	store := newFakeAnalyseStore()
	store.metrics[7] = RideMetrics{}
	store.started = map[int64]time.Time{7: started}
	store.rides[7] = Stored{ID: 7, StartedAt: started}
	store.loads = []trainingload.RideLoad{{At: started, TSS: 100}, {At: analyseNow().Add(-time.Hour), TSS: 300}}
	asker := &fakeAsker{answers: []string{document("said again")}}

	newTestAnalyser(t, store, asker).Reanalyse(t.Context(), "rider-a", 7)

	require.Len(t, asker.prompts, 1)
	day := trainingload.Timeline(store.loads, started, time.UTC)
	assert.Contains(t, asker.prompts[0], loadOnRideDay+":")
	assert.Contains(t, asker.prompts[0], fmt.Sprintf("fatigue %.0f", day[len(day)-1].TSSFatigue))
	assert.NotContains(t, asker.prompts[0], loadNow)
}

func TestReanalyseKeepsWhatStoodWhenTheRequestFails(t *testing.T) {
	t.Parallel()
	store := newFakeAnalyseStore()
	store.metrics[7] = RideMetrics{}
	store.started = map[int64]time.Time{7: analyseNow()}
	store.rides[7] = Stored{ID: 7, StartedAt: analyseNow()}
	store.stored[7] = Analysis{Text: "what stood"}

	result := newTestAnalyser(t, store, &fakeAsker{failure: FailureAllowance, answers: []string{""}}).
		Reanalyse(t.Context(), "rider-a", 7)

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureAllowance}, result)
	assert.Equal(t, "what stood", store.stored[7].Text)
}

func TestReanalyseAsksNothingForARideItCannotAnswerFor(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*fakeAnalyseStore){
		"a ride the target lacks": func(*fakeAnalyseStore) {},
		"an underived ride":       func(s *fakeAnalyseStore) { s.started = map[int64]time.Time{7: analyseNow()} },
		"a slot nobody owns": func(s *fakeAnalyseStore) {
			s.owner, s.started, s.metrics[7] = "", map[int64]time.Time{7: analyseNow()}, RideMetrics{}
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, asker := newFakeAnalyseStore(), &fakeAsker{}
			arrange(store)

			assert.Equal(t, Result{Outcome: Unchanged}, newTestAnalyser(t, store, asker).Reanalyse(t.Context(), "rider-a", 7))
			assert.Empty(t, asker.prompts)
		})
	}
}

func TestReanalyseReportsStoreFailuresAsState(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*fakeAnalyseStore){
		"the owner":   func(s *fakeAnalyseStore) { s.ownerErr = errFakeAnalyseStore },
		"the start":   func(s *fakeAnalyseStore) { s.startedErr = errFakeAnalyseStore },
		"the context": func(s *fakeAnalyseStore) { s.contextErr = errFakeAnalyseStore },
	}
	for name, breakStore := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := newFakeAnalyseStore()
			store.metrics[7] = RideMetrics{}
			store.started = map[int64]time.Time{7: analyseNow()}
			breakStore(store)

			result := newTestAnalyser(t, store, &fakeAsker{answers: []string{document("answer")}}).Reanalyse(t.Context(), "rider-a", 7)
			assert.Equal(t, Result{Outcome: Failed, Failure: FailureState}, result)
		})
	}
}

// Two rides can start in the same second; the bundle is read for the owed one,
// and a ride gone between listing and reading is a state failure, never a
// prompt about a zero-valued ride.
func TestAnalysePicksTheOwedRideOutOfItsStartSecond(t *testing.T) {
	t.Parallel()
	startedAt := analyseNow().Add(-time.Hour)
	ride := PendingAnalysis{ID: 2, StartedAt: startedAt}
	store := newFakeAnalyseStore(ride)
	store.rides[1] = Stored{ID: 1, StartedAt: startedAt, DistanceMetres: 99000}
	store.rides[2] = Stored{ID: 2, StartedAt: startedAt, DistanceMetres: 42000}
	asker := &fakeAsker{answers: []string{document("the right ride")}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Polled, Analysed: 1}, result)
	require.Len(t, asker.prompts, 1)
	assert.Contains(t, asker.prompts[0], "distance 42.0 km")
	assert.NotContains(t, asker.prompts[0], "99.0 km")
}

func TestAnalyseFailsAsStateWhenTheOwedRideIsGone(t *testing.T) {
	t.Parallel()
	ride := PendingAnalysis{ID: 1, StartedAt: analyseNow().Add(-time.Hour)}
	store := newFakeAnalyseStore(ride)
	delete(store.rides, 1)
	asker := &fakeAsker{answers: []string{document("never asked")}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Failed, Failure: FailureState}, result)
	assert.Empty(t, asker.prompts)
}

// An older owed ride is read against what came before it: a later ride's
// climb attempt and its row in the recent-rides table are left out.
func TestAnalyseCutsHistoryAtTheRidesStart(t *testing.T) {
	t.Parallel()
	ride := PendingAnalysis{ID: 1, StartedAt: analyseNow().Add(-48 * time.Hour)}
	store := newFakeAnalyseStore(ride)
	key := route.NewKey(route.ProviderVeloPlanner, 42, 0)
	store.matches = map[int64]RouteMatch{1: {Key: key}, 3: {Key: key}}
	store.stageLine = []measure.Coordinate{{Latitude: 47, Longitude: 8}, {Latitude: 47.02, Longitude: 8}}
	store.stageElevations = []float64{100, 400}
	store.climbAttempts = []StoredClimbAttempt{
		{WorkoutID: 1, RiddenAt: ride.StartedAt, ClimbAttempt: ClimbAttempt{Seconds: 600}},                     //nolint:modernize // Explicit type keeps the rows scannable.
		{WorkoutID: 3, RiddenAt: ride.StartedAt.Add(24 * time.Hour), ClimbAttempt: ClimbAttempt{Seconds: 100}}, //nolint:modernize // Explicit type keeps the rows scannable.
	}
	store.recentRides = []Stored{{ID: 3, StartedAt: ride.StartedAt.Add(24 * time.Hour), DistanceMetres: 77000}}
	asker := &fakeAsker{answers: []string{document("an older ride")}}

	result := newTestAnalyser(t, store, asker).Analyse(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Polled, Analysed: 1}, result)
	require.Len(t, asker.prompts, 1)
	assert.Contains(t, asker.prompts[0], "Climbs:")
	assert.NotContains(t, asker.prompts[0], "77.0")
	assert.NotContains(t, asker.prompts[0], ",1.7,") // the later attempt's 100 s would be the best
}
