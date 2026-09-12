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
		BikeMassKG:                    rider.Set(8.4),
		DragAreaM2:                    rider.Set(0.38),
		RollingResistance:             rider.Set(0.007),
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
		DragAreaM2: rider.Set(0.38), RollingResistance: rider.Set(0.007),
	}), "SetRiderProfile()")
	require.NoError(t, store.SetRiderProfile(t.Context(), "rider-a", rider.Profile{
		MaxHeartRateBPM: rider.Set(190),
	}), "SetRiderProfile() again")

	read, err := store.RiderProfile(t.Context(), "rider-a")
	require.NoError(t, err, "RiderProfile()")
	assert.Equal(t, rider.Set(190), read.MaxHeartRateBPM)
	assert.False(t, read.BikeMassKG.Set, "a parameter left out of the second write is cleared")
	assert.False(t, read.DragAreaM2.Set, "the bicycle's drag area is cleared the same way")
	assert.False(t, read.RollingResistance.Set, "and its rolling resistance too")
}

func TestRiderProfileReportsAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.RiderProfile(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the rider profile")
	require.ErrorContains(t, store.SetRiderProfile(t.Context(), "rider-a", rider.Profile{}),
		"storing the rider profile")
	_, err = store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow())
	require.ErrorContains(t, err, "reading the recorded samples")
}

func TestRiderCredentialsIsEmptyForASubjectThatHasEnteredNothing(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	credentials, err := store.RiderCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderCredentials()")
	assert.Empty(t, credentials, "an unwritten set of credentials is empty, not missing")
}

func TestRiderCredentialsRoundTripAndAreNotAnotherSubjects(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail:    rider.NewCredential([]byte("rider@example.test")),
		rider.CredentialZwiftPassword: rider.NewCredential([]byte("opensesame")),
	}), "SetRiderCredentials()")

	read, err := store.RiderCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderCredentials()")
	require.Contains(t, read, rider.CredentialZwiftEmail)
	assert.Equal(t, []byte("rider@example.test"), read[rider.CredentialZwiftEmail].Bytes())
	assert.Equal(t, []byte("opensesame"), read[rider.CredentialZwiftPassword].Bytes())

	other, err := store.RiderCredentials(t.Context(), "rider-b")
	require.NoError(t, err, "RiderCredentials() for another subject")
	assert.Empty(t, other, "one rider's credentials are not another's")
}

// A save that names only one credential leaves the other exactly as it was.
func TestSetRiderCredentialsPartialSaveLeavesTheOtherCredential(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail:    rider.NewCredential([]byte("rider@example.test")),
		rider.CredentialZwiftPassword: rider.NewCredential([]byte("opensesame")),
	}), "SetRiderCredentials()")

	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftPassword: rider.NewCredential([]byte("newpassword")),
	}), "SetRiderCredentials() with one name")

	read, err := store.RiderCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderCredentials()")
	assert.Equal(t, []byte("rider@example.test"), read[rider.CredentialZwiftEmail].Bytes(),
		"the credential left out of the second write")
	assert.Equal(t, []byte("newpassword"), read[rider.CredentialZwiftPassword].Bytes())
}

// A credential written with no value is removed, the same rule a deployment
// credential follows.
func TestSetRiderCredentialsRemovesACredentialWrittenWithNoValue(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: rider.NewCredential([]byte("rider@example.test")),
	}), "SetRiderCredentials()")

	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: {},
	}), "SetRiderCredentials() with no value")

	read, err := store.RiderCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderCredentials()")
	assert.NotContains(t, read, rider.CredentialZwiftEmail)
}

// ClearRiderCredentials removes both names, the deliberate exception a rider's
// own account gets that a deployment credential does not.
func TestClearRiderCredentialsRemovesBoth(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail:    rider.NewCredential([]byte("rider@example.test")),
		rider.CredentialZwiftPassword: rider.NewCredential([]byte("opensesame")),
	}), "SetRiderCredentials()")

	require.NoError(t, store.ClearRiderCredentials(t.Context(), "rider-a"), "ClearRiderCredentials()")

	read, err := store.RiderCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderCredentials()")
	assert.Empty(t, read)
}

// The subject and the name together are the associated data, so a ciphertext
// moved to another subject fails to open rather than authenticating as that
// subject's credential.
func TestRiderCredentialsBindToTheirSubject(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: rider.NewCredential([]byte("rider@example.test")),
	}), "SetRiderCredentials()")

	_, err := store.database.ExecContext(t.Context(), `
		INSERT INTO rider_credentials (subject, name, value, updated_at_unix)
		SELECT ?, name, value, updated_at_unix FROM rider_credentials WHERE subject = ?
	`, "rider-b", "rider-a")
	require.NoError(t, err, "copy the ciphertext to another subject")

	_, err = store.RiderCredentials(t.Context(), "rider-b")
	require.ErrorIs(t, err, ErrStateUnreadable, "RiderCredentials()")
}

func TestRiderCredentialsReportAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.RiderCredentials(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the rider credentials")
	require.ErrorContains(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: rider.NewCredential([]byte("x")),
	}), "rider credential")
	require.ErrorContains(t, store.ClearRiderCredentials(t.Context(), "rider-a"), "clearing the rider credentials")
}

// A row that fails to write or clear mid-transaction is the store's own
// failure, named as one rather than surfaced as sqlite's.
func TestSetRiderCredentialsReportsAQueryFailure(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	_, err := store.database.ExecContext(t.Context(), `
		CREATE TRIGGER reject_rider_credential_write BEFORE INSERT ON rider_credentials
		BEGIN SELECT RAISE(ABORT, 'credential write failed'); END
	`)
	require.NoError(t, err)

	require.ErrorContains(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: rider.NewCredential([]byte("x")),
	}), "storing a rider credential")
}

func TestSetRiderCredentialsReportsADeleteFailure(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: rider.NewCredential([]byte("x")),
	}), "SetRiderCredentials()")
	_, err := store.database.ExecContext(t.Context(), `
		CREATE TRIGGER reject_rider_credential_delete BEFORE DELETE ON rider_credentials
		BEGIN SELECT RAISE(ABORT, 'credential clear failed'); END
	`)
	require.NoError(t, err)

	require.ErrorContains(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail: {},
	}), "clearing a rider credential")
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

// strapped records a heart rate that is present but not beating, which is what
// an unpaired strap writes: the reading is there, and it is nought.
func strapped(seconds int, heartRate, power float64) activity.FIT {
	records := make([]activity.Record, seconds)
	for index := range records {
		records[index] = activity.Record{
			Time:         activityNow().Add(time.Duration(index) * time.Second),
			HeartRateBPM: heartRate, HasHeartRate: true,
			PowerWatts: power, HasPower: power > 0,
		}
	}

	return activity.FIT{Records: records}
}

// A strap that never paired suggests nothing, rather than suggesting nought.
// The derivation path drops a heart rate of nought and this one has to agree,
// or a rider is offered a zone scheme cut from zero.
func TestRiderSuggestionsIgnoreAStrapThatRecordedNoBeat(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, strapped(1500, 0, 200), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.MaxHeartRateBPM.Set, "nought is no maximum")
	assert.False(t, suggestions.ThresholdHeartRateBPM.Set, "and no threshold")
	assert.True(t, suggestions.FunctionalThresholdPowerWatts.Set, "the power beside it is still a reading")
}

func TestRiderSuggestionsReadTheBestEffortAcrossTheCallersRides(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 2, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 200), activity.RecordsVersion),
		"StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 2, steady(1500, 170, 240), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.InDelta(t, 170.0, suggestions.MaxHeartRateBPM.Number, 0.5, "the harder ride's minute")
	assert.InDelta(t, 170.0, suggestions.ThresholdHeartRateBPM.Number, 0.5, "the same ride's best twenty minutes, unscaled")
	assert.InDelta(t, 228.0, suggestions.FunctionalThresholdPowerWatts.Number, 0.5, "240 W taken at 95%")
}

// A sensor the rides do not carry yields no suggestion rather than a zero, which
// is what lets the page offer one field a figure and not the next.
func TestRiderSuggestionsOmitASensorTheRidesDoNotCarry(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 0), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.True(t, suggestions.MaxHeartRateBPM.Set, "the rides carried a heart-rate strap")
	assert.False(t, suggestions.FunctionalThresholdPowerWatts.Set, "no ride carried a meter")
}

// shaped is a ride recorded once a second, holding each power in turn for the
// seconds it is paired with and carrying no strap.
func shaped(stretches [][2]float64) activity.FIT {
	var records []activity.Record
	for _, stretch := range stretches {
		for range int(stretch[0]) {
			records = append(records, activity.Record{
				Time:       activityNow().Add(time.Duration(len(records)) * time.Second),
				PowerWatts: stretch[1], HasPower: true,
			})
		}
	}

	return activity.FIT{Records: records}
}

// rampRide is the protocol's own shape: seventeen minutes warm, then
// one-minute steps of twenty watts to a peak of 330, which is where a recorded
// ramp test's one-to-five-minute ratio of 1.14 comes from.
func rampRide() activity.FIT {
	stretches := [][2]float64{{17 * 60, 110}}
	for watts := 130.0; watts <= 330; watts += 20 {
		stretches = append(stretches, [2]float64{60, watts})
	}

	return shaped(stretches)
}

// Both the twenty-minute estimate and the ramp estimate are floors on the same
// number, true only for a rider who performed that protocol, so the higher one
// is offered.
func TestRiderSuggestionsPreferTheRampEstimateWhenItIsHigher(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 2, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 205), activity.RecordsVersion),
		"StoreActivityRecords()") // 205 W at 95% is 194.75 W, the twenty-minute estimate.
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 2, rampRide(), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.InDelta(t, 247.25, suggestions.FunctionalThresholdPowerWatts.Number, 0.5,
		"75% of the ramp's 330 W peak minute, over the steady ride's 194.75 W")
}

// A maximal five-minute effort sits inside the ratio band and is read as a ramp
// test. What keeps that harmless is the arithmetic rather than the shape: the
// ratio holds any ramp estimate under 94% of the ride's own best five minutes,
// which a rider's real threshold clears, so the higher-wins fold discards it.
func TestRiderSuggestionsDiscardAMisreadRampBelowTheTwentyMinuteEstimate(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 2, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 280), activity.RecordsVersion),
		"StoreActivityRecords()") // 280 W at 95% is 266 W.
	// Twenty-five minutes with one all-out five in the middle: a ratio of 1.13,
	// so the shape admits it and 340 W at 75% offers 255 W.
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 2,
		shaped([][2]float64{{600, 120}, {60, 340}, {240, 290}, {600, 120}}), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.InDelta(t, 266.0, suggestions.FunctionalThresholdPowerWatts.Number, 0.5,
		"the misread ride's 255 W lost to the twenty-minute estimate")
}

// A ride too short to hold the window suggests nothing rather than its own mean.
func TestRiderSuggestionsIgnoreARideShorterThanTheWindow(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, storeTestActivity(t, store, "rider-a", 1, 100), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(300, 150, 200), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
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
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 190, 400), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(time.Hour))
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
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, steady(1500, 150, 240), activity.RecordsVersion),
		"StoreActivityRecords()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-b", 1, steady(1500, 170, 200), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a", "rider-b"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.InDelta(t, 170.0, suggestions.MaxHeartRateBPM.Number, 0.5, "the second target's minute")
	assert.InDelta(t, 228.0, suggestions.FunctionalThresholdPowerWatts.Number, 0.5, "the first target's twenty")
}

func TestRiderSuggestionsAreEmptyForACallerWithNoTarget(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	suggestions, err := store.RiderSuggestions(t.Context(), nil, nil, activityNow().Add(-time.Hour))
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
	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-b", 1, steady(1500, 190, 400), activity.RecordsVersion),
		"StoreActivityRecords()")

	suggestions, err := store.RiderSuggestions(t.Context(), []string{"rider-a"}, nil, activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.MaxHeartRateBPM.Set, "another rider's ride is not this rider's suggestion")
}

// storeStoppingRide stores one ride of the given type that moved for an hour
// and stood still for the given seconds.
func storeStoppingRide(
	t *testing.T, store *Store, targetID string, id int64, typeID int, stoppedSeconds float64,
) {
	t.Helper()
	require.NoError(t, store.StoreActivity(t.Context(), targetID,
		activity.Listing{ID: id, TypeID: typeID, LocationID: 1, Starts: activityNow()},
		activity.Summary{
			DistanceMetres: 20_000, MovingSeconds: 3600, ElapsedSeconds: 3600 + stoppedSeconds,
			AscentMetres: 120, Raw: []byte(`{}`),
		},
		activityNow(),
	), "StoreActivity()")
}

func outdoorTypes() []int { return []int{15} }

func TestRiderSuggestionsReadTheCallersOwnStoppingHabit(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	for index, seconds := range []float64{100, 200, 300, 400, 500} {
		storeStoppingRide(t, store, "rider-a", int64(index+1), 15, seconds)
	}

	suggestions, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a"}, outdoorTypes(), activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	require.True(t, suggestions.Stopping.Set, "five eligible rides are a habit")
	assert.InDelta(t, 300.0, suggestions.Stopping.MedianSecondsPerHour, 0.001)
	assert.InDelta(t, 200.0, suggestions.Stopping.LowerQuartileSecondsPerHour, 0.001)
	assert.InDelta(t, 400.0, suggestions.Stopping.UpperQuartileSecondsPerHour, 0.001)
	assert.Equal(t, 5, suggestions.Stopping.Rides)
}

// A habit is the rider's own: another target's rides are not read, however many
// of them there are.
func TestRiderSuggestionsNeverPoolStoppingAcrossRiders(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	for index, seconds := range []float64{100, 200, 300, 400, 500} {
		storeStoppingRide(t, store, "rider-b", int64(index+1), 15, seconds)
	}

	suggestions, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a"}, outdoorTypes(), activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.Stopping.Set, "another rider's rides are not this rider's habit")
}

// An indoor trainer stands still without stopping, so its type is not eligible
// and its rides never reach the habit.
func TestRiderSuggestionsReadStoppingOnlyFromEligibleTypes(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	for index, seconds := range []float64{100, 200, 300, 400, 500} {
		storeStoppingRide(t, store, "rider-a", int64(index+1), 61, seconds)
	}

	suggestions, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a"}, outdoorTypes(), activityNow().Add(-time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.Stopping.Set, "an indoor ride carries no stopping habit")
}

func TestRiderSuggestionsReadStoppingOnlySinceTheCutoff(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	for index, seconds := range []float64{100, 200, 300, 400, 500} {
		storeStoppingRide(t, store, "rider-a", int64(index+1), 15, seconds)
	}

	suggestions, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a"}, outdoorTypes(), activityNow().Add(time.Hour))
	require.NoError(t, err, "RiderSuggestions()")
	assert.False(t, suggestions.Stopping.Set, "rides before the cutoff are not read")
}

func TestRiderSuggestionsReportAnUnreadableStoppingCorpus(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, err := store.RiderSuggestions(
		t.Context(), []string{"rider-a"}, outdoorTypes(), activityNow())
	require.ErrorContains(t, err, "reading the recorded ride summaries")
}

// The poll reads the two Zwift names alone, and a rider who has entered
// neither has an empty pair rather than an error.
func TestRiderZwiftCredentialsReadsBothNames(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	email, password, err := store.RiderZwiftCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderZwiftCredentials()")
	assert.Empty(t, email, "an unentered email")
	assert.Empty(t, password, "an unentered password")

	require.NoError(t, store.SetRiderCredentials(t.Context(), "rider-a", map[rider.CredentialName]rider.Credential{
		rider.CredentialZwiftEmail:    rider.NewCredential([]byte("rider@example.test")),
		rider.CredentialZwiftPassword: rider.NewCredential([]byte("hunter2")),
	}), "SetRiderCredentials()")

	email, password, err = store.RiderZwiftCredentials(t.Context(), "rider-a")
	require.NoError(t, err, "RiderZwiftCredentials()")
	assert.Equal(t, []byte("rider@example.test"), email)
	assert.Equal(t, []byte("hunter2"), password)
}
