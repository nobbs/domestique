package activity

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

const (
	// PromptRevision names the prompt below; an analysis records the one it
	// was asked with.
	PromptRevision = 2

	// MaximumAnalysisCharacters is the contract's bound on a stored answer.
	MaximumAnalysisCharacters = 2000

	// analysisRidesPerRun bounds how long one run holds the activities.
	analysisRidesPerRun = 5
	// analysisContextAnalyses is how many earlier analyses a prompt carries.
	analysisContextAnalyses = 5
	// trainerCopyHold is one zwift:poll interval and an hour past a ride's end:
	// a head unit's indoor ride inside it may still be replaced by the Zwift copy.
	trainerCopyHold = 7 * time.Hour
)

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

// Analysis is what a language model made of one ride, with what produced it.
type Analysis struct {
	AnalysedAt     time.Time
	Text           string
	Model          string
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
	metrics map[int64]RideMetrics
	// load is the rider's training load at loadLabel's day, absent before any derived ride.
	load      *trainingload.Day
	loadLabel string
	profile   rider.Profile
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
	location, err := time.LoadLocation(a.timezone())
	if err != nil {
		location = time.UTC
	}

	run := analysisContext{metrics: metrics, profile: profile, loadLabel: loadNow}
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
	text, model, err := a.asker.Ask(ctx, composePrompt(&run.profile, &metrics, run.load, run.loadLabel, earlier))
	if err != nil {
		return a.asker.FailureOf(err)
	}
	if text == "" || utf8.RuneCountInString(text) > MaximumAnalysisCharacters {
		return FailureUnusable
	}
	if err := a.store.StoreActivityAnalysis(ctx, targetID, ride.ID, Analysis{
		AnalysedAt: a.now(), Text: text, Model: model, PromptRevision: PromptRevision,
	}); err != nil {
		return FailureState
	}

	return FailureNone
}
