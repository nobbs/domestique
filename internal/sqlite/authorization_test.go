package sqlite

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreAuthorizesAndEncryptsRefreshToken(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")

	target, err := store.Target(t.Context(), "rider-a")
	require.NoError(t, err, "Target()")
	assert.Equal(t, AuthorizationAuthorized, target.AuthorizationState, "Target().AuthorizationState")
	assert.Equal(t, "wahoo-user", target.WahooUserID, "Target().WahooUserID")

	var encrypted []byte
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted), "query encrypted token")
	assert.NotContains(t, string(encrypted), "refresh-token", "the database stores the refresh token in plaintext")

	got, err := store.RefreshToken(t.Context(), "rider-a")
	require.NoError(t, err, "RefreshToken()")
	assert.Equal(t, "refresh-token", got, "RefreshToken()")
}

func TestStoreRejectsDuplicateWahooUser(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "token-a"), "AuthorizeTarget(rider-a)")
	require.ErrorIs(t, store.AuthorizeTarget(t.Context(), "rider-b", "wahoo-user", "token-b"), ErrWahooUserAlreadyAuthorized, "AuthorizeTarget(rider-b)")
}

func TestStoreBindsTokenToTarget(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user-a", "token-a"), "AuthorizeTarget()")

	var encrypted []byte
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted), "query encrypted token")
	_, err := store.database.ExecContext(t.Context(), "UPDATE targets SET refresh_token = ? WHERE slot = ?", encrypted, "rider-b")
	require.NoError(t, err, "copy encrypted token")

	_, err = store.RefreshToken(t.Context(), "rider-b")
	require.ErrorIs(t, err, ErrStateUnreadable, "RefreshToken()")
}

func TestStoreRejectsDifferentEncryptionKey(t *testing.T) {
	t.Parallel()
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, openErr := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, openErr, "Open()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")
	require.NoError(t, store.Close(), "Close()")

	reopened, err := Open(t.Context(), databasePath, testKey(2))
	require.NoError(t, err, "Open() with different key")
	t.Cleanup(func() {
		assert.NoError(t, reopened.Close(), "Close()")
	})
	_, err = reopened.RefreshToken(t.Context(), "rider-a")
	require.ErrorIs(t, err, ErrStateUnreadable, "RefreshToken()")
}

func TestStoreMarksTargetForReauthorization(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")
	require.NoError(t, store.MarkNeedsReauthorization(t.Context(), "rider-a"), "MarkNeedsReauthorization()")

	target, err := store.Target(t.Context(), "rider-a")
	require.NoError(t, err, "Target()")
	assert.Equal(t, AuthorizationNeedsReauthorization, target.AuthorizationState, "Target().AuthorizationState")
	_, err = store.RefreshToken(t.Context(), "rider-a")
	require.ErrorIs(t, err, ErrRefreshTokenUnavailable, "RefreshToken()")
}

func TestStoreReplacesRefreshToken(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "old-refresh-token"), "AuthorizeTarget()")
	require.NoError(t, store.ReplaceRefreshToken(t.Context(), "rider-a", "new-refresh-token"), "ReplaceRefreshToken()")

	got, err := store.RefreshToken(t.Context(), "rider-a")
	require.NoError(t, err, "RefreshToken()")
	assert.Equal(t, "new-refresh-token", got, "RefreshToken()")
}

func TestStoreConsumesCallerBoundOAuthAuthorization(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	digest := bytes.Repeat([]byte{1}, 32)
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		digest,
		time.Now().Add(time.Minute),
	), "BeginAuthorization()")

	_, err := store.ConsumeAuthorization(t.Context(), "other@example.ts.net", digest)
	require.ErrorIs(t, err, ErrOAuthTransactionIdentityMismatch, "ConsumeAuthorization() with another caller")
	targetID, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	require.NoError(t, err, "ConsumeAuthorization()")
	assert.Equal(t, "rider-a", targetID, "ConsumeAuthorization() target")
	_, err = store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	require.ErrorIs(t, err, ErrOAuthTransactionUsed, "ConsumeAuthorization() after use")
}

func TestStoreRejectsExpiredOAuthAuthorization(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	digest := bytes.Repeat([]byte{2}, 32)
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		digest,
		time.Now().Add(time.Minute),
	), "BeginAuthorization()")
	_, err := store.database.ExecContext(
		t.Context(),
		"UPDATE oauth_transactions SET expires_at_unix = ? WHERE state_digest = ?",
		time.Now().Add(-time.Second).Unix(),
		digest,
	)
	require.NoError(t, err, "expiring OAuth authorization")

	_, err = store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	require.ErrorIs(t, err, ErrOAuthTransactionExpired, "ConsumeAuthorization()")
}

func TestStoreReportsOnlyLiveAuthorizationsAsPending(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-c"), "EnsureTargetOwner()")
	begin := func(targetID string, digestByte byte) []byte {
		digest := bytes.Repeat([]byte{digestByte}, 32)
		require.NoError(t, store.BeginAuthorization(
			t.Context(),
			targetID,
			"rider@example.ts.net",
			digest,
			time.Now().Add(time.Minute),
		), "BeginAuthorization()")

		return digest
	}
	begin("rider-a", 3)
	consumed := begin("rider-b", 4)
	expired := begin("rider-c", 5)
	_, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", consumed)
	require.NoError(t, err, "ConsumeAuthorization()")
	_, err = store.database.ExecContext(
		t.Context(),
		"UPDATE oauth_transactions SET expires_at_unix = ? WHERE state_digest = ?",
		time.Now().Add(-time.Second).Unix(),
		expired,
	)
	require.NoError(t, err, "expiring OAuth authorization")

	var pending []string
	require.NoError(t, store.ForEachPendingAuthorization(t.Context(), func(targetID string) error {
		pending = append(pending, targetID)

		return nil
	}), "ForEachPendingAuthorization()")

	// A flow that was completed and one that ran out of time are both over, and
	// a slot waiting on neither must not read as one an operator is midway
	// through connecting.
	assert.Equal(t, []string{"rider-a"}, pending, "pending target slots")
}

// The visitor is this method's entire output, so a caller that supplied none and
// a visitor that fails partway both have to be answered rather than iterated
// past: the status view reads a slot as pending on the strength of a visit, and
// a swallowed failure would report a half-read table as a whole one.
func TestStoreStopsReadingPendingAuthorizationsOnVisitorFailure(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		bytes.Repeat([]byte{6}, 32),
		time.Now().Add(time.Minute),
	), "BeginAuthorization()")

	require.Error(
		t,
		store.ForEachPendingAuthorization(t.Context(), nil),
		"ForEachPendingAuthorization() without a visitor",
	)

	visitErr := errors.New("visiting pending authorization")
	assert.ErrorIs(t, store.ForEachPendingAuthorization(t.Context(), func(string) error {
		return visitErr
	}), visitErr, "ForEachPendingAuthorization() with a failing visitor")
}
