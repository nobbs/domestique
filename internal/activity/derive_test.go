package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDeriveStore is stored state as a derivation sees it, and a record of what
// it was asked to write.
type fakeDeriveStore struct {
	ownerErr         error
	profileErr       error
	owedErr          error
	samplesErr       error
	storeErr         error
	clearErr         error
	estimateErr      error
	rides            map[int64]activity.RideSamples
	written          map[int64]activity.RideMetrics
	estimated        map[int64][]measure.Estimate
	owner            string
	owed             []int64
	writeOrder       []int64
	estimatedRecords []int64
	profile          rider.Profile
	owedInputs       trainingload.Inputs
	cleared          int
	clearedRows      int
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

func (s *fakeDeriveStore) ActivityRideSamples(
	_ context.Context, _ string, id int64,
) (activity.RideSamples, error) {
	return s.rides[id], s.samplesErr
}

func (s *fakeDeriveStore) StoreEstimatedPower(
	_ context.Context, _ string, id int64, records []int64, estimates []measure.Estimate,
) error {
	if s.estimateErr != nil {
		return s.estimateErr
	}
	if s.estimated == nil {
		s.estimated = map[int64][]measure.Estimate{}
	}
	s.estimated[id] = estimates
	s.estimatedRecords = records

	return nil
}

func (s *fakeDeriveStore) ClearActivityMetrics(context.Context, string) (int, error) {
	s.cleared++

	return s.clearedRows, s.clearErr
}

//nolint:gocritic // value param: this method conforms to the activity.DeriveStore contract.
func (s *fakeDeriveStore) StoreActivityMetrics(
	_ context.Context, _ string, id int64, metrics activity.RideMetrics,
) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	if s.written == nil {
		s.written = map[int64]activity.RideMetrics{}
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
		rides: map[int64]activity.RideSamples{
			7: {HeartRate: heartRateRide(600, 150)},
			8: {HeartRate: heartRateRide(600, 160)},
		},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Polled, result.Outcome)
	assert.Equal(t, 2, result.Derived)
	assert.ElementsMatch(t, []int64{7, 8}, store.writeOrder)
	assert.True(t, store.written[7].Load.HasZones, "a maximum alone still cuts zones")
	assert.InDelta(t, 190.0, store.owedInputs.MaxHeartRateBPM, 1e-9,
		"the rides owed one are those against the profile as it stands")
}

// A rider who has entered nothing has nothing to derive from, and holds no
// stored row either. The rides wait for a profile rather than being written as
// rows of nothing.
func TestDeriveIsNotReadyWithoutAProfile(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{owner: "rider-a"}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.NotReady, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.Empty(t, store.written, "and no ride was written")
}

// Clearing the whole profile takes the ground from under every stored row at
// once. Leaving them would go on serving numbers worked out from parameters
// the rider has removed.
func TestDeriveClearsEveryRowWhenTheProfileIsCleared(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{owner: "rider-a", clearedRows: 12}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Polled, result.Outcome, "rows going is a change")
	assert.Equal(t, 12, result.Derived, "the rows it settled")
	assert.Equal(t, 1, store.cleared, "in one statement, not a ride at a time")
}

func TestDeriveReportsAStoreThatCannotClear(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a", clearErr: errors.New("unwritable")}, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureState, result.Failure)
}

// A slot nobody owns has no profile to derive against, which is a slot with
// nothing to do rather than a failure.
func TestDeriveLeavesAnUnownedTargetAlone(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{}, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Unchanged, deriver.Derive(t.Context(), "rider-a").Outcome)
}

func TestDeriveIsUnchangedWhenNoRideIsOwedOne(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a", profile: fullProfile()}, nil, nil, nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
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
			rides:    map[int64]activity.RideSamples{7: {HeartRate: heartRateRide(600, 150)}},
			storeErr: errors.New("unwritable"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			deriver, err := activity.NewDeriver(store, nil, nil, nil)
			require.NoError(t, err, "NewDeriver()")

			result := deriver.Derive(t.Context(), "rider-a")
			assert.Equal(t, activity.Failed, result.Outcome)
			assert.Equal(t, activity.FailureState, result.Failure)
		})
	}
}

// trackRide is a ride recorded once a second at a steady speed on the flat,
// carrying position, altitude and distance and no meter.
func trackRide(seconds int) activity.RideSamples {
	samples := activity.RideSamples{}
	base := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	for index := range seconds {
		samples.Track = append(samples.Track, measure.Sample{
			At:             base.Add(time.Duration(index) * time.Second),
			DistanceMetres: 7.5 * float64(index),
			AltitudeMetres: 100,
		})
		samples.TrackRecords = append(samples.TrackRecords, int64(index))
	}

	return samples
}

// The estimate is worked out beside the rest, and its average lands on the
// ride's row so a ride page can show it without reading every sample.
func TestDeriveEstimatesPowerForARideWithNoMeter(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner: "rider-a",
		profile: rider.Profile{
			MaxHeartRateBPM: rider.Set(190), RestingHeartRateBPM: rider.Set(48),
			RiderMassKG: rider.Set(74), BikeMassKG: rider.Set(8),
		},
		owed:  []int64{7},
		rides: map[int64]activity.RideSamples{7: trackRide(120)},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.True(t, store.written[7].Load.HasEstimatedPower, "the ride's average estimate")
	assert.Positive(t, store.written[7].Load.EstimatedPowerWatts)
	assert.Len(t, store.estimated[7], 120, "an entry per track sample")
	assert.Len(t, store.estimatedRecords, 120, "each naming the record it came from")
}

// The quality diagnostics stored on the row are exactly what EstimateSeries
// itself reports for the same track, not a value the deriver works out on its
// own by some other route.
func TestDeriveStoresTheEstimateQualityEstimateSeriesReports(t *testing.T) {
	t.Parallel()
	track := trackRide(120)
	wantEstimates, wantQuality, ok := measure.EstimateSeries(track.Track, 82)
	require.True(t, ok, "EstimateSeries()")
	require.NotEmpty(t, wantEstimates)
	store := &fakeDeriveStore{
		owner: "rider-a",
		profile: rider.Profile{
			MaxHeartRateBPM: rider.Set(190), RestingHeartRateBPM: rider.Set(48),
			RiderMassKG: rider.Set(74), BikeMassKG: rider.Set(8),
		},
		owed:  []int64{7},
		rides: map[int64]activity.RideSamples{7: track},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.Equal(t, wantQuality, store.written[7].EstimateQuality)
	assert.True(t, store.written[7].HasEstimateQuality)
}

// An estimate exists because there is no meter. Putting one beside a real
// reading only invites the two to be confused.
func TestDeriveEstimatesNoPowerForARideThatCarriesAMeter(t *testing.T) {
	t.Parallel()
	ride := trackRide(120)
	ride.Power = heartRateRide(120, 220)
	store := &fakeDeriveStore{
		owner: "rider-a",
		profile: rider.Profile{
			FunctionalThresholdPowerWatts: rider.Set(250),
			RiderMassKG:                   rider.Set(74), BikeMassKG: rider.Set(8),
		},
		owed:  []int64{7},
		rides: map[int64]activity.RideSamples{7: ride},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.False(t, store.written[7].Load.HasEstimatedPower, "the ride measured its own power")
	assert.Empty(t, store.estimated[7], "and the stored series is cleared rather than filled")
	assert.True(t, store.written[7].Load.HasPower, "the measured numbers are still worked out")
	assert.Zero(t, store.written[7].EstimateQuality, "no estimate means no quality to report either")
}

// A ride with no usable track is skipped rather than estimated as zero, and so
// is a rider who has entered only half a mass.
func TestDeriveEstimatesNoPowerWithoutATrackOrAMass(t *testing.T) {
	t.Parallel()
	for name, store := range map[string]*fakeDeriveStore{
		"no track": {
			owner: "rider-a",
			profile: rider.Profile{
				MaxHeartRateBPM: rider.Set(190), RiderMassKG: rider.Set(74), BikeMassKG: rider.Set(8),
			},
			owed:  []int64{7},
			rides: map[int64]activity.RideSamples{7: {HeartRate: heartRateRide(600, 150)}},
		},
		"only half a mass": {
			owner: "rider-a",
			profile: rider.Profile{
				MaxHeartRateBPM: rider.Set(190), RiderMassKG: rider.Set(74),
			},
			owed:  []int64{7},
			rides: map[int64]activity.RideSamples{7: trackRide(120)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			deriver, err := activity.NewDeriver(store, nil, nil, nil)
			require.NoError(t, err, "NewDeriver()")

			require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
			assert.False(t, store.written[7].Load.HasEstimatedPower)
			assert.Empty(t, store.estimated[7])
		})
	}
}

// A mass change makes every estimate stale, so it is one of the values a row
// records having been worked out against.
func TestDeriveRecordsTheMassItWorkedTheEstimateOutAgainst(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: rider.Profile{RiderMassKG: rider.Set(74), BikeMassKG: rider.Set(8)},
		owed:    []int64{7},
		rides:   map[int64]activity.RideSamples{7: trackRide(120)},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.InDelta(t, 82.0, store.owedInputs.TotalMassKG, 1e-9, "rider and bicycle together")
	assert.InDelta(t, 82.0, store.written[7].Load.Inputs.TotalMassKG, 1e-9)
}

// The metrics row is what says a ride has been derived, so a failure writing
// the series must not leave one behind claiming otherwise.
func TestDeriveWritesNoMetricsWhenTheEstimateCannotBeStored(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:       "rider-a",
		profile:     rider.Profile{RiderMassKG: rider.Set(74), BikeMassKG: rider.Set(8)},
		owed:        []int64{7},
		rides:       map[int64]activity.RideSamples{7: trackRide(120)},
		estimateErr: errors.New("unwritable"),
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Empty(t, store.written, "no row claims this ride was derived")
}

func TestNewDeriverNeedsAStore(t *testing.T) {
	t.Parallel()
	_, err := activity.NewDeriver(nil, nil, nil, nil)
	require.ErrorContains(t, err, "a store is required")
}

// varyingRide is a series that rises by one each second, so a mean and a peak
// that were swapped, or a peak taken as the last reading, are both visible.
func varyingRide(seconds int, first float64) []trainingload.Sample {
	start := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	samples := make([]trainingload.Sample, seconds)
	for index := range samples {
		samples[index] = trainingload.Sample{
			At:    start.Add(time.Duration(index) * time.Second),
			Value: first + float64(index),
		}
	}

	return samples
}

// The plain figures a rider reads before any training load, worked out from the
// samples already stored rather than asked of Wahoo again.
func TestDeriveWritesTheRidesSensorAverages(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7},
		rides: map[int64]activity.RideSamples{
			// 100..104, 60..64 and 200..204: mean is the middle, peak the last.
			7: {
				HeartRate: varyingRide(5, 100),
				Cadence:   varyingRide(5, 60),
				Power:     varyingRide(5, 200),
			},
		},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	averages := store.written[7].Averages
	assert.InDelta(t, 102.0, averages.HeartRateBPM, 1e-9)
	assert.InDelta(t, 104.0, averages.MaxHeartRateBPM, 1e-9, "the peak, not the last reading")
	assert.InDelta(t, 62.0, averages.CadenceRPM, 1e-9)
	assert.InDelta(t, 202.0, averages.PowerWatts, 1e-9)
	assert.True(t, averages.HasHeartRate && averages.HasCadence && averages.HasPower)
}

// The regression: a cadence of zero is the rider freewheeling, not pedalling at
// nought, and averaging it in drags the figure below what every other platform
// reports for the same ride. Measured power is the other way round — a
// freewheeling rider really is putting out nothing.
func TestDeriveAveragesCadenceOverThePedallingOnly(t *testing.T) {
	t.Parallel()
	coasted := append(varyingRide(5, 60), trainingload.Sample{
		At: time.Date(2026, 8, 24, 6, 0, 5, 0, time.UTC), Value: 0,
	})
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7},
		rides: map[int64]activity.RideSamples{
			7: {Cadence: coasted, Power: append(varyingRide(5, 200), trainingload.Sample{
				At: time.Date(2026, 8, 24, 6, 0, 5, 0, time.UTC), Value: 0,
			})},
		},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	averages := store.written[7].Averages
	assert.InDelta(t, 62.0, averages.CadenceRPM, 1e-9, "the coasting second is not pedalling")
	assert.InDelta(t, 1010.0/6, averages.PowerWatts, 1e-9, "but it is a reading of the meter")
}

// A cadence sensor that read nought for the whole ride recorded no cadence,
// which is not the same as a ride that averaged nought.
func TestDeriveLeavesCadenceAbsentWhereTheRiderNeverPedalled(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7},
		rides:   map[int64]activity.RideSamples{7: {Cadence: varyingRide(5, 0), HeartRate: varyingRide(5, 100)}},
	}
	// varyingRide(5, 0) rises 0..4, so only its first sample is a zero; a ride
	// that never turned the cranks is every sample at nought.
	for index := range store.rides[7].Cadence {
		store.rides[7].Cadence[index].Value = 0
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.False(t, store.written[7].Averages.HasCadence)
}

// A ride carries the sensors it carries: one with no strap has no heart rate
// rather than a heart rate of nought.
func TestDeriveLeavesAnAverageAbsentWhereTheRideCarriedNoSensor(t *testing.T) {
	t.Parallel()
	for name, samples := range map[string]activity.RideSamples{
		"no strap":  {Cadence: varyingRide(5, 60), Power: varyingRide(5, 200)},
		"no meter":  {HeartRate: varyingRide(5, 100), Cadence: varyingRide(5, 60)},
		"no sensor": {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := &fakeDeriveStore{
				owner:   "rider-a",
				profile: fullProfile(),
				owed:    []int64{7},
				rides:   map[int64]activity.RideSamples{7: samples},
			}
			deriver, err := activity.NewDeriver(store, nil, nil, nil)
			require.NoError(t, err, "NewDeriver()")
			deriver.Derive(t.Context(), "rider-a")

			averages := store.written[7].Averages
			assert.Equal(t, len(samples.HeartRate) > 0, averages.HasHeartRate)
			assert.Equal(t, len(samples.Cadence) > 0, averages.HasCadence)
			assert.Equal(t, len(samples.Power) > 0, averages.HasPower)
		})
	}
}

// A ride whose only sensor is one the load figures cannot use still has
// something to say, so its row is written rather than removed.
func TestDeriveWritesARideThatOnlyYieldsAnAverage(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7},
		rides:   map[int64]activity.RideSamples{7: {Cadence: varyingRide(5, 60)}},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	written := store.written[7]
	assert.False(t, written.Load.Derived(), "the load figures yielded nothing")
	assert.True(t, written.Derived(), "but the ride still averaged a cadence")
}
