package oauth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceCompletesBoundAuthorization(t *testing.T) {
	store := &fakeStateStore{}
	wahoo := &fakeWahoo{accessToken: "access-token", refreshToken: "refresh-token", userID: "wahoo-user"}
	service, err := New(store, wahoo)
	require.NoError(t, err)

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	require.NoError(t, err)
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")
	require.NotEqualf(t, url, state, "Start() URL = %q, want an authorization state", url)

	require.NoError(t, service.Complete(t.Context(), "rider@example.ts.net", state, "authorization-code"))
	assert.Equal(t, "rider-a", store.authorizedTarget)
	assert.Equal(t, "wahoo-user", store.authorizedUser)
	assert.Equal(t, "refresh-token", store.refreshToken)
}

func TestServiceRejectsInvalidOrReusedAuthorization(t *testing.T) {
	store := &fakeStateStore{}
	service, err := New(store, &fakeWahoo{accessToken: "access", refreshToken: "refresh", userID: "wahoo-user"})
	require.NoError(t, err)

	require.ErrorIs(t,
		service.Complete(t.Context(), "rider@example.ts.net", "not-base64", "code"),
		ErrInvalidAuthorization, "a state that is not even base64")

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	require.NoError(t, err)
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")

	require.ErrorIs(t,
		service.Complete(t.Context(), "other@example.ts.net", state, "code"),
		ErrInvalidAuthorization, "a different caller completing the authorization")
	require.NoError(t,
		service.Complete(t.Context(), "rider@example.ts.net", state, "code"),
		"the original caller")
	require.ErrorIs(t,
		service.Complete(t.Context(), "rider@example.ts.net", state, "code"),
		ErrInvalidAuthorization, "reusing the state")
}

func TestServiceHidesUpstreamFailure(t *testing.T) {
	store := &fakeStateStore{}
	service, err := New(store, &fakeWahoo{exchangeErr: errors.New("code=private-code")})
	require.NoError(t, err)

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	require.NoError(t, err)
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")

	err = service.Complete(t.Context(), "rider@example.ts.net", state, "code")
	require.ErrorIs(t, err, ErrAuthorizationFailed)
	assert.NotContains(t, err.Error(), "private-code", "Complete() exposed the upstream error")
}

type fakeStateStore struct {
	expiresAt        time.Time
	authorizedTarget string
	authorizedUser   string
	refreshToken     string
	targetID         string
	caller           string
	digest           []byte
	used             bool
}

func (s *fakeStateStore) BeginAuthorization(
	_ context.Context,
	targetID, callerLogin string,
	stateDigest []byte,
	expiresAt time.Time,
) error {
	s.digest = append([]byte(nil), stateDigest...)
	s.targetID = targetID
	s.caller = callerLogin
	s.expiresAt = expiresAt

	return nil
}

func (s *fakeStateStore) ConsumeAuthorization(_ context.Context, callerLogin string, stateDigest []byte) (string, error) {
	if s.used || !s.expiresAt.After(time.Now()) {
		return "", errors.New("state was unavailable")
	}
	if callerLogin != s.caller || !bytes.Equal(stateDigest, s.digest) {
		return "", errors.New("state did not match")
	}
	s.used = true

	return s.targetID, nil
}

func (s *fakeStateStore) AuthorizeTarget(_ context.Context, targetID, wahooUserID, refreshToken string) error {
	s.authorizedTarget = targetID
	s.authorizedUser = wahooUserID
	s.refreshToken = refreshToken

	return nil
}

type fakeWahoo struct {
	exchangeErr  error
	accessToken  string
	refreshToken string
	userID       string
}

func (w *fakeWahoo) AuthorizationURL(state string) (string, error) {
	return "https://wahoo.example.test/authorize?state=" + state, nil
}

func (w *fakeWahoo) ExchangeAuthorizationCode(_ context.Context, _ string) (accessToken, refreshToken string, err error) {
	if w.exchangeErr != nil {
		return "", "", w.exchangeErr
	}

	return w.accessToken, w.refreshToken, nil
}

func (w *fakeWahoo) AuthenticatedUser(_ context.Context, _ string) (string, error) {
	return w.userID, nil
}
