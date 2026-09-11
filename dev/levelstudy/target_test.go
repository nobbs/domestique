package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/sqlite"
)

func targetTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	var key [32]byte
	for index := range key {
		key[index] = byte(index)
	}
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"), key)
	require.NoError(t, err, "sqlite.Open()")
	t.Cleanup(func() { assert.NoError(t, store.Close(), "Close()") })

	return store
}

// The regression: the offline study must cap heart rate at a rider's own
// maximum before modelling it, the same as live derivation does, or a strap
// spike above it can move the fitted bridge and the reported error. This
// covers where that ceiling comes from; measure.CapHeartRate's own behaviour
// at it is covered in internal/measure.
func TestMaxHeartRateForTargetReadsTheRidersOwnMaximum(t *testing.T) {
	t.Parallel()
	store := targetTestStore(t)
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.SetRiderProfile(t.Context(), "rider-a", rider.Profile{
		MaxHeartRateBPM: rider.Set(188),
	}), "SetRiderProfile()")

	maxHeartRate, err := maxHeartRateForTarget(t.Context(), store, "rider-a", map[string]float64{})

	require.NoError(t, err)
	assert.InDelta(t, 188, maxHeartRate, 1e-9)
}

// A target with no rider profile at all -- an unowned slot, or one nobody
// has entered a maximum for -- yields zero, which measure.CapHeartRate reads
// as no ceiling to cap against: the same as a rider who never set one sees live.
func TestMaxHeartRateForTargetIsZeroWithoutAProfile(t *testing.T) {
	t.Parallel()
	store := targetTestStore(t)
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	maxHeartRate, err := maxHeartRateForTarget(t.Context(), store, "rider-a", map[string]float64{})

	require.NoError(t, err)
	assert.Zero(t, maxHeartRate)
}

// The regression: a heart-rate spike above the rider's own maximum must be
// capped, not modelled as though the rider's heart really reached it.
func TestOfflineHeartRateIsCappedTheSameWayLiveDerivationCapsIt(t *testing.T) {
	t.Parallel()
	const maxHeartRateBPM = 190.0
	raw := []measure.Reading{
		{At: start(), Value: 150},
		{At: start().Add(time.Second), Value: 240}, // a strap spike above the rider's own maximum
		{At: start().Add(2 * time.Second), Value: 150},
	}

	capped := measure.CapHeartRate(raw, maxHeartRateBPM)

	assert.LessOrEqual(t, capped[1].Value, maxHeartRateBPM,
		"a spike above the rider's own maximum must not reach the model as read")
}
