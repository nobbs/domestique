package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// derivedMetrics is a row with something in every part, so a round trip that
// drops one is visible.
func derivedMetrics(inputs trainingload.Inputs) activity.RideMetrics {
	return activity.RideMetrics{
		Load: trainingload.Metrics{
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
		},
		Averages: activity.RideAverages{
			HeartRateBPM: 142.5, MaxHeartRateBPM: 178, HasHeartRate: true,
			CadenceRPM: 81.5, HasCadence: true,
			PowerWatts: 196.25, HasPower: true,
		},
		HasEstimateQuality: true,
		EstimateQuality: measure.Quality{
			Autocorrelation1:           0.912,
			MeanAbsDeltaWattsPerSecond: 14.2,
			ClipBiasWatts:              2.6,
		},
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
	assert.Equal(t, stored.Load.Zones, read[1].Load.Zones)
	assert.InDelta(t, stored.Load.TRIMP, read[1].Load.TRIMP, 1e-9)
	assert.InDelta(t, stored.Load.HeartRateTSS, read[1].Load.HeartRateTSS, 1e-9)
	assert.InDelta(t, stored.Load.Power.NormalizedWatts, read[1].Load.Power.NormalizedWatts, 1e-9)
	assert.InDelta(t, stored.Load.EstimatedPowerWatts, read[1].Load.EstimatedPowerWatts, 1e-9)
	assert.True(t, read[1].Load.HasZones && read[1].Load.HasTRIMP && read[1].Load.HasHeartRateTSS && read[1].Load.HasPower)
	assert.True(t, read[1].Load.HasEstimatedPower, "the ride's average estimate")
	assert.Equal(t, stored.Averages, read[1].Averages, "and the plain sensor figures beside them")
	assert.InDelta(t, stored.EstimateQuality.Autocorrelation1, read[1].EstimateQuality.Autocorrelation1, 1e-9)
	assert.InDelta(t, stored.EstimateQuality.MeanAbsDeltaWattsPerSecond,
		read[1].EstimateQuality.MeanAbsDeltaWattsPerSecond, 1e-9)
	assert.InDelta(t, stored.EstimateQuality.ClipBiasWatts, read[1].EstimateQuality.ClipBiasWatts, 1e-9)
	// The two rates the zones were cut at come back, so the page can say what
	// each zone covered without reading the rider's current profile.
	assert.InDelta(t, stored.Load.Inputs.ThresholdHeartRateBPM,
		read[1].Load.Inputs.ThresholdHeartRateBPM, 1e-9)
	assert.InDelta(t, stored.Load.Inputs.MaxHeartRateBPM, read[1].Load.Inputs.MaxHeartRateBPM, 1e-9)
}

// A ride with heart rate but no meter keeps its zones and loses nothing to a
// column that was never filled.
func TestActivityMetricsKeepEachPartAbsentOnItsOwn(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs(), TRIMP: 30, HasTRIMP: true},
	}), "StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.True(t, read[1].Load.HasTRIMP)
	assert.False(t, read[1].Load.HasZones, "no zones were worked out")
	assert.False(t, read[1].Load.HasPower, "and no ride carried a meter")
	assert.False(t, read[1].Averages.HasCadence, "nor a cadence sensor")
	assert.False(t, read[1].Load.HasEstimatedPower, "nor an estimate")
	assert.False(t, read[1].HasEstimateQuality, "so its quality reads back absent, not zero as a value")
}

// A profile edit that takes a parameter away takes its numbers with it: a
// derivation that now yields nothing removes the row rather than leaving one
// nothing can tell from a fresh derivation.
func TestStoreActivityMetricsRemovesARowThatYieldsNothing(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics()")

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{}),
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

// A row an earlier derivation wrote cannot hold the figures this one produces,
// and NULL in a new column is indistinguishable from a sensor the ride never
// carried. Such a row is therefore owed a derivation again even though the
// profile behind it has not moved, and the next run fills it in.
func TestActivitiesAwaitingDerivationRelistsARowAnEarlierDerivationWrote(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics()")
	// Exactly what migration 043 leaves behind for every row written before it.
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE activity_metrics SET derivation_version = 0,
			average_heart_rate_bpm = NULL, max_heart_rate_bpm = NULL,
			average_cadence_rpm = NULL, average_power_watts = NULL`)
	require.NoError(t, err, "ageing the stored row")

	owed, listErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.NoError(t, listErr, "ActivitiesAwaitingDerivation()")
	assert.Equal(t, []int64{1}, owed, "the row predates the figures this derivation produces")
}

// The same relisting, a version later: a row version 2 wrote has no quality
// diagnostics either, exactly what migration 045 leaves behind.
func TestActivitiesAwaitingDerivationRelistsARowVersion2Wrote(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics()")
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE activity_metrics SET derivation_version = 2,
			estimate_autocorrelation = NULL, estimate_delta_watts_per_second = NULL,
			estimate_clip_bias_watts = NULL`)
	require.NoError(t, err, "ageing the stored row to version 2")

	owed, listErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.NoError(t, listErr, "ActivitiesAwaitingDerivation()")
	assert.Equal(t, []int64{1}, owed, "the row predates the estimate quality this derivation produces")

	// And the next run fills it in rather than leaving the columns null for good.
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"StoreActivityMetrics() on the next run")
	read, readErr := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, readErr, "ActivityMetrics()")
	assert.True(t, read[1].Averages.HasHeartRate)
	assert.InDelta(t, 142.5, read[1].Averages.HeartRateBPM, 1e-9)

	settled, settledErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.NoError(t, settledErr, "ActivitiesAwaitingDerivation() after the refill")
	assert.Empty(t, settled, "and the refilled row is owed nothing further")
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
				CadenceRPM: 88, HasCadence: true,
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
	require.Len(t, samples.Cadence, 1, "and the one that carried a cadence sensor")
	assert.InDelta(t, 88.0, samples.Cadence[0].Value, 1e-9)
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
		[]int64{0, 1}, []measure.Estimate{{}, {Watts: 214, Known: true}}),
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
		[]int64{0}, []measure.Estimate{{}, {}}), "an estimate per record or none")
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

// The whole of what the fitness timeline folds: one row per derived ride, with
// the moment it was ridden, oldest first.
func TestActivityRideLoadsCarryTheDayAndTheLoadOfEachRide(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2)
	withMeter := derivedMetrics(testInputs())
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, withMeter),
		"StoreActivityMetrics()")
	// A ride with a strap and no meter: its stress score is the heart-rate one.
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 2, activity.RideMetrics{
		Load: trainingload.Metrics{
			Inputs: testInputs(), TRIMP: 30, HasTRIMP: true,
			HeartRateTSS: 55, HasHeartRateTSS: true,
			Zones: trainingload.Zones{10, 20, 30, 40, 50}, HasZones: true,
		},
	}), "StoreActivityMetrics()")

	loads, err := store.ActivityRideLoads(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityRideLoads()")
	require.Len(t, loads, 2)
	assert.Equal(t, activityNow(), loads[0].At, "the day it was ridden")
	assert.InDelta(t, withMeter.Load.Power.TSS, loads[0].TSS, 1e-9, "power where the ride had a meter")
	assert.InDelta(t, 55.0, loads[1].TSS, 1e-9, "and heart rate where it did not")
	assert.InDelta(t, 30.0, loads[1].TRIMP, 1e-9)
	assert.Equal(t, trainingload.Zones{10, 20, 30, 40, 50}, loads[1].Zones)
}

// A ride nothing has derived is not in the fold: it has no load to contribute.
func TestActivityRideLoadsSkipARideWithNoDerivedRow(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	loads, err := store.ActivityRideLoads(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityRideLoads()")
	assert.Empty(t, loads)
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
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{}),
		"clearing the activity metrics")
	_, err = store.TargetOwner(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the target owner")
	_, err = store.ClearActivityMetrics(t.Context(), "rider-a")
	require.ErrorContains(t, err, "clearing the activity metrics")
	_, err = store.ActivityRideLoads(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the activity ride loads")
}

func TestActivityMetricsReadsNoQualityForAnEstimateDerivedBeforeItExisted(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs(), EstimatedPowerWatts: 150, HasEstimatedPower: true},
	}), "StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.True(t, read[1].Load.HasEstimatedPower)
	assert.False(t, read[1].HasEstimateQuality, "an estimate alone is not a quality")
}
