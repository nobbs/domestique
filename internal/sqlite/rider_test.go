package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiderProfileIsEmptyForASubjectThatHasEnteredNothing(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	profile, err := store.RiderProfile(t.Context(), "rider-a")
	require.NoError(t, err, "RiderProfile()")
	assert.Equal(t, rider.Profile{}, profile, "an unwritten profile is empty, not missing")
}

func TestRiderProfileRoundTripsAndIsNotAnotherSubjects(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	stored := rider.Profile{
		MaxHeartRateBPM:               rider.Set(188),
		RestingHeartRateBPM:           rider.Set(46),
		FunctionalThresholdPowerWatts: rider.Set(268),
		RiderMassKG:                   rider.Set(74.5),
	}
	require.NoError(t, store.SetRiderProfile(t.Context(), "rider-a", stored), "SetRiderProfile()")

	read, err := store.RiderProfile(t.Context(), "rider-a")
	require.NoError(t, err, "RiderProfile()")
	assert.Equal(t, stored, read)
	assert.False(t, read.ThresholdHeartRateBPM.Set, "a parameter never entered stays absent")

	other, err := store.RiderProfile(t.Context(), "rider-b")
	require.NoError(t, err, "RiderProfile() for another subject")
	assert.Equal(t, rider.Profile{}, other, "one rider's profile is not another's")
}

// A second write replaces the profile whole: a parameter the rider cleared is
// cleared in the store rather than kept from the write before.
func TestSetRiderProfileReplacesTheWholeProfile(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderProfile(t.Context(), "rider-a", rider.Profile{
		MaxHeartRateBPM: rider.Set(188), BikeMassKG: rider.Set(8.4),
	}), "SetRiderProfile()")
	require.NoError(t, store.SetRiderProfile(t.Context(), "rider-a", rider.Profile{
		MaxHeartRateBPM: rider.Set(190),
	}), "SetRiderProfile() again")

	read, err := store.RiderProfile(t.Context(), "rider-a")
	require.NoError(t, err, "RiderProfile()")
	assert.Equal(t, rider.Set(190), read.MaxHeartRateBPM)
	assert.False(t, read.BikeMassKG.Set, "a parameter left out of the second write is cleared")
}

func TestRiderProfileReportsAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.RiderProfile(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the rider profile")
	require.ErrorContains(t, store.SetRiderProfile(t.Context(), "rider-a", rider.Profile{}),
		"storing the rider profile")
	_, err = store.RiderSuggestions(t.Context(), []string{"rider-a"}, activityNow())
	require.ErrorContains(t, err, "reading the recorded samples")
}

// steady is a ride recorded once a second, holding one heart rate and one power
// for as long as it lasts.
func steady(seconds int, heartRate, power float64) activity.FIT {
	records := make([]activity.Record, seconds)
	for index := range records {
		records[index] = activity.Record{
			Time:         activityNow().Add(time.Duration(index) * time.Second),
			HeartRateBPM: heartRate, HasHeartRate: heartRate > 0,
			PowerWatts: power, HasPower: power > 0,
		}
	}

	return activity.FIT{Records: records}
}

func TestRiderSuggestionsReadTheBestEffortAcrossTheCallersRides(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 2, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 200)),
		"StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 2, steady(1500, 170, 240)),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.InDelta(t, 170.0, suggestions.MaxHeartRateBPM.Number, 0.5, "the harder ride's minute")
	assert.InDelta(t, 228.0, suggestions.FunctionalThresholdPowerWatts.Number, 0.5, "240 W taken at 95%")
}

// A sensor the rides do not carry yields no suggestion rather than a zero, which
// is what lets the page offer one field a figure and not the next.
func TestRiderSuggestionsOmitASensorTheRidesDoNotCarry(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 0)),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.True(t, suggestions.MaxHeartRateBPM.Set, "the rides carried a heart-rate strap")
	assert.False(t, suggestions.FunctionalThresholdPowerWatts.Set, "no ride carried a meter")
}

// A ride too short to hold the window suggests nothing rather than its own mean.
func TestRiderSuggestionsIgnoreARideShorterThanTheWindow(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(300, 150, 200)),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.True(t, suggestions.MaxHeartRateBPM.Set, "five minutes covers the minute asked for")
	assert.False(t, suggestions.FunctionalThresholdPowerWatts.Set, "and not the twenty")
}

// Only the rides inside the window count: fitness moves, and a best effort from
// years ago is not a suggestion about this rider now.
func TestRiderSuggestionsReadOnlyRidesSinceTheCutoff(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 190, 400)),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, activityNow().Add(time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.MaxHeartRateBPM.Set, "the only ride started before the cutoff")
}

// Two targets read in one query: a ride's series must not run into the next
// target's, and the best is the best across both.
func TestRiderSuggestionsTakeTheBestAcrossEveryTargetAsked(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-b", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 240)),
		"StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-b", 1, steady(1500, 170, 200)),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a", "rider-b"}, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.InDelta(t, 170.0, suggestions.MaxHeartRateBPM.Number, 0.5, "the second target's minute")
	assert.InDelta(t, 228.0, suggestions.FunctionalThresholdPowerWatts.Number, 0.5, "the first target's twenty")
}

func TestRiderSuggestionsAreEmptyForACallerWithNoTarget(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	suggestions, err := store.RiderSuggestions(t.Context(), nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.Equal(t, rider.Suggestions{}, suggestions, "no target, nothing to read")
}

// A ride belongs to the target that recorded it, and a suggestion is read over
// the targets it was asked for and no others.
func TestRiderSuggestionsReadOnlyTheTargetsAsked(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-b", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-b", 1, steady(1500, 190, 400)),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.MaxHeartRateBPM.Set, "another rider's ride is not this rider's suggestion")
}
