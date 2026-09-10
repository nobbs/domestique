package activity

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

// RideSamples is one ride's recorded series, split by what each is for. Track
// is only the records that carried a position, an altitude and a distance;
// TrackRecords names which record each of them came from, so an estimate can be
// written back beside the sample it describes.
type RideSamples struct {
	HeartRate []trainingload.Sample
	Cadence   []trainingload.Sample
	Power     []trainingload.Sample
	// Temperature is every sample that carried one, positioned or not, so a
	// reading can be paired with the heart rate recorded at the same second.
	Temperature []trainingload.Sample
	// Speed is km/h, already capped at measure.MaxPlausibleSpeedKmh by the
	// reader that built it.
	Speed        []trainingload.Sample
	Track        []measure.Sample
	TrackRecords []int64
}

// EstimatePower works out the ride's estimated power series and its
// pedalling mean and share.
//
// A ride that already carries measured power yields none: an estimate exists
// because there is no meter, and putting one beside a real reading only invites
// the two to be confused. A ride with no track yields none either — it is
// skipped, never estimated as zero.
// The records and the estimates are returned together and are always the same
// length, so a caller cannot pair one ride's estimates with another's records.
func (s *RideSamples) EstimatePower(
	totalMassKG float64, coefficients measure.Coefficients,
) (records []int64, estimates []measure.Estimate, watts, share float64, ok bool) {
	if len(s.Power) > 0 {
		return nil, nil, 0, 0, false
	}
	estimates, ok = measure.EstimateSeries(s.Track, totalMassKG, coefficients)
	if !ok {
		return nil, nil, 0, 0, false
	}
	watts, share, ok = measure.PedallingMean(s.Track, estimates)

	return s.TrackRecords, estimates, watts, share, ok
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
	// Rides an earlier derivation wrote are listed too: its row cannot hold
	// every figure this one produces.
	ActivitiesAwaitingDerivation(
		ctx context.Context, targetID string, inputs trainingload.Inputs, coefficients measure.Coefficients,
	) ([]int64, error)
	// ActivityRideSamples reads one ride's recorded series, split by what each
	// is for.
	ActivityRideSamples(ctx context.Context, targetID string, id int64) (RideSamples, error)
	// ActivityMovingSeconds is one ride's own moving time, which a load
	// figure's series coverage is judged against.
	ActivityMovingSeconds(ctx context.Context, targetID string, id int64) (movingSeconds float64, found bool, err error)
	StoreActivityMetrics(ctx context.Context, targetID string, id int64, metrics RideMetrics) error
	// StoreEstimatedPower replaces one ride's estimated power series. An empty
	// series clears whatever was there.
	StoreEstimatedPower(ctx context.Context, targetID string, id int64,
		recordIndices []int64, estimates []measure.Estimate) error
	// ClearActivityMetrics removes every derived row one target holds and
	// reports how many went.
	ClearActivityMetrics(ctx context.Context, targetID string) (int, error)
	RouteMatchStore
}

// Deriver works out what each of a target's rides says about how hard it was,
// and what it was ridden through.
type Deriver struct {
	store        DeriveStore
	weatherStore WeatherStore
	weather      WeatherSource
	now          func() time.Time
	// indoorTypes are the workout types ridden over no ground, which are asked
	// nothing about the weather and attributed to no route.
	indoorTypes []int
}

// NewDeriver builds a deriver over stored state.
//
// The weather source and its store are optional together: a build wired
// without them derives the training numbers and asks nobody about the weather,
// rather than refusing to derive at all. indoorTypes is not optional: an empty
// list would ask a query to name no type at all, and the guard it carries is
// what keeps a virtual world's coordinates out of a forecast and a route match.
func NewDeriver(
	store DeriveStore, weatherStore WeatherStore, weather WeatherSource,
	indoorTypes []int, now func() time.Time,
) (*Deriver, error) {
	if store == nil {
		return nil, errors.New("activity: a store is required")
	}
	if len(indoorTypes) == 0 {
		return nil, errors.New("activity: the indoor workout types are required")
	}
	if now == nil {
		now = time.Now
	}

	return &Deriver{
		store: store, weatherStore: weatherStore, weather: weather,
		indoorTypes: slices.Clone(indoorTypes), now: now,
	}, nil
}

// Derive settles what this service can work out about one target's rides: the
// training numbers, which follow the rider's profile and are worked out again
// whenever it changes; the weather each ride was ridden through, which is asked
// of a provider once and never again; and which library route each was ridden
// on, which follows the library's geometry.
//
// The three are independent — a rider who has entered no profile still rode
// through weather, and still rode somewhere — so none holds the others back,
// and the run reports whichever of them came to the more serious thing.
func (d *Deriver) Derive(ctx context.Context, targetID string) Result {
	metrics := d.deriveMetrics(ctx, targetID)
	weather := d.readWeather(ctx, targetID)
	matches := d.matchRoutes(ctx, targetID)

	// At equal severity the metrics pass is reported, having done the work a
	// derivation is named for.
	worst := metrics
	for _, pass := range []Result{weather, matches} {
		if severityOf(pass.Outcome) > severityOf(worst.Outcome) {
			worst = pass
		}
	}
	// Each pass keeps its own count whichever of them is reported, so a run that
	// matched rides does not read as having done nothing because the profile it
	// works from has not changed.
	worst.Derived = metrics.Derived
	worst.WeatherRead = weather.WeatherRead
	worst.Matched = matches.Matched

	return worst
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

// bicycleOf is the coefficients a ride is estimated at. The road bicycle on
// the hoods is what a rider who has entered no bicycle is estimated at.
func bicycleOf(profile *rider.Profile) measure.Coefficients {
	if profile.DragAreaM2.Set && profile.RollingResistance.Set {
		return measure.Coefficients{DragArea: profile.DragAreaM2.Number, RollingResistance: profile.RollingResistance.Number}
	}

	return measure.DefaultCoefficients()
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
	coefficients := bicycleOf(&profile)
	ids, err := d.store.ActivitiesAwaitingDerivation(ctx, targetID, inputs, coefficients)
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
		// Not found leaves it at zero, which a coverage share judged against it
		// reads as unmeasured rather than failed — the same as any other ride
		// whose moving time this derivation cannot yet supply.
		movingSeconds, _, movingErr := d.store.ActivityMovingSeconds(ctx, targetID, id)
		if movingErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Derived: derived}
		}
		heartRate := measure.CapHeartRate(samples.HeartRate, inputs.MaxHeartRateBPM)
		load := trainingload.Derive(heartRate, samples.Power, movingSeconds, inputs)
		records, estimates, watts, share, estimated := samples.EstimatePower(inputs.TotalMassKG, coefficients)
		load.EstimatedPowerWatts, load.HasEstimatedPower = watts, estimated
		metrics := RideMetrics{
			Load:                       load,
			Averages:                   samples.Averages(),
			Decoupling:                 samples.Decoupling(heartRate),
			HeatDrift:                  samples.HeatDrift(heartRate, inputs.FunctionalThresholdPowerWatts),
			PowerBests:                 samples.PowerBests(),
			EstimatedPedallingShare:    share,
			HasEstimatedPedallingShare: estimated,
			Coefficients:               coefficients,
		}
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
