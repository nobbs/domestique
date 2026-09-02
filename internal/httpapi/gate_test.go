package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/session"
)

const (
	testSessionToken = "session-token"
	testSubject      = "github|123456"
	testDisplay      = "rider@example.test"
)

// fakeSessions stands in for internal/session and records what it was asked.
type fakeSessions struct {
	verifyErr   error
	beginErr    error
	completeErr error
	revokeErr   error
	identity    session.Identity
	login       session.Login
	completion  session.Completion
	completed   []string
	revoked     []string
	beginCalls  int
}

// newFakeSessions is a session service that admits testSessionToken and
// nothing else.
func newFakeSessions() *fakeSessions {
	return &fakeSessions{
		identity: session.Identity{Subject: testSubject, Display: testDisplay},
		login: session.Login{
			AuthorizationURL: "https://tenant.example.test/authorize?state=login-state",
			State:            "login-state",
		},
		completion: session.Completion{
			Token:     testSessionToken,
			Identity:  session.Identity{Subject: testSubject, Display: testDisplay},
			ExpiresAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func (s *fakeSessions) Verify(_ context.Context, token string) (session.Identity, error) {
	if s.verifyErr != nil {
		return session.Identity{}, s.verifyErr
	}
	if token != testSessionToken {
		return session.Identity{}, errors.New("no such session")
	}

	return s.identity, nil
}

func (s *fakeSessions) Begin(context.Context) (session.Login, error) {
	s.beginCalls++
	if s.beginErr != nil {
		return session.Login{}, s.beginErr
	}

	return s.login, nil
}

func (s *fakeSessions) Complete(_ context.Context, state, cookieState, code string) (session.Completion, error) {
	s.completed = append(s.completed, state+"|"+cookieState+"|"+code)
	if s.completeErr != nil {
		return session.Completion{}, s.completeErr
	}

	return s.completion, nil
}

func (s *fakeSessions) Revoke(_ context.Context, token string) error {
	s.revoked = append(s.revoked, token)

	return s.revokeErr
}

// newSessionHandler builds a handler gated by the given session service.
func newSessionHandler(t *testing.T, sessions Sessions) *Handler {
	t.Helper()

	handler, err := New(
		&Options{
			Alerts:           &fakeAlerts{},
			Tasks:            &fakeTasks{},
			Settings:         settingsWith(testBasemaps()),
			Sessions:         sessions,
			BrowserOriginURL: testBrowserOriginURL,
			Auth0Domain:      testAuth0Domain,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// signedInRequest carries the session cookie a signed-in browser holds, plus
// the Origin a browser attaches to a state-changing request.
func signedInRequest(method, target string) *http.Request {
	request := httptest.NewRequestWithContext(context.Background(), method, target, http.NoBody)
	withSession(request)
	withBrowserOrigin(request)

	return request
}

// withSession attaches the session cookie a signed-in browser holds.
func withSession(request *http.Request) {
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: testSessionToken, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// setCookie is the response's Set-Cookie for one name, or nil.
func setCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}

	return nil
}

func TestIdentityOfReturnsZeroValueWithoutAnIdentity(t *testing.T) {
	assert.Zero(t, identityOf(context.Background()), "a context the gate never touched must yield no identity")
}

func TestGateAdmitsAValidSessionCookie(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/v1/status"))

	assert.Equal(t, http.StatusOK, response.Code)
}

// An API caller with no usable session is told so in this service's own error
// shape, and a cookie the gate refused is cleared rather than left to be
// resent on every later request.
func TestGateRefusesAnUnusableSession(t *testing.T) {
	tests := map[string]func(request *http.Request){
		"no cookie": func(*http.Request) {},
		"garbage cookie": func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "nonsense", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		},
	}

	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			handler := newSessionHandler(t, newFakeSessions())
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/status", http.NoBody)
			prepare(request)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusUnauthorized, response.Code)
			assert.Contains(t, response.Body.String(), "unauthorized")
		})
	}
}

func TestGateClearsACookieItCouldNotVerify(t *testing.T) {
	sessions := newFakeSessions()
	sessions.verifyErr = errors.New("session expired at 12:00 for row 7")
	handler := newSessionHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/v1/status"))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	// The rejection must not describe the check the caller has to defeat.
	assert.NotContains(t, response.Body.String(), "row 7")
	cleared := setCookie(t, response, sessionCookie)
	require.NotNil(t, cleared, "the refused cookie was not cleared")
	assert.Negative(t, cleared.MaxAge, "the refused cookie must be expired")
}

// A browser asking for a page is sent to sign in rather than handed JSON it
// would render as text.
func TestGateSendsAPageRequestToSignIn(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/catalogue", http.NoBody)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/auth/login", response.Header().Get("Location"))
}

// The gate never re-issues the session cookie: the lifetime is fixed, so
// there is nothing for a request to move.
func TestGateNeverReissuesTheCookie(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodGet, "/v1/status"))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Nil(t, setCookie(t, response, sessionCookie), "the gate must not set a session cookie")
}

// Every answer depends on the cookie, so none of them may be reused for
// another caller by a cache in between.
func TestEveryResponseVariesOnTheCookie(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())

	for _, target := range []string{"/healthz", "/auth/login", "/v1/status"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, signedInRequest(http.MethodGet, target))
		assert.Equalf(t, "Cookie", response.Header().Get("Vary"), "%s", target)
	}
}

// Without a session service there is no gate at all.
func TestNewRequiresASessionService(t *testing.T) {
	_, err := New(&Options{
		Alerts:           &fakeAlerts{},
		Tasks:            &fakeTasks{},
		Settings:         settingsWith(testBasemaps()),
		BrowserOriginURL: testBrowserOriginURL,
	}, &fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{}, &fakeWeather{})
	require.Error(t, err, "New() built a handler with no way to authenticate anyone")
}

// A handler with nowhere to ask a forecast could not serve GET /v1/weather.
func TestNewRequiresAWeatherProvider(t *testing.T) {
	_, err := New(&Options{
		Alerts:           &fakeAlerts{},
		Tasks:            &fakeTasks{},
		Settings:         settingsWith(testBasemaps()),
		Sessions:         newFakeSessions(),
		BrowserOriginURL: testBrowserOriginURL,
	}, &fakeOAuth{}, &fakeState{}, &fakeSync{}, &fakeAssets{}, nil)
	require.Error(t, err, "New() accepted a nil weather provider")
}

// Health stays reachable without any identity, because Docker probes it over
// loopback.
func TestHealthRemainsUngated(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
}

// loginRecordingOAuth captures the identity the gate passes downstream.
type loginRecordingOAuth struct {
	startLogin, completeLogin string
}

func (o *loginRecordingOAuth) Start(_ context.Context, login, _ string) (string, error) {
	o.startLogin = login

	return "https://wahoo.example.test/authorize", nil
}

func (o *loginRecordingOAuth) Complete(_ context.Context, login, _, _ string) error {
	o.completeLogin = login

	return nil
}

// The Wahoo OAuth state is bound to the caller's own subject: with more than
// one allowed subject, a shared constant would let one operator complete
// another's authorization.
func TestWahooOAuthBindsTheContextIdentity(t *testing.T) {
	oauthService := &loginRecordingOAuth{}
	handler, err := New(
		&Options{
			Alerts:           &fakeAlerts{},
			Tasks:            &fakeTasks{},
			Settings:         settingsWith(testBasemaps()),
			Sessions:         newFakeSessions(),
			BrowserOriginURL: testBrowserOriginURL,
		},
		oauthService, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{},
	)
	require.NoError(t, err, "New()")

	handler.ServeHTTP(httptest.NewRecorder(),
		signedInRequest(http.MethodGet, "/oauth/wahoo/start/rider-a"))
	handler.ServeHTTP(httptest.NewRecorder(),
		signedInRequest(http.MethodGet, "/oauth/wahoo/callback?state=s&code=c"))

	assert.Equal(t, testSubject, oauthService.startLogin, "start login")
	assert.Equal(t, oauthService.startLogin, oauthService.completeLogin,
		"the two requests resolved to different principals")
}
