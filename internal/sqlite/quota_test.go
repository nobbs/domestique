package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreWahooQuotaRoundTrips(t *testing.T) {
	store := openTestStore(t, testKey(1))
	observedAt := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	quota := WahooQuota{
		ObservedAt: observedAt,
		ExpiresAt:  observedAt.Add(5 * time.Minute),
		ResetAt:    observedAt.Add(2 * time.Minute),
		NotBefore:  observedAt.Add(2 * time.Minute),
		Remaining:  17,
	}

	require.NoError(t, store.StoreWahooQuota(t.Context(), &quota), "StoreWahooQuota()")
	stored, found, err := store.WahooQuota(t.Context())
	require.NoError(t, err, "WahooQuota()")
	require.True(t, found, "the quota that was stored is not readable")
	assert.Equal(t, quota, stored)
}

// Absent is the ordinary state of a service no response has ever answered.
func TestStoreWahooQuotaIsAbsentUntilOneIsStored(t *testing.T) {
	store := openTestStore(t, testKey(1))

	_, found, err := store.WahooQuota(t.Context())
	require.NoError(t, err, "WahooQuota()")
	assert.False(t, found, "a quota was reported before one was observed")
}

// Wahoo names neither instant on most responses, and an unnamed one must come
// back unnamed rather than as 1970.
func TestStoreWahooQuotaKeepsAnUnnamedInstantAbsent(t *testing.T) {
	store := openTestStore(t, testKey(1))
	observedAt := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)

	require.NoError(t, store.StoreWahooQuota(t.Context(), &WahooQuota{
		ObservedAt: observedAt, ExpiresAt: observedAt.Add(5 * time.Minute), Remaining: 42,
	}), "StoreWahooQuota()")

	stored, found, err := store.WahooQuota(t.Context())
	require.NoError(t, err, "WahooQuota()")
	require.True(t, found)
	assert.True(t, stored.ResetAt.IsZero(), "an unnamed reset came back as an instant")
	assert.True(t, stored.NotBefore.IsZero(), "an unnamed notBefore came back as an instant")
}

// One row, replaced: the newest reading is the only one worth keeping.
func TestStoreWahooQuotaReplacesTheStoredReading(t *testing.T) {
	store := openTestStore(t, testKey(1))
	observedAt := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	first := WahooQuota{ObservedAt: observedAt, ExpiresAt: observedAt.Add(5 * time.Minute), Remaining: 42}
	second := WahooQuota{
		ObservedAt: observedAt.Add(time.Minute),
		ExpiresAt:  observedAt.Add(6 * time.Minute),
		Remaining:  41,
	}

	require.NoError(t, store.StoreWahooQuota(t.Context(), &first), "StoreWahooQuota() first")
	require.NoError(t, store.StoreWahooQuota(t.Context(), &second), "StoreWahooQuota() second")

	stored, found, err := store.WahooQuota(t.Context())
	require.NoError(t, err, "WahooQuota()")
	require.True(t, found)
	assert.Equal(t, second, stored)
}

func TestStoreWahooQuotaRefusesAReadingWithoutItsInstants(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.Error(t, store.StoreWahooQuota(t.Context(), &WahooQuota{Remaining: 42}),
		"StoreWahooQuota() accepted a reading with neither instant")
}

func TestStoreWahooQuotaReportsAnUnreadableDatabase(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, _, err := store.WahooQuota(t.Context())
	require.Error(t, err, "WahooQuota() on a closed database")
	require.Error(t, store.StoreWahooQuota(t.Context(), &WahooQuota{
		ObservedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
	}), "StoreWahooQuota() on a closed database")
}
