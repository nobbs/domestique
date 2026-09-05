package oauth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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

// All four refusals reach the caller as one of two sentinels, so the log line is
// the only thing that says which step refused — an expired state, a rejected
// client secret, and an account already bound to another slot are one answer.
func TestServiceNamesTheStepThatRefused(t *testing.T) {
	authorized := func() *fakeWahoo {
		return &fakeWahoo{accessToken: "access", refreshToken: "refresh", userID: "wahoo-user"}
	}
	tests := map[string]struct {
		store  *fakeStateStore
		wahoo  *fakeWahoo
		reason string
	}{
		"state spent or expired": {
			store: &fakeStateStore{used: true}, wahoo: authorized(), reason: "state_not_consumed",
		},
		"wahoo refused the code": {
			store: &fakeStateStore{}, wahoo: &fakeWahoo{exchangeErr: errors.New("rejected")},
			reason: "code_exchange_failed",
		},
		"wahoo named no user": {
			store: &fakeStateStore{}, wahoo: &fakeWahoo{accessToken: "access", refreshToken: "refresh"},
			reason: "wahoo_user_unknown",
		},
		"slot would not bind": {
			store: &fakeStateStore{authorizeErr: errors.New("already authorized")}, wahoo: authorized(),
			reason: "target_not_bound",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			logged := captureLogs(t)
			service, err := New(test.store, test.wahoo)
			require.NoError(t, err)

			url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
			require.NoError(t, err)
			state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")

			require.Error(t, service.Complete(t.Context(), "rider@example.ts.net", state, "code"))
			assert.Contains(t, logged.String(), "reason="+test.reason, "logged refusal")
		})
	}
}

// A callback missing its code is not a callback with an unreadable state, and
// the reason says which — matching the name the HTTP handler gives the same
// cause, so one taxonomy covers both entry points.
func TestServiceNamesAnUnusableCallback(t *testing.T) {
	for name, test := range map[string]struct{ state, code, reason string }{
		"state is missing":    {state: "", code: "code", reason: "callback_state_missing"},
		"state is not base64": {state: "not-base64", code: "code", reason: "callback_state_unusable"},
		"state is short":      {state: "c2hvcnQ", code: "code", reason: "callback_state_unusable"},
	} {
		t.Run(name, func(t *testing.T) {
			logged := captureLogs(t)
			service, err := New(&fakeStateStore{}, &fakeWahoo{})
			require.NoError(t, err)

			require.ErrorIs(t,
				service.Complete(t.Context(), "rider@example.ts.net", test.state, test.code),
				ErrInvalidAuthorization)
			assert.Contains(t, logged.String(), "reason="+test.reason, "logged refusal")
		})
	}
}

// The state decodes, so only the missing code can refuse it.
func TestServiceNamesACallbackMissingItsCode(t *testing.T) {
	logged := captureLogs(t)
	store := &fakeStateStore{}
	service, err := New(store, &fakeWahoo{})
	require.NoError(t, err)

	url, err := service.Start(t.Context(), "rider@example.ts.net", "rider-a")
	require.NoError(t, err)
	state := strings.TrimPrefix(url, "https://wahoo.example.test/authorize?state=")

	require.ErrorIs(t,
		service.Complete(t.Context(), "rider@example.ts.net", state, ""), ErrInvalidAuthorization)
	assert.Contains(t, logged.String(), "reason=callback_code_missing", "logged refusal")
	assert.False(t, store.used, "an empty code still spent the one-time state")
}

// captureLogs redirects the default logger for one test. Package-level state, so
// these tests must not run in parallel.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return buffer
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
	authorizeErr     error
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
	if s.authorizeErr != nil {
		return s.authorizeErr
	}
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
