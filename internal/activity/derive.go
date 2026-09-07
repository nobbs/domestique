package activity

import (
	"context"
	"errors"
	"time"

	"github.com/nobbs/domestique/internal/powerestimate"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

// RideSamples is one ride's recorded series, split by what each is for. Track
// is only the records that carried a position, an altitude and a distance;
// TrackRecords names which record each of them came from, so an estimate can be
// written back beside the sample it describes.
type RideSamples struct {
	HeartRate    []trainingload.Sample
	Power        []trainingload.Sample
	Track        []powerestimate.Sample
	TrackRecords []int64
}

// EstimatePower works out the ride's estimated power series and its average.
//
// A ride that already carries measured power yields none: an estimate exists
// because there is no meter, and putting one beside a real reading only invites
// the two to be confused. A ride with no track yields none either — it is
// skipped, never estimated as zero.
// The records and the estimates are returned together and are always the same
// length, so a caller cannot pair one ride's estimates with another's records.
func (s *RideSamples) EstimatePower(
	totalMassKG float64,
) (records []int64, estimates []powerestimate.Estimate, average powerestimate.Estimate) {
	if len(s.Power) > 0 {
		return nil, nil, powerestimate.Estimate{}
	}
	estimates, ok := powerestimate.Series(s.Track, totalMassKG)
	if !ok {
		return nil, nil, powerestimate.Estimate{}
	}
	mean, hasMean := powerestimate.Average(estimates)

	return s.TrackRecords, estimates, powerestimate.Estimate{Watts: mean, Known: hasMean}
}

// DeriveStore is what working out a ride's training numbers needs of stored
// state. Reads samples and a profile, writes metrics: no upstream is involved,
// which is why a derivation runs on a rider's own profile edit rather than
// waiting for a poll.
type DeriveStore interface {
	// TargetOwner is the subject whose target this is, empty for a slot that
	// predates ownership.
	TargetOwner(ctx context.Context, targetID string) (string, error)
	RiderProfile(ctx context.Context, subject string) (rider.Profile, error)
	// ActivitiesAwaitingDerivation lists the rides whose stored samples could
	// yield something these profile values allow: those never derived, and
	// those derived against different values.
	ActivitiesAwaitingDerivation(ctx context.Context, targetID string, inputs trainingload.Inputs) ([]int64, error)
	// ActivityRideSamples reads one ride's recorded series, split by what each
	// is for.
	ActivityRideSamples(ctx context.Context, targetID string, id int64) (RideSamples, error)
	StoreActivityMetrics(ctx context.Context, targetID string, id int64, metrics trainingload.Metrics) error
	// StoreEstimatedPower replaces one ride's estimated power series. An empty
	// series clears whatever was there.
	StoreEstimatedPower(ctx context.Context, targetID string, id int64,
		recordIndices []int64, estimates []powerestimate.Estimate) error
	// ClearActivityMetrics removes every derived row one target holds and
	// reports how many went.
	ClearActivityMetrics(ctx context.Context, targetID string) (int, error)
}

// Deriver works out what each of a target's rides says about how hard it was,
// and what it was ridden through.
type Deriver struct {
	store        DeriveStore
	weatherStore WeatherStore
	weather      WeatherSource
	now          func() time.Time
}

// NewDeriver builds a deriver over stored state.
//
// The weather source and its store are optional together: a build wired
// without them derives the training numbers and asks nobody about the weather,
// rather than refusing to derive at all.
func NewDeriver(store DeriveStore, weatherStore WeatherStore, weather WeatherSource, now func() time.Time) (*Deriver, error) {
	if store == nil {
		return nil, errors.New("activity: a store is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Deriver{store: store, weatherStore: weatherStore, weather: weather, now: now}, nil
}

// Derive settles what this service can work out about one target's rides: the
// training numbers, which follow the rider's profile and are worked out again
// whenever it changes, and the weather each ride was ridden through, which is
// asked of a provider once and never again.
//
// The two are independent — a rider who has entered no profile still rode
// through weather — so neither holds the other back, and the run reports
// whichever of them came to the more serious thing.
func (d *Deriver) Derive(ctx context.Context, targetID string) Result {
	metrics := d.deriveMetrics(ctx, targetID)
	weather := d.readWeather(ctx, targetID)
	if severityOf(weather.Outcome) > severityOf(metrics.Outcome) {
		return weather
	}
	// At equal severity the metrics pass is reported, having done the work a
	// derivation is named for.
	return metrics
}

// severityOf orders what a pass came to, worst highest, so a run over both
// reports the one an operator would act on first.
func severityOf(outcome Outcome) int {
	switch outcome {
	case Failed:
		return 3
	case Polled:
		return 2
	case Unchanged:
		return 1
	case NotReady:
	}

	return 0
}

// deriveMetrics works out every ride of one target that is owed a derivation,
// against the owner's profile as it stands now.
//
// A rider who has entered no profile is not a failure: there is nothing to work
// out yet, and the rides wait for the profile rather than being written as
// rows of nothing. A ride the derivation yields nothing for has its row
// removed, so a profile edit that takes a parameter away takes its numbers with it.
func (d *Deriver) deriveMetrics(ctx context.Context, targetID string) Result {
	subject, err := d.store.TargetOwner(ctx, targetID)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if subject == "" {
		return Result{Outcome: Unchanged}
	}
	profile, err := d.store.RiderProfile(ctx, subject)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	inputs := trainingload.InputsOf(&profile)
	// A rider who has cleared every parameter a derivation reads has taken the
	// ground from under every stored row at once, so those rows go rather than
	// going on being served. Cleared in one statement rather than a ride at a
	// time: there is nothing left to work out for any of them.
	if inputs == (trainingload.Inputs{}) {
		removed, clearErr := d.store.ClearActivityMetrics(ctx, targetID)
		if clearErr != nil {
			return Result{Outcome: Failed, Failure: FailureState}
		}
		if removed == 0 {
			return Result{Outcome: NotReady}
		}

		return Result{Outcome: Polled, Derived: removed}
	}
	ids, err := d.store.ActivitiesAwaitingDerivation(ctx, targetID, inputs)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	derived := 0
	for _, id := range ids {
		// A read or write that fails part way keeps what it already stored: the
		// rides left are still owed a derivation, and the next attempt finds them
		// exactly as this one did.
		samples, samplesErr := d.store.ActivityRideSamples(ctx, targetID, id)
		if samplesErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Derived: derived}
		}
		metrics := trainingload.Derive(samples.HeartRate, samples.Power, inputs)
		records, estimates, average := samples.EstimatePower(inputs.TotalMassKG)
		metrics.EstimatedPowerWatts, metrics.HasEstimatedPower = average.Watts, average.Known
		// The series first: a metrics row is what says a ride has been derived,
		// so it must not appear before the samples it describes are in place.
		if storeErr := d.store.StoreEstimatedPower(ctx, targetID, id, records, estimates); storeErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Derived: derived}
		}
		if storeErr := d.store.StoreActivityMetrics(ctx, targetID, id, metrics); storeErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Derived: derived}
		}
		derived++
	}
	if derived == 0 {
		return Result{Outcome: Unchanged}
	}

	return Result{Outcome: Polled, Derived: derived}
}
