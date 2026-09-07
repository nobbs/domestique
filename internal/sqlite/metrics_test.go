package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/powerestimate"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// derivedMetrics is a row with something in every part, so a round trip that
// drops one is visible.
func derivedMetrics(inputs trainingload.Inputs) trainingload.Metrics {
	return trainingload.Metrics{
		Inputs:              inputs,
		Zones:               trainingload.Zones{60, 120, 180, 240, 300},
		HasZones:            true,
		TRIMP:               42.5,
		HasTRIMP:            true,
		HeartRateTSS:        88.25,
		HasHeartRateTSS:     true,
		Power:               trainingload.Power{NormalizedWatts: 214, IntensityFactor: 0.856, TSS: 73.3},
		HasPower:            true,
		EstimatedPowerWatts: 168.5,
		HasEstimatedPower:   true,
	}
}

func testInputs() trainingload.Inputs {
	return trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 48,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
		TotalMassKG: 82,
	}
}

func metricsStore(t *testing.T, ids ...int64) *Store {
	t.Helper()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	for _, id := range ids {
		require.NoError(t, storeTestActivity(t, store, "rider-a", id, 100), "StoreActivity()")
	}

	return store
}

func TestActivityMetricsRoundTrip(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	stored := derivedMetrics(testInputs())

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, stored),
		"StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	require.Contains(t, read, int64(1))
	assert.Equal(t, stored.Zones, read[1].Zones)
	assert.InDelta(t, stored.TRIMP, read[1].TRIMP, 1e-9)
	assert.InDelta(t, stored.HeartRateTSS, read[1].HeartRateTSS, 1e-9)
	assert.InDelta(t, stored.Power.NormalizedWatts, read[1].Power.NormalizedWatts, 1e-9)
	assert.InDelta(t, stored.EstimatedPowerWatts, read[1].EstimatedPowerWatts, 1e-9)
	assert.True(t, read[1].HasZones && read[1].HasTRIMP && read[1].HasHeartRateTSS && read[1].HasPower)
	assert.True(t, read[1].HasEstimatedPower, "the ride's average estimate")
}

// A ride with heart rate but no meter keeps its zones and loses nothing to a
// column that was never filled.
func TestActivityMetricsKeepEachPartAbsentOnItsOwn(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, trainingload.Metrics{
		Inputs: testInputs(), TRIMP: 30, HasTRIMP: true,
	}), "StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.True(t, read[1].HasTRIMP)
	assert.False(t, read[1].HasZones, "no zones were worked out")
	assert.False(t, read[1].HasPower, "and no ride carried a meter")
}

// A profile edit that takes a parameter away takes its numbers with it: a
// derivation that now yields nothing removes the row rather than leaving one
// nothing can tell from a fresh derivation.
func TestStoreActivityMetricsRemovesARowThatYieldsNothing(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics()")

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, trainingload.Metrics{}),
		"StoreActivityMetrics() with nothing derived")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.NotContains(t, read, int64(1))
}

// The acceptance criterion behind the recompute: a row worked out against the
// profile as it stands is left alone, and one worked out against anything else
// is owed a derivation again.
func TestActivitiesAwaitingDerivationFindsTheStaleAndTheUnderived(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2, 3)
	for _, id := range []int64{1, 2, 3} {
		require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", id, activity.FIT{
			Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
		}), "StoreActivityRecords()")
	}
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics()")
	stale := testInputs()
	stale.FunctionalThresholdPowerWatts = 200
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 2, derivedMetrics(stale)),
		"StoreActivityMetrics() against another profile")

	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.NoError(t, err, "ActivitiesAwaitingDerivation()")
	assert.ElementsMatch(t, []int64{2, 3}, owed,
		"the row against the old profile and the ride never derived, and no other")
}

// A ride still waiting for its FIT has nothing to derive from, so it waits for
// the download rather than being derived into an empty row.
func TestActivitiesAwaitingDerivationSkipsARideWithNoStoredRecords(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.NoError(t, err, "ActivitiesAwaitingDerivation()")
	assert.Empty(t, owed)
}

func TestActivityRideSamplesSplitTheSeriesAndLeaveOutTheAbsent(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{
				Time: activityNow(), HeartRateBPM: 140, HasHeartRate: true, PowerWatts: 200, HasPower: true,
				Latitude: 49, Longitude: 8, HasPosition: true,
				AltitudeMetres: 100, HasAltitude: true, DistanceMetres: 0, HasDistance: true,
			},
			{
				Time: activityNow().Add(time.Second), HeartRateBPM: 142, HasHeartRate: true,
				Latitude: 49.001, Longitude: 8, HasPosition: true,
				AltitudeMetres: 101, HasAltitude: true, DistanceMetres: 7.5, HasDistance: true,
			},
			// No sensor and no position: in no series at all.
			{Time: activityNow().Add(2 * time.Second)},
		},
	}), "StoreActivityRecords()")

	// A latitude without a longitude, which the FIT decoder cannot produce but a
	// hand-edited database can. The track endpoint would never serve such a
	// sample, so it must not shape an estimate either.
	_, execErr := store.database.ExecContext(t.Context(),
		`INSERT INTO activity_records (target_slot, workout_id, record_index, recorded_at_unix,
		  distance_metres, latitude, altitude_metres) VALUES ('rider-a', 1, 3, ?, 15, 49.002, 102)`,
		activityNow().Add(3*time.Second).Unix())
	require.NoError(t, execErr, "inserting a half-positioned record")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.HeartRate, 2, "the two records that carried a strap")
	require.Len(t, samples.Power, 1, "and the one that carried a meter")
	require.Len(t, samples.Track, 2, "and the two that carried a whole positioned sample")
	assert.InDelta(t, 140.0, samples.HeartRate[0].Value, 1e-9)
	assert.Equal(t, activityNow(), samples.HeartRate[0].At)
	assert.Equal(t, []int64{0, 1}, samples.TrackRecords, "each naming the record it came from")
}

// The series is written beside the samples it describes and read back on the
// track, under its own name.
func TestStoreEstimatedPowerWritesTheSeriesAndClearsItAgain(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{
				Time: activityNow(), Latitude: 49, Longitude: 8, HasPosition: true,
				AltitudeMetres: 100, HasAltitude: true,
			},
			{
				Time: activityNow().Add(time.Second), Latitude: 49.001, Longitude: 8, HasPosition: true,
				AltitudeMetres: 101, HasAltitude: true,
			},
		},
	}), "StoreActivityRecords()")

	require.NoError(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1,
		[]int64{0, 1}, []powerestimate.Estimate{{}, {Watts: 214, Known: true}}),
		"StoreEstimatedPower()")

	track, err := store.ActivityTrack(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityTrack()")
	require.Len(t, track, 2)
	assert.False(t, track[0].HasEstimatedPower, "the first sample had no step behind it")
	require.True(t, track[1].HasEstimatedPower)
	assert.InDelta(t, 214.0, track[1].EstimatedPowerWatts, 1e-9)

	// A ride that has stopped yielding an estimate keeps none from the mass the
	// rider has since changed.
	require.NoError(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1, nil, nil),
		"StoreEstimatedPower() with nothing to store")
	track, err = store.ActivityTrack(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityTrack() again")
	assert.False(t, track[1].HasEstimatedPower)
}

func TestStoreEstimatedPowerRefusesMismatchedSeries(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.ErrorContains(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1,
		[]int64{0}, []powerestimate.Estimate{{}, {}}), "an estimate per record or none")
}

// A mass change makes every estimate stale, so a row worked out against another
// mass is owed a derivation again.
func TestActivitiesAwaitingDerivationNoticesAMassChange(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics()")

	unchanged, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.NoError(t, err, "ActivitiesAwaitingDerivation()")
	assert.Empty(t, unchanged, "nothing changed")

	heavier := testInputs()
	heavier.TotalMassKG = 84
	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", heavier)
	require.NoError(t, err, "ActivitiesAwaitingDerivation() after a mass change")
	assert.Equal(t, []int64{1}, owed)
}

func TestTargetOwnerIsEmptyForASlotThisDeploymentDoesNotHave(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)

	owner, err := store.TargetOwner(t.Context(), "rider-a")
	require.NoError(t, err, "TargetOwner()")
	assert.Equal(t, "rider-a", owner)

	missing, err := store.TargetOwner(t.Context(), "nobody")
	require.NoError(t, err, "TargetOwner() for an unknown slot")
	assert.Empty(t, missing, "an unknown slot is not a failure")
}

// A rider clearing their whole profile takes every stored row with it, in one
// statement rather than a ride at a time.
func TestClearActivityMetricsRemovesEveryRowAndCountsThem(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2)
	for _, id := range []int64{1, 2} {
		require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", id, derivedMetrics(testInputs())),
			"StoreActivityMetrics()")
	}

	removed, err := store.ClearActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ClearActivityMetrics()")
	assert.Equal(t, 2, removed)

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.Empty(t, read)

	again, err := store.ClearActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ClearActivityMetrics() again")
	assert.Zero(t, again, "a rider who never had a profile is not a rider who cleared one")
}

func TestActivityMetricsReportAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the activity metrics")
	_, err = store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.ErrorContains(t, err, "listing activities awaiting derivation")
	_, err = store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.ErrorContains(t, err, "reading the recorded samples")
	require.ErrorContains(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1, nil, nil),
		"starting the estimated power write")
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"storing the activity metrics")
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, trainingload.Metrics{}),
		"clearing the activity metrics")
	_, err = store.TargetOwner(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the target owner")
	_, err = store.ClearActivityMetrics(t.Context(), "rider-a")
	require.ErrorContains(t, err, "clearing the activity metrics")
}
