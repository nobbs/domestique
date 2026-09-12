package sqlite

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// derivedMetrics is a row with something in every part, so a round trip that
// drops one is visible.
func derivedMetrics(inputs trainingload.Inputs, coefficients measure.Coefficients) activity.RideMetrics {
	return activity.RideMetrics{
		Load: trainingload.Metrics{
			Inputs:               inputs,
			Zones:                trainingload.Zones{60, 120, 180, 240, 300},
			HasZones:             true,
			TRIMP:                42.5,
			HasTRIMP:             true,
			HeartRateTSS:         88.25,
			HasHeartRateTSS:      true,
			Power:                trainingload.Power{NormalizedWatts: 214, IntensityFactor: 0.856, TSS: 73.3},
			HasPower:             true,
			EstimatedPowerWatts:  168.5,
			HasEstimatedPower:    true,
			HeartRateCoverage:    0.97,
			HasHeartRateCoverage: true,
			PowerCoverage:        0.94,
			HasPowerCoverage:     true,
		},
		Averages: activity.RideAverages{
			HeartRateBPM: 142.5, MaxHeartRateBPM: 178, HasHeartRate: true,
			CadenceRPM: 81.5, HasCadence: true,
			PowerWatts: 196.25, HasPower: true,
			MaxSpeedKmh: 47.3, HasSpeed: true,
		},
		EstimatedPedallingShare:    0.83,
		HasEstimatedPedallingShare: true,
		Coefficients:               coefficients,
	}
}

func testInputs() trainingload.Inputs {
	return trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 48,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
		TotalMassKG: 82,
	}
}

func testCoefficients() measure.Coefficients {
	return measure.Coefficients{DragArea: 0.38, RollingResistance: 0.007}
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
	stored := derivedMetrics(testInputs(), testCoefficients())

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
	require.True(t, read[1].Load.HasHeartRateCoverage && read[1].Load.HasPowerCoverage)
	assert.InDelta(t, stored.Load.HeartRateCoverage, read[1].Load.HeartRateCoverage, 1e-9)
	assert.InDelta(t, stored.Load.PowerCoverage, read[1].Load.PowerCoverage, 1e-9)
	assert.Equal(t, stored.Averages, read[1].Averages, "and the plain sensor figures beside them")
	assert.InDelta(t, stored.EstimatedPedallingShare, read[1].EstimatedPedallingShare, 1e-9)
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
	assert.Zero(t, read[1].EstimatedPedallingShare, "no estimate, no share")
	assert.False(t, read[1].Load.HasHeartRateCoverage || read[1].Load.HasPowerCoverage, "nor a coverage share for either series")
}

// A row written before migration 057 holds an estimate but no share: it must
// come back as no share rather than a false zero.
func TestActivityMetricsPreMigration057RowHasNoShare(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.queries.UpsertActivityMetrics(t.Context(), sqlcgen.UpsertActivityMetricsParams{
		TargetSlot: "rider-a", WorkoutID: 1,
		EstimatedPowerWatts:     sql.NullFloat64{Float64: 168.5, Valid: true},
		EstimatedPedallingShare: sql.NullFloat64{Valid: false},
		InputDragArea:           testCoefficients().DragArea,
		InputRollingResistance:  testCoefficients().RollingResistance,
	}), "UpsertActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	require.Contains(t, read, int64(1))
	assert.True(t, read[1].Load.HasEstimatedPower)
	assert.False(t, read[1].HasEstimatedPedallingShare, "no share was ever written")
}

// A profile edit that takes a parameter away takes its numbers with it: a
// derivation that now yields nothing removes the row rather than leaving one
// nothing can tell from a fresh derivation.
func TestStoreActivityMetricsRemovesARowThatYieldsNothing(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
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
		}, activity.RecordsVersion), "StoreActivityRecords()")
	}
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics()")
	stale := testInputs()
	stale.FunctionalThresholdPowerWatts = 200
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 2, derivedMetrics(stale, testCoefficients())),
		"StoreActivityMetrics() against another profile")

	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
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
	}, activity.RecordsVersion), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics()")
	// Exactly what migration 043 leaves behind for every row written before it.
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE activity_metrics SET derivation_version = 0,
			average_heart_rate_bpm = NULL, max_heart_rate_bpm = NULL,
			average_cadence_rpm = NULL, average_power_watts = NULL`)
	require.NoError(t, err, "ageing the stored row")

	owed, listErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.NoError(t, listErr, "ActivitiesAwaitingDerivation()")
	assert.Equal(t, []int64{1}, owed, "the row predates the figures this derivation produces")
}

// The same relisting, a version later: a row an older version wrote is owed a
// derivation again purely because its version differs from this one's.
func TestActivitiesAwaitingDerivationRelistsARowVersion2Wrote(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}, activity.RecordsVersion), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics()")
	_, err := store.database.ExecContext(t.Context(), `UPDATE activity_metrics SET derivation_version = 2`)
	require.NoError(t, err, "ageing the stored row to version 2")

	owed, listErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.NoError(t, listErr, "ActivitiesAwaitingDerivation()")
	assert.Equal(t, []int64{1}, owed, "the row predates what this derivation produces")

	// And the next run fills it in rather than leaving the columns null for good.
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics() on the next run")
	read, readErr := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, readErr, "ActivityMetrics()")
	assert.True(t, read[1].Averages.HasHeartRate)
	assert.InDelta(t, 142.5, read[1].Averages.HeartRateBPM, 1e-9)

	settled, settledErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.NoError(t, settledErr, "ActivitiesAwaitingDerivation() after the refill")
	assert.Empty(t, settled, "and the refilled row is owed nothing further")
}

// The same relisting, a version later still: a row version 7 wrote cannot
// hold the ride's maximum speed, exactly what migration 051 leaves behind.
func TestActivitiesAwaitingDerivationRelistsARowVersion7Wrote(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}, activity.RecordsVersion), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics()")
	_, err := store.database.ExecContext(t.Context(), `UPDATE activity_metrics SET derivation_version = 7`)
	require.NoError(t, err, "ageing the stored row to version 7")

	owed, listErr := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.NoError(t, listErr, "ActivitiesAwaitingDerivation()")
	assert.Equal(t, []int64{1}, owed, "the row predates the maximum speed this derivation produces")
}

// A ride still waiting for its FIT has nothing to derive from, so it waits for
// the download rather than being derived into an empty row.
func TestActivitiesAwaitingDerivationSkipsARideWithNoStoredRecords(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
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
				TemperatureCelsius: 18, HasTemperatureCelsius: true,
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
	}, activity.RecordsVersion), "StoreActivityRecords()")

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
	// The track sample carries the cadence and temperature the record held,
	// and marks them absent where the column was never set.
	assert.True(t, samples.Track[0].HasCadence)
	assert.InDelta(t, 88.0, samples.Track[0].CadenceRPM, 1e-9)
	assert.True(t, samples.Track[0].HasTemperature)
	assert.InDelta(t, 18.0, samples.Track[0].TemperatureCelsius, 1e-9)
	assert.False(t, samples.Track[1].HasCadence, "the second record carried neither")
	assert.False(t, samples.Track[1].HasTemperature)
}

// An unpaired strap writes nought, and nought is no heart rate: it must not
// enter the series as a reading a rider never took.
// A record carrying only a temperature reading -- no heart rate, power,
// cadence, speed or distance -- must still reach the temperature series: the
// query's own filter is a sensor test, and temperature is one, even alone.
func TestActivityRideSamplesKeepsARecordWithOnlyATemperatureReading(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), TemperatureCelsius: 18, HasTemperatureCelsius: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.Temperature, 1, "the record's only reading must not be filtered out")
	assert.InDelta(t, 18.0, samples.Temperature[0].Value, 1e-9)
}

func TestActivityRideSamplesLeavesOutAZeroHeartRate(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), HeartRateBPM: 0, HasHeartRate: true},
			{Time: activityNow().Add(time.Second), HeartRateBPM: 140, HasHeartRate: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.HeartRate, 1, "the nought is not a reading")
	assert.InDelta(t, 140.0, samples.HeartRate[0].Value, 1e-9)
}

// A device that records its own speed is trusted over the odometer: the
// series is built from speed_ms per row, converted to km/h.
func TestActivityRideSamplesReadsSpeedFromTheDeviceWhereRecorded(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), SpeedMS: 8, HasSpeed: true, DistanceMetres: 0, HasDistance: true},
			{Time: activityNow().Add(time.Second), SpeedMS: 20, HasSpeed: true, DistanceMetres: 1000, HasDistance: true},
			// A record with no device reading of its own is left out, not read
			// as a stop: the ride is on the device's series, not the odometer's.
			{Time: activityNow().Add(2 * time.Second), HeartRateBPM: 140, HasHeartRate: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.Speed, 2, "the device's own reading, not the odometer's")
	assert.InDelta(t, 28.8, samples.Speed[0].Value, 1e-9)
	assert.InDelta(t, 72.0, samples.Speed[1].Value, 1e-9)
}

// The regression: an odometer whose every derived rate exceeds the ceiling
// has no usable reading at all.
func TestActivityRideSamplesTreatsAnAllExcessiveOdometerRateAsUnknown(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), DistanceMetres: 0, HasDistance: true},
			// 10 km in one second: the only derived rate, and implausible.
			{Time: activityNow().Add(time.Second), DistanceMetres: 10000, HasDistance: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	assert.Empty(t, samples.Speed, "the only derived rate was implausible and dropped")
}

// A ride with no device speed reading falls back to the odometer: distance
// over time between consecutive records, the same rule speedSeries applies.
func TestActivityRideSamplesFallsBackToTheOdometerWithoutADeviceSpeed(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), DistanceMetres: 0, HasDistance: true},
			{Time: activityNow().Add(time.Second), DistanceMetres: 10, HasDistance: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.Speed, 1, "the rate names the step it ends, not the one it starts")
	assert.Equal(t, activityNow().Add(time.Second), samples.Speed[0].At)
	assert.InDelta(t, 36.0, samples.Speed[0].Value, 1e-9)
}

// A record between two distance readings that carries no distance of its own
// -- a temperature-only one, say -- must not become the step the next
// distance reading is measured from: that would turn every distance sample
// after it into a dropped one rather than a step from the last real reading.
func TestActivityRideSamplesSkipsADistancelessRowWhenBuildingTheOdometerStep(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), DistanceMetres: 0, HasDistance: true},
			{Time: activityNow().Add(time.Second), TemperatureCelsius: 18, HasTemperatureCelsius: true},
			{Time: activityNow().Add(2 * time.Second), DistanceMetres: 20, HasDistance: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.Speed, 1,
		"the distance step across the temperature-only record must still be measured")
	assert.Equal(t, activityNow().Add(2*time.Second), samples.Speed[0].At)
	assert.InDelta(t, 36.0, samples.Speed[0].Value, 1e-9, "20 m over the full two seconds")
}

// A single implausible spike — a clock or odometer hiccup, not a rider — is
// dropped, and the rest of the series stands.
func TestActivityRideSamplesDropsASpeedReadingAboveTheCeiling(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), SpeedMS: 10, HasSpeed: true},
			{Time: activityNow().Add(time.Second), SpeedMS: 400, HasSpeed: true},
			{Time: activityNow().Add(2 * time.Second), SpeedMS: 12, HasSpeed: true},
		},
	}, activity.RecordsVersion), "StoreActivityRecords()")

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	require.Len(t, samples.Speed, 2, "the implausible spike, and only it, is dropped")
	averages := samples.Averages()
	assert.True(t, averages.HasSpeed)
	assert.InDelta(t, 43.2, averages.MaxSpeedKmh, 1e-9, "the peak of what remains")
}

// A ride with no readable records has no speed series at all.
func TestActivityRideSamplesHasNoSpeedWithoutRecords(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	samples, err := store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityRideSamples()")
	assert.Empty(t, samples.Speed)
	assert.False(t, samples.Averages().HasSpeed)
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
	}, activity.RecordsVersion), "StoreActivityRecords()")

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

// The regression: a track with samples but no known estimate among them --
// every step fell in a recording gap, say -- is not "a ride that now yields
// an estimate". Its existing match must survive, not be redone against a
// series that was never actually written.
func TestStoreEstimatedPowerKeepsTheMatchWhenNoEstimateWasKnown(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 1, matchOf(key), []activity.ClimbAttempt{attemptOf(0, 780)}, "library-1", activityNow(),
	), "StoreActivityRouteMatch()")

	require.NoError(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1,
		[]int64{0, 1}, []measure.Estimate{{}, {}}), "StoreEstimatedPower() with a track but no known estimate")

	attempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteClimbAttempts()")
	assert.Len(t, attempts, 1, "no estimate was ever written, so the existing match owes it nothing")
}

func TestStoreEstimatedPowerRefusesMismatchedSeries(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.ErrorContains(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1,
		[]int64{0}, []measure.Estimate{{}, {}}), "an estimate per record or none")
}

// The regression: written separately, a metrics failure after a successful
// estimate write would leave the track endpoint serving an estimate no
// metrics row stands behind. One transaction rolls both back together.
func TestStoreRideDerivationRollsBackTheEstimateWhenTheMetricsWriteFails(t *testing.T) {
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
	}, activity.RecordsVersion), "StoreActivityRecords()")
	_, err := store.database.ExecContext(t.Context(), `
		CREATE TRIGGER reject_activity_metrics_write BEFORE INSERT ON activity_metrics
		BEGIN SELECT RAISE(ABORT, 'metrics write failed'); END
	`)
	require.NoError(t, err)

	err = store.StoreRideDerivation(t.Context(), "rider-a", 1,
		[]int64{0, 1}, []measure.Estimate{{}, {Watts: 214, Known: true}},
		derivedMetrics(testInputs(), testCoefficients()))
	require.Error(t, err)

	track, trackErr := store.ActivityTrack(t.Context(), "rider-a", 1)
	require.NoError(t, trackErr, "ActivityTrack()")
	require.Len(t, track, 2)
	assert.False(t, track[1].HasEstimatedPower, "the estimate must not survive a metrics write that failed beside it")
}

// A mass change makes every estimate stale, so a row worked out against another
// mass is owed a derivation again.
func TestActivitiesAwaitingDerivationNoticesAMassChange(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}, activity.RecordsVersion), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics()")

	unchanged, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.NoError(t, err, "ActivitiesAwaitingDerivation()")
	assert.Empty(t, unchanged, "nothing changed")

	heavier := testInputs()
	heavier.TotalMassKG = 84
	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", heavier, testCoefficients())
	require.NoError(t, err, "ActivitiesAwaitingDerivation() after a mass change")
	assert.Equal(t, []int64{1}, owed)
}

// A bicycle change makes every estimate stale, so a row worked out against
// another drag area or rolling resistance is owed a derivation again.
func TestActivitiesAwaitingDerivationNoticesABicycleChange(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow(), HeartRateBPM: 150, HasHeartRate: true}},
	}, activity.RecordsVersion), "StoreActivityRecords()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics()")

	unchanged, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.NoError(t, err, "ActivitiesAwaitingDerivation()")
	assert.Empty(t, unchanged, "nothing changed")

	slipperier := testCoefficients()
	slipperier.DragArea = 0.30
	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), slipperier)
	require.NoError(t, err, "ActivitiesAwaitingDerivation() after a bicycle change")
	assert.Equal(t, []int64{1}, owed)
}

// A metered ride holds no estimate, so a bicycle change has nothing in it to
// go stale and must not re-list it.
func TestActivitiesAwaitingDerivationIgnoresABicycleChangeForAMeteredRide(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	metered := derivedMetrics(testInputs(), testCoefficients())
	metered.Load.HasEstimatedPower = false
	metered.HasEstimatedPedallingShare = false
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, metered),
		"StoreActivityMetrics()")

	slipperier := testCoefficients()
	slipperier.DragArea = 0.30
	owed, err := store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), slipperier)
	require.NoError(t, err, "ActivitiesAwaitingDerivation() after a bicycle change")
	assert.Empty(t, owed, "a metered ride has no estimate to go stale")
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
		require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", id, derivedMetrics(testInputs(), testCoefficients())),
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

// The regression: clearing every profile parameter must not leave the
// estimate series, or the route match and climb attempt read from it,
// outliving the metrics row it was derived beside.
func TestClearActivityMetricsTakesTheEstimateSeriesAndTheRouteMatchWithIt(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2)
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
	}, activity.RecordsVersion), "StoreActivityRecords()")
	require.NoError(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1,
		[]int64{0, 1}, []measure.Estimate{{}, {Watts: 214, Known: true}}), "StoreEstimatedPower()")
	key := storeTestLibrary(t, store, 7, "hash-a")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 1, matchOf(key), []activity.ClimbAttempt{attemptOf(0, 780)}, "library-1", activityNow(),
	), "StoreActivityRouteMatch()")
	// A second, metered ride: its match owes the profile nothing and must
	// survive a clear that names no bicycle or mass at all.
	meteredKey := storeTestLibrary(t, store, 8, "hash-b")
	require.NoError(t, store.StoreActivityRouteMatch(
		t.Context(), "rider-a", 2, matchOf(meteredKey), []activity.ClimbAttempt{attemptOf(0, 600)}, "library-1", activityNow(),
	), "StoreActivityRouteMatch()")
	// Stored last: StoreActivityRecords and StoreEstimatedPower each clear a
	// stale metrics row of their own, and this test means to clear a row that
	// still stands for what it just wrote.
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1,
		derivedMetrics(testInputs(), testCoefficients())), "StoreActivityMetrics()")

	removed, err := store.ClearActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ClearActivityMetrics()")
	assert.Equal(t, 1, removed)

	track, err := store.ActivityTrack(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityTrack()")
	require.Len(t, track, 2)
	assert.False(t, track[1].HasEstimatedPower, "the estimate must not outlive the profile it was derived from")

	attempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", key)
	require.NoError(t, err, "RouteClimbAttempts()")
	assert.Empty(t, attempts, "no profile, no match to attribute a climb attempt to")

	meteredAttempts, err := store.RouteClimbAttempts(t.Context(), "rider-a", meteredKey)
	require.NoError(t, err, "RouteClimbAttempts() for the metered ride")
	assert.Len(t, meteredAttempts, 1, "a metered ride's match owes the profile nothing and survives the clear")
}

// The whole of what the fitness timeline folds: one row per derived ride, with
// the moment it was ridden, oldest first.
func TestActivityRideLoadsCarryTheDayAndTheLoadOfEachRide(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2)
	withMeter := derivedMetrics(testInputs(), testCoefficients())
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
	_, err = store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs(), testCoefficients())
	require.ErrorContains(t, err, "listing activities awaiting derivation")
	_, err = store.ActivityRideSamples(t.Context(), "rider-a", 1)
	require.ErrorContains(t, err, "reading the recorded samples")
	require.ErrorContains(t, store.StoreEstimatedPower(t.Context(), "rider-a", 1, nil, nil),
		"starting the estimated power write")
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"storing the activity metrics")
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{}),
		"clearing the activity metrics")
	require.ErrorContains(t, store.StoreRideDerivation(t.Context(), "rider-a", 1, nil, nil, activity.RideMetrics{}),
		"starting the ride derivation write")
	_, err = store.TargetOwner(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the target owner")
	_, err = store.ClearActivityMetrics(t.Context(), "rider-a")
	require.ErrorContains(t, err, "starting the activity metrics clear")
	_, err = store.ActivityRideLoads(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the activity ride loads")
}

func TestActivityMetricsRoundTripTheDriftReadings(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load:       trainingload.Metrics{Inputs: testInputs()},
		Decoupling: activity.Decoupling{Percent: 4.25, Known: true},
		HeatDrift: activity.HeatDrift{
			HeartRateBPM: 141.5, TemperatureCelsius: 29.5, Samples: 1800, Known: true,
		},
	}), "StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	require.True(t, read[1].Decoupling.Known)
	assert.InDelta(t, 4.25, read[1].Decoupling.Percent, 0.001)
	require.True(t, read[1].HeatDrift.Known)
	assert.InDelta(t, 141.5, read[1].HeatDrift.HeartRateBPM, 0.001)
	assert.InDelta(t, 29.5, read[1].HeatDrift.TemperatureCelsius, 0.001)
	assert.Equal(t, 1800, read[1].HeatDrift.Samples)
}

// A ride the derivation found neither reading for stores neither, and reads
// back as absent rather than as nought.
func TestActivityMetricsReadNoDriftForARideThatYieldedNone(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs(), TRIMP: 42, HasTRIMP: true},
	}), "StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.False(t, read[1].Decoupling.Known, "no decoupling is not a decoupling of nought")
	assert.False(t, read[1].HeatDrift.Known, "and no reading is not a reading at nought degrees")
}

// A reading is the whole of what a derivation writes. A row carrying a heart
// rate and a temperature but no count is half a write, and half a write is not
// a point on a season's drift: served, its count would be a nought the contract
// does not allow.
func TestActivityMetricsReadNoDriftFromAHalfWrittenReading(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs()},
		HeatDrift: activity.HeatDrift{
			HeartRateBPM: 141.5, TemperatureCelsius: 29.5, Samples: 1800, Known: true,
		},
	}), "StoreActivityMetrics()")
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE activity_metrics SET heat_drift_samples = NULL WHERE target_slot = ? AND workout_id = ?`,
		"rider-a", 1)
	require.NoError(t, err, "clearing the sample count")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.False(t, read[1].HeatDrift.Known, "a reading with no count is not a reading")
}

// curveOf is a stored ride's bests: the same watts at every duration named.
func curveOf(points map[int]float64) rider.PowerCurve {
	curve := rider.PowerCurve{}
	for point, value := range points {
		curve.Watts[point], curve.Held[point] = value, true
	}

	return curve
}

func TestPowerCurveFoldsTheBestOfEveryStoredRide(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs()}, PowerBests: curveOf(map[int]float64{0: 900, 4: 250}),
	}), "StoreActivityMetrics()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 2, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs()}, PowerBests: curveOf(map[int]float64{0: 820, 4: 268}),
	}), "StoreActivityMetrics()")

	curve, err := store.PowerCurve(t.Context(), []string{"rider-a"}, time.Time{}, farFuture())
	require.NoError(t, err, "PowerCurve()")
	assert.InDelta(t, 900.0, curve.Watts[0], 0.001, "the better sprint")
	assert.InDelta(t, 268.0, curve.Watts[rider.ThresholdPowerPoint], 0.001, "the better twenty")
	assert.False(t, curve.Held[5], "an hour neither ride was long enough for")
}

// A curve is the caller's own: another target's rides never enter it.
func TestPowerCurveNeverPoolsAcrossRiders(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs()}, PowerBests: curveOf(map[int]float64{0: 900}),
	}), "StoreActivityMetrics()")

	curve, err := store.PowerCurve(t.Context(), []string{"rider-b"}, time.Time{}, farFuture())
	require.NoError(t, err, "PowerCurve()")
	assert.False(t, curve.Any(), "another rider's rides are not this rider's curve")
}

func TestPowerCurveIsEmptyForACallerWithNoTarget(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)

	curve, err := store.PowerCurve(t.Context(), nil, time.Time{}, farFuture())
	require.NoError(t, err, "PowerCurve()")
	assert.False(t, curve.Any(), "no target, nothing to fold")
}

func TestPowerCurveReportsAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.Close(), "Close()")

	_, err := store.PowerCurve(t.Context(), []string{"rider-a"}, time.Time{}, farFuture())
	require.ErrorContains(t, err, "reading the stored power bests")
}

func TestActivityMetricsRoundTripTheStoredBests(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs()}, PowerBests: curveOf(map[int]float64{1: 640, 3: 310}),
	}), "StoreActivityMetrics()")

	read, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.InDelta(t, 640.0, read[1].PowerBests.Watts[1], 0.001)
	assert.InDelta(t, 310.0, read[1].PowerBests.Watts[3], 0.001)
	assert.False(t, read[1].PowerBests.Held[0], "a duration the ride never reached stays absent")
}

// farFuture is a window end past every stored ride, for a test about something
// other than the window.
func farFuture() time.Time { return activityNow().Add(24 * time.Hour) }

// The curve covers exactly the rides the timeline beside it does, so a window
// that closes before a ride leaves that ride's bests out of it.
func TestPowerCurveLeavesOutARideAfterTheWindowCloses(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{
		Load: trainingload.Metrics{Inputs: testInputs()}, PowerBests: curveOf(map[int]float64{0: 900}),
	}), "StoreActivityMetrics()")

	curve, err := store.PowerCurve(t.Context(), []string{"rider-a"}, time.Time{}, activityNow())
	require.NoError(t, err, "PowerCurve()")
	assert.False(t, curve.Any(), "the window is half open, so a ride at its end is outside it")
}
