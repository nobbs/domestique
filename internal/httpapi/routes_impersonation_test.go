package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/session"
)

const testRiderB = "auth0|rider-b"

// newImpersonationHandler is a handler whose state holds a target owned by the
// admin and one owned by testRiderB.
func newImpersonationHandler(t *testing.T, sessions Sessions) *Handler {
	t.Helper()

	state := &fakeState{
		targets: []fakeTarget{
			{id: testSubject, authorization: "authorized", owner: testSubject},
			{id: testRiderB, authorization: "authorized", owner: testRiderB},
		},
		nicknames: map[string]string{testRiderB: "Bee"},
	}
	handler, err := New(
		&Options{
			schemaCache:      testSchemaCache,
			Alerts:           &fakeAlerts{},
			Tasks:            &fakeTasks{},
			Settings:         settingsWith(testBasemaps()),
			Sessions:         sessions,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, state, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler
}

// withCookie attaches a cookie with the attributes this service sets it with.
func withCookie(request *http.Request, name, value string) {
	request.AddCookie(&http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func impersonateRequest(t *testing.T, subject string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/impersonate",
		strings.NewReader(url.Values{"subject": {subject}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withSession(request)
	withBrowserOrigin(request)

	return request
}

func TestStartImpersonationSwapsTheSessionAndKeepsTheAdminToken(t *testing.T) {
	sessions := newFakeSessions()
	handler := newImpersonationHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, impersonateRequest(t, testRiderB))

	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	assert.Equal(t, []string{testRiderB + "|Bee"}, sessions.impersonated)
	current := setCookie(t, response, sessionCookie)
	require.NotNil(t, current)
	assert.Equal(t, testImpersonatedToken, current.Value)
	kept := setCookie(t, response, impersonatorCookie)
	require.NotNil(t, kept)
	assert.Equal(t, testSessionToken, kept.Value)
	assert.True(t, kept.HttpOnly && kept.Secure)
}

func TestStartImpersonationRefuses(t *testing.T) {
	for name, tc := range map[string]struct {
		prepare func(*fakeSessions, *http.Request)
		subject string
		code    int
	}{
		"non-admin":      {prepare: func(s *fakeSessions, _ *http.Request) { s.identity.Admin = false }, subject: testRiderB, code: http.StatusForbidden},
		"foreign origin": {prepare: func(_ *fakeSessions, r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, subject: testRiderB, code: http.StatusForbidden},
		"no session":     {prepare: func(_ *fakeSessions, r *http.Request) { r.Header.Del("Cookie") }, subject: testRiderB, code: http.StatusUnauthorized},
		"unknown rider":  {subject: "auth0|nobody", code: http.StatusBadRequest},
		"own subject":    {subject: testSubject, code: http.StatusBadRequest},
		"empty subject":  {subject: "", code: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			sessions := newFakeSessions()
			handler := newImpersonationHandler(t, sessions)
			request := impersonateRequest(t, tc.subject)
			if tc.prepare != nil {
				tc.prepare(sessions, request)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, tc.code, response.Code)
			assert.Empty(t, sessions.impersonated)
			assert.Nil(t, setCookie(t, response, impersonatorCookie))
		})
	}
}

func stopRequest(t *testing.T, impersonator string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/impersonate/stop", http.NoBody)
	withCookie(request, sessionCookie, testImpersonatedToken)
	if impersonator != "" {
		withCookie(request, impersonatorCookie, impersonator)
	}
	withBrowserOrigin(request)

	return request
}

func TestStopImpersonationRestoresTheAdminSession(t *testing.T) {
	sessions := newFakeSessions()
	handler := newImpersonationHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, stopRequest(t, testSessionToken))

	require.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, []string{testImpersonatedToken}, sessions.revoked)
	restored := setCookie(t, response, sessionCookie)
	require.NotNil(t, restored)
	assert.Equal(t, testSessionToken, restored.Value)
	cleared := setCookie(t, response, impersonatorCookie)
	require.NotNil(t, cleared)
	assert.Negative(t, cleared.MaxAge)
}

// A kept token that no longer verifies as admin restores nothing: the caller
// ends up signed out rather than back in someone's session.
func TestStopImpersonationRefusesAKeptTokenThatIsNoLongerAdmin(t *testing.T) {
	for name, prepare := range map[string]func(*fakeSessions){
		"lost admin": func(s *fakeSessions) { s.identity.Admin = false },
		"expired":    func(s *fakeSessions) { s.verifyErr = assert.AnError },
	} {
		t.Run(name, func(t *testing.T) {
			sessions := newFakeSessions()
			prepare(sessions)
			handler := newImpersonationHandler(t, sessions)

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, stopRequest(t, testSessionToken))

			assert.Equal(t, http.StatusUnauthorized, response.Code)
			cleared := setCookie(t, response, sessionCookie)
			require.NotNil(t, cleared)
			assert.Negative(t, cleared.MaxAge)
		})
	}
}

func TestStopImpersonationWithoutOneUnderWay(t *testing.T) {
	sessions := newFakeSessions()
	handler := newImpersonationHandler(t, sessions)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, stopRequest(t, ""))

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Empty(t, sessions.revoked)
}

func TestLogoutWhileImpersonatingEndsBothSessions(t *testing.T) {
	sessions := newFakeSessions()
	handler := newImpersonationHandler(t, sessions)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/logout", http.NoBody)
	withCookie(request, sessionCookie, testImpersonatedToken)
	withCookie(request, impersonatorCookie, testSessionToken)
	withBrowserOrigin(request)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.ElementsMatch(t, []string{testImpersonatedToken, testSessionToken}, sessions.revoked)
	assert.NotNil(t, setCookie(t, response, impersonatorCookie))
}

func TestWebUIConfigSaysWhenImpersonating(t *testing.T) {
	for name, impersonating := range map[string]bool{"impersonating": true, "own session": false} {
		t.Run(name, func(t *testing.T) {
			sessions := newFakeSessions()
			sessions.identity = session.Identity{Subject: testRiderB, Display: "Bee"}
			handler := newImpersonationHandler(t, sessions)
			request := signedInRequest(http.MethodGet, "/v1/webui/config")
			if impersonating {
				withCookie(request, impersonatorCookie, "kept")
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)

			var body struct {
				Identity struct {
					Impersonating *bool `json:"impersonating"`
				} `json:"identity"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, impersonating, body.Identity.Impersonating != nil && *body.Identity.Impersonating)
		})
	}
}
