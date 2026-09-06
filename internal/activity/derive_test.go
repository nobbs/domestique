package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDeriveStore is stored state as a derivation sees it, and a record of what
// it was asked to write.
type fakeDeriveStore struct {
	ownerErr   error
	profileErr error
	owedErr    error
	samplesErr error
	storeErr   error
	samples    map[int64][]trainingload.Sample
	written    map[int64]trainingload.Metrics
	owner      string
	owed       []int64
	writeOrder []int64
	profile    rider.Profile
	owedInputs trainingload.Inputs
}

func (s *fakeDeriveStore) TargetOwner(context.Context, string) (string, error) {
	return s.owner, s.ownerErr
}

func (s *fakeDeriveStore) RiderProfile(context.Context, string) (rider.Profile, error) {
	return s.profile, s.profileErr
}

func (s *fakeDeriveStore) ActivitiesAwaitingDerivation(
	_ context.Context, _ string, inputs trainingload.Inputs,
) ([]int64, error) {
	s.owedInputs = inputs

	return s.owed, s.owedErr
}

func (s *fakeDeriveStore) ActivitySensorSamples(
	_ context.Context, _ string, id int64,
) (heartRate, power []trainingload.Sample, err error) {
	return s.samples[id], nil, s.samplesErr
}

//nolint:gocritic // value param: this method conforms to the activity.DeriveStore contract.
func (s *fakeDeriveStore) StoreActivityMetrics(
	_ context.Context, _ string, id int64, metrics trainingload.Metrics,
) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	if s.written == nil {
		s.written = map[int64]trainingload.Metrics{}
	}
	s.written[id] = metrics
	s.writeOrder = append(s.writeOrder, id)

	return nil
}

func heartRateRide(seconds int, value float64) []trainingload.Sample {
	start := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	samples := make([]trainingload.Sample, seconds)
	for index := range samples {
		samples[index] = trainingload.Sample{At: start.Add(time.Duration(index) * time.Second), Value: value}
	}

	return samples
}

func fullProfile() rider.Profile {
	return rider.Profile{
		MaxHeartRateBPM:     rider.Set(190),
		RestingHeartRateBPM: rider.Set(48),
	}
}

func TestDeriveWritesEveryRideOwedOne(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7, 8},
		samples: map[int64][]trainingload.Sample{
			7: heartRateRide(600, 150),
			8: heartRateRide(600, 160),
		},
	}
	deriver, err := activity.NewDeriver(store)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Polled, result.Outcome)
	assert.Equal(t, 2, result.Derived)
	assert.ElementsMatch(t, []int64{7, 8}, store.writeOrder)
	assert.True(t, store.written[7].HasZones, "a maximum alone still cuts zones")
	assert.InDelta(t, 190.0, store.owedInputs.MaxHeartRateBPM, 1e-9,
		"the rides owed one are those against the profile as it stands")
}

// A rider who has entered nothing has nothing to derive from. The rides wait
// for a profile rather than being written as rows of nothing.
func TestDeriveIsNotReadyWithoutAProfile(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a"})
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.NotReady, deriver.Derive(t.Context(), "rider-a").Outcome)
}

// A slot nobody owns has no profile to derive against, which is a slot with
// nothing to do rather than a failure.
func TestDeriveLeavesAnUnownedTargetAlone(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{})
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Unchanged, deriver.Derive(t.Context(), "rider-a").Outcome)
}

func TestDeriveIsUnchangedWhenNoRideIsOwedOne(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a", profile: fullProfile()})
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Unchanged, deriver.Derive(t.Context(), "rider-a").Outcome)
}

// A read that fails part way keeps what it already stored: the rides left are
// still owed one, and the next attempt finds them exactly as this one did.
func TestDeriveKeepsWhatItStoredWhenAReadFails(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:      "rider-a",
		profile:    fullProfile(),
		owed:       []int64{7},
		samplesErr: errors.New("unreadable"),
	}
	deriver, err := activity.NewDeriver(store)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureState, result.Failure)
	assert.Empty(t, store.written)
}

func TestDeriveReportsAnUnreadableStore(t *testing.T) {
	t.Parallel()
	for name, store := range map[string]*fakeDeriveStore{
		"the owner cannot be read":   {ownerErr: errors.New("unreadable")},
		"the profile cannot be read": {owner: "rider-a", profileErr: errors.New("unreadable")},
		"the rides cannot be listed": {owner: "rider-a", profile: fullProfile(), owedErr: errors.New("unreadable")},
		"the metrics cannot be written": {
			owner: "rider-a", profile: fullProfile(), owed: []int64{7},
			samples:  map[int64][]trainingload.Sample{7: heartRateRide(600, 150)},
			storeErr: errors.New("unwritable"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			deriver, err := activity.NewDeriver(store)
			require.NoError(t, err, "NewDeriver()")

			result := deriver.Derive(t.Context(), "rider-a")
			assert.Equal(t, activity.Failed, result.Outcome)
			assert.Equal(t, activity.FailureState, result.Failure)
		})
	}
}

func TestNewDeriverNeedsAStore(t *testing.T) {
	t.Parallel()
	_, err := activity.NewDeriver(nil)
	require.ErrorContains(t, err, "a store is required")
}
