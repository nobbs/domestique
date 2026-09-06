package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// derivedMetrics is a row with something in every part, so a round trip that
// drops one is visible.
func derivedMetrics(inputs trainingload.Inputs) trainingload.Metrics {
	return trainingload.Metrics{
		Inputs:          inputs,
		Zones:           trainingload.Zones{60, 120, 180, 240, 300},
		HasZones:        true,
		TRIMP:           42.5,
		HasTRIMP:        true,
		HeartRateTSS:    88.25,
		HasHeartRateTSS: true,
		Power:           trainingload.Power{NormalizedWatts: 214, IntensityFactor: 0.856, TSS: 73.3},
		HasPower:        true,
	}
}

func testInputs() trainingload.Inputs {
	return trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 48,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
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
	assert.True(t, read[1].HasZones && read[1].HasTRIMP && read[1].HasHeartRateTSS && read[1].HasPower)
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

func TestActivitySensorSamplesSplitTheSensorsAndLeaveOutTheAbsent(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{
			{Time: activityNow(), HeartRateBPM: 140, HasHeartRate: true, PowerWatts: 200, HasPower: true},
			{Time: activityNow().Add(time.Second), HeartRateBPM: 142, HasHeartRate: true},
			{Time: activityNow().Add(2 * time.Second)},
		},
	}), "StoreActivityRecords()")

	heartRate, power, err := store.ActivitySensorSamples(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivitySensorSamples()")
	require.Len(t, heartRate, 2, "the two records that carried a strap")
	require.Len(t, power, 1, "and the one that carried a meter")
	assert.InDelta(t, 140.0, heartRate[0].Value, 1e-9)
	assert.Equal(t, activityNow(), heartRate[0].At)
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

func TestActivityMetricsReportAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the activity metrics")
	_, err = store.ActivitiesAwaitingDerivation(t.Context(), "rider-a", testInputs())
	require.ErrorContains(t, err, "listing activities awaiting derivation")
	_, _, err = store.ActivitySensorSamples(t.Context(), "rider-a", 1)
	require.ErrorContains(t, err, "reading the recorded samples")
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs())),
		"storing the activity metrics")
	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, trainingload.Metrics{}),
		"clearing the activity metrics")
	_, err = store.TargetOwner(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the target owner")
}
