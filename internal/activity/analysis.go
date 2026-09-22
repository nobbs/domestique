package activity

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/trainingload"
)

const (
	// PromptRevision names the prompt below; an analysis records the one it
	// was asked with.
	PromptRevision = 3

	// MaximumAnalysisCharacters is the contract's bound on a stored summary.
	MaximumAnalysisCharacters = 2000
	// MaximumDocumentBytes bounds the whole structured answer.
	MaximumDocumentBytes = 16384

	// analysisRidesPerRun bounds how long one run holds the activities.
	analysisRidesPerRun = 5
	// analysisContextAnalyses is how many earlier analyses a prompt carries.
	analysisContextAnalyses = 5
	// trainerCopyHold is one zwift:poll interval and an hour past a ride's end:
	// a head unit's indoor ride inside it may still be replaced by the Zwift copy.
	trainerCopyHold = 7 * time.Hour
	// recentRideHistoryDays and recentRideHistoryLimit bound the "recent rides"
	// section of the prompt, and analysisSplitMetres its splits table.
	recentRideHistoryDays  = 90
	recentRideHistoryLimit = 20
	analysisSplitMetres    = 5000
	// sameSecondRides bounds how many rides one start second is read for.
	sameSecondRides = 8
)

// errRideGone is an owed ride that was removed between being listed and read.
var errRideGone = errors.New("activity: the ride is no longer stored")

// rideTypes is every ride_type the schema allows.
var rideTypes = []string{ //nolint:gochecknoglobals // A constant list of enum values, never mutated.
	"recovery", "endurance", "tempo", "threshold", "intervals", "race", "mixed", "commute",
}

// AnalysisDocument is the structured answer prompt revision 3 asks for.
type AnalysisDocument struct {
	// RideType is one of rideTypes.
	RideType    string   `json:"ride_type"` //nolint:tagliatelle // Mirrors the schema's own field names.
	Headline    string   `json:"headline"`
	Summary     string   `json:"summary"`
	LoadEffect  string   `json:"load_effect"` //nolint:tagliatelle // Mirrors the schema's own field names.
	Highlights  []string `json:"highlights"`
	Concerns    []string `json:"concerns"`
	NextSession struct {
		Advice            string `json:"advice"`
		SuggestedRestDays int    `json:"suggested_rest_days"` //nolint:tagliatelle // Mirrors the schema's own field names.
	} `json:"next_session"` //nolint:tagliatelle // Mirrors the schema's own field names.
	DataGaps []string `json:"data_gaps"` //nolint:tagliatelle // Mirrors the schema's own field names.
}

const (
	// FailureToken means the Claude token was refused.
	FailureToken Failure = "token"
	// FailureAllowance means the subscription's allowance is exhausted.
	FailureAllowance Failure = "allowance"
	// FailureExecutable means the claude executable failed or timed out.
	FailureExecutable Failure = "executable"
	// FailureUnusable means an answer arrived empty or over the bound.
	FailureUnusable Failure = "unusable"
)

// AnalysisSchema is the JSON Schema AnalysisDocument is asked for under,
// handed to claude.Options.Schema; each call returns its own copy.
func AnalysisSchema() []byte { return []byte(analysisSchema) }

const analysisSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["ride_type", "headline", "summary", "load_effect", "highlights", "concerns", "next_session", "data_gaps"],
  "properties": {
    "ride_type": {"type": "string", "enum": ["recovery", "endurance", "tempo", "threshold", "intervals", "race", "mixed", "commute"]},
    "headline": {"type": "string", "maxLength": 200},
    "summary": {"type": "string", "maxLength": 2000},
    "load_effect": {"type": "string", "maxLength": 1000},
    "highlights": {"type": "array", "maxItems": 6, "items": {"type": "string", "maxLength": 200}},
    "concerns": {"type": "array", "maxItems": 6, "items": {"type": "string", "maxLength": 200}},
    "next_session": {
      "type": "object",
      "additionalProperties": false,
      "required": ["advice", "suggested_rest_days"],
      "properties": {
        "advice": {"type": "string", "maxLength": 500},
        "suggested_rest_days": {"type": "integer", "minimum": 0, "maximum": 7}
      }
    },
    "data_gaps": {"type": "array", "maxItems": 6, "items": {"type": "string", "maxLength": 200}}
  }
}`

// Analysis is what a language model made of one ride, with what produced it.
type Analysis struct {
	AnalysedAt     time.Time
	StartedAt      time.Time
	Text           string
	Model          string
	Document       AnalysisDocument
	PromptRevision int
}

// PendingAnalysis is a derived ride owed an analysis.
type PendingAnalysis struct {
	StartedAt time.Time
	ID        int64
}

// Asker answers one prompt.
type Asker interface {
	Ask(ctx context.Context, prompt string) (text, model string, err error)
	// FailureOf is the stable category of an error Ask returned.
	FailureOf(err error) Failure
}

// AnalyseStore is what asking about a target's rides reads and writes.
//
//nolint:interfacebloat // One bundle, one read model: splitting it would not shrink what a caller must satisfy.
type AnalyseStore interface {
	TargetOwner(ctx context.Context, targetID string) (string, error)
	RiderProfile(ctx context.Context, subject string) (rider.Profile, error)
	// RiderZwiftCredentials are the rider's own Zwift email and password, each
	// empty when it has not been entered.
	RiderZwiftCredentials(ctx context.Context, subject string) (email, password []byte, err error)
	// ActivitiesAwaitingAnalysis lists derived, unanalysed rides started at or
	// after since, oldest first, leaving out a head unit's ride of heldTypeIDs
	// that ended at or after heldSince.
	ActivitiesAwaitingAnalysis(ctx context.Context, targetID string, since, heldSince time.Time,
		heldTypeIDs []int, limit int) ([]PendingAnalysis, error)
	ActivityMetrics(ctx context.Context, targetID string) (map[int64]RideMetrics, error)
	ActivityRideLoads(ctx context.Context, targetID string) ([]trainingload.RideLoad, error)
	// AnalysesBefore is what was said about rides started before one instant,
	// newest first.
	AnalysesBefore(ctx context.Context, targetID string, before time.Time, limit int) ([]Analysis, error)
	StoreActivityAnalysis(ctx context.Context, targetID string, id int64, analysis Analysis) error
	// ActivityStartedAt is when one ride started, and whether the target holds it.
	ActivityStartedAt(ctx context.Context, targetID string, id int64) (time.Time, bool, error)

	// ActivitiesBetween is one target's recorded activities started within
	// [from, to), newest first, at most limit of them.
	ActivitiesBetween(ctx context.Context, targetID string, from, to time.Time, limit int) ([]Stored, error)
	ActivitySessions(ctx context.Context, targetID string) (map[int64]Session, error)
	ActivityRouteMatches(ctx context.Context, targetID string) (map[int64]RouteMatch, error)
	// RouteName is a route's own display name, for the prompt only: never
	// served to a browser, which reads the library for that.
	RouteName(ctx context.Context, key route.Key) (name string, found bool, err error)
	ActivityWeatherSummaries(ctx context.Context, targetID string) (map[int64]WeatherSummary, error)
	ActivityWeatherSteps(ctx context.Context, targetID string, id int64) ([]WeatherStep, error)
	ActivitySeries(ctx context.Context, targetID string, id int64) ([]SampleRow, error)
	ActivityTrack(ctx context.Context, targetID string, id int64) ([]TrackPoint, error)
	StageProfile(ctx context.Context, key route.Key) (line []measure.Coordinate, elevations []float64, found bool, err error)
	// RouteClimbAttempts is every attempt any of the target's rides made at
	// one route's climbs, newest ride first.
	RouteClimbAttempts(ctx context.Context, targetID string, key route.Key) ([]StoredClimbAttempt, error)
	// PowerCurve is the rider's all-time best over the power curve durations.
	PowerCurve(ctx context.Context, targetIDs []string, from, to time.Time) (rider.PowerCurve, error)
}

// Analyser asks a language model about each ride owed an analysis.
type Analyser struct {
	enabledSince time.Time
	store        AnalyseStore
	asker        Asker
	now          func() time.Time
	timezone     func() string
	indoorTypes  []int
}

// NewAnalyser builds an analyser over stored state. enabledSince is the
// instant the analysis was first enabled; timezone names the IANA zone a
// training day is cut in.
func NewAnalyser(
	store AnalyseStore, asker Asker, enabledSince time.Time, indoorTypes []int,
	timezone func() string, now func() time.Time,
) (*Analyser, error) {
	if store == nil || asker == nil || timezone == nil || now == nil {
		return nil, errors.New("activity: a store, an asker, a timezone and a clock are required")
	}
	if enabledSince.IsZero() || len(indoorTypes) == 0 {
		return nil, errors.New("activity: the enabled instant and the indoor workout types are required")
	}

	return &Analyser{
		enabledSince: enabledSince, store: store, asker: asker, now: now, timezone: timezone,
		indoorTypes: slices.Clone(indoorTypes),
	}, nil
}

// Analyse asks about a bounded few of one target's owed rides, oldest first,
// and stops at the first that fails: the rest stay owed for the next run.
func (a *Analyser) Analyse(ctx context.Context, targetID string) Result {
	subject, err := a.store.TargetOwner(ctx, targetID)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if subject == "" {
		return Result{Outcome: Unchanged}
	}
	now := a.now()
	held, err := a.heldTypes(ctx, subject)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	pending, err := a.store.ActivitiesAwaitingAnalysis(
		ctx, targetID, a.enabledSince, now.Add(-trainerCopyHold), held, analysisRidesPerRun)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if len(pending) == 0 {
		return Result{Outcome: Unchanged}
	}
	subjectContext, err := a.readContext(ctx, targetID, subject, now)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	analysed := 0
	for _, ride := range pending {
		if failure := a.analyseOne(ctx, targetID, ride, &subjectContext); failure != FailureNone {
			if ctx.Err() != nil {
				return Result{Outcome: Failed, Failure: failure, Analysed: analysed}
			}
			slog.Warn("ride analysis failed", "target", targetID, "analysed", analysed, "failure", string(failure))

			return Result{Outcome: Failed, Failure: failure, Analysed: analysed}
		}
		analysed++
	}
	slog.Info("activities analysed", "target", targetID, "analysed", analysed)

	return Result{Outcome: Polled, Analysed: analysed}
}

// Reanalyse asks once more about one derived ride, whenever it started and
// whatever stands for it; a failed request leaves the stored analysis in place.
func (a *Analyser) Reanalyse(ctx context.Context, targetID string, id int64) Result {
	subject, err := a.store.TargetOwner(ctx, targetID)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	startedAt, held, err := a.store.ActivityStartedAt(ctx, targetID, id)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if subject == "" || !held {
		return Result{Outcome: Unchanged}
	}
	// The ride may be a year old, so it is read against the load it left behind.
	run, err := a.readContext(ctx, targetID, subject, startedAt)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	run.loadLabel = loadOnRideDay
	if _, derived := run.metrics[id]; !derived {
		return Result{Outcome: Unchanged}
	}
	if failure := a.analyseOne(ctx, targetID, PendingAnalysis{ID: id, StartedAt: startedAt}, &run); failure != FailureNone {
		if ctx.Err() == nil {
			slog.Warn("ride re-analysis failed", "target", targetID, "failure", string(failure))
		}

		return Result{Outcome: Failed, Failure: failure}
	}
	slog.Info("activity re-analysed", "target", targetID)

	return Result{Outcome: Polled, Analysed: 1}
}

// buildBundle gathers everything analyseOne's prompt is composed from: the
// run-level context readContext already read, plus this one ride's own
// records, its route's climbs and this rider's other attempts at them.
func (a *Analyser) buildBundle(
	ctx context.Context, targetID string, ride PendingAnalysis, run *analysisContext, metrics *RideMetrics,
	earlier []Analysis,
) (*bundle, error) {
	// Start times are not unique, so the second the ride started in is read
	// and the ride picked out of it by id.
	rides, err := a.store.ActivitiesBetween(ctx, targetID, ride.StartedAt, ride.StartedAt.Add(time.Second), sameSecondRides)
	if err != nil {
		return nil, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	index := slices.IndexFunc(rides, func(stored Stored) bool { return stored.ID == ride.ID })
	if index < 0 {
		return nil, errRideGone
	}
	stored := rides[index]
	weatherSteps, err := a.store.ActivityWeatherSteps(ctx, targetID, ride.ID)
	if err != nil {
		return nil, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	series, err := a.store.ActivitySeries(ctx, targetID, ride.ID)
	if err != nil {
		return nil, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	track, err := a.store.ActivityTrack(ctx, targetID, ride.ID)
	if err != nil {
		return nil, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}

	b := &bundle{
		profile: run.profile, powerCurve: run.powerCurve,
		ride: stored, indoor: slices.Contains(run.indoorTypes, stored.TypeID),
		session: run.sessions[ride.ID], metrics: metrics,
		weatherSteps: weatherSteps, series: series, track: track,
		recent: ridesBefore(run.recent, ride.StartedAt), recentByID: run.metrics, matches: run.matches,
		routeNames: map[route.Key]string{}, indoorTypes: run.indoorTypes,
		day: run.load, loadLabel: run.loadLabel, loads: run.loads,
		location: run.location, at: run.at, earlier: earlier,
	}
	if weather, ok := run.weatherSums[ride.ID]; ok {
		b.weather = &weather
	}
	if match, ok := run.matches[ride.ID]; ok {
		b.match = &match
		if name, found, nameErr := a.store.RouteName(ctx, match.Key); nameErr == nil && found {
			b.routeName = name
			b.routeNames[match.Key] = name
		}
		line, elevations, found, profileErr := a.store.StageProfile(ctx, match.Key)
		if profileErr == nil && found && elevations != nil {
			b.climbs = RouteClimbs(line, elevations)
			attempts, attemptErr := a.store.RouteClimbAttempts(ctx, targetID, match.Key)
			if attemptErr == nil {
				b.thisAttempts = map[int]ClimbAttempt{}
				for _, attempt := range attempts {
					switch {
					case attempt.WorkoutID == ride.ID:
						b.thisAttempts[attempt.ClimbIndex] = attempt.ClimbAttempt
					case attempt.RiddenAt.Before(ride.StartedAt):
						b.otherAttempts = append(b.otherAttempts, attempt)
					}
				}
			}
		}
	}
	// Every distinct route among the recent rides, so recentRows can name one
	// without asking the store again per ride.
	for index := range run.recent {
		match, matched := run.matches[run.recent[index].ID]
		if !matched {
			continue
		}
		if _, known := b.routeNames[match.Key]; known {
			continue
		}
		if name, found, nameErr := a.store.RouteName(ctx, match.Key); nameErr == nil && found {
			b.routeNames[match.Key] = name
		}
	}

	return b, nil
}

// ridesBefore is the rides of a run's recent window that started before one
// instant: an older owed ride is read against what came before it, not after.
func ridesBefore(rides []Stored, before time.Time) []Stored {
	var kept []Stored
	for index := range rides {
		if rides[index].StartedAt.Before(before) {
			kept = append(kept, rides[index])
		}
	}

	return kept
}

// heldTypes is the indoor types a rider with Zwift credentials has held back
// for the Zwift copy; a rider without them has nothing held.
func (a *Analyser) heldTypes(ctx context.Context, subject string) ([]int, error) {
	email, password, err := a.store.RiderZwiftCredentials(ctx, subject)
	if err != nil {
		return nil, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	if len(email) == 0 || len(password) == 0 {
		return nil, nil
	}

	return a.indoorTypes, nil
}

// analysisContext is what every prompt of one run reads beside its ride.
type analysisContext struct {
	at      time.Time
	metrics map[int64]RideMetrics
	// load is the rider's training load at loadLabel's day, absent before any derived ride.
	load        *trainingload.Day
	location    *time.Location
	sessions    map[int64]Session
	matches     map[int64]RouteMatch
	weatherSums map[int64]WeatherSummary
	loadLabel   string
	recent      []Stored
	loads       []trainingload.RideLoad
	indoorTypes []int
	profile     rider.Profile
	powerCurve  rider.PowerCurve
}

func (a *Analyser) readContext(ctx context.Context, targetID, subject string, now time.Time) (analysisContext, error) {
	profile, err := a.store.RiderProfile(ctx, subject)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	metrics, err := a.store.ActivityMetrics(ctx, targetID)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	loads, err := a.store.ActivityRideLoads(ctx, targetID)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	sessions, err := a.store.ActivitySessions(ctx, targetID)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	matches, err := a.store.ActivityRouteMatches(ctx, targetID)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	weatherSums, err := a.store.ActivityWeatherSummaries(ctx, targetID)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	powerCurve, err := a.store.PowerCurve(ctx, []string{targetID}, time.Time{}, now)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	recent, err := a.store.ActivitiesBetween(ctx, targetID, now.Add(-recentRideHistoryDays*24*time.Hour), now, recentRideHistoryLimit)
	if err != nil {
		return analysisContext{}, err //nolint:wrapcheck // Reported as FailureState; the error itself is never shown.
	}
	location, err := time.LoadLocation(a.timezone())
	if err != nil {
		location = time.UTC
	}

	run := analysisContext{
		metrics: metrics, profile: profile, loadLabel: loadNow, powerCurve: powerCurve,
		sessions: sessions, matches: matches, weatherSums: weatherSums, loads: loads, recent: recent,
		location: location, at: now, indoorTypes: a.indoorTypes,
	}
	if days := trainingload.Timeline(loads, now, location); len(days) > 0 {
		run.load = &days[len(days)-1]
	}

	return run, nil
}

func (a *Analyser) analyseOne(
	ctx context.Context, targetID string, ride PendingAnalysis, run *analysisContext,
) Failure {
	metrics, derived := run.metrics[ride.ID]
	if !derived {
		return FailureState
	}
	earlier, err := a.store.AnalysesBefore(ctx, targetID, ride.StartedAt, analysisContextAnalyses)
	if err != nil {
		return FailureState
	}
	bundle, err := a.buildBundle(ctx, targetID, ride, run, &metrics, earlier)
	if err != nil {
		return FailureState
	}
	text, model, err := a.asker.Ask(ctx, bundle.compose())
	if err != nil {
		return a.asker.FailureOf(err)
	}
	if len(text) > MaximumDocumentBytes {
		return FailureUnusable
	}
	var document AnalysisDocument
	if err := json.Unmarshal([]byte(text), &document); err != nil {
		return FailureUnusable
	}
	if document.Summary == "" || utf8.RuneCountInString(document.Summary) > MaximumAnalysisCharacters ||
		!slices.Contains(rideTypes, document.RideType) {
		return FailureUnusable
	}
	if err := a.store.StoreActivityAnalysis(ctx, targetID, ride.ID, Analysis{
		AnalysedAt: a.now(), Text: document.Summary, Document: document, Model: model, PromptRevision: PromptRevision,
	}); err != nil {
		return FailureState
	}

	return FailureNone
}
