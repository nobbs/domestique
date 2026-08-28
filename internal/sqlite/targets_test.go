package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slot gets its durable record on the save that names it, so the OAuth
// onboarding that follows has a row to authorize without a restart in between.
func TestStoreCreatesATargetRecordForANewlyNamedSlot(t *testing.T) {
	store := openTestStore(t, testKey(1))
	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	values.Wahoo.Targets = []string{"rider-a"}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings()")

	target, err := store.Target(t.Context(), "rider-a")
	require.NoError(t, err, "Target()")
	assert.Equal(t, AuthorizationNotAuthorized, target.AuthorizationState, "a new slot waits for its onboarding")
}

// A slot taken out of the list keeps what it was authorized with: that is what
// a slot named back would want, and nothing reads a record whose slot is gone.
func TestStoreKeepsTheAuthorizationOfASlotThatWasRemoved(t *testing.T) {
	store := openTestStore(t, testKey(1))
	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	values.Wahoo.Targets = []string{"rider-a"}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")

	values.Wahoo.Targets = nil
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings() without the slot")

	values.Wahoo.Targets = []string{"rider-a"}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings() with the slot back")

	token, err := store.RefreshToken(t.Context(), "rider-a")
	require.NoError(t, err, "RefreshToken()")
	assert.Equal(t, "refresh-token", token, "the slot is authorized as it was")
}
