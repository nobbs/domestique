package activity

import (
	"context"
	"errors"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

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
	// ActivitySensorSamples reads one ride's heart-rate and power series.
	ActivitySensorSamples(ctx context.Context, targetID string, id int64) (heartRate, power []trainingload.Sample, err error)
	StoreActivityMetrics(ctx context.Context, targetID string, id int64, metrics trainingload.Metrics) error
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
		heartRate, power, samplesErr := d.store.ActivitySensorSamples(ctx, targetID, id)
		if samplesErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Derived: derived}
		}
		if storeErr := d.store.StoreActivityMetrics(
			ctx, targetID, id, trainingload.Derive(heartRate, power, inputs),
		); storeErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Derived: derived}
		}
		derived++
	}
	if derived == 0 {
		return Result{Outcome: Unchanged}
	}

	return Result{Outcome: Polled, Derived: derived}
}
