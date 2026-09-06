package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Nothing is seeded: a service nobody has calibrated predicts with the built-in
// pair, which the composition root substitutes rather than the store.
func TestRideModelCoefficientsAreAbsentUntilACalibrationStoresThem(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	_, found, err := store.RideModelCoefficients(t.Context())
	require.NoError(t, err, "RideModelCoefficients()")
	assert.False(t, found, "no calibration has run yet")
}

func TestStoreRideModelCoefficientsRoundTripsAndReplaces(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	now := time.Unix(1700000000, 0)
	fitted := ridemodel.Coefficients{
		CalibrationCutoff: "2025-08-01",
		SecondsPerKM:      145.3578,
		SecondsPerAscentM: 3.2190,
		EvaluatedRides:    42,
		BiasPercent:       -1.2,
		MAEPercent:        6.8,
		P90Percent:        14.1,

		TrainingWindowMonths: 12,
	}
	require.NoError(t, store.StoreRideModelCoefficients(t.Context(), fitted, now))

	read, found, err := store.RideModelCoefficients(t.Context())
	require.NoError(t, err, "RideModelCoefficients()")
	require.True(t, found)
	assert.Equal(t, fitted.WithFingerprint(), read, "every field must survive the round trip")

	// The row is a singleton: a second calibration replaces it rather than
	// leaving two pairs for a reader to choose between.
	replacement := fitted
	replacement.SecondsPerKM, replacement.CalibrationCutoff = 150.0, "2026-02-14"
	require.NoError(t, store.StoreRideModelCoefficients(t.Context(), replacement, now.Add(time.Hour)))

	read, _, err = store.RideModelCoefficients(t.Context())
	require.NoError(t, err)
	assert.Equal(t, replacement.WithFingerprint(), read, "the later calibration must be the one in force")
}

// The built-in pair carries no cutoff, so the column has to accept its absence
// and read back as absence rather than as the Unix epoch.
func TestStoreRideModelCoefficientsKeepsAnAbsentCutoffAbsent(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.StoreRideModelCoefficients(t.Context(), ridemodel.Default(), time.Unix(1700000000, 0)))

	read, found, err := store.RideModelCoefficients(t.Context())
	require.NoError(t, err)
	require.True(t, found)
	assert.Empty(t, read.CalibrationCutoff, "a pair fitted from no dated corpus records no cutoff")
	assert.False(t, read.HasValidation(), "the built-in pair has never been measured")
}

func TestStoreRideModelCoefficientsRejectsAMalformedCutoff(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	coefficients := ridemodel.Default()
	coefficients.CalibrationCutoff = "the first of August"

	require.Error(t, store.StoreRideModelCoefficients(t.Context(), coefficients, time.Unix(1700000000, 0)))
}

func TestStoreRideModelCoefficientsRefusesAnImplausiblePair(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	bad := ridemodel.Default()
	bad.SecondsPerKM = 0
	require.ErrorContains(t, store.StoreRideModelCoefficients(t.Context(), bad, time.Unix(1, 0)), "refusing")
}
