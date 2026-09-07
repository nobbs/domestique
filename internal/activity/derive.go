package activity

import (
	"context"
	"errors"

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

// Deriver works out what each of a target's rides says about how hard it was.
type Deriver struct {
	store DeriveStore
}

// NewDeriver builds a deriver over stored state.
func NewDeriver(store DeriveStore) (*Deriver, error) {
	if store == nil {
		return nil, errors.New("activity: a store is required")
	}

	return &Deriver{store: store}, nil
}

// Derive works out every ride of one target that is owed a derivation, against
// the owner's profile as it stands now.
//
// A rider who has entered no profile is not a failure: there is nothing to work
// out yet, and the rides wait for the profile rather than being written as
// rows of nothing. A ride the derivation yields nothing for has its row
// removed, so a profile edit that takes a parameter away takes its numbers with it.
func (d *Deriver) Derive(ctx context.Context, targetID string) Result {
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
