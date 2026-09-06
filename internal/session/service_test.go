package session

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeginStoresDigestAndPassesStateToProvider(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{access: true}
	clock := newFakeClock()
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)

	require.Len(t, store.logins, 1)
	var digestKey string
	for d := range store.logins {
		digestKey = d
	}
	assert.NotEqual(t, digestKey, login.State, "store should hold a digest, not the raw state")
	sum := sha256.Sum256(mustDecode(t, login.State))
	assert.Equal(t, string(sum[:]), digestKey)
	assert.Len(t, store.logins[digestKey].nonce, 43)
	assert.Len(t, store.logins[digestKey].verifier, 43)
	assert.Equal(t, login.State, provider.gotState)
	assert.Equal(t, store.logins[digestKey].nonce, provider.gotNonce)
	assert.Equal(t, store.logins[digestKey].verifier, provider.gotVerifier)
}

func TestBeginStoresNothingWhenTheAuthorizationURLFails(t *testing.T) {
	store := newFakeStore()
	service, err := New(store, &fakeProvider{authURLErr: errors.New("issuer misconfigured")}, newFakeClock().now)
	require.NoError(t, err)

	_, err = service.Begin(t.Context())
	require.Error(t, err)
	assert.Empty(t, store.logins, "a login nobody can return for must not be left behind")
}

func TestCompleteRejectsMismatchedCookieState(t *testing.T) {
	store := newFakeStore()
	service, err := New(store, &fakeProvider{}, newFakeClock().now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)

	_, err = service.Complete(t.Context(), login.State, "not-the-same-state-value-1234567890", "code")
	require.Error(t, err)
	assert.Zero(t, store.consumeCalls)
}

// TestCompleteRejectsADifferentValidState pairs two well-formed states, so the
// rejection comes from the comparison rather than from decoding.
func TestCompleteRejectsADifferentValidState(t *testing.T) {
	store := newFakeStore()
	service, err := New(store, &fakeProvider{}, newFakeClock().now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	other, err := service.Begin(t.Context())
	require.NoError(t, err)
	require.Len(t, login.State, len(other.State), "both states must be the same length")

	_, err = service.Complete(t.Context(), login.State, other.State, "code")
	require.Error(t, err)
	assert.Zero(t, store.consumeCalls)
}

func TestCompleteHappyPath(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{subject: "rider", email: "rider@example.ts.net", access: true}
	clock := newFakeClock()
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	wantNonce, wantVerifier := provider.gotNonce, provider.gotVerifier

	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)
	assert.Equal(t, 1, store.consumeCalls)
	assert.Equal(t, wantNonce, provider.exchangedNonce, "Complete should exchange with the stored nonce")
	assert.Equal(t, wantVerifier, provider.exchangedVerifier, "Complete should exchange with the stored verifier")
	assert.Equal(t, "rider", completion.Identity.Subject)
	assert.Equal(t, "rider@example.ts.net", completion.Identity.Display)
	assert.False(t, completion.Identity.Admin)

	sum := sha256.Sum256(mustDecode(t, completion.Token))
	session, ok := store.sessions[string(sum[:])]
	require.True(t, ok, "store should hold the completion token's digest")
	assert.Equal(t, "rider", session.subject)
}

// The admin claim round-trips through CreateSession and back out of Session,
// not just through the value Complete happens to return in the same call.
func TestCompleteAndVerifyRoundTripTheAdminClaim(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{subject: "rider", access: true, admin: true}
	clock := newFakeClock()
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)
	assert.True(t, completion.Identity.Admin)

	identity, err := service.Verify(t.Context(), completion.Token)
	require.NoError(t, err)
	assert.True(t, identity.Admin)
}

func TestCompleteDisplayFallsBackToNameThenSubject(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()

	provider := &fakeProvider{subject: "rider", name: "Rider Name", access: true}
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)
	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)
	assert.Equal(t, "Rider Name", completion.Identity.Display)

	provider2 := &fakeProvider{subject: "rider", access: true}
	service2, err := New(store, provider2, clock.now)
	require.NoError(t, err)
	login2, err := service2.Begin(t.Context())
	require.NoError(t, err)
	completion2, err := service2.Complete(t.Context(), login2.State, login2.State, "code")
	require.NoError(t, err)
	assert.Equal(t, "rider", completion2.Identity.Display)
}

// The nickname claim round-trips through CreateSession and back out of
// Session, and a token without one leaves it empty rather than falling back
// to display or subject the way Display does.
func TestCompleteAndVerifyRoundTripTheNickname(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{subject: "rider", nickname: "Rider", access: true}
	clock := newFakeClock()
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)
	assert.Equal(t, "Rider", completion.Identity.Nickname)

	identity, err := service.Verify(t.Context(), completion.Token)
	require.NoError(t, err)
	assert.Equal(t, "Rider", identity.Nickname)
}

func TestCompleteWithoutNicknameStoresEmpty(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{subject: "rider", access: true}
	clock := newFakeClock()
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)
	assert.Empty(t, completion.Identity.Nickname)

	identity, err := service.Verify(t.Context(), completion.Token)
	require.NoError(t, err)
	assert.Empty(t, identity.Nickname)
}

func TestCompleteReuseOfStateFails(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	service, err := New(store, &fakeProvider{subject: "rider", access: true}, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	_, err = service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)

	_, err = service.Complete(t.Context(), login.State, login.State, "code")
	require.Error(t, err, "reusing a consumed state must fail")
}

func TestCompleteExpiredLoginFails(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	service, err := New(store, &fakeProvider{subject: "rider", access: true}, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	clock.advance(loginLifetime + time.Second)

	_, err = service.Complete(t.Context(), login.State, login.State, "code")
	require.Error(t, err)
}

func TestCompleteProviderErrorCreatesNoSession(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	provider := &fakeProvider{exchangeErr: errors.New("nonce mismatch")}
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	_, err = service.Complete(t.Context(), login.State, login.State, "code")
	require.Error(t, err)
	assert.Empty(t, store.sessions)
}

// A subject Auth0 authenticated but did not assert the access claim for is
// refused, whatever its own display name would have been.
func TestCompleteRefusesASubjectWithoutTheAccessClaim(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	provider := &fakeProvider{subject: "intruder", access: false}
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	_, err = service.Complete(t.Context(), login.State, login.State, "code")
	require.Error(t, err)

	var notAllowed *NotAllowedError
	require.ErrorAs(t, err, &notAllowed)
	assert.Equal(t, "intruder", notAllowed.Subject)
	assert.NotContains(t, err.Error(), "intruder")
	assert.Empty(t, store.sessions)
}

func TestVerifyRejectsGarbageAndUnknownTokens(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	service, err := New(store, &fakeProvider{}, clock.now)
	require.NoError(t, err)

	_, err = service.Verify(t.Context(), "not-valid-base64!!")
	require.Error(t, err)
	assert.Zero(t, store.sessionCalls, "garbage token must not reach the store")

	_, err = service.Verify(t.Context(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	require.Error(t, err, "unknown token")
}

func TestRevokeDeletesSession(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	provider := &fakeProvider{subject: "rider", email: "rider@example.ts.net", access: true}
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)

	require.NoError(t, service.Revoke(t.Context(), completion.Token))
	_, err = service.Verify(t.Context(), completion.Token)
	require.Error(t, err)
}

func TestNewDefaultsClockToTimeNow(t *testing.T) {
	store := newFakeStore()
	service, err := New(store, &fakeProvider{}, nil)
	require.NoError(t, err)

	before := time.Now()
	_, err = service.Begin(t.Context())
	require.NoError(t, err)

	require.Len(t, store.logins, 1)
	for _, login := range store.logins {
		assert.WithinDuration(t, before.Add(loginLifetime), login.expiresAt, time.Minute, "Begin() should stamp expiry from time.Now")
	}
}

func TestBeginPropagatesStoreError(t *testing.T) {
	store := newFakeStore()
	store.beginLoginErr = errors.New("disk full")
	service, err := New(store, &fakeProvider{}, newFakeClock().now)
	require.NoError(t, err)

	_, err = service.Begin(t.Context())
	require.Error(t, err)
}

func TestBeginPropagatesProviderError(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{authURLErr: errors.New("issuer unreachable")}
	service, err := New(store, provider, newFakeClock().now)
	require.NoError(t, err)

	_, err = service.Begin(t.Context())
	require.Error(t, err)
}

func TestCompleteRejectsInvalidState(t *testing.T) {
	store := newFakeStore()
	service, err := New(store, &fakeProvider{}, newFakeClock().now)
	require.NoError(t, err)

	_, err = service.Complete(t.Context(), "not-valid-base64!!", "not-valid-base64!!", "code")
	require.Error(t, err)
	assert.Zero(t, store.consumeCalls, "an invalid state must not reach ConsumeLogin")
}

func TestCompleteCreateSessionErrorPropagates(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{subject: "rider", access: true}
	service, err := New(store, provider, newFakeClock().now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	store.createSessionErr = errors.New("disk full")

	_, err = service.Complete(t.Context(), login.State, login.State, "code")
	require.Error(t, err)
}

func TestRevokeRejectsGarbageTokenWithoutStoreCall(t *testing.T) {
	store := newFakeStore()
	service, err := New(store, &fakeProvider{}, newFakeClock().now)
	require.NoError(t, err)

	err = service.Revoke(t.Context(), "not-valid-base64!!")
	require.Error(t, err)
	assert.Zero(t, store.deleteCalls, "a garbage token must not reach the store")
}

func TestRevokePropagatesStoreError(t *testing.T) {
	store := newFakeStore()
	clock := newFakeClock()
	provider := &fakeProvider{subject: "rider", access: true}
	service, err := New(store, provider, clock.now)
	require.NoError(t, err)

	login, err := service.Begin(t.Context())
	require.NoError(t, err)
	completion, err := service.Complete(t.Context(), login.State, login.State, "code")
	require.NoError(t, err)

	store.deleteSessionErr = errors.New("disk full")
	err = service.Revoke(t.Context(), completion.Token)
	require.Error(t, err)
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	store := newFakeStore()
	provider := &fakeProvider{}

	_, err := New(nil, provider, newFakeClock().now)
	require.Error(t, err)

	_, err = New(store, nil, newFakeClock().now)
	require.Error(t, err)
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(s)
	require.NoError(t, err)
	return raw
}

type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

type fakeLogin struct {
	expiresAt       time.Time
	nonce, verifier string
	consumed        bool
}

type fakeSession struct {
	expiresAt                  time.Time
	subject, display, nickname string
	admin                      bool
}

type fakeStore struct {
	logins           map[string]*fakeLogin
	sessions         map[string]*fakeSession
	beginLoginErr    error
	createSessionErr error
	deleteSessionErr error
	consumeCalls     int
	sessionCalls     int
	deleteCalls      int
}

func newFakeStore() *fakeStore {
	return &fakeStore{logins: map[string]*fakeLogin{}, sessions: map[string]*fakeSession{}}
}

func (f *fakeStore) BeginLogin(_ context.Context, stateDigest []byte, nonce, codeVerifier string, _, expiresAt time.Time) error {
	if f.beginLoginErr != nil {
		return f.beginLoginErr
	}
	f.logins[string(stateDigest)] = &fakeLogin{nonce: nonce, verifier: codeVerifier, expiresAt: expiresAt}
	return nil
}

func (f *fakeStore) ConsumeLogin(_ context.Context, stateDigest []byte, now time.Time) (nonce, codeVerifier string, err error) {
	f.consumeCalls++
	login, ok := f.logins[string(stateDigest)]
	if !ok || login.consumed {
		return "", "", errors.New("login not found")
	}
	login.consumed = true
	if !login.expiresAt.After(now) {
		return "", "", errors.New("login expired")
	}
	return login.nonce, login.verifier, nil
}

func (f *fakeStore) CreateSession(
	_ context.Context, tokenDigest []byte, subject, display, nickname string, admin bool, _, expiresAt time.Time,
) error {
	if f.createSessionErr != nil {
		return f.createSessionErr
	}
	f.sessions[string(tokenDigest)] = &fakeSession{
		subject: subject, display: display, nickname: nickname, admin: admin, expiresAt: expiresAt,
	}
	return nil
}

func (f *fakeStore) Session(
	_ context.Context, tokenDigest []byte, now time.Time,
) (subject, display, nickname string, admin bool, err error) {
	f.sessionCalls++
	session, ok := f.sessions[string(tokenDigest)]
	if !ok {
		return "", "", "", false, errors.New("session not found")
	}
	if !session.expiresAt.After(now) {
		return "", "", "", false, errors.New("session expired")
	}
	return session.subject, session.display, session.nickname, session.admin, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, tokenDigest []byte) error {
	f.deleteCalls++
	if f.deleteSessionErr != nil {
		return f.deleteSessionErr
	}
	delete(f.sessions, string(tokenDigest))
	return nil
}

type fakeProvider struct {
	subject, email, name, nickname string
	exchangeErr                    error
	authURLErr                     error

	gotState, gotNonce, gotVerifier   string
	exchangedNonce, exchangedVerifier string

	access, admin bool
}

func (p *fakeProvider) AuthorizationURL(_ context.Context, state, nonce, codeVerifier string) (string, error) {
	if p.authURLErr != nil {
		return "", p.authURLErr
	}
	p.gotState, p.gotNonce, p.gotVerifier = state, nonce, codeVerifier
	return "https://issuer.example.test/authorize?state=" + state, nil
}

func (p *fakeProvider) Exchange(
	_ context.Context, _, codeVerifier, nonce string,
) (ExchangedIdentity, error) {
	if p.exchangeErr != nil {
		return ExchangedIdentity{}, p.exchangeErr
	}
	p.exchangedNonce, p.exchangedVerifier = nonce, codeVerifier
	return ExchangedIdentity{
		Subject: p.subject, Email: p.email, Name: p.name, Nickname: p.nickname,
		Access: p.access, Admin: p.admin,
	}, nil
}

func TestTokenDigestRejectsAnythingButAWellFormedToken(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodedLen(tokenBytes)
	cases := map[string]string{
		"empty":                    "",
		"too short":                "abc",
		"too long":                 strings.Repeat("a", encoded+1),
		"much too long":            strings.Repeat("a", 1<<20),
		"right length, not base64": strings.Repeat("!", encoded),
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tokenDigest(wire)
			assert.Error(t, err)
		})
	}

	wire, digest, err := randomToken()
	require.NoError(t, err)
	require.Len(t, wire, encoded)
	got, err := tokenDigest(wire)
	require.NoError(t, err)
	assert.Equal(t, digest, got)
}
