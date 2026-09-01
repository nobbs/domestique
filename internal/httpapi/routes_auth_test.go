package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/session"
)

// loginRequest is a browser arriving at a sign-in route: no session, and the
// Origin a browser attaches to a form post.
func loginRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, http.NoBody)
	withBrowserOrigin(request)

	return request
}

func TestLoginPageIsServedWithoutASession(t *testing.T) {
	sessions := newFakeSessions()
	handler := newSessionHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/auth/login", http.NoBody))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, response.Body.String(), `action="/auth/start"`)
	// No state is minted by a page a crawler or a prefetch can reach.
	assert.Zero(t, sessions.beginCalls, "serving the login page started a sign-in")
}

// The sign-in pages get their own tight policy; the application keeps the one
// its map needs.
func TestSignInPagesCarryTheirOwnPolicy(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/auth/login", http.NoBody))
	assert.Equal(t, authPolicy, page.Header().Get("Content-Security-Policy"))

	app := httptest.NewRecorder()
	handler.ServeHTTP(app, signedInRequest(http.MethodGet, "/"))
	assert.Contains(t, app.Header().Get("Content-Security-Policy"), "script-src 'self'")
	assert.NotEqual(t, authPolicy, app.Header().Get("Content-Security-Policy"))
}

// The sign-in flow is the way in, so nothing about it may need a session.
func TestSignInRoutesAreNotGated(t *testing.T) {
	handler := newSessionHandler(t, newFakeSessions())

	for _, target := range []string{"/auth/login", "/auth/start", "/auth/logout"} {
		method := http.MethodPost
		if target == "/auth/login" {
			method = http.MethodGet
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, loginRequest(t, method, target))
		assert.NotEqualf(t, http.StatusUnauthorized, response.Code, "%s", target)
	}
}

func TestStartLoginRequiresTheBrowserOrigin(t *testing.T) {
	sessions := newFakeSessions()
	handler := newSessionHandler(t, sessions)

	for name, origin := range map[string]string{
		"absent":     "",
		"cross site": "https://evil.example.test",
		"opaque":     "null",
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/start", http.NoBody)
			if origin != "" {
				request.Header.Set("Origin", origin)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Zero(t, sessions.beginCalls, "a refused request still started a sign-in")
		})
	}
}

func TestStartLoginRefusesWhenBeginFails(t *testing.T) {
	sessions := newFakeSessions()
	sessions.beginErr = errors.New("provider unreachable")
	handler := newSessionHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, loginRequest(t, http.MethodPost, "/auth/start"))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "Sign-in could not be completed")
	assert.Nil(t, setCookie(t, response, loginCookie), "a failed Begin still set the login cookie")
}

func TestStartLoginRefusesAnUnusableAuthorizationURL(t *testing.T) {
	for name, authorizationURL := range map[string]string{
		"not https": "http://tenant.example.test/authorize?state=login-state",
		"no host":   "https:///authorize?state=login-state",
	} {
		t.Run(name, func(t *testing.T) {
			sessions := newFakeSessions()
			sessions.login.AuthorizationURL = authorizationURL
			handler := newSessionHandler(t, sessions)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, loginRequest(t, http.MethodPost, "/auth/start"))

			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Contains(t, response.Body.String(), "Sign-in could not be completed")
			assert.Nil(t, setCookie(t, response, loginCookie), "an unusable authorization url still set the login cookie")
		})
	}
}

func TestStartLoginRedirectsToTheProviderAndRemembersTheState(t *testing.T) {
	sessions := newFakeSessions()
	handler := newSessionHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, loginRequest(t, http.MethodPost, "/auth/start"))

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, sessions.login.AuthorizationURL, response.Header().Get("Location"))
	cookie := setCookie(t, response, loginCookie)
	require.NotNil(t, cookie, "the pending state was not carried in a cookie")
	assert.Equal(t, sessions.login.State, cookie.Value)
	assert.True(t, cookie.Secure && cookie.HttpOnly, "the login cookie must be Secure and HttpOnly")
	// Lax, not Strict: the callback is a top-level cross-site navigation, and
	// Strict would withhold this cookie exactly there.
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
}

// A callback missing any of the three values it needs must not reach the
// exchange at all.
func TestCompleteLoginRefusesAnIncompleteCallback(t *testing.T) {
	tests := map[string]struct {
		target string
		cookie string
	}{
		"no cookie": {target: "/auth/callback?state=abc&code=xyz"},
		"no state":  {target: "/auth/callback?code=xyz", cookie: "abc"},
		"no code":   {target: "/auth/callback?state=abc", cookie: "abc"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sessions := newFakeSessions()
			handler := newSessionHandler(t, sessions)

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, test.target, http.NoBody)
			if test.cookie != "" {
				request.AddCookie(&http.Cookie{Name: loginCookie, Value: test.cookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Contains(t, response.Body.String(), "Sign-in could not be completed")
			assert.Empty(t, sessions.completed, "an incomplete callback reached the exchange")
			assert.Nil(t, setCookie(t, response, sessionCookie), "a refused callback issued a session")
		})
	}
}

// A cookie that disagrees with the returned state is refused by the session
// service, which compares them; every such refusal reads the same here.
func TestCompleteLoginRefusesAMismatchedState(t *testing.T) {
	sessions := newFakeSessions()
	sessions.completeErr = errors.New("login state did not match")
	handler := newSessionHandler(t, sessions)

	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/auth/callback?state=abc&code=xyz", http.NoBody)
	request.AddCookie(&http.Cookie{Name: loginCookie, Value: "different", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "Sign-in could not be completed")
	// The refusal says nothing about which check refused it.
	assert.NotContains(t, response.Body.String(), "state did not match")
	assert.Nil(t, setCookie(t, response, sessionCookie), "a refused callback issued a session")
}

// A subject that authenticated but is not allowed is the one case a reader has
// to be told about by name: they need to know which account was refused.
func TestCompleteLoginNamesARefusedSubject(t *testing.T) {
	sessions := newFakeSessions()
	sessions.completeErr = &session.NotAllowedError{Subject: "github|999"}
	handler := newSessionHandler(t, sessions)

	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/auth/callback?state=abc&code=xyz", http.NoBody)
	request.AddCookie(&http.Cookie{Name: loginCookie, Value: "abc", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Contains(t, response.Body.String(), "github|999")
	assert.Nil(t, setCookie(t, response, sessionCookie), "a refused sign-in issued a session")
}

func TestCompleteLoginIssuesTheSessionCookie(t *testing.T) {
	sessions := newFakeSessions()
	handler := newSessionHandler(t, sessions)

	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/auth/callback?state=abc&code=xyz", http.NoBody)
	request.AddCookie(&http.Cookie{Name: loginCookie, Value: "abc", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/", response.Header().Get("Location"))
	assert.Equal(t, []string{"abc|abc|xyz"}, sessions.completed, "the exchange saw other values")

	cookie := setCookie(t, response, sessionCookie)
	require.NotNil(t, cookie, "a completed sign-in issued no session cookie")
	assert.Equal(t, testSessionToken, cookie.Value)
	// The `__Host-` prefix is a browser-side check, so every attribute it
	// requires is set by hand here.
	assert.True(t, cookie.Secure, "Secure")
	assert.True(t, cookie.HttpOnly, "HttpOnly")
	assert.Equal(t, "/", cookie.Path, "Path")
	assert.Empty(t, cookie.Domain, "Domain")
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite, "SameSite")
	assert.WithinDuration(t, sessions.completion.ExpiresAt, cookie.Expires, 0)

	cleared := setCookie(t, response, loginCookie)
	require.NotNil(t, cleared, "the pending login cookie was not cleared")
	assert.Negative(t, cleared.MaxAge)
}

func TestLogoutRequiresTheBrowserOrigin(t *testing.T) {
	sessions := newFakeSessions()
	handler := newSessionHandler(t, sessions)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/logout", http.NoBody)
	withSession(request)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Empty(t, sessions.revoked, "a refused request still revoked a session")
}

func TestLogoutRevokesAndClearsTheCookie(t *testing.T) {
	sessions := newFakeSessions()
	handler := newSessionHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, signedInRequest(http.MethodPost, "/auth/logout"))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, []string{testSessionToken}, sessions.revoked)
	cleared := setCookie(t, response, sessionCookie)
	require.NotNil(t, cleared, "the session cookie was not cleared")
	assert.Negative(t, cleared.MaxAge)
}

// An expired or already-revoked session must still be able to clear its own
// cookie, so logout is not gated and a store that cannot answer changes nothing.
func TestLogoutSucceedsWithoutAUsableSession(t *testing.T) {
	for name, prepare := range map[string]func(*fakeSessions, *http.Request){
		"no cookie":     func(*fakeSessions, *http.Request) {},
		"revoke failed": func(s *fakeSessions, r *http.Request) { s.revokeErr = errors.New("store is down"); withSession(r) },
	} {
		t.Run(name, func(t *testing.T) {
			sessions := newFakeSessions()
			handler := newSessionHandler(t, sessions)
			request := loginRequest(t, http.MethodPost, "/auth/logout")
			prepare(sessions, request)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusNoContent, response.Code)
			require.NotNil(t, setCookie(t, response, sessionCookie), "the session cookie was not cleared")
		})
	}
}
