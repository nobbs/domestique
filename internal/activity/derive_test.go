package activity_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDeriveStore is stored state as a derivation sees it, and a record of what
// it was asked to write.
type fakeDeriveStore struct {
	series           map[int64][]activity.SampleRow
	seriesErr        error
	climbAttempts    map[int64][]activity.ClimbAttempt
	climbAttemptErr  error
	ownerErr         error
	profileErr       error
	owedErr          error
	samplesErr       error
	movingErr        error
	storeErr         error
	clearErr         error
	estimateErr      error
	libraryErr       error
	owedMatchesErr   error
	clearMatchesErr  error
	trackErr         error
	matchErr         error
	rides            map[int64]activity.RideSamples
	movingSeconds    map[int64]float64
	written          map[int64]activity.RideMetrics
	estimated        map[int64][]measure.Estimate
	tracks           map[int64][]activity.TrackPoint
	matches          map[int64]*activity.RouteMatch
	owner            string
	libraryHash      string
	matchedAgainst   string
	owed             []int64
	writeOrder       []int64
	estimatedRecords []int64
	library          []activity.RouteCandidate
	owedMatches      []int64
	matchWriteOrder  []int64
	profile          rider.Profile
	owedInputs       trainingload.Inputs
	cleared          int
	clearedRows      int
	clearedMatches   int
}

func (s *fakeDeriveStore) ClearActivityRouteMatches(context.Context, string) (int, error) {
	if s.clearMatchesErr != nil {
		return 0, s.clearMatchesErr
	}

	return s.clearedMatches, nil
}

func (s *fakeDeriveStore) LibraryRoutes(context.Context) ([]activity.RouteCandidate, string, error) {
	return s.library, s.libraryHash, s.libraryErr
}

// indoorWorkoutTypes are the same ids wahoo.IndoorWorkoutTypes() returns,
// written out here so the activity package's tests import no adapter.
func indoorWorkoutTypes() []int { return []int{12, 49, 61, 68} }

func (s *fakeDeriveStore) ActivitiesAwaitingRouteMatch(_ context.Context, _, libraryHash string, _ []int) ([]int64, error) {
	s.matchedAgainst = libraryHash

	return s.owedMatches, s.owedMatchesErr
}

func (s *fakeDeriveStore) ActivityTrack(_ context.Context, _ string, id int64) ([]activity.TrackPoint, error) {
	return s.tracks[id], s.trackErr
}

// ActivitySeries answers with whatever the test seeded for the ride, which for
// most of them is nothing: a ride with no series still matches a route.
func (s *fakeDeriveStore) ActivitySeries(
	_ context.Context, _ string, id int64,
) ([]activity.SampleRow, error) {
	return s.series[id], s.seriesErr
}

// The match and its attempts arrive together, as one write, because an attempt
// is read through the match it belongs to.
func (s *fakeDeriveStore) StoreActivityRouteMatch(
	_ context.Context, _ string, id int64,
	match *activity.RouteMatch, attempts []activity.ClimbAttempt, _ string, _ time.Time,
) error {
	if s.matchErr != nil {
		return s.matchErr
	}
	if s.climbAttemptErr != nil {
		return s.climbAttemptErr
	}
	if s.climbAttempts == nil {
		s.climbAttempts = map[int64][]activity.ClimbAttempt{}
	}
	s.climbAttempts[id] = attempts
	if s.matches == nil {
		s.matches = map[int64]*activity.RouteMatch{}
	}
	s.matches[id] = match
	s.matchWriteOrder = append(s.matchWriteOrder, id)

	return nil
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

func (s *fakeDeriveStore) ActivityMovingSeconds(
	_ context.Context, _ string, id int64,
) (float64, bool, error) {
	if s.movingErr != nil {
		return 0, false, s.movingErr
	}
	seconds, found := s.movingSeconds[id]

	return seconds, found, nil
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Polled, result.Outcome, "rows going is a change")
	assert.Equal(t, 12, result.Derived, "the rows it settled")
	assert.Equal(t, 1, store.cleared, "in one statement, not a ride at a time")
}

func TestDeriveReportsAStoreThatCannotClear(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a", clearErr: errors.New("unwritable")}, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureState, result.Failure)
}

// A slot nobody owns has no profile to derive against, which is a slot with
// nothing to do rather than a failure.
func TestDeriveLeavesAnUnownedTargetAlone(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{}, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Unchanged, deriver.Derive(t.Context(), "rider-a").Outcome)
}

func TestDeriveIsUnchangedWhenNoRideIsOwedOne(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a", profile: fullProfile()}, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
			deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.False(t, store.written[7].Load.HasEstimatedPower, "the ride measured its own power")
	assert.Empty(t, store.estimated[7], "and the stored series is cleared rather than filled")
	assert.True(t, store.written[7].Load.HasPower, "the measured numbers are still worked out")
	assert.False(t, store.written[7].HasEstimateQuality, "no estimate means no quality to report either")
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
			deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Empty(t, store.written, "no row claims this ride was derived")
}

// A heart-rate reading above the profile's own maximum is a sensor fault: it
// is interpolated across before any load is derived, exactly what
// measure.CapHeartRate itself would produce on the same series.
func TestDeriveCapsHeartRateAboveTheProfilesMaximumBeforeDerivingLoad(t *testing.T) {
	t.Parallel()
	spiked := heartRateRide(600, 150)
	spiked[300].Value = 250 // far past any plausible maximum
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(), // MaxHeartRateBPM 190, RestingHeartRateBPM 48
		owed:    []int64{7},
		rides:   map[int64]activity.RideSamples{7: {HeartRate: spiked}},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)

	capped := measure.CapHeartRate(spiked, 190)
	wantTRIMP, ok := trainingload.TRIMP(capped, 190, 48)
	require.True(t, ok)
	assert.InDelta(t, wantTRIMP, store.written[7].Load.TRIMP, 1e-9)

	uncappedTRIMP, ok := trainingload.TRIMP(spiked, 190, 48)
	require.True(t, ok)
	assert.NotEqual(t, uncappedTRIMP, store.written[7].Load.TRIMP,
		"the spike was interpolated across before load was derived, not left standing")
}

// A profile with no maximum has nothing to cap against, so CapHeartRate is a
// no-op and the load is worked out from the series exactly as recorded.
func TestDeriveLeavesHeartRateAloneWithoutAProfileMaximum(t *testing.T) {
	t.Parallel()
	spiked := heartRateRide(600, 150)
	spiked[300].Value = 250
	store := &fakeDeriveStore{
		owner: "rider-a",
		profile: rider.Profile{
			ThresholdHeartRateBPM: rider.Set(170), RestingHeartRateBPM: rider.Set(48),
		},
		owed:  []int64{7},
		rides: map[int64]activity.RideSamples{7: {HeartRate: spiked}},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)

	wantTSS, ok := trainingload.HeartRateTSS(spiked, 170, 48)
	require.True(t, ok)
	assert.InDelta(t, wantTSS, store.written[7].Load.HeartRateTSS, 1e-9,
		"a profile with no maximum leaves the series uncapped")
}

// A strap that held for a sixth of the ride's own moving time must not leave
// an understated TRIMP, stress score or zone table behind it unmarked — the
// same rule trainingload.Derive enforces, wired through the ride's own stored
// moving time rather than a value the caller made up.
func TestDeriveWithholdsHeartRateLoadBelowMinSeriesCoverage(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner: "rider-a",
		profile: rider.Profile{
			MaxHeartRateBPM: rider.Set(190), RestingHeartRateBPM: rider.Set(48),
			ThresholdHeartRateBPM: rider.Set(170),
		},
		owed:          []int64{7},
		rides:         map[int64]activity.RideSamples{7: {HeartRate: heartRateRide(600, 150)}},
		movingSeconds: map[int64]float64{7: 3600},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	require.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)

	load := store.written[7].Load
	assert.False(t, load.HasZones)
	assert.False(t, load.HasTRIMP)
	assert.False(t, load.HasHeartRateTSS)
}

func TestNewDeriverNeedsAStore(t *testing.T) {
	t.Parallel()
	_, err := activity.NewDeriver(nil, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	averages := store.written[7].Averages
	assert.InDelta(t, 102.0, averages.HeartRateBPM, 1e-9)
	assert.InDelta(t, 104.0, averages.MaxHeartRateBPM, 1e-9, "the peak, not the last reading")
	assert.InDelta(t, 62.0, averages.CadenceRPM, 1e-9)
	assert.InDelta(t, 202.0, averages.PowerWatts, 1e-9)
	assert.True(t, averages.HasHeartRate && averages.HasCadence && averages.HasPower)
}

// The peak of the ride's speed series is its maximum speed; the mean is
// worked out too but never surfaced, as RideAverages carries no average speed.
func TestDeriveWritesTheRidesMaxSpeed(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7},
		rides:   map[int64]activity.RideSamples{7: {Speed: varyingRide(5, 30)}},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	averages := store.written[7].Averages
	assert.True(t, averages.HasSpeed)
	assert.InDelta(t, 34.0, averages.MaxSpeedKmh, 1e-9, "the peak, not the mean")
}

// A ride with no speed series at all — no records, or none carrying a usable
// distance or device reading — has no maximum speed rather than one of nought.
func TestDeriveLeavesMaxSpeedAbsentWithoutASpeedSeries(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{
		owner:   "rider-a",
		profile: fullProfile(),
		owed:    []int64{7},
		rides:   map[int64]activity.RideSamples{7: {HeartRate: varyingRide(5, 100)}},
	}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	assert.False(t, store.written[7].Averages.HasSpeed)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
			deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
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
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.Polled, deriver.Derive(t.Context(), "rider-a").Outcome)
	written := store.written[7]
	assert.False(t, written.Load.Derived(), "the load figures yielded nothing")
	assert.True(t, written.Derived(), "but the ride still averaged a cadence")
}

// squareRoute is a 500 m square, and squareTrack is a ride round it. They are
// the same ground, so the ride covers the route entirely.
func squareRoute() []measure.Coordinate {
	const metresPerDegree = measure.EarthRadiusMetres * math.Pi / 180

	corners := [][2]float64{{0, 0}, {500, 0}, {500, 500}, {0, 500}, {0, 0}}
	points := []measure.Coordinate{}
	for index := 1; index < len(corners); index++ {
		start, end := corners[index-1], corners[index]
		for step := range 25 {
			ratio := float64(step) / 25
			east := start[0] + ratio*(end[0]-start[0])
			north := start[1] + ratio*(end[1]-start[1])
			points = append(points, measure.Coordinate{
				Latitude:  49.9 + north/metresPerDegree,
				Longitude: 8.2 + east/(metresPerDegree*math.Cos(49.9*math.Pi/180)),
			})
		}
	}

	return points
}

func squareTrack() []activity.TrackPoint {
	track := []activity.TrackPoint{}
	for _, point := range squareRoute() {
		track = append(track, activity.TrackPoint{Latitude: point.Latitude, Longitude: point.Longitude})
	}

	return track
}

func libraryStore(owed []int64, tracks map[int64][]activity.TrackPoint) *fakeDeriveStore {
	return &fakeDeriveStore{
		owner:       "rider-a",
		library:     []activity.RouteCandidate{{Key: route.NewKey(route.ProviderVeloPlanner, 4, 1), Geometry: squareRoute()}},
		libraryHash: "library-1",
		owedMatches: owed,
		tracks:      tracks,
	}
}

func TestDeriveMatchesARideToTheRouteItWasRiddenOn(t *testing.T) {
	t.Parallel()
	store := libraryStore([]int64{11}, map[int64][]activity.TrackPoint{11: squareTrack()})
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, 1, result.Matched)
	require.NotNil(t, store.matches[11], "the ride was ridden on the route")
	assert.Equal(t, route.NewKey(route.ProviderVeloPlanner, 4, 1), store.matches[11].Key)
	assert.InDelta(t, 1, store.matches[11].RouteCoverage, 0.02)
	assert.Equal(t, "library-1", store.matchedAgainst,
		"the rides owed a match are those measured against another library")
}

// A ride on no library route is recorded as being on none. Without that row it
// would be offered for matching again on every run for as long as it is stored.
func TestDeriveRecordsARideThatMatchedNoRoute(t *testing.T) {
	t.Parallel()
	elsewhere := []activity.TrackPoint{
		{Latitude: 52.5, Longitude: 13.4},
		{Latitude: 52.51, Longitude: 13.41},
		{Latitude: 52.52, Longitude: 13.42},
	}
	store := libraryStore([]int64{11}, map[int64][]activity.TrackPoint{11: elsewhere})
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, 1, result.Matched)
	require.Contains(t, store.matches, int64(11), "the answer was still recorded")
	assert.Nil(t, store.matches[11])
}

// An empty library is not an answer about any ride: recording every ride as
// having matched nothing would have to be undone by the first route stored.
func TestDeriveMatchesNothingAgainstAnEmptyLibrary(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{owner: "rider-a", owedMatches: []int64{11}}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Empty(t, store.matches)
	assert.Equal(t, activity.NotReady, result.Outcome, "the rides wait for a route")
}

// A library emptied after the fact is another matter: its matches name routes
// that are gone, and leaving them would have a ride point at a route no page
// can show.
func TestDeriveClearsTheMatchesAnEmptiedLibraryLeftBehind(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{owner: "rider-a", clearedMatches: 9}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Polled, result.Outcome)
	assert.Equal(t, 9, result.Matched, "the matches that named a route now gone")
}

func TestDeriveReportsAStoreThatCannotClearMatches(t *testing.T) {
	t.Parallel()
	store := &fakeDeriveStore{owner: "rider-a", clearMatchesErr: errors.New("unavailable")}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureState, result.Failure)
}

// The passes are independent: a rider with no profile still rode somewhere.
func TestDeriveMatchesRoutesForARiderWithNoProfile(t *testing.T) {
	t.Parallel()
	store := libraryStore([]int64{11}, map[int64][]activity.TrackPoint{11: squareTrack()})
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, 1, result.Matched)
	assert.Equal(t, 0, result.Derived)
}

func TestDeriveKeepsTheMatchesItStoredWhenATrackReadFails(t *testing.T) {
	t.Parallel()
	store := libraryStore([]int64{11}, nil)
	store.trackErr = errors.New("unavailable")
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureState, result.Failure)
	assert.Empty(t, store.matches)
}

func TestDeriveReportsALibraryItCannotRead(t *testing.T) {
	t.Parallel()
	store := libraryStore(nil, nil)
	store.libraryErr = errors.New("unavailable")
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureState, result.Failure)
}

// A run that matched rides has done something, whatever the metrics pass found.
func TestDeriveCountsBothWhatItDerivedAndWhatItMatched(t *testing.T) {
	t.Parallel()
	store := libraryStore([]int64{11}, map[int64][]activity.TrackPoint{11: squareTrack()})
	store.profile = fullProfile()
	store.owed = []int64{7}
	store.rides = map[int64]activity.RideSamples{7: {HeartRate: heartRateRide(600, 150)}}
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Polled, result.Outcome)
	assert.Equal(t, 1, result.Derived)
	assert.Equal(t, 1, result.Matched)
}

// The passes share a result but not a counter: whichever of them is reported,
// each says what it settled. Before this, a run reported for its weather told
// the caller nothing had been derived or matched.
func TestDeriveKeepsEachPassesCountWhicheverIsReported(t *testing.T) {
	t.Parallel()
	store := libraryStore([]int64{11}, map[int64][]activity.TrackPoint{11: squareTrack()})
	store.profile = fullProfile()
	store.owed = []int64{7}
	store.rides = map[int64]activity.RideSamples{7: {HeartRate: heartRateRide(600, 150)}}
	weather := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 8, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
	}
	deriver, err := activity.NewDeriver(store, weather, &fakeWeatherSource{}, indoorWorkoutTypes(), weatherNow)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, 1, result.Derived, "the metrics pass")
	assert.Equal(t, 1, result.Matched, "the route match pass")
	assert.Equal(t, 1, result.WeatherRead, "the weather pass")
}

// climbingRoute is a straight route rising steadily enough to hold one climb,
// so a ride down it can be timed.
func climbingRoute() (line []measure.Coordinate, elevations []float64) {
	for index := range 200 {
		metres := float64(index) * 20
		line = append(line, measure.Coordinate{Latitude: 0, Longitude: metres / 111_320})
		height := 100.0
		if metres > 1000 {
			height += min(metres-1000, 600) * 0.06
		}
		elevations = append(elevations, height)
	}

	return line, elevations
}

// climbingLibraryStore is one library route with height, and a ride down it.
func climbingLibraryStore() *fakeDeriveStore {
	line, elevations := climbingRoute()
	track := []activity.TrackPoint{}
	series := []activity.SampleRow{}
	at := time.Date(2026, 7, 4, 7, 0, 0, 0, time.UTC)
	for _, point := range line {
		track = append(track, activity.TrackPoint{
			Time: at, Latitude: point.Latitude, Longitude: point.Longitude,
		})
		series = append(series, activity.SampleRow{
			Time: at, HeartRateBPM: activity.Reading{Value: 158, Known: true},
		})
		at = at.Add(4 * time.Second)
	}

	return &fakeDeriveStore{
		owner: "rider-a",
		library: []activity.RouteCandidate{{
			Key: route.NewKey(route.ProviderVeloPlanner, 4, 1), Geometry: line, Elevations: elevations,
		}},
		libraryHash: "library-1",
		owedMatches: []int64{11},
		tracks:      map[int64][]activity.TrackPoint{11: track},
		series:      map[int64][]activity.SampleRow{11: series},
	}
}

func TestDeriveTimesAMatchedRideOverTheRoutesClimbs(t *testing.T) {
	t.Parallel()
	store := climbingLibraryStore()
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, 1, result.Matched)
	require.Len(t, store.climbAttempts[11], 1, "the route's one climb, ridden")
	assert.Equal(t, 0, store.climbAttempts[11][0].ClimbIndex)
	assert.Positive(t, store.climbAttempts[11][0].Seconds)
	assert.True(t, store.climbAttempts[11][0].HasHeartRate)
}

// A route whose stored geometry carries no height has no climbs to be timed
// over, which costs the ride its attempts and not its match.
func TestDeriveTimesNothingOverARouteWithNoHeight(t *testing.T) {
	t.Parallel()
	store := climbingLibraryStore()
	store.library[0].Elevations = nil
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, 1, result.Matched)
	require.NotNil(t, store.matches[11], "the ride still matched the route")
	assert.Empty(t, store.climbAttempts[11])
}

func TestDeriveReportsASeriesItCannotRead(t *testing.T) {
	t.Parallel()
	store := climbingLibraryStore()
	store.seriesErr = errors.New("the samples are unreadable")
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Failed, result.Outcome)
}

func TestDeriveReportsAStoreThatCannotKeepClimbAttempts(t *testing.T) {
	t.Parallel()
	store := climbingLibraryStore()
	store.climbAttemptErr = errors.New("the attempts cannot be stored")
	deriver, err := activity.NewDeriver(store, nil, nil, indoorWorkoutTypes(), nil)
	require.NoError(t, err, "NewDeriver()")

	result := deriver.Derive(t.Context(), "rider-a")

	assert.Equal(t, activity.Failed, result.Outcome)
}

// The indoor types are the guard that keeps a virtual world's coordinates out
// of a forecast and a route match; an empty list would ask a query to name no
// type at all, so a deriver refuses to be built without them.
func TestNewDeriverRefusesAnEmptyIndoorTypeList(t *testing.T) {
	t.Parallel()

	_, err := activity.NewDeriver(&fakeDeriveStore{}, nil, nil, nil, nil)
	require.ErrorContains(t, err, "indoor workout types are required")
}
