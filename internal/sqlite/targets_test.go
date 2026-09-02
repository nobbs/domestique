package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A subject's first "Connect" creates their target, owned by their own value,
// waiting for its one-time OAuth onboarding.
func TestEnsureTargetOwnerCreatesANotAuthorizedRecordForANewSubject(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	target, err := store.Target(t.Context(), "rider-a")
	require.NoError(t, err, "Target()")
	assert.Equal(t, "rider-a", target.OwnerSubject, "the target is owned by the subject that created it")
	assert.Equal(t, AuthorizationNotAuthorized, target.AuthorizationState, "a new target waits for its onboarding")
}

// A rider's tenth "Connect" click is as safe as their first: an existing
// target is left exactly as it was, authorization and all.
func TestEnsureTargetOwnerLeavesAnExistingTargetAlone(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")

	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner() again")

	token, err := store.RefreshToken(t.Context(), "rider-a")
	require.NoError(t, err, "RefreshToken()")
	assert.Equal(t, "refresh-token", token, "the target kept its authorization")
}

// A blank subject names no one to own the target, so it is refused rather
// than silently creating an unowned row.
func TestEnsureTargetOwnerRejectsABlankSubject(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.Error(t, store.EnsureTargetOwner(t.Context(), "   "), "EnsureTargetOwner() with a blank subject")
}

// A query failure is reported rather than swallowed.
func TestEnsureTargetOwnerReportsAQueryFailure(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner() over a closed store")
}
